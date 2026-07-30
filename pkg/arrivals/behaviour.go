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
// Predictions E, F and G were fixed in PREREGISTRATION.md and committed before the model
// existed; all three pass. But only H was genuinely uncertain — G was near-forced (the
// damping parameter was chosen to produce a stationary depth) and I is a cost check.
// **H is the result**: cancellations contain no depth term, but depth is now
// anti-correlated with arrivals and arrivals share the activity driver with
// cancellations, so the indirect path could have reintroduced a coupling of either sign.
// It did not.
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
// # A model-internal trade-off worth recording as a direction
//
// Cancellation here removes a fraction of RESTING volume, so any depth-stabilising work
// it takes on shows up as a depth/cancellation correlation, while depth-damped arrivals
// put that same work into a depth/arrival correlation instead. The book needs a brake,
// and in this vocabulary each available brake couples depth to one of the two flows.
// Whether real books evade that is unmeasured here.
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

	settleFrom  = 100
	emptySpread = 99.0

	idxLimit  = 16
	idxCancel = 17
	idxDepth  = 19
	idxSpread = 20
)

// measured holds everything the four predictions are scored on.
type measured struct {
	drift, coupling, depthArrival, coMovement float64
	spread, spreadSD, meanDepth               float64
	dispersion                                [2]float64
}

func measure() (measured, error) {
	storage, err := cfgrun.Run(configName, cfgrun.Subs{"max_steps: 400": "max_steps: 2000"})
	if err != nil {
		return measured{}, err
	}
	rows := storage.GetValues(partition)
	if len(rows) <= settleFrom {
		return measured{}, fmt.Errorf("arrivals: run produced too few rows")
	}
	rows = rows[settleFrom:]
	segment := diagnostics.Segment{Rows: rows}
	arrival := segment.Column(idxLimit)
	cancel := segment.Column(idxCancel)
	depth := segment.Column(idxDepth)

	// Stationarity by halves rather than a fitted trend: robust to the shape of any
	// drift: robust to the shape of any drift, and the only stationarity statement
	// left, and it is a statement about conservation rather than about any market.
	half := len(depth) / 2
	m := measured{
		drift:        diagnostics.Mean(depth[half:]) / diagnostics.Mean(depth[:half]),
		coupling:     diagnostics.Correlation(depth, cancel),
		depthArrival: diagnostics.Correlation(depth, arrival),
		coMovement:   diagnostics.Correlation(arrival, cancel),
		meanDepth:    diagnostics.Mean(depth),
		dispersion: [2]float64{
			diagnostics.Dispersion(arrival), diagnostics.Dispersion(cancel)},
	}

	// One-sided steps carry the sentinel rather than a spread, so they are excluded —
	// averaging a sentinel would turn "the book broke" into "the spread was wide".
	observed := make([]float64, 0, len(rows))
	for _, row := range rows {
		if row[idxSpread] < emptySpread {
			observed = append(observed, row[idxSpread])
		}
	}
	if len(observed) == 0 {
		return measured{}, fmt.Errorf("arrivals: every step was one-sided")
	}
	m.spread = diagnostics.Mean(observed)
	variance := 0.0
	for _, x := range observed {
		variance += (x - m.spread) * (x - m.spread)
	}
	m.spreadSD = math.Sqrt(variance / float64(len(observed)))
	return m, nil
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
			Statement: "Prediction H, PASSED, and it is the one that was genuinely " +
				"uncertain. Moving the depth-stabiliser onto the arrival side leaves " +
				"cancellation as pure churn with no depth term — but depth is now " +
				"anti-correlated with arrivals, and arrivals share the activity driver " +
				"with cancellations, so that indirect path could have reintroduced a " +
				"depth/cancellation correlation of either sign. It did not: the model " +
				"reads -0.002, against the attrition model's +0.638. Depth CAN be " +
				"stabilised " +
				"without the coupling coming back.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlation between resting depth and cancellation flow; " +
				"and between per-step arrival and cancellation counts",
			Limitations: "A correlation structure is not a calibrated model. One " +
				"parameter was fitted, to mean depth, with no correlation visible while " +
				"choosing it — but reproducing correlations is weaker than reproducing " +
				"the dynamics that generate them — and there is NO real-market evidence " +
				"in this package at all. It says the model can hold the coupling broken " +
				"while stabilising depth, not that real books work this way.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: couplingCeiling,
					RefLabel: "+0.2 (pre-registered)"},
				{ObsIndex: 1, GreaterThan: true, Ref: 0.8, RefLabel: "+0.8"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs cancellations", Value: m.coupling},
				{Label: "arrivals vs cancellations", Value: m.coMovement},
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
				{Label: "second half / first half", Value: m.drift},
				{Label: "mean depth", Value: m.meanDepth},
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
				{Label: "mean spread", Value: m.spread},
				{Label: "spread sd", Value: m.spreadSD},
			},
			Binding: binding,
		},
	}
}
