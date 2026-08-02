// Package noisefloor measures how much this project's correlations move between windows
// when nothing else changes — the number every other correlation here should be read
// against, and which none of them has ever carried.
//
// # Why this exists
//
// The out-of-sample test found the five-symbol mean corr(depth, arrivals) moving 0.145
// and corr(depth, cancels) 0.162 between Thursday and Saturday. The tempting reading is a
// weekday/weekend regime change. That reading is unavailable until the ORDINARY
// variability of these quantities is known, and it never has been: every correlation in
// CLAIMS.md and DECISIONS.md is quoted to three decimals with nothing beside it.
//
// # The asymmetry this design has, stated before the result
//
// Repeat windows can REFUTE the regime reading — if windows minutes apart vary as much as
// two days apart did, the shift needs no explanation. They cannot CONFIRM it, because
// variance may grow with separation. This is a falsification test, run first because it
// is the cheap half and can make the expensive half unnecessary.
//
// # What is measured
//
// Five windows inside one Sunday morning at ten-minute starts, plus the Saturday window
// recorded ~23 hours earlier, all five symbols, identical protocol throughout. Two ranges
// come out of that and they answer different questions:
//
//   - the WITHIN-MORNING range over the five Sunday windows — the noise floor at the
//     minutes scale, which understates variability because adjacent windows share market
//     conditions;
//   - the SIX-WINDOW range including Saturday — a ~23-hour figure, much nearer the
//     timescale of the gap being explained, and what AL and AM are scored on.
//
// # Why these numbers cannot reach CLAIMS.md
//
// They need recorded Binance segments, which the licence does not permit redistributing,
// so this package is not registered in internal/claimset — as pkg/crypto, pkg/replication
// and pkg/oos are not. The result is recorded in DECISIONS.md with its provenance.
package noisefloor

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
	"github.com/umbralcalc/cryptobook/pkg/feed"
)

const (
	phase   = "2 — Residual diagnostics"
	dataset = "Binance spot — six weekend windows (five inside one Sunday morning at " +
		"ten-minute starts, plus the Saturday window ~23 hours earlier), five symbols each"

	// Pre-registered in 89776b3, as corrected in the commit that followed it. These are
	// the Thursday-to-Saturday gaps the within-weekend ranges must come in under.
	arrivalGap      = 0.145
	cancelGap       = 0.162
	couplingCeiling = 0.2

	// wanderFloor is DESCRIPTIVE and post-hoc — no bound on the noise floor was
	// pre-registered, because measuring it was the point rather than predicting it. It
	// asserts the direction that matters: the wander is LARGE. If a future change made
	// these correlations materially steadier, this claim would break and say so, which
	// is the only way a measured floor stays honest as the code moves under it.
	wanderFloor = 0.05

	// The two reference means AN separates on.
	saturdayCancelMean = 0.035
	thursdayCancelMean = -0.1266
)

var symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT"}

// windows are the six, in recording order. The Saturday one is the out-of-sample capture
// already on disk; nf1..nf5 are the Sunday morning repeats.
var windows = []struct{ label, prefix string }{
	{"Sat 08:51", "oos_"},
	{"Sun w1", "nf1_"}, {"Sun w2", "nf2_"}, {"Sun w3", "nf3_"},
	{"Sun w4", "nf4_"}, {"Sun w5", "nf5_"},
}

