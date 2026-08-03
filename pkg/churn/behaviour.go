// Package churn scores the pre-registered predictions for cfg/lob_churn.yaml.
//
// # The result: A ✓, B ✗, C ✓ — and B was the one that mattered
//
// PREREGISTRATION.md fixed three predictions and committed them BEFORE the model
// existed, precisely because this is the hypothesis most at risk of being confirmed
// by construction.
//
//	A  corr(arrivals, cancels) > +0.5     measured +0.62   PASS
//	B  corr(depth, cancels)    < +0.2     measured +0.60   FAIL
//	C  dispersion              > 1.5      measured 27/12/4 PASS
//
// A was recorded in advance as near-certain and low-information: couple two streams
// and they correlate. It is claimed here only so it cannot later be presented as a
// discovery.
//
// # C is a genuine positive, and the first of its kind here
//
// A Poisson mixed over a gamma intensity is over-dispersed, and the measured
// variance/mean of 27.4 and 12.3 is the first departure from Poisson any model in this
// project produces without being told to — the two earlier models sit at ~1.0 by
// construction. The claim is that the driver PRODUCES overdispersion, not that it
// matches any particular market magnitude.
//
// # B failed, and the diagnosis is not what I expected
//
// I expected a common activity factor to couple everything to everything, including
// depth to cancellations. It does not. Measured against the priced model:
//
//	                  corr(depth, arrivals)   corr(depth, cancels)
//	priced                    -0.015                 +0.638
//	churn                     +0.019                 +0.596
//	crypto (Binance)          -0.21                  -0.12
//
// Arrivals are UNCORRELATED with depth in the churn model. Cancellations remain
// strongly coupled to it. The crypto row is Binance data, recorded from public
// endpoints. The asymmetry is the clue: arrivals
// have no depth-dependent term anywhere, while the cancellation path still has two —
// the residual attrition drawn as poisson(cancel_rate * resting), and the min(...)
// clip that stops volume going negative.
//
// So making churn dominate the cancellation flow did not remove the depth
// dependence, because the depth-dependent parts of that path were still there.
//
// # What this does and does not settle
//
// The pre-registered reading for "A ✓, B ✗" was: churn reproduces the co-movement
// but not the missing coupling, so churn is not the mechanism. That reading stands
// as written, and this package pins it.
//
// But it should be reported as INCONCLUSIVE ABOUT THE MECHANISM rather than as a
// refutation of it, for a reason that is a fault in my own pre-registration. The
// stated parameter criterion — "churn flow comparable to arrival flow" — does not
// pin the regime, because what governs whether the clip binds is churn relative to
// DEPTH, not to arrivals. At the rates chosen the intended churn is a large
// fraction of the resting book each step, so the clip binds often and mechanically
// ties cancellations to depth.
//
// Re-testing in a regime where churn is a modest fraction of depth needs a FRESH
// pre-registration, not a re-run of this one. Adjusting rates now and reporting a
// pass would be exactly the failure the pre-registration exists to prevent.
// # RE-SCORED ON ENSEMBLES 2026-08-02 — every verdict unchanged
//
// Every number here is now a 32-member ensemble mean at 8000 steps rather than one seed.
// Values moved slightly — A +0.62 to +0.60, C 27.4/12.3 to 26.4/12.0, E's drift 2.72 to
// 2.90 — and no verdict changed. Like pkg/recycled, this block's results were never close
// enough to their bounds for the seed to decide them.
package churn

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/baseline"
	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — the churn generator, measured with the same code as the " +
		"real-market diagnostics"

	// The pre-registered bounds, unchanged since PREREGISTRATION.md was committed.
	churnFloor      = 0.5
	couplingCeiling = 0.2
	dispersionFloor = 1.5

	// lowChurnRate is the rate D/E/F were scored at. It was originally fitted to a
	// lowChurnRate is an ARBITRARY value, pinned only so the D/E/F scores stay
	// reproducible. It is NOT fitted to market data and must not be described as such.
	lowChurnRate = "1.15"

	// D/E/F's pre-registered bounds, for the attrition-removed variant.
	driftFloor      = 1.5
	spreadCeiling   = 2.5
	spreadSDCeiling = 0.5
)

// model is the churn generator's row layout, matching cfg/lob_churn.yaml.
var model = baseline.Model{
	Label: "churn", Config: "lob_churn.yaml", Partition: "lob_churn",
	Steps: "max_steps: 400", LongSteps: "max_steps: 2000",
	Limit: 16, Cancel: 17, Market: 18, Depth: 19,
}

