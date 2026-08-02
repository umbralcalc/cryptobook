// Package arrivals scores the depth-dependent-arrivals model — the mechanism that
// stabilises the book without coupling cancellation to depth.
//
// # The question it answers
//
// Removing the depth-proportional attrition term broke the model's correlation
// signatures free of depth and destroyed the model: depth grew without bound and the
// spread collapsed to a constant, because attrition was its only depth-stabilising
// force. That left one narrow question, answerable without any market data — can depth
// be stabilised WITHOUT reintroducing the depth/cancellation coupling?
//
// It can. Moving the stabiliser to the arrival side (posting into a level slows as that
// level fills) keeps cancellation pure churn, and the coupling stays broken:
// corr(depth, cancels) reads -0.002 against the attrition model's +0.638.
//
// # What is and is not established
//
// Predictions G, H and I were fixed in PREREGISTRATION.md and committed before the model
// existed; all three pass. G was near-forced (the damping parameter was chosen to produce
// a stationary depth) and I is a cost check.
//
// H was recorded as the one genuinely uncertain prediction, on the argument that arrivals
// and cancellations share the activity driver so an indirect path could have reintroduced
// a coupling of either sign. **That argument does not survive, and is withdrawn as of
// 2026-07-31.** The driver is drawn iid per step; the depth this correlates against is
// depth at the START of the step, so it depends on activity only up to t-1, while
// cancellation depends on activity at t. They are independent by construction, so this
// correlation sits near zero whatever mechanism is present — the indirect path cannot
// carry a contemporaneous correlation at all.
//
// What H still establishes is narrower and worth keeping: no depth term leaked into
// cancellation. What it does NOT establish is that a coupling was available and avoided.
// That structural point is also why persistence is the next thing tried — see
// PREREGISTRATION.md's X-AB block.
//
//	                        attrition model   this model
//	depth drift                   2.72          1.008
//	corr(depth, cancels)         +0.638        -0.002
//	corr(arrivals, cancels)      +0.950        +0.897
//	spread                     collapsed     2.17 +/- 0.41
//
// **These are statements about the model's internal structure, not about markets.**
// Nothing here is a calibration, nothing is fitted to data, and no claim is made that
// the mechanism resembles a real book. The real-market work in this project is in
// pkg/crypto and pkg/replication, on Binance spot data.
//
// # The parameters are arbitrary, and that is load-bearing
//
// arrival_scale 19.0 and churn_rate 1.15 are **chosen values, not fitted ones**. They
// are pinned only because the G/H/I predictions were scored at them, and the configs and
// tests say so. Describing them as fitted to anything would be false, and fitting them
// would need its own pre-registration.
//
// # RE-SCORED ON ENSEMBLES 2026-08-02 — all verdicts unchanged, one fragility resolved
//
// Every number here is now a 32-member ensemble mean at 8000 steps rather than one seed
// at 2000. G, H and I are unaffected: H reads +0.052 against a +0.2 ceiling where the
// single seed gave -0.002, so the sign moved and the verdict did not.
//
// The descriptive brake claim WAS fragile — the audit found its -0.05 bound sitting 0.001
// from the single-seed spread. On ensemble means the arrival side is -0.077 with a
// standard error of ~0.002, clearing the bound by roughly twelve standard errors, so the
// claim is now a property of the model rather than of the seed.
//
// # A model-internal trade-off, now claimed rather than only noted
//
// Cancellation here removes a fraction of RESTING volume, so any depth-stabilising work
// it takes on shows up as a depth/cancellation correlation, while depth-damped arrivals
// put that same work into a depth/arrival correlation instead. The book needs a brake,
// and in this vocabulary each available brake couples depth to one of the two flows.
// The brake therefore MOVED rather than vanished, and that is pinned as
// depth_stabilisation_moves_the_brake_onto_the_arrival_side: -0.116 on the arrival side
// against -0.002 on the cancellation side.
//
// Whether real books evade the trade-off is measured, but not here — the Binance
// segments cannot be redistributed, so nothing in this package may depend on them.
// DECISIONS.md carries that comparison, and the short version is that the real
// signature is not this one: on every segment recorded, BOTH flows are mildly
// anti-correlated with depth, where this model concentrates the whole brake on one.
//
// # An engine parameterisation error, found and fixed
//
// The engine's gamma is (shape, RATE), not (shape, scale): gamma(2, 0.5) has mean 4.0,
// not 1.0. Both config comments claimed a scale and mean activity 1, so every model here
// has run at MEAN ACTIVITY 4. No scored result changed — the shipped values were never
// altered, only their description.
package arrivals

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	configName = "lob_arrivals.yaml"
	partition  = "lob_arrivals"

	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — depth-dependent-arrivals generator at an arbitrary " +
		"parameterisation. Model-internal only: no real-market comparison is made here"

	// The pre-registered bounds, unchanged since 3b1f756.
	driftCeiling    = 1.3
	couplingCeiling = 0.2
	spreadSDFloor   = 0.1

	// brakeBound pins where the depth-stabilising force sits: the arrival correlation
	// below it, the cancellation correlation above it. Unlike the three above it is
	// DESCRIPTIVE and post-hoc — chosen after measuring, to hold a structural fact the
	// model already has, not to score a prediction. It is stated as a bound anyway so
	// that a change quietly moving the brake back onto cancellation breaks a claim
	// rather than passing silently.
	brakeBound = -0.05

	settleFrom  = 100
	emptySpread = 99.0

	idxLimit  = 16
	idxCancel = 17
	idxDepth  = 19
	idxSpread = 20
)

