// Package queue holds the FOURTH stability output — queue-position distribution
// under varying tick regimes — which two earlier audits marked NOT ANSWERABLE.
//
// # The correction this rests on
//
// pkg/stability found it unanswerable against the minimal generator (no order identity, no
// prices) and pkg/priced against the priced ladder (prices yes, order identity no). The
// latter called the blocker a capability gap: "assigning k simultaneous arrivals to the
// first k free slots requires a scan across lanes".
//
// That framing was wrong, and PREREGISTRATION.md says so before any of this ran.
// First-free-slot allocation is not FIFO — when a mid-queue order cancels the orders
// behind it MOVE UP, they do not leave a hole for a newcomer to jump into. Allocation to
// holes would let a late order inherit an early position, which is what a queue forbids.
// The right operation is COMPACTION, whose only non-trivial ingredient is an exclusive
// prefix sum, and the lazy-`where` idiom could always express that.
//
// So this output needed no engine feature: cfg/lob_queue.yaml uses NO scan. The gap was in
// the formulation reached for, not in the engine. The upstream `scan` work remains
// worthwhile on its own merits, but it cannot be justified by this output.
//
// # What is measured, and the gate on it
//
// Two structural predictions check the queue is a queue before any distribution is read
// off it: fills come from the front (BW) and cancellation reads nothing about position
// (BX). PREREGISTRATION.md states that if either fails, the tick-regime results are NOT to
// be reported — a distribution over a broken queue is not evidence about ticks. Both pass,
// as does a direct check that occupancy is binary and hole-free within every level.
//
// # What this can never be
//
// Queue position is UNOBSERVABLE in the permitted feed at any bucket size: a Binance diff
// stream carries aggregated volume per price, never per-order identity or arrival order.
// Every claim here is a MODEL COUNTERFACTUAL. None is compared to market data, and none can
// be — which is a statement about the feed, not about effort.
package queue

import (
	"fmt"
	"math"
	"sync"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	configName = "lob_queue.yaml"
	partition  = "lob_queue"

	phase = "4 — Stability outputs"
	// The framing that matters most for this package: not merely synthetic, but a
	// quantity the permitted feed cannot express at all.
	dataset = "synthetic — a model counterfactual only. Queue position is unobservable " +
		"in a Binance diff feed at any bucket size, so no limb of this is or can be " +
		"compared to market data"

	nSlots   = 64
	nGroups  = 8 // 2 sides x 4 levels
	perLevel = 8 // FIFO slots per level

	// Observable columns, which must match cfg/lob_queue.yaml's outputs.
	colRestPosSum   = 64
	colRestCount    = 65
	colFillPosSum   = 66
	colFillCount    = 67
	colCancelPosSum = 68
	colCancelCount  = 69
	colDeepCount    = 70

	seeds  = 8
	steps  = 2000
	settle = 100
)

// regimes is the pre-registered tick sweep. Coarser ticks collect more arrivals per level
// and decay faster across levels; nothing else moves between them.
var regimes = []struct {
	label string
	tick  string
}{
	{"fine (tick 0.5)", "0.5"},
	{"reference (tick 1.0)", "1.0"},
	{"coarse (tick 2.0)", "2.0"},
}

type measured struct {
	queueLength    float64 // mean occupied slots per level
	restPosition   float64 // mean position of a resting order
	fillPosition   float64 // mean position of an order consumed by marketable flow
	cancelPosition float64
	deepShare      float64 // share of resting orders at position >= 4
	orderBreaks    float64 // occupied slots sitting behind an empty one, per member
	nonBinary      float64 // slots holding something other than 0 or 1
}

