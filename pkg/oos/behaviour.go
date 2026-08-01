// Package oos scores the out-of-sample test: does the calibrated model still agree with
// the market in a window it was never fitted to?
//
// # What makes this different from every other package here
//
// pkg/damping fitted one parameter to one Binance number measured on 2026-07-30, and two
// further numbers landed without being fitted to. That is in-sample agreement in time:
// every number came from one Thursday-morning window. This package records a SECOND
// window — Saturday 2026-08-01, same five symbols, same protocol, same eight minutes —
// and asks whether the frozen model still agrees.
//
// THE MODEL IS FROZEN. cfg/lob_damping.yaml as shipped, damping_gamma 0.6. Nothing is
// refitted here and nothing may ever be: re-tuning after seeing this window would destroy
// the only out-of-sample evidence the project has. TestTheModelIsFrozen asserts it.
//
// # The gate that decides whether this test means anything
//
// A fresh window reading the same as the old one tests nothing — the model would
// "predict" numbers that never changed. PREREGISTRATION.md fixes the gate in advance: if
// the fresh five-symbol mean corr(depth, arrivals) is within 0.03 of the old -0.2128, the
// result is WEAK BY CONSTRUCTION whatever AH-AK say. The market's own drift is reported
// alongside the model's error on every claim, so the strength of the test is visible
// rather than asserted.
//
// # Why these numbers cannot reach CLAIMS.md
//
// They need recorded Binance segments, which the licence does not permit redistributing.
// This package is therefore NOT registered in internal/claimset, exactly as pkg/crypto and
// pkg/replication are not: publishing the numbers would imply a guarantee that anyone can
// re-derive them, and they cannot without recording their own window. The result is
// recorded in DECISIONS.md with its provenance, and these tests enforce it for anyone
// holding the data.
package oos

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
	"github.com/umbralcalc/cryptobook/pkg/feed"
)

const (
	dampingConfig = "lob_damping.yaml"
	partition     = "lob_damping"

	phase   = "2 — Residual diagnostics"
	dataset = "Binance spot, a SECOND eight-minute window (2026-08-01, Saturday) the " +
		"model was never fitted to, against the frozen cfg/lob_damping.yaml"

	// Pre-registered in f166872, before the window was recorded.
	tolerance       = 0.12
	coMovementBound = 0.15
	weakGate        = 0.03

	// The old window's five-symbol means, which gamma was fitted against.
	oldArrivalMean = -0.2128
	oldCancelMean  = -0.1266

	settleFrom = 100
	idxDepthM  = 19
	idxLimitM  = 16
	idxCancelM = 17
)

var symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT"}

// oldPerSymbol is the 2026-07-30 window, for reporting the market's own drift.
var oldPerSymbol = map[string][2]float64{
	"BTCUSDT":  {-0.267, -0.220},
	"ETHUSDT":  {-0.339, -0.246},
	"SOLUSDT":  {-0.121, -0.074},
	"XRPUSDT":  {-0.131, -0.015},
	"DOGEUSDT": {-0.206, -0.078},
}