// measured holds everything the four predictions are scored on.
// measured is this model's ensemble summary — every field a mean over 32 members at
// 8000 steps, with the across-member spread attached. See cfgrun.DefaultSeeds.
type measured struct {
	drift, coupling, depthArrival, coMovement cfgrun.EnsembleStat
	spread, spreadSD, meanDepth               cfgrun.EnsembleStat
	dispersion                                [2]cfgrun.EnsembleStat
}

func measure() (measured, error) {
	stores, err := cfgrun.RunEnsemble(configName, cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", cfgrun.DefaultSteps),
	}, cfgrun.DefaultSeeds)
	if err != nil {
		return measured{}, err
	}
	var drift, can, arr, com, sprMean, sprSD, depth, dispA, dispC []float64
	for _, storage := range stores {
		rows := storage.GetValues(partition)
		if len(rows) <= settleFrom {
			return measured{}, fmt.Errorf("arrivals: a member produced too few rows")
		}
		rows = rows[settleFrom:]
		segment := diagnostics.Segment{Rows: rows}
		arrival := segment.Column(idxLimit)
		cancel := segment.Column(idxCancel)
		d := segment.Column(idxDepth)
		half := len(d) / 2

		drift = append(drift, diagnostics.Mean(d[half:])/diagnostics.Mean(d[:half]))
		can = append(can, diagnostics.Correlation(d, cancel))
		arr = append(arr, diagnostics.Correlation(d, arrival))
		com = append(com, diagnostics.Correlation(arrival, cancel))
		depth = append(depth, diagnostics.Mean(d))
		dispA = append(dispA, diagnostics.Dispersion(arrival))
		dispC = append(dispC, diagnostics.Dispersion(cancel))

		observed := make([]float64, 0, len(rows))
		for _, row := range rows {
			if row[idxSpread] < emptySpread {
				observed = append(observed, row[idxSpread])
			}
		}
		if len(observed) == 0 {
			return measured{}, fmt.Errorf("arrivals: a member was one-sided at every step")
		}
		mean := diagnostics.Mean(observed)
		variance := 0.0
		for _, x := range observed {
			variance += (x - mean) * (x - mean)
		}
		sprMean = append(sprMean, mean)
		sprSD = append(sprSD, math.Sqrt(variance/float64(len(observed))))
	}
	return measured{
		drift:        cfgrun.Summarise(drift),
		coupling:     cfgrun.Summarise(can),
		depthArrival: cfgrun.Summarise(arr),
		coMovement:   cfgrun.Summarise(com),
		spread:       cfgrun.Summarise(sprMean),
		spreadSD:     cfgrun.Summarise(sprSD),
		meanDepth:    cfgrun.Summarise(depth),
		dispersion: [2]cfgrun.EnsembleStat{
			cfgrun.Summarise(dispA), cfgrun.Summarise(dispC)},
	}, nil
}