func measureRegime(tick string) (measured, error) {
	stores, err := cfgrun.RunEnsemble(configName, cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", steps),
		"tick: [1.0]":    "tick: [" + tick + "]",
	}, cfgrun.DefaultSeeds[:seeds])
	if err != nil {
		return measured{}, err
	}
	var qlen, rest, fill, cancel, deep, breaks, nonBin []float64
	for _, storage := range stores {
		rows := storage.GetValues(partition)
		if len(rows) <= settle {
			return measured{}, fmt.Errorf("queue: %s produced too few rows", configName)
		}
		rows = rows[settle:]
		var restSum, restN, fillSum, fillN, cancelSum, cancelN, deepN float64
		var nBreak, nNonBin float64
		for _, row := range rows {
			// Compaction must leave every level's occupied slots as a contiguous PREFIX.
			// An occupied slot behind an empty one means a survivor failed to move up, or
			// an arrival was appended into a hole — either way the queue is not a queue.
			for g := 0; g < nGroups; g++ {
				empty := false
				for p := 0; p < perLevel; p++ {
					v := row[g*perLevel+p]
					if v != 0 && v != 1 {
						nNonBin++
					}
					if v == 0 {
						empty = true
					} else if empty {
						nBreak++
					}
				}
			}
			restSum += row[colRestPosSum]
			restN += row[colRestCount]
			fillSum += row[colFillPosSum]
			fillN += row[colFillCount]
			cancelSum += row[colCancelPosSum]
			cancelN += row[colCancelCount]
			deepN += row[colDeepCount]
		}
		if restN == 0 || fillN == 0 || cancelN == 0 {
			return measured{}, fmt.Errorf("queue: tick %s produced no resting, filled or "+
				"cancelled orders, so a mean position is undefined", tick)
		}
		// Pooled over the run rather than averaged per step: a step with one fill and a
		// step with ten should not weigh the same.
		rest = append(rest, restSum/restN)
		fill = append(fill, fillSum/fillN)
		cancel = append(cancel, cancelSum/cancelN)
		deep = append(deep, deepN/restN)
		qlen = append(qlen, restN/float64(len(rows))/float64(nGroups))
		breaks = append(breaks, nBreak)
		nonBin = append(nonBin, nNonBin)
	}
	return measured{
		queueLength: diagnostics.Mean(qlen), restPosition: diagnostics.Mean(rest),
		fillPosition: diagnostics.Mean(fill), cancelPosition: diagnostics.Mean(cancel),
		deepShare: diagnostics.Mean(deep), orderBreaks: diagnostics.Mean(breaks),
		nonBinary: diagnostics.Mean(nonBin),
	}, nil
}