func segmentPath(symbol string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("oos: cannot locate this package's source path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "dat", "oos_"+symbol+".log")
}

// Available reports whether all five fresh segments are present. All or nothing: a
// partial set would silently weaken the mean the predictions are scored on.
func Available() bool {
	for _, s := range symbols {
		if _, err := os.Stat(segmentPath(s)); err != nil {
			return false
		}
	}
	return true
}

type segment struct {
	symbol                      string
	arrival, cancel, coMovement float64
	rows                        int
}

type result struct {
	fresh                                  []segment
	freshArrival, freshCancel, freshCoMove float64
	modelArrival, modelCancel, modelCoMove float64
	driftArrival, driftCancel              float64
}

func measure() (result, error) {
	var r result
	for _, s := range symbols {
		seg, _, err := diagnostics.LoadSegment(segmentPath(s))
		if err != nil {
			return r, fmt.Errorf("%s: %w", s, err)
		}
		depth := seg.Column(feed.IdxDepthStart)
		limit := seg.Column(feed.IdxLimit)
		cancel := seg.Column(feed.IdxCancel)
		r.fresh = append(r.fresh, segment{
			symbol:     s,
			arrival:    diagnostics.Correlation(depth, limit),
			cancel:     diagnostics.Correlation(depth, cancel),
			coMovement: diagnostics.Correlation(limit, cancel),
			rows:       len(depth),
		})
	}
	for _, f := range r.fresh {
		r.freshArrival += f.arrival / float64(len(r.fresh))
		r.freshCancel += f.cancel / float64(len(r.fresh))
		r.freshCoMove += f.coMovement / float64(len(r.fresh))
	}
	r.driftArrival = r.freshArrival - oldArrivalMean
	r.driftCancel = r.freshCancel - oldCancelMean

	// The frozen model, read from the shipped config rather than hardcoded, so a config
	// edit cannot silently detach these predictions from the model they are about.
	storage, err := cfgrun.Run(dampingConfig, cfgrun.Subs{"max_steps: 400": "max_steps: 2000"})
	if err != nil {
		return r, err
	}
	rows := storage.GetValues(partition)[settleFrom:]
	m := diagnostics.Segment{Rows: rows}
	r.modelArrival = diagnostics.Correlation(m.Column(idxDepthM), m.Column(idxLimitM))
	r.modelCancel = diagnostics.Correlation(m.Column(idxDepthM), m.Column(idxCancelM))
	r.modelCoMove = diagnostics.Correlation(m.Column(idxLimitM), m.Column(idxCancelM))
	return r, nil
}

// Weak reports whether the pre-registered gate fired: a window that did not move enough
// to test anything.
func (r result) Weak() bool { return math.Abs(r.driftArrival) < weakGate }

// orderingHolds counts the fresh segments on which arrivals are the stronger brake.
func (r result) orderingHolds() (int, string) {
	count, failed := 0, ""
	for _, f := range r.fresh {
		if math.Abs(f.arrival) > math.Abs(f.cancel) {
			count++
		} else if failed == "" {
			failed = f.symbol
		}
	}
	return count, failed
}

// ObservedBehaviour scores the pre-registered predictions AH, AI, AJ and AK.
//
// THREE OF FOUR FAILED, and the failure is the point of having run it.
func ObservedBehaviour() []claims.Claim {
	r, err := measure()
	if err != nil {
		panic("oos: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestOutOfSampleWindow",
		TestFile: "pkg/oos/behaviour_test.go",
	}
	held, failedOn := r.orderingHolds()
	drift := claims.Observation{
		Label: "market drift in the arrival correlation between windows",
		Value: r.driftArrival}

	return []claims.Claim{
		{
			ID: "prediction_ah_the_fitted_correlation_does_not_survive_a_second_window",
			Statement: "Prediction AH, FAILED, and it is the most informative failure " +
				"this recording could have produced. The damping exponent was fitted to " +
				"corr(depth, arrivals) = -0.2128, the five-symbol mean of a Thursday " +
				"morning. Two days later the same five symbols over the same eight " +
				"minutes read -0.068 — the market moved +0.145, and BTCUSDT alone moved " +
				"+0.325 from -0.267 to +0.058. The frozen model sits 0.166 away, outside " +
				"the pre-registered 0.12. THE FIT TARGET WAS A PROPERTY OF ONE WINDOW, " +
				"not of this market, and the calibration must be restated as calibrated " +
				"to a transient.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "absolute difference between the frozen model and the fresh five-symbol " +
				"mean; and the market's own drift between the two windows",
			Limitations: "Two windows two days apart on one venue. This shows the " +
				"signature is not stable ACROSS THESE TWO WINDOWS; it does not establish " +
				"a distribution for how much it varies, which would need many more " +
				"recordings. Nor does it identify what changed — Saturday flow is " +
				"thinner and less intermediated than Thursday's, which is a plausible " +
				"cause and an untested one. The pre-registered weak-test gate did NOT " +
				"fire (drift 0.145 against a 0.03 threshold), so the window genuinely " +
				"differed and the test is not vacuous.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: tolerance,
					RefLabel: "0.12 (the pre-registered tolerance it exceeded)"},
				{ObsIndex: 1, GreaterThan: true, Ref: weakGate,
					RefLabel: "0.03 (the gate the market cleared, so the test is real)"},
			},
			Observations: []claims.Observation{
				{Label: "model minus fresh mean, absolute",
					Value: math.Abs(r.modelArrival - r.freshArrival)},
				{Label: "market drift, absolute", Value: math.Abs(r.driftArrival)},
				{Label: "fresh five-symbol mean", Value: r.freshArrival},
			},
			Binding: binding,
		},
		{
			ID: "prediction_ai_the_held_out_prediction_does_not_survive_out_of_sample",
			Statement: "Prediction AI, FAILED. This was the test. In-sample the model " +
				"put corr(depth, cancels) at -0.138 against a market -0.127, within " +
				"0.011 of a number it was never fitted to — the result the calibration " +
				"block reported. Out of sample that agreement does not survive: the " +
				"fresh five-symbol mean is +0.035, having moved +0.162 and changed SIGN, " +
				"and the frozen model sits 0.173 away against a pre-registered 0.12. The " +
				"in-sample agreement was real and it was not predictive.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "absolute difference between the frozen model and the fresh five-symbol " +
				"mean; and the market's own drift between the two windows",
			Limitations: "This does not show the model is wrong about a stable market — " +
				"it shows the market was not stable between these two windows, and the " +
				"model cannot track that because nothing in it varies by window. A model " +
				"that DID track it would need a regime that changes over hours, which " +
				"none of the mechanisms tested here has. It also does not retract the " +
				"in-sample number, which stands as measured: what it retracts is the " +
				"reading that the in-sample agreement was evidence of prediction.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: tolerance,
					RefLabel: "0.12 (the pre-registered tolerance it exceeded)"},
			},
			Observations: []claims.Observation{
				{Label: "model minus fresh mean, absolute",
					Value: math.Abs(r.modelCancel - r.freshCancel)},
				{Label: "market drift, absolute", Value: math.Abs(r.driftCancel)},
				{Label: "fresh five-symbol mean", Value: r.freshCancel},
				drift,
			},
			Binding: binding,
		},
		{
			ID: "prediction_aj_the_co_movement_gap_stayed_bounded",
			Statement: "Prediction AJ, PASSED, and it passed partly because the MARKET " +
				"came down rather than because the model went up. The bound asked that " +
				"the model's +0.876 stay within 0.15 of the fresh mean; the fresh mean " +
				"is +0.904 against the old window's +0.94-+0.98, so the gap is 0.028. " +
				"Quote churn is the one signature that stayed in the same region across " +
				"both windows, and it is the signature the model has always been closest " +
				"to.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "absolute difference between the frozen model and the fresh five-symbol mean",
			Limitations: "A bound, deliberately loose, on the one quantity that did not " +
				"move much — so passing it says less than AH and AI failing. Note the " +
				"market's co-movement DID fall, from a +0.940 floor across five symbols " +
				"to +0.852 on XRPUSDT and +0.867 on DOGEUSDT, so the cross-segment " +
				"replication's +0.9 floor would not hold on this window. That is recorded " +
				"here as context and is not a rescoring of that claim, which was " +
				"pre-registered against the segments it was measured on.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: coMovementBound,
					RefLabel: "0.15 (pre-registered bound)"},
			},
			Observations: []claims.Observation{
				{Label: "model minus fresh mean, absolute",
					Value: math.Abs(r.modelCoMove - r.freshCoMove)},
				{Label: "fresh five-symbol mean", Value: r.freshCoMove},
			},
			Binding: binding,
		},
		{
			ID: "prediction_ak_the_brake_ordering_does_not_replicate_on_every_segment",
			Statement: "Prediction AK, FAILED. Arrivals were the stronger brake on all " +
				"six segments recorded before this window, and on four of the five here " +
				"— but not on " + failedOn + ", where the arrival correlation is +0.020 " +
				"and the cancellation correlation +0.040. AK was scored per segment " +
				"rather than on the mean precisely because one counterexample matters " +
				"more than an average, and this is that counterexample. With both " +
				"correlations near zero in this window the ordering has little left to " +
				"order.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "number of the five fresh segments on which arrivals are the stronger brake",
			Limitations: "The ordering failing when both quantities are within 0.05 of " +
				"zero is much weaker evidence than it would be at the magnitudes the old " +
				"window showed — it may be noise around zero rather than a reversal. " +
				"That reading does not rescue the prediction, which was written without " +
				"a magnitude qualifier, but it does mean this failure carries less than " +
				"AH's and AI's.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 5,
					RefLabel: "5 (AK required all five)"},
			},
			Observations: []claims.Observation{
				{Label: "segments where arrivals are the stronger brake", Value: float64(held)},
			},
			Binding: binding,
		},
	}
}