// ObservedBehaviour scores the pre-registered predictions E, F and G.
func ObservedBehaviour() []claims.Claim {
	m, err := measure()
	if err != nil {
		panic("arrivals: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestDepthDependentArrivals",
		TestFile: "pkg/arrivals/behaviour_test.go",
	}

	return []claims.Claim{
		{
			ID: "prediction_h_stabilising_depth_through_arrivals_keeps_the_coupling_broken",
			Statement: "Prediction H, PASSED. Moving the depth-stabiliser onto the " +
				"arrival side leaves cancellation as pure churn with no depth term, and " +
				"the coupling stays broken: the model reads -0.002, against the attrition " +
				"model's +0.638. Depth CAN be stabilised without the coupling coming " +
				"back. This was recorded at the time as the block's one GENUINELY " +
				"UNCERTAIN prediction, on the argument that arrivals and cancellations " +
				"share the activity driver so an indirect path could have reintroduced a " +
				"coupling of either sign. That framing was wrong and is withdrawn — see " +
				"the qualification below.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlation between resting depth and cancellation flow; " +
				"and between per-step arrival and cancellation counts",
			Limitations: "QUALIFIED 2026-07-31: this was NEARER TO FORCED than it was " +
				"recorded as being, and the reason is structural. The activity driver is " +
				"drawn iid per step; the depth this correlates against is depth at the " +
				"START of the step, so it depends on activity only up to t-1, while " +
				"cancellation depends on activity at t. Those are independent by " +
				"construction, so this correlation sits near zero WHATEVER mechanism is " +
				"present — the indirect path the original statement worried about cannot " +
				"carry a contemporaneous correlation at all. The measured -0.002 still " +
				"establishes that no depth term leaked into cancellation, which is a real " +
				"and checkable property, but it is not evidence that a coupling was " +
				"available and avoided. Separately: a correlation structure is not a " +
				"calibrated model, one parameter was fitted to mean depth, and there is " +
				"NO real-market evidence in this package at all.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: couplingCeiling,
					RefLabel: "+0.2 (pre-registered)"},
				{ObsIndex: 1, GreaterThan: true, Ref: 0.8, RefLabel: "+0.8"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs cancellations", Value: m.coupling.Mean},
				{Label: "arrivals vs cancellations", Value: m.coMovement.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_g_depth_dependent_arrivals_restore_stationarity",
			Statement: "Prediction G, PASSED. Posting into a level slowing as that level " +
				"fills gives the book a restoring force again, and depth becomes " +
				"stationary — 1.008, against 2.72 for the variant with no stabiliser at " +
				"all. A conserved book gives ~1; no market figure is compared against.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "mean resting depth over the second half of the scored window divided " +
				"by the first half (a conserved book gives ~1); and mean depth in lots",
			Limitations: "Near-forced: the one free parameter was fitted precisely to " +
				"make mean depth match the attrition model's, so a stationary result was " +
				"the intended outcome rather than a discovery. Claimed so it is not " +
				"mistaken for the finding, which is " +
				"[[prediction_h_stabilising_depth_through_arrivals_keeps_the_coupling_broken]].",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: driftCeiling,
					RefLabel: "1.3 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "second half / first half", Value: m.drift.Mean},
				{Label: "mean depth", Value: m.meanDepth.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_i_the_spread_survives_depth_dependent_arrivals",
			Statement: "Prediction I, PASSED — the cost check that the previous variant " +
				"failed. A conserved book leaves the touch free to move, so the spread " +
				"keeps a live distribution rather than pinning at its floor with zero " +
				"variance. The spread-response output Spike 4.2 unlocked therefore " +
				"survives this mechanism, which it did not survive the attrition-free one.",
			Gate:  "4.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "mean and standard deviation of the spread in ticks, over two-sided steps",
			Limitations: "It shows the output still exists, not that its distribution " +
				"resembles a real book's — no spread distribution has been compared " +
				"against market data at any point in this project.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 1, GreaterThan: true, Ref: spreadSDFloor,
					RefLabel: "0.1 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "mean spread", Value: m.spread.Mean},
				{Label: "spread sd", Value: m.spreadSD.Mean},
			},
			Binding: binding,
		},
		{
			ID: "depth_stabilisation_moves_the_brake_onto_the_arrival_side",
			Statement: "The book needs a brake, and in this model's vocabulary every " +
				"available brake couples depth to one of the two flows: cancellation " +
				"removes a fraction of RESTING volume, so any stabilising work it does " +
				"shows up as a depth/cancellation correlation, while damping arrivals by " +
				"depth puts that same work into a depth/ARRIVAL correlation instead. So " +
				"the brake did not disappear when the coupling broke, it MOVED — depth " +
				"against arrivals reads -0.116, while depth against cancellations sits at " +
				"-0.002. This is the price of " +
				"[[prediction_h_stabilising_depth_through_arrivals_keeps_the_coupling_broken]], " +
				"recorded so the trade-off is visible rather than only the half of it that " +
				"passed.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between resting depth and each of the two flows",
			Limitations: "RESOLVED BY ENSEMBLING 2026-08-02: the audit found this claim's -0.05 " +
				"bound sitting 0.001 from the single-seed spread, one seed from breaking. " +
				"The 32-member mean is -0.077 with a standard error of ~0.002, so it clears " +
				"the bound by about twelve standard errors and the claim now holds as a " +
				"property of the model. The bound was NOT widened. Separately: the SIGN is " +
				"forced — arrival intensity is damped by resting " +
				"depth, so a negative correlation is what the config states and finding " +
				"one is not a discovery. What this records is the magnitude and which " +
				"flow carries the brake. It is model-internal — no market number appears " +
				"in this package, and whether real books evade the trade-off by splitting " +
				"the brake across both flows needs data that cannot be redistributed, so " +
				"that comparison lives in DECISIONS.md instead of here. The -0.05 bounds " +
				"are descriptive and post-hoc, not pre-registered predictions.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: brakeBound,
					RefLabel: "-0.05 (descriptive)"},
				{ObsIndex: 1, GreaterThan: true, Ref: brakeBound,
					RefLabel: "-0.05 (descriptive)"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs arrivals", Value: m.depthArrival.Mean},
				{Label: "depth vs cancellations", Value: m.coupling.Mean},
			},
			Binding: binding,
		},
	}
}
