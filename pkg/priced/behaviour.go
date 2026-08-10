// Package priced holds the expected-behaviour claims for the priced order book in
// cfg/lob_priced.yaml — the domain-model step Spike 2.2 sent the project back for.
//
// # What it unlocks
//
// Spike 4.2 lists four stability outputs. Against the minimal generator
// only one was answerable (see pkg/stability). Adding prices, an emergent spread
// and marketable orders that walk the book takes that to three:
//
//	spread response to an arrival-intensity shock    NOW ANSWERABLE
//	fraction of liquidity surviving a large order    NOW ANSWERABLE
//	depth recovery after a liquidity event           already answerable
//	queue-position distribution across tick regimes  ANSWERED ELSEWHERE (pkg/queue)
//
// CORRECTION, 2026-08-08. This package used to say the fourth output "is not a
// scoping choice but a capability gap: assigning k simultaneous arrivals to the
// first k free slots requires a scan across lanes that the expressions DSL cannot
// express". That was wrong, and cfg/lob_queue.yaml now answers the output using NO
// scan at all.
//
// Allocation to first-free slots is not FIFO. When a mid-queue order cancels the
// orders behind it MOVE UP; they do not leave a hole for a newcomer to jump into.
// The right operation is COMPACTION, whose only non-trivial ingredient is an
// exclusive prefix sum — which the lazy-`where` idiom documented in this very
// config could always express. The gap was in the formulation reached for, not in
// the engine. See pkg/queue and PREREGISTRATION.md BW-BZ.
//
// # The spread is an output, not a parameter
//
// Nothing in the config sets a spread. The touch is wherever the innermost occupied
// level happens to be, so the spread is whatever the balance of arrivals,
// cancellations and marketable flow leaves behind. Both directions come out
// economically right, and neither was put there by hand: more limit arrivals
// tighten it, more marketable flow widens it.
//
// That is worth stating carefully, because "the model reproduces a known
// stylised fact" is a weaker claim than it sounds. These are consequences of the
// mechanism, and confirming them mostly says the implementation does what the
// mechanism implies. What it establishes is that the OUTPUT EXISTS and responds —
// which is precisely what Spike 4.2 gates on and what the minimal generator could
// not offer at all.
//
// # What it does not fix
//
// This model has not been calibrated against anything. Spike 2.2's failure was
// measured against cfg/lob_generator.yaml, and whether a priced book with decaying
// arrival intensity does better on the depth-coupling and churn diagnostics is
// UNTESTED. Nothing here should be read as having answered that.
package priced

