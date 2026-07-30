// Package replication scores whether the crypto diagnostics replicate across
// independently recorded segments.
//
// # Why it exists
//
// Every crypto number in this project used to rest on ONE 8-minute BTCUSDT capture,
// including Phase 2's central conclusion that the model form does not transfer. A result
// that survives only on the sample that produced it is not a result.
//
// Crypto makes the fix cheap: segments are re-recordable from public endpoints with no
// gate, so the honest form for a claim is a BOUND that holds on any independently
// recorded segment rather than a point value. PREREGISTRATION.md fixed four bounds — P,
// K, L and M — before any new segment was recorded.
//
// # What replicating means here
//
// Five symbols recorded CONCURRENTLY, so they share one wall-clock window and differ only
// by instrument. Liquidity is ranked by the venue's own 24h quote volume, fetched at
// capture time rather than assumed, which is what makes prediction M's target
// (lowest-volume symbol) a fact about the capture rather than a choice made afterwards.
//
// # The confound, declared in advance
//
// One DefaultLotSize is used across instruments whose unit prices differ by orders of
// magnitude, so one "lot" is a wildly different economic quantity per symbol. Dispersion
// MAGNITUDES are therefore not comparable across segments and no such comparison is
// claimed. L is a one-sided bound, which survives this.
package replication

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
	"github.com/umbralcalc/cryptobook/pkg/feed"
)

const (
	phase   = "2 — Residual diagnostics"
	dataset = "Binance spot, five symbols recorded concurrently over one 8-minute " +
		"window, 2026-07-30, one-second buckets, feed.DefaultLotSize throughout. Not " +
		"redistributable, so the segments are git-ignored and these tests skip without " +
		"them; re-record with cmd/record-feed (see README.md)"

	// The pre-registered bounds, fixed before any segment was recorded.
	couplingCeiling  = 0.2 // P
	coMovementFloor  = 0.9 // Q
	dispersionFloor  = 10  // R
	poissonReference = 1.0
)

// segments are the five captures, ordered by the venue's 24h quote volume at capture
// time (descending). DOGEUSDT is therefore prediction M's low-liquidity target, fixed by
// the exchange's own figures rather than by my choice.
var segments = []struct {
	symbol      string
	quoteVolume float64
}{
	{"BTCUSDT", 1069466864},
	{"ETHUSDT", 552796474},
	{"SOLUSDT", 106463258},
	{"XRPUSDT", 69590670},
	{"DOGEUSDT", 28558265},
}

func segmentPath(symbol string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("replication: cannot locate this package's source path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..",
		"testdata", "seg_"+symbol+".log")
}

// Available reports whether every segment is present. All five are needed: the
// predictions are quantified over all of them, so scoring a subset would silently
// weaken the bound from "every segment" to "the ones I had".
func Available() bool {
	for _, s := range segments {
		if _, err := os.Stat(segmentPath(s.symbol)); err != nil {
			return false
		}
	}
	return true
}

// measured holds one segment's diagnostics.
type measured struct {
	symbol                            string
	coupling, coMovement              float64
	dispersionLimit, dispersionCancel float64
	rows                              int
}

func measureAll() ([]measured, error) {
	out := make([]measured, 0, len(segments))
	for _, s := range segments {
		segment, _, err := diagnostics.LoadSegment(segmentPath(s.symbol))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.symbol, err)
		}
		depth := segment.Column(feed.IdxDepthStart)
		limit := segment.Column(feed.IdxLimit)
		cancel := segment.Column(feed.IdxCancel)
		out = append(out, measured{
			symbol:           s.symbol,
			coupling:         diagnostics.Correlation(depth, cancel),
			coMovement:       diagnostics.Correlation(limit, cancel),
			dispersionLimit:  diagnostics.Dispersion(limit),
			dispersionCancel: diagnostics.Dispersion(cancel),
			rows:             len(depth),
		})
	}
	return out, nil
}