// noAttrition runs the low-churn model with the depth-proportional attrition term
// removed, returning the four quantities D/E/F are scored on.
//
// The config default keeps attrition at 0.08, the value in place when the churn
// ratio was fitted, so the D/E scoring stays reproducible. This variant is reached
// by substitution rather than by moving the default.
func noAttrition() (
	coupling, coMovement, drift, spread, spreadSD cfgrun.EnsembleStat,
	err error,
) {
	stores, err := cfgrun.RunEnsemble("lob_churn.yaml", cfgrun.Subs{
		"max_steps: 400":      fmt.Sprintf("max_steps: %d", cfgrun.DefaultSteps),
		"churn_rate: [3.0]":   "churn_rate: [" + lowChurnRate + "]",
		"cancel_rate: [0.08]": "cancel_rate: [0.0]",
	}, cfgrun.DefaultSeeds)
	if err != nil {
		return coupling, coMovement, drift, spread, spreadSD, err
	}
	var cs, ms, ds, ss, sds []float64
	for _, storage := range stores {
		rows := storage.GetValues("lob_churn")
		if len(rows) <= 100 {
			return coupling, coMovement, drift, spread, spreadSD,
				fmt.Errorf("churn: a member produced too few rows")
		}
		rows = rows[100:]
		segment := diagnostics.Segment{Rows: rows}
		arrivals := segment.Column(model.Limit)
		cancels := segment.Column(model.Cancel)
		depth := segment.Column(model.Depth)

		// Stationarity: a book with a restoring force has this near 1, a drifting one
		// does not. Comparing halves rather than fitting a trend keeps it robust to the
		// shape of the drift.
		half := len(depth) / 2
		ds = append(ds, diagnostics.Mean(depth[half:])/diagnostics.Mean(depth[:half]))
		cs = append(cs, diagnostics.Correlation(depth, cancels))
		ms = append(ms, diagnostics.Correlation(arrivals, cancels))

		// One-sided steps carry the sentinel, not a spread, so they are excluded.
		observed := make([]float64, 0, len(rows))
		for _, row := range rows {
			if row[20] < 99 {
				observed = append(observed, row[20])
			}
		}
		if len(observed) == 0 {
			return coupling, coMovement, drift, spread, spreadSD,
				fmt.Errorf("churn: a member was one-sided at every step")
		}
		mean := diagnostics.Mean(observed)
		variance := 0.0
		for _, x := range observed {
			variance += (x - mean) * (x - mean)
		}
		ss = append(ss, mean)
		sds = append(sds, math.Sqrt(variance/float64(len(observed))))
	}
	return cfgrun.Summarise(cs), cfgrun.Summarise(ms), cfgrun.Summarise(ds),
		cfgrun.Summarise(ss), cfgrun.Summarise(sds), nil
}

