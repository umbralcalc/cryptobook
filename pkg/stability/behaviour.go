// Package stability holds Spike 4.2 — the counterfactual output suite —
// and, more importantly, the audit of which of its four outputs this model can
// actually support.
//
// # The audit result: one of four
//
// Spike 4.2 asks which outputs are answerable "given the calibrated
// parameterisation", and instructs that unanswerable ones be MARKED AS SUCH rather
// than approximated, because "an honestly absent output is worth more than a
// plausible one that the calibration doesn't support". Three of the four are
// structurally impossible here, for reasons that are not a matter of tuning:
//
//	spread response to an arrival-intensity shock   NOT ANSWERABLE
//	  The model has no prices. Its ladder is six slots addressed by 0/1 masks
//	  (touch_bid, touch_ask), so there is no mid, no spread, and nothing a spread
//	  could respond with.
//
//	fraction of resting liquidity surviving a
//	large marketable order                          NOT ANSWERABLE
//	  Market orders consume at the touch only and are clipped to what is there
//	  (takes = min(resting, ...)). A "large" order cannot sweep deeper levels, so
//	  the surviving fraction is a fixed property of ladder geometry rather than a
//	  response to the order. The question's entire content is the sweep.
//
//	queue-position distribution under varying
//	tick regimes                                    NOT ANSWERABLE
//	  Volume is a continuous quantity with no order identity and no FIFO queue, so
//	  there is no queue position; and with no prices there is no tick size to vary.
//
//	depth recovery following a liquidity event      ANSWERABLE
//
// Those three verdicts are about THIS model and remain correct for it. All three
// have since been answered by richer models: cfg/lob_priced.yaml added prices, an
// emergent spread and orders that walk the book (pkg/priced, outputs one and two),
// and cfg/lob_queue.yaml added per-order FIFO queues (pkg/queue, output four). Spike
// 4.2 is four of four; what stays true here is that the MINIMAL generator supports
// exactly one of them.
//
// # Why the fourth one works
//
// Depth is what this model is about. Arrivals add at a fixed rate per level and
// cancellations remove in proportion to what is resting, so depth obeys
// dq/dt = limit_rate - cancel_rate*q and relaxes exponentially towards
// limit_rate/cancel_rate with timescale 1/cancel_rate.
//
// That splits into two claims below, and the second is the non-obvious one: the
// LEVEL the book refills to is set by the ratio of the two rates, while the TIME it
// takes is set by the cancellation rate alone. Tripling the arrival rate refills a
// book to nearly three times the depth in the same number of steps.
//
// # Framing
//
// Both are counterfactuals about market state — how much liquidity, and how long it
// takes to come back — with no price and no direction in them. the framing
// discipline is trivially satisfied here, for the unflattering reason that the
// model has no price process to violate it with.
package stability

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

const (
	configName = "lob_depth_recovery.yaml"

	phase = "4 — Stability outputs"
	// The model is synthetic, and after Spike 2.2 it is known NOT to describe either
	// market measured. Saying so in the dataset field means no claim here can be read
	// as being about a real book.
	dataset = "synthetic — the minimal generator, which Spike 2.2 established does " +
		"NOT reproduce real order flow in either market tested"

	// shockStep and the depth column must match cfg/lob_depth_recovery.yaml.
	shockStep     = 201
	colDepthStart = 9

	// baselineFrom/To is the pre-shock window the equilibrium depth is estimated
	// over: after the run has settled, before the shock lands.
	baselineFrom = 100
	baselineTo   = 200

	// recoveredFraction is how much of the pre-shock equilibrium counts as
	// recovered. Depth fluctuates by roughly a sixth of its mean at these rates, so
	// a threshold much above 0.9 would be measuring noise rather than relaxation.
	recoveredFraction = 0.9
)

// seeds are averaged over. A single realisation's crossing time is noisy; the claim
// is about the model's response, not about one run of it.
var seeds = []int{11, 22, 33, 44, 55}

// recovery runs the shock config and returns the seed-averaged recovery time in
// steps and the pre-shock equilibrium depth.
func recovery(subs cfgrun.Subs) (steps, equilibrium float64, err error) {
	totalSteps, totalDepth := 0.0, 0.0
	for _, seed := range seeds {
		withSeed := cfgrun.Subs{"seed: 20260728": fmt.Sprintf("seed: %d", seed)}
		for key, value := range subs {
			withSeed[key] = value
		}
		storage, err := cfgrun.Run(configName, withSeed)
		if err != nil {
			return 0, 0, err
		}
		rows := storage.GetValues("lob_flow")
		if len(rows) <= shockStep+1 {
			return 0, 0, fmt.Errorf("stability: run ended before the shock")
		}
		baseline := 0.0
		for i := baselineFrom; i < baselineTo; i++ {
			baseline += rows[i][colDepthStart]
		}
		baseline /= float64(baselineTo - baselineFrom)

		// The first step at or after the shock whose depth is back to the threshold.
		crossed := -1.0
		for i := shockStep + 1; i < len(rows); i++ {
			if rows[i][colDepthStart] >= recoveredFraction*baseline {
				crossed = float64(i - shockStep)
				break
			}
		}
		if crossed < 0 {
			return 0, 0, fmt.Errorf(
				"stability: depth never returned to %.0f%% of baseline %.1f — the run "+
					"is too short to measure a recovery", recoveredFraction*100, baseline)
		}
		totalSteps += crossed
		totalDepth += baseline
	}
	n := float64(len(seeds))
	return totalSteps / n, totalDepth / n, nil
}