func measureAllUncached() ([]measured, error) {
	out := make([]measured, 0, len(regimes))
	for _, r := range regimes {
		m, err := measureRegime(r.tick)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// measureAll caches the sweep: ObservedBehaviour and the structural guard both need it, and
// three ensembles is enough work that running them twice is worth avoiding.
var (
	measureOnce   sync.Once
	measureCached []measured
	measureErr    error
)

func measureAll() ([]measured, error) {
	measureOnce.Do(func() { measureCached, measureErr = measureAllUncached() })
	return measureCached, measureErr
}

// ObservedBehaviour states the queue-position output as claims.
func ObservedBehaviour() []claims.Claim {
	m, err := measureAll()
	if err != nil {
		panic("queue: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestQueuePosition",
		TestFile: "pkg/queue/behaviour_test.go",
	}

	var breaks, fills, ratios, neutrality, lengths, deeps []claims.Observation
	var breakT, fillT, neutralT []claims.Threshold
	for i, r := range regimes {
		breaks = append(breaks, claims.Observation{Label: r.label, Value: m[i].orderBreaks})
		fills = append(fills, claims.Observation{Label: r.label, Value: m[i].fillPosition})
		ratios = append(ratios, claims.Observation{
			Label: r.label + ", vs resting", Value: m[i].fillPosition / m[i].restPosition,
		})
		neutrality = append(neutrality, claims.Observation{
			Label: r.label,
			Value: math.Abs(m[i].cancelPosition-m[i].restPosition) / m[i].restPosition,
		})
		lengths = append(lengths, claims.Observation{Label: r.label, Value: m[i].queueLength})
		deeps = append(deeps, claims.Observation{Label: r.label, Value: m[i].deepShare})
		breakT = append(breakT, claims.Threshold{
			ObsIndex: i, GreaterThan: false, Ref: 0.5, RefLabel: "0.5 (i.e. exactly none)",
		})
		neutralT = append(neutralT, claims.Threshold{
			ObsIndex: i, GreaterThan: false, Ref: 0.20, RefLabel: "20% relative",
		})
	}
	for i := range regimes {
		fillT = append(fillT, claims.Threshold{
			ObsIndex: i, GreaterThan: false, Ref: 1.0, RefLabel: "position 1.0",
		})
		fillT = append(fillT, claims.Threshold{
			ObsIndex: len(regimes) + i, GreaterThan: false, Ref: 0.5,
			RefLabel: "0.5 x the mean resting position",
		})
	}

	return []claims.Claim{
		{
			ID: "compaction_leaves_every_level_a_contiguous_queue",
			Statement: "Within every price level, the occupied slots are always a " +
				"contiguous prefix: no order rests behind an empty slot, and no slot holds " +
				"anything but one order or none. This is what distinguishes a queue from a " +
				"bag of orders. When a mid-queue order cancels the orders behind it move " +
				"up rather than leaving a hole, so no later arrival can inherit an earlier " +
				"position, and the position an order holds is its true place in arrival " +
				"order rather than an artefact of which slot happened to be free.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit: "occupied slots sitting behind an empty one, counted over 1900 steps x 8 " +
				"levels per member",
			Limitations: "A structural check on the implementation, not a finding about " +
				"markets — it says the model does what it claims, which is the weakest " +
				"useful thing a test can say. It is a GATE: PREREGISTRATION.md states the " +
				"tick-regime results are not to be reported if this or the priority check " +
				"fails. It does not verify that arrival ORDER across levels is faithful, " +
				"only that within a level the ordering is internally consistent.",
			Thresholds:   breakT,
			Observations: breaks,
			Binding:      binding,
		},
		{
			ID: "marketable_flow_consumes_from_the_front_of_the_queue",
			Statement: "Orders consumed by marketable flow sit far nearer the front than a " +
				"resting order does — below position 1 in absolute terms, and below half " +
				"the mean resting position in relative terms. Price-time priority is not " +
				"imposed anywhere in the config: it falls out of counting how many resting " +
				"orders lie ahead of each one on its own side and consuming those with " +
				"fewest ahead. The relative limb is the real test, since the absolute one " +
				"could pass on short queues alone.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit: "mean queue position of a consumed order (0 = front), and that position " +
				"as a fraction of the mean resting position",
			Limitations: "Confirms priority holds, which is a property built into the " +
				"consumption rule rather than discovered — a useful check that the rule was " +
				"implemented, not evidence about real books. It says nothing about whether " +
				"real venues honour price-time priority, which varies by venue and order " +
				"type. All orders here are UNIT SIZE, so nothing in it speaks to " +
				"size-weighted queue value or to priority under partial fills.",
			Thresholds:   fillT,
			Observations: append(append([]claims.Observation{}, fills...), ratios...),
			Binding:      binding,
		},
		{
			ID: "cancellation_is_position_neutral_within_the_queue",
			Statement: "The mean position of a cancelled order matches the mean position of " +
				"a resting one to within 20% at every tick regime, so cancellation neither " +
				"favours nor spares the front of the queue. This makes the tick-regime " +
				"results below interpretable: a hazard that quietly preferred deep orders " +
				"would flatten the queue-position distribution by itself, and the shift " +
				"attributed to the tick would be the cancellation rule instead.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "absolute difference in mean position, cancelled vs resting, relative",
			Limitations: "The measured deviation is small but SYSTEMATICALLY POSITIVE — " +
				"cancelled orders sit slightly deeper than resting ones — and that is " +
				"expected rather than noise: marketable flow removes front orders first, so " +
				"the cancellation draw is taken over a population already depleted at the " +
				"front. The claim bounds that bias rather than denying it. It also holds " +
				"only for the hazard as configured, which reads nothing about position; a " +
				"position-dependent rule would break it, and that would be a modelling " +
				"choice rather than a defect.",
			Thresholds:   neutralT,
			Observations: neutrality,
			Binding:      binding,
		},
		{
			ID: "a_coarser_tick_lengthens_the_queue_at_each_level",
			Statement: "Mean occupied slots per level rises monotonically as the tick " +
				"coarsens across 0.5, 1.0 and 2.0. A coarser tick makes each level span " +
				"more price, so it collects proportionally more arrivals while the decay " +
				"across levels steepens by the same factor — liquidity concentrates into " +
				"fewer, longer queues. This is the substantive half of the fourth " +
				"output and its answer was not known when it was predicted.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "mean occupied FIFO slots per level, at tick 0.5 / 1.0 / 2.0",
			Limitations: "The queue is capped at 8 slots per level, and the coarse regime " +
				"runs close enough to that cap that the rise from 1.0 to 2.0 is COMPRESSED " +
				"by saturation rather than being the model's unconstrained response. The " +
				"direction is therefore trustworthy and the magnitude is not; a deeper " +
				"queue would show a larger gap. The tick parameterisation is also a " +
				"modelling choice — rate and decay scaled by the same factor — and a real " +
				"venue's tick change need not act that cleanly.",
			Monotone:     1,
			Observations: lengths,
			Binding:      binding,
		},
		{
			ID: "a_coarser_tick_pushes_resting_orders_deeper_into_the_queue",
			Statement: "The share of resting orders sitting at position 4 or worse rises " +
				"monotonically as the tick coarsens, from under a tenth to about a third. " +
				"This is the distributional statement the length claim cannot make: queues " +
				"do not merely get longer on average, the mass moves back, so a typical " +
				"order joins a coarse-tick book with materially more ahead of it and waits " +
				"through more of the queue before reaching the front.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "share of resting orders at position >= 4, at tick 0.5 / 1.0 / 2.0",
			Limitations: "Position 4 of 8 is an arbitrary cut chosen before measuring, and " +
				"the same saturation that compresses the length claim applies here. It is a " +
				"statement about where orders SIT, not how long they WAIT — this package " +
				"never measures time-to-fill, which is the quantity a trader would actually " +
				"want and which would need per-order age tracking this model does not carry.",
			Monotone:     1,
			Observations: deeps,
			Binding:      binding,
		},
	}
}