func segmentPath(prefix, symbol string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("noisefloor: cannot locate this package's source path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "dat", prefix+symbol+".log")
}

// Available reports whether all six windows are present for all five symbols. All or
// nothing: a partial set would change the range these predictions are scored on.
func Available() bool {
	for _, w := range windows {
		for _, s := range symbols {
			if _, err := os.Stat(segmentPath(w.prefix, s)); err != nil {
				return false
			}
		}
	}
	return true
}

type windowResult struct {
	label                                   string
	arrivalMean, cancelMean, coMovementMean float64
	perSymbolCancel                         []float64
	worstCancel                             float64
	worstSymbol                             string
}

type result struct {
	perWindow []windowResult
	// Ranges: six-window (what AL and AM score) and within-Sunday (the noise floor).
	arrivalRange, cancelRange                                    float64
	arrivalRangeSunday, cancelRangeSunday, coMovementRangeSunday float64
	nearerSaturday                                               int
	worstCancel                                                  float64
	worstWhere                                                   string
}

func rangeOf(values []float64) float64 {
	lo, hi := values[0], values[0]
	for _, v := range values[1:] {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return hi - lo
}

func measure() (result, error) {
	var r result
	r.worstCancel = math.Inf(-1)
	for _, w := range windows {
		wr := windowResult{label: w.label, worstCancel: math.Inf(-1)}
		for _, s := range symbols {
			seg, _, err := diagnostics.LoadSegment(segmentPath(w.prefix, s))
			if err != nil {
				return r, fmt.Errorf("%s %s: %w", w.label, s, err)
			}
			depth := seg.Column(feed.IdxDepthStart)
			a := diagnostics.Correlation(depth, seg.Column(feed.IdxLimit))
			c := diagnostics.Correlation(depth, seg.Column(feed.IdxCancel))
			wr.arrivalMean += a / float64(len(symbols))
			wr.cancelMean += c / float64(len(symbols))
			wr.coMovementMean += diagnostics.Correlation(
				seg.Column(feed.IdxLimit), seg.Column(feed.IdxCancel)) / float64(len(symbols))
			wr.perSymbolCancel = append(wr.perSymbolCancel, c)
			if c > wr.worstCancel {
				wr.worstCancel, wr.worstSymbol = c, s
			}
			if c > r.worstCancel {
				r.worstCancel, r.worstWhere = c, w.label+" "+s
			}
		}
		if math.Abs(wr.cancelMean-saturdayCancelMean) < math.Abs(wr.cancelMean-thursdayCancelMean) {
			r.nearerSaturday++
		}
		r.perWindow = append(r.perWindow, wr)
	}
	arrivals := make([]float64, len(r.perWindow))
	cancels := make([]float64, len(r.perWindow))
	coMove := make([]float64, len(r.perWindow))
	for i, w := range r.perWindow {
		arrivals[i], cancels[i], coMove[i] = w.arrivalMean, w.cancelMean, w.coMovementMean
	}
	r.arrivalRange, r.cancelRange = rangeOf(arrivals), rangeOf(cancels)
	r.arrivalRangeSunday, r.cancelRangeSunday = rangeOf(arrivals[1:]), rangeOf(cancels[1:])
	r.coMovementRangeSunday = rangeOf(coMove[1:])
	return r, nil
}

// ObservedBehaviour scores the pre-registered predictions AL, AM, AN and AO, and pins the
// measured noise floor that is this block's real deliverable.
func ObservedBehaviour() []claims.Claim {
	r, err := measure()
	if err != nil {
		panic("noisefloor: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestNoiseFloor",
		TestFile: "pkg/noisefloor/behaviour_test.go",
	}

	return []claims.Claim{
		{
			ID: "these_correlations_wander_by_about_a_tenth_between_windows_minutes_apart",
			Statement: "THE NOISE FLOOR, and the deliverable this block existed for. Five " +
				"eight-minute windows inside ONE Sunday morning, ten minutes apart, same " +
				"five symbols, identical protocol: the five-symbol mean corr(depth, " +
				"arrivals) spans 0.079 and corr(depth, cancels) spans 0.112. Nothing " +
				"changed between those windows except the clock. Every correlation this " +
				"project has published is quoted to three decimals with nothing beside " +
				"it, and this is what should be beside it. The co-movement is steadier but " +
				"not steady: it spans 0.032 over the same five windows, which is the same " +
				"size as the margin prediction M was scored on.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "range (max - min) of the five-symbol mean correlation across five " +
				"windows inside one morning",
			Limitations: "DESCRIPTIVE and post-hoc: no bound on this was pre-registered, " +
				"and it is claimed to pin a measured fact rather than to score a " +
				"prediction. It is also an UNDERSTATEMENT — windows ten minutes apart " +
				"share market conditions, so the true variability at longer separations " +
				"is at least this and probably more. Five windows give a range, not a " +
				"distribution; a standard error would need far more. It says nothing " +
				"about variability at other window lengths, and an eight-minute window is " +
				"a choice this project made rather than a property of the market.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: wanderFloor,
					RefLabel: "0.05 (descriptive, post-hoc)"},
				{ObsIndex: 1, GreaterThan: true, Ref: wanderFloor,
					RefLabel: "0.05 (descriptive, post-hoc)"},
			},
			Observations: []claims.Observation{
				{Label: "within-morning range, depth vs arrivals", Value: r.arrivalRangeSunday},
				{Label: "within-morning range, depth vs cancellations", Value: r.cancelRangeSunday},
				{Label: "within-morning range, arrivals vs cancellations", Value: r.coMovementRangeSunday},
			},
			Binding: binding,
		},
		{
			ID: "prediction_al_the_arrival_correlation_range_stays_under_the_two_day_gap",
			Statement: "Prediction AL, PASSED, and the margin is what matters rather than " +
				"the verdict. The six-window range of the mean corr(depth, arrivals) is " +
				"0.111 against the 0.145 Thursday-to-Saturday gap it had to come in " +
				"under — so the gap is only about 1.3x the ordinary spread of weekend " +
				"windows, and about 1.8x the spread of windows ten minutes apart. AL " +
				"passing does NOT establish that the two-day gap is a regime change; it " +
				"establishes that it is barely larger than noise.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "range of the five-symbol mean corr(depth, arrivals) across six windows; " +
				"and across the five within one morning",
			Limitations: "A range over six windows is a weak statistic and this one is " +
				"close to its bound. The pre-registered reading treated an AL pass as " +
				"clearing the way for a weekday study; that reading assumed a comfortable " +
				"margin and there is not one. Read together with " +
				"[[prediction_an_the_windows_do_not_group_by_day]], which failed, the " +
				"honest conclusion is that no day-type effect has been demonstrated.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: arrivalGap,
					RefLabel: "0.145 (the Thursday-to-Saturday gap)"},
			},
			Observations: []claims.Observation{
				{Label: "six-window range", Value: r.arrivalRange},
				{Label: "within-morning range", Value: r.arrivalRangeSunday},
			},
			Binding: binding,
		},
		{
			ID: "prediction_am_the_cancellation_correlation_range_almost_equals_the_two_day_gap",
			Statement: "Prediction AM, PASSED BY 0.008. The six-window range of the mean " +
				"corr(depth, cancels) is 0.155 against a bound of 0.162 — that is 95% of " +
				"the entire Thursday-to-Saturday gap, reproduced among weekend windows " +
				"where no day changed at all. Treating this as a pass is correct by the " +
				"letter of the pre-registration and misleading by its spirit: the " +
				"quantity that moved 0.162 between the two days moves 0.155 when nothing " +
				"moves but the clock.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "range of the five-symbol mean corr(depth, cancels) across six windows; " +
				"and across the five within one morning",
			Limitations: "A margin of 0.008 on a range computed from six windows carries " +
				"no information about which side of the bound the truth lies; a seventh " +
				"window could put it either way. The pre-registration did not anticipate " +
				"a pass this narrow and its outcome table has no row for one, so the " +
				"reading below is written after the fact and is weaker than a " +
				"pre-registered one.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: cancelGap,
					RefLabel: "0.162 (the Thursday-to-Saturday gap)"},
			},
			Observations: []claims.Observation{
				{Label: "six-window range", Value: r.cancelRange},
				{Label: "within-morning range", Value: r.cancelRangeSunday},
			},
			Binding: binding,
		},
		{
			ID: "prediction_an_the_windows_do_not_group_by_day",
			Statement: "Prediction AN, FAILED, and it is the decisive one. AN required " +
				"all six weekend windows to sit nearer the Saturday reference (+0.035) " +
				"than the Thursday one (-0.127) on the cancellation side. Only three do " +
				"— and one of those three is the Saturday window itself, so of the five " +
				"genuinely new windows, two land nearer THURSDAY than nearer the other " +
				"weekend window recorded a day earlier. The windows do not group by day. " +
				"Whatever moved between Thursday and Saturday is not a day-type effect, " +
				"and a weekday study would be chasing the wrong variable.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "number of the six weekend windows nearer the Saturday reference than the Thursday one",
			Limitations: "One of the six is the Saturday reference itself and passes " +
				"trivially, so this is really five tests and the claim is stated on six " +
				"because that is how it was pre-registered. The two references are " +
				"0.162 apart, which is about the size of the noise floor measured here, " +
				"so 'nearer to' is a weak discriminator — that weakness is itself the " +
				"finding rather than a caveat against it.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 6,
					RefLabel: "6 (AN required all six)"},
			},
			Observations: []claims.Observation{
				{Label: "windows nearer Saturday", Value: float64(r.nearerSaturday)},
			},
			Binding: binding,
		},
		{
			ID: "prediction_ao_the_absent_coupling_holds_across_thirty_measurements",
			Statement: "Prediction AO, PASSED, and this is now the most heavily tested " +
				"result in the project. corr(depth, cancels) stays below +0.2 on all " +
				"five symbols in all six windows — thirty measurements — with a worst " +
				"case of +0.083. The coupling the model's parameterisation requires, " +
				"cancel_rate x resting depth, is absent everywhere it has ever been " +
				"looked for: two days, seven windows counting Thursday, five instruments. " +
				"It is the one conclusion in this project that has never weakened, and " +
				"the noise floor measured here does not threaten it — the margin to the " +
				"bound is 0.117, larger than the wander.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "worst (least negative) corr(depth, cancels) across five symbols x six windows",
			Limitations: "Absence of a positive coupling is not evidence for any " +
				"particular alternative, and this says nothing about which mechanism " +
				"produces real cancellation flow — four candidates have been eliminated " +
				"and none confirmed. All six windows are weekend mornings on one venue " +
				"within 24 hours; the Thursday segments are the only weekday data the " +
				"project has.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: couplingCeiling,
					RefLabel: "+0.2 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "worst of thirty", Value: r.worstCancel},
			},
			Binding: binding,
		},
	}
}