import (
	"fmt"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

const (
	configName = "lob_priced.yaml"

	phase = "4 — Stability outputs"
	// Uncalibrated, and the dataset field says so — no claim here may be read as
	// describing a real book.
	dataset = "synthetic — the priced generator, NOT calibrated against any market"

	// State row indices, matching cfg/lob_priced.yaml.
	bidFrom     = 0
	askFrom     = 8
	levels      = 8
	idxMarket   = 18
	idxDepth    = 19
	idxSpread   = 20
	emptySpread = 99.0
	shockStep   = 200
	settleFrom  = 100
)

// seeds are averaged over; a single realisation's spread is noisy.
var seeds = []int{11, 22, 33}

// sideDepth totals one side of the ladder.
func sideDepth(row []float64, from int) float64 {
	total := 0.0
	for i := range levels {
		total += row[from+i]
	}
	return total
}

// steadyState runs the unshocked model and returns the mean spread over
// two-sided steps, and the fraction of steps on which a side emptied entirely.
//
// The spread average EXCLUDES one-sided steps rather than folding in the sentinel,
// because averaging a sentinel would silently turn "the book broke" into "the
// spread was wide". The fraction is reported separately so the failure stays
// visible instead of being smeared into the mean.
func steadyState(subs cfgrun.Subs) (spread, oneSided float64, err error) {
	for _, seed := range seeds {
		withSeed := cfgrun.Subs{"seed: 20260729": fmt.Sprintf("seed: %d", seed)}
		for key, value := range subs {
			withSeed[key] = value
		}
		storage, err := cfgrun.Run(configName, withSeed)
		if err != nil {
			return 0, 0, err
		}
		rows := storage.GetValues(configName[:len(configName)-5])
		var total, counted, empty float64
		for _, row := range rows[settleFrom:] {
			if row[idxSpread] >= emptySpread {
				empty++
				continue
			}
			total += row[idxSpread]
			counted++
		}
		if counted == 0 {
			return 0, 0, fmt.Errorf("priced: every step was one-sided; no spread to average")
		}
		spread += total / counted
		oneSided += empty / float64(len(rows)-settleFrom)
	}
	n := float64(len(seeds))
	return spread / n, oneSided / n, nil
}

// survival fires a marketable buy of the given size and returns the fraction of
// ask-side depth left standing immediately afterwards.
func survival(size string) (float64, error) {
	total := 0.0
	for _, seed := range seeds {
		storage, err := cfgrun.Run(configName, cfgrun.Subs{
			"shock_step: [-1.0]": fmt.Sprintf("shock_step: [%d.0]", shockStep),
			"shock_size: [0.0]":  "shock_size: [" + size + ".0]",
			"seed: 20260729":     fmt.Sprintf("seed: %d", seed),
		})
		if err != nil {
			return 0, err
		}
		rows := storage.GetValues(configName[:len(configName)-5])
		// Row i is the state AFTER step i, so the shock at step N lands in row N and
		// the book it hit is row N-1. Getting this off by one reports a RECOVERING
		// book as the survivor and can read above 100%.
		before := sideDepth(rows[shockStep-1], askFrom)
		after := sideDepth(rows[shockStep], askFrom)
		if before <= 0 {
			return 0, fmt.Errorf("priced: the ask side was already empty before the shock")
		}
		total += after / before * 100
	}
	return total / float64(len(seeds)), nil
}

// ObservedBehaviour measures the priced model's response claims.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		panic("priced: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	binding := claims.Binding{
		TestName: "TestPricedBookExpectedBehaviour",
		TestFile: "pkg/priced/behaviour_test.go",
	}

	byArrival := make([]claims.Observation, 0, 3)
	for _, rate := range []string{"1.0", "2.0", "4.0"} {
		spread, _, err := steadyState(cfgrun.Subs{
			"limit_rate: [2.0]": "limit_rate: [" + rate + "]"})
		if err != nil {
			return nil, err
		}
		byArrival = append(byArrival,
			claims.Observation{Label: "at " + rate, Value: spread})
	}

	byMarketable := make([]claims.Observation, 0, 3)
	for _, rate := range []string{"0.6", "1.2", "2.4"} {
		spread, _, err := steadyState(cfgrun.Subs{
			"market_rate: [1.2]": "market_rate: [" + rate + "]"})
		if err != nil {
			return nil, err
		}
		byMarketable = append(byMarketable,
			claims.Observation{Label: "at " + rate, Value: spread})
	}

	bySize := make([]claims.Observation, 0, 3)
	for _, size := range []string{"4", "8", "16"} {
		fraction, err := survival(size)
		if err != nil {
			return nil, err
		}
		bySize = append(bySize,
			claims.Observation{Label: "size " + size, Value: fraction})
	}

	return []claims.Claim{
		{
			ID: "spread_tightens_as_limit_order_arrivals_increase",
			Statement: "The spread is an OUTPUT of this model, not a parameter — the " +
				"touch is wherever the innermost occupied level happens to be. Raising " +
				"the limit-order arrival rate fills the inner levels faster than they are " +
				"emptied, and the spread narrows.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("mean spread in ticks over two-sided steps, averaged over "+
				"%d seeds; one-sided steps are excluded rather than averaged in", len(seeds)),
			Limitations: "A consequence of the mechanism rather than independent " +
				"evidence: arrivals refill the inner levels, so of course the spread " +
				"narrows. What it establishes is that the output EXISTS and responds, " +
				"which the minimal generator could not offer at all. The model is " +
				"uncalibrated, so the magnitudes mean nothing about a real book.",
			Monotone:     -1,
			Observations: byArrival,
			Binding:      binding,
		},
		{
			ID: "spread_widens_as_marketable_flow_increases",
			Statement: "Raising the rate of marketable orders empties the inner levels " +
				"faster than arrivals replace them, and the spread widens. Together with " +
				"the arrival response this is the spread-response output, which the " +
				"minimal generator could not answer because it had no prices at all.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("mean spread in ticks over two-sided steps, averaged over "+
				"%d seeds", len(seeds)),
			Limitations: "Same caveat as the arrival response — mechanism, not evidence. " +
				"It also understates what happens at the top end: at the highest " +
				"marketable rate a side of the book empties outright on a substantial " +
				"minority of steps, and those steps are excluded from this average " +
				"rather than counted as an infinitely wide spread.",
			Monotone:     1,
			Observations: byMarketable,
			Binding:      binding,
		},
		{
			ID: "resting_liquidity_surviving_a_large_marketable_order_falls_with_its_size",
			Statement: "A marketable order now WALKS THE BOOK rather than being clipped " +
				"at the touch, so the fraction of resting liquidity left standing falls " +
				"as the order grows. This is the output the minimal generator could not " +
				"support even in principle: there, consumption was capped at the touch, " +
				"so the surviving fraction was a constant of ladder geometry rather than " +
				"a response to the order.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("percent of ask-side depth remaining immediately after a "+
				"marketable buy of the stated size, averaged over %d seeds", len(seeds)),
			Limitations: "Measured on an uncalibrated model with eight price levels, so " +
				"it says nothing about how a real book absorbs size. The response also " +
				"saturates: orders at or above the total resting depth clear the side " +
				"entirely and leave the book one-sided, so sizes were chosen below that " +
				"cliff to measure a graded response rather than a floor.",
			Monotone:     -1,
			Observations: bySize,
			Binding:      binding,
		},
	}, nil
}