// ObservedBehaviour measures the Spike 4.2 outputs that are answerable.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		panic("stability: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	binding := claims.Binding{
		TestName: "TestDepthRecoveryExpectedBehaviour",
		TestFile: "pkg/stability/behaviour_test.go",
	}

	// Sweep the cancellation rate: the relaxation timescale is 1/cancel_rate, so
	// recovery must get faster as it rises.
	byCancel := make([]claims.Observation, 0, 3)
	for _, rate := range []string{"0.10", "0.15", "0.225"} {
		steps, _, err := recovery(cfgrun.Subs{
			"cancel_rate: [0.15]": "cancel_rate: [" + rate + "]"})
		if err != nil {
			return nil, err
		}
		byCancel = append(byCancel,
			claims.Observation{Label: "at " + rate, Value: steps})
	}

	// Sweep the arrival rate: it sets the level recovered to, not the time taken.
	var fastest, slowest, shallowest, deepest float64
	fastest, shallowest = math.Inf(1), math.Inf(1)
	for _, rate := range []string{"0.8", "1.2", "1.8"} {
		steps, depth, err := recovery(cfgrun.Subs{
			"limit_rate: [1.2]": "limit_rate: [" + rate + "]"})
		if err != nil {
			return nil, err
		}
		fastest, slowest = math.Min(fastest, steps), math.Max(slowest, steps)
		shallowest, deepest = math.Min(shallowest, depth), math.Max(deepest, depth)
	}

	return []claims.Claim{
		{
			ID: "depth_recovery_after_a_liquidity_event_is_faster_when_cancellation_is_faster",
			Statement: "After a liquidity event removes 90% of resting depth, the book " +
				"refills, and it refills faster the higher the cancellation rate is. " +
				"That is not a paradox: the same rate that empties a queue also sets how " +
				"quickly it relaxes back to its own equilibrium, so a book that cancels " +
				"aggressively is also a book that recovers quickly — to a shallower level.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("steps until total resting depth returns to %.0f%% of its "+
				"pre-shock mean, averaged over %d seeds", recoveredFraction*100, len(seeds)),
			Limitations: "A counterfactual about this model, and Spike 2.2 established " +
				"that this model does not reproduce real order flow — so it is not a " +
				"statement about any real book's resilience. It also depends on the " +
				"recovery threshold: depth fluctuates by roughly a sixth of its mean " +
				"here, so a stricter threshold would measure noise as well as relaxation.",
			Monotone: -1,
			Thresholds: []claims.Threshold{
				{ObsIndex: 2, GreaterThan: false, Ref: 10, RefLabel: "10 steps"},
			},
			Observations: byCancel,
			Binding:      binding,
		},
		{
			ID: "recovery_time_is_set_by_the_cancellation_rate_not_the_arrival_rate",
			Statement: "Tripling the limit-order arrival rate refills the book to nearly " +
				"three times the depth in about the same number of steps. The LEVEL " +
				"liquidity returns to is set by the ratio of arrival to cancellation " +
				"rate; the TIME it takes is set by the cancellation rate alone. So a " +
				"venue wanting faster replenishment and a venue wanting deeper books are " +
				"pulling on different levers, and only one of them is the arrival rate.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit: "spread in recovery time across a 0.8-to-1.8 arrival-rate sweep, in " +
				"steps; and the ratio of deepest to shallowest equilibrium depth over " +
				"that same sweep",
			Limitations: "The invariance is exact in the model by construction — depth " +
				"relaxes as dq/dt = limit_rate - cancel_rate*q, whose timescale contains " +
				"no arrival term — so this measurement confirms the implementation " +
				"matches its own mathematics, and is NOT independent evidence about real " +
				"markets. Whether real replenishment separates this cleanly is exactly " +
				"what Spike 2.2 found this model cannot speak to.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 4, RefLabel: "4 steps"},
				{ObsIndex: 1, GreaterThan: true, Ref: 2, RefLabel: "2x"},
			},
			Observations: []claims.Observation{
				{Label: "recovery-time spread", Value: slowest - fastest},
				{Label: "depth ratio", Value: deepest / shallowest},
			},
			Binding: binding,
		},
	}, nil
}