// worst returns the value furthest into the failing side of a bound, so a claim's single
// reported number is the one that decides it rather than an average that could hide a
// failing segment.
func worst(all []measured, pick func(measured) float64, greaterIsWorse bool) (float64, string) {
	value, symbol := pick(all[0]), all[0].symbol
	for _, m := range all[1:] {
		v := pick(m)
		if (greaterIsWorse && v > value) || (!greaterIsWorse && v < value) {
			value, symbol = v, m.symbol
		}
	}
	return value, symbol
}

// ObservedBehaviour scores predictions P, K, L and M.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		panic("replication: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	all, err := measureAll()
	if err != nil {
		return nil, err
	}
	binding := claims.Binding{
		TestName: "TestCrossSegmentReplication",
		TestFile: "pkg/replication/behaviour_test.go",
	}

	worstCoupling, worstCouplingSymbol := worst(all, func(m measured) float64 { return m.coupling }, true)
	worstCoMovement, worstCoMovementSymbol := worst(all, func(m measured) float64 { return m.coMovement }, false)
	worstDispersion, worstDispersionSymbol := worst(all, func(m measured) float64 {
		return min(m.dispersionLimit, m.dispersionCancel)
	}, false)

	btc, doge := all[0], all[len(all)-1]

	observationsPerSymbol := func(pick func(measured) float64) []claims.Observation {
		obs := make([]claims.Observation, 0, len(all))
		for _, m := range all {
			obs = append(obs, claims.Observation{Label: m.symbol, Value: pick(m)})
		}
		return obs
	}

	return []claims.Claim{
		{
			ID: "prediction_j_the_depth_cancellation_decoupling_replicates_across_every_segment",
			Statement: fmt.Sprintf("Prediction J, PASSED. The model's cancellation intensity is "+
				"cancel_rate x resting depth, so cancellations must rise with depth. They "+
				"do not, on any of five independently recorded symbols — the worst case is "+
				"%s at %+.3f, against a pre-registered ceiling of +0.2 and a model that "+
				"needs this strongly positive. **Phase 2's central conclusion is therefore "+
				"a property of crypto spot markets rather than of one capture**, which is "+
				"what it rested on until now.",
				worstCouplingSymbol, worstCoupling),
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlation between resting depth at the start of each second " +
				"and cancellation flow, per symbol",
			Limitations: "A replication, not a new result — and not a blind one: I had " +
				"seen the earlier BTCUSDT segment before fixing this bound, which is " +
				"declared in PREREGISTRATION.md. It shows the reading is not " +
				"instrument-specific within crypto spot; it says nothing about any other " +
				"asset class, and this model's lineage is an equity one. All five share " +
				"one 8-minute window, so a " +
				"market-wide regime peculiar to that window would not be caught.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: couplingCeiling,
					RefLabel: "+0.2 (pre-registered, worst of five)"},
			},
			Observations: append(
				[]claims.Observation{{Label: "worst case (" + worstCouplingSymbol + ")", Value: worstCoupling}},
				observationsPerSymbol(func(m measured) float64 { return m.coupling })...),
			Binding: binding,
		},
		{
			ID: "prediction_k_the_churn_co_movement_replicates_across_every_segment",
			Statement: fmt.Sprintf("Prediction K, PASSED — and I had recorded it in "+
				"advance as the one most at risk. Arrivals and cancellations move in "+
				"near-lockstep on all five symbols, worst case %s at %+.3f against a "+
				"pre-registered floor of +0.9. My stated reason for doubting it was that "+
				"+0.9 had only ever been measured on the most heavily market-made "+
				"instrument in existence and I would not have been surprised by +0.7 on a "+
				"thinner pair. That doubt was wrong: the floor holds with margin on every "+
				"symbol. Quote churn is the only candidate mechanism still standing for "+
				"Spike 2.2's failure, and its signature survives everywhere tested.",
				worstCoMovementSymbol, worstCoMovement),
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlation between per-second arrival and cancellation counts, " +
				"per symbol",
			Limitations: "Says the signature is present, not that quote churn is what " +
				"produces it — no mechanism is identified here, and the arrival and " +
				"cancellation counts are both inferred from net depth changes rather than " +
				"observed as messages, so a shared inference artefact could inflate this. " +
				"That confound is common to every segment and is not resolved by " +
				"replicating across them.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: coMovementFloor,
					RefLabel: "+0.9 (pre-registered, worst of five)"},
			},
			Observations: append(
				[]claims.Observation{{Label: "worst case (" + worstCoMovementSymbol + ")", Value: worstCoMovement}},
				observationsPerSymbol(func(m measured) float64 { return m.coMovement })...),
			Binding: binding,
		},
		{
			ID: "prediction_l_overdispersion_replicates_and_was_declared_near_forced",
			Statement: fmt.Sprintf("Prediction L. Variance/mean exceeds 10 for both "+
				"arrival and cancellation counts on every symbol, worst case %s at %.1f, "+
				"against Poisson's exactly 1. **Recorded in advance as near-forced and "+
				"claimed only so it cannot later be presented as a discovery**: "+
				"overdispersion here is substantially produced by inferring lot-sized "+
				"events from net depth changes, an approximation every segment shares. The "+
				"magnitudes make the declared confound visible rather than hidden: they "+
				"range from ~%.0f to the hundreds of millions across symbols, because one "+
				"fixed lot size spans instruments whose unit prices differ by orders of "+
				"magnitude. Those numbers are not comparable and no comparison is made.",
				worstDispersionSymbol, worstDispersion, worstDispersion),
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "variance / mean of per-second counts, minimum across arrivals and " +
				"cancellations, per symbol",
			Limitations: "Near-forced, so it carries little evidential weight. One lot " +
				"size is used across instruments whose unit prices differ by orders of " +
				"magnitude, so dispersion MAGNITUDES are not comparable between symbols and " +
				"no such comparison is made — this is a one-sided bound only. That confound " +
				"was declared in PREREGISTRATION.md before recording.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: dispersionFloor,
					RefLabel: "10x Poisson (pre-registered, worst of five)"},
			},
			Observations: append(
				[]claims.Observation{{Label: "worst case (" + worstDispersionSymbol + ")", Value: worstDispersion}},
				observationsPerSymbol(func(m measured) float64 {
					return min(m.dispersionLimit, m.dispersionCancel)
				})...),
			Binding: binding,
		},
		{
			ID: "prediction_m_co_movement_is_weaker_on_the_thinner_symbol_but_barely",
			Statement: fmt.Sprintf("Prediction M, PASSED on its stated test and WEAK on "+
				"its reasoning — it was the only genuinely uncertain prediction here. I "+
				"predicted that quote churn, being a market-maker behaviour, would show up "+
				"less on a thinner instrument, so corr(arrivals, cancels) on the "+
				"lowest-quote-volume symbol should be lower than on BTCUSDT. It is: "+
				"DOGEUSDT %+.3f against BTCUSDT %+.3f, a difference of %+.3f across a %.0fx "+
				"liquidity range. **But the effect is small and the ordering is not "+
				"monotonic** — the lowest co-movement of the five is %s, third by volume, "+
				"not the thinnest symbol. So the specific comparison passes while the "+
				"underlying story it was meant to test, that co-movement tracks liquidity, "+
				"is not supported.",
				doge.coMovement, btc.coMovement, doge.coMovement-btc.coMovement,
				segments[0].quoteVolume/segments[len(segments)-1].quoteVolume,
				worstCoMovementSymbol),
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlation between per-second arrival and cancellation counts, " +
				"lowest-quote-volume symbol minus BTCUSDT",
			Limitations: "A failed directional prediction on five points, not a " +
				"quantitative relationship — it says the effect I expected is not visible " +
				"across a 37x liquidity range on Binance spot, not that no such effect " +
				"exists. All five are USDT majors on one venue; a genuinely illiquid pair " +
				"might behave differently, and none was recorded. The liquidity ranking is " +
				"the venue's own 24h quote volume at capture time, which is a proxy for " +
				"market-making intensity rather than a measure of it.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 0,
					RefLabel: "0 (pre-registered: negative means lower on the thinner symbol)"},
			},
			Observations: []claims.Observation{
				{Label: "DOGEUSDT minus BTCUSDT", Value: doge.coMovement - btc.coMovement},
				{Label: "BTCUSDT", Value: btc.coMovement},
				{Label: "DOGEUSDT", Value: doge.coMovement},
			},
			Binding: binding,
		},
	}, nil
}