// ObservedBehaviour scores the pre-registered predictions.
func ObservedBehaviour() []claims.Claim {
	coupling, churn, dispersion, err := baseline.Measure(model)
	if err != nil {
		panic("churn: measuring observed behaviour: " + err.Error())
	}
	noAttrCoupling, noAttrCoMovement, drift, spread, spreadSD, err := noAttrition()
	if err != nil {
		panic("churn: measuring the attrition-free variant: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestChurnPredictions",
		TestFile: "pkg/churn/behaviour_test.go",
	}

	return []claims.Claim{
		{
			ID: "prediction_a_a_shared_driver_couples_arrivals_and_cancellations",
			Statement: "Prediction A, PASSED. A shared per-step activity factor makes " +
				"arrivals and cancellations move together, where both earlier models had " +
				"them independent at about zero.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between per-step arrival and cancellation counts",
			Limitations: "Near-certain by construction and recorded as such BEFORE the " +
				"model was written: coupling two streams through one driver and then " +
				"reporting that they are coupled is not evidence of anything. It is " +
				"claimed only so it cannot later be presented as a discovery. It also " +
				"reaches only +0.60, so it does not even match " +
				"the magnitude.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: churnFloor, RefLabel: "+0.5 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "arrivals vs cancellations", Value: churn.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_b_churn_fails_to_break_the_depth_coupling",
			Statement: "Prediction B, FAILED — and it was the one that mattered. The " +
				"depth/cancellation coupling was predicted to collapse below +0.2 and " +
				"instead stayed near where the priced model left it. Real markets read " +
				"about zero. Making churn dominate the cancellation FLOW did not remove " +
				"the depth dependence, because the cancellation PATH still contains " +
				"depth-dependent terms: residual attrition drawn against resting volume, " +
				"and the clip that stops volume going negative.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between resting depth and cancellation flow",
			Limitations: "INCONCLUSIVE ABOUT THE MECHANISM rather than a refutation of " +
				"it, because of a fault in my own pre-registration: the stated parameter " +
				"criterion (\"churn comparable to arrival flow\") does not pin the regime, " +
				"since what decides whether the clip binds is churn relative to DEPTH. At " +
				"these rates churn is a large fraction of the book each step. Re-testing " +
				"in a lighter regime needs a fresh pre-registration, not a re-run of this " +
				"one — adjusting rates now and reporting a pass is precisely what " +
				"pre-registration exists to prevent.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: couplingCeiling,
					RefLabel: "+0.2 (the pre-registered bound it had to fall below)"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs cancellations", Value: coupling.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_c_a_shared_driver_reproduces_real_overdispersion",
			Statement: "Prediction C, PASSED. A Poisson count mixed over a gamma " +
				"intensity is over-dispersed, and the measured variance/mean lands far " +
				"above 1 where both earlier models sit at about 1.0 by construction — a " +
				"consequence of the shared driver rather than something it was told to " +
				"do. No real-market magnitude is compared against: the claim is that the " +
				"driver PRODUCES overdispersion, not that it matches one.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "variance / mean of per-step counts (Poisson requires exactly 1)",
			Limitations: "Very weak evidence for a mechanism — many processes are " +
				"over-dispersed, and no real-market magnitude is claimed. It says a " +
				"common activity factor is SUFFICIENT for overdispersion, not that it " +
				"is what real markets do.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: dispersionFloor, RefLabel: "1.5 (pre-registered)"},
				{ObsIndex: 1, GreaterThan: true, Ref: dispersionFloor, RefLabel: "1.5 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "arrivals", Value: dispersion[0].Mean},
				{Label: "cancellations", Value: dispersion[1].Mean},
				{Label: "market orders", Value: dispersion[2].Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_d_removing_attrition_eliminates_the_depth_coupling",
			Statement: "Prediction D, PASSED — and recorded in advance as near-forced, " +
				"because deleting the depth-proportional cancellation term is deleting " +
				"the coupling. With attrition gone the depth/cancellation correlation " +
				"falls to ~0 and the co-movement improves to +0.95. These are statements " +
				"about the model alone; no market comparison is made here.",
			Gate:  "2.2",
			Phase: phase,
			Data: "synthetic — churn generator at an arbitrary churn_rate of " +
				lowChurnRate + " with the attrition term removed. Model-internal only: " +
				"no real-market comparison is made",
			Unit: "Pearson correlations at churn_rate " + lowChurnRate +
				" with cancel_rate 0",
			Limitations: "This is the prediction that could not fail, and it is claimed " +
				"only so it cannot be presented as the result. Matching four " +
				"correlations is worth much less than it sounds given what it cost — see " +
				"[[prediction_e_removing_attrition_destroys_depth_stationarity]], which " +
				"is the finding.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: couplingCeiling,
					RefLabel: "+0.2 (pre-registered)"},
				{ObsIndex: 1, GreaterThan: true, Ref: 0.9, RefLabel: "+0.9"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs cancellations", Value: noAttrCoupling.Mean},
				{Label: "arrivals vs cancellations", Value: noAttrCoMovement.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_e_removing_attrition_destroys_depth_stationarity",
			Statement: "Prediction E, PASSED, and THIS is the finding. Attrition is the " +
				"model's only depth-stabilising force — arrivals, churn and the " +
				"marketable sweep are all depth-independent — so removing it leaves depth " +
				"a random walk with drift. The book grows without bound: mean depth in " +
				"the second half of the run is nearly three times the first half, where a " +
				"conserved book would give ~1. So the depth/cancellation " +
				"coupling is fixable ONLY by deleting the mechanism that conserves the " +
				"book, which trades a correlation failure for a worse one.",
			Gate:  "2.2",
			Phase: phase,
			Data: "synthetic — churn generator with attrition removed. The stationarity " +
				"target is conservation of the book (ratio ~1), not a market measurement",
			Unit: "mean resting depth over the second half of the scored window divided " +
				"by the mean over the first half (a conserved book gives ~1)",
			Limitations: "Measured on one run length; a longer run would drift further, " +
				"so the ratio is a symptom rather than a calibrated quantity. It also " +
				"does not prove no depth-independent stabiliser exists — only that this " +
				"model has none once attrition is gone.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: driftFloor,
					RefLabel: "1.5 (pre-registered; arithmetic predicted ~2.7)"},
			},
			Observations: []claims.Observation{
				{Label: "second half / first half", Value: drift.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_f_removing_attrition_collapses_the_spread",
			Statement: "Prediction F, PASSED, and it was the least certain of the three. " +
				"A book that grows without bound keeps its inner levels permanently " +
				"occupied, so the touch never moves and the spread sits at its floor with " +
				"zero variance. The spread-response output that the priced model unlocked " +
				"is therefore destroyed by this variant: there is nothing left for it to " +
				"respond with.",
			Gate:  "4.2",
			Phase: phase,
			Data:  "synthetic — churn generator with attrition removed",
			Unit:  "mean and standard deviation of the spread in ticks, over two-sided steps",
			Limitations: "A consequence of the unbounded growth rather than an " +
				"independent failure, so it is really the same finding measured a second " +
				"way. It is worth stating separately because it shows the cost reaching a " +
				"Spike 4.2 output, not just an internal diagnostic.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: spreadCeiling,
					RefLabel: "2.5 ticks (pre-registered)"},
				{ObsIndex: 1, GreaterThan: false, Ref: spreadSDCeiling,
					RefLabel: "0.5 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "mean spread", Value: spread.Mean},
				{Label: "spread sd", Value: spreadSD.Mean},
			},
			Binding: binding,
		},
	}
}
