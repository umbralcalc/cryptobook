// Package crypto holds the residual diagnostics for the recorded crypto segment —
// PLAN.md's Spike 2.2, run against real market data for the first time.
//
// # The result is negative, and it is the useful kind
//
// PLAN.md puts Spike 2.2 before believing any calibration, on the reasoning that
// "residuals bad across the board → the model form is wrong, not the parameters".
// Measured against Binance BTCUSDT spot, that is where this lands, and the specific
// failure is worse than "the fit is loose":
//
//	corr(depth, n_cancel)     ~ -0.1   the model REQUIRES this to be strongly positive
//	corr(n_limit, n_cancel)   ~ +0.98  a mechanism the model does not have at all
//	Var/Mean of the counts     >  1000  where Poisson requires exactly 1
//
// The first line is the one that stops Phase 2. The model's cancellation intensity
// is `cancel_rate x resting depth`, and PREREGISTRATION.md called that "the most
// plausible of the model's assumptions and the one Phase 1's identification rests
// on", with the pre-committed consequence: "If this one fails, the parameterisation
// does not transfer to real data at all and Phase 2 stops rather than being tuned."
// It failed. So no parameters are fitted here.
//
// The second line says what is there instead. Arrivals and cancellations move in
// near-lockstep because market makers replace quotes — pulling and re-posting
// together — and the model treats the two as independent streams. This is not a
// mis-tuned rate; it is a missing mechanism.
//
// The third was predicted, but the predicted MECHANISM was wrong, and the
// difference matters. PREREGISTRATION.md attributed overdispersion to lot
// discretisation, which would make it curable by choosing a larger lot. Raising the
// lot tenfold made the measured dispersion WORSE, because the dominant cause is
// cross-level correlation — a market maker adding or pulling a whole ladder at once
// — which no choice of lot size can absorb.
//
// # Why no calibration is run here
//
// The machinery would happily produce a tight posterior on this data; the
// synthetic phases show it converges. Producing a fitted cancellation rate for a
// market whose cancellations do not scale with depth would be precisely the
// confident, unsupported number the project exists to avoid, and the bar for not
// doing it was set in advance rather than after seeing the result.
package crypto

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
	"github.com/umbralcalc/cryptobook/pkg/feed"
)

const (
	phase = "2 — First contact with real market data"

	// dataset names the market explicitly, so a reader cannot merge results measured
	// on different symbols or venues.
	dataset = "Binance BTCUSDT spot, one recorded 8-minute segment, 2026-07-28 " +
		"(crypto spot — one venue, one symbol, one window)"

	// limitations is shared by every claim here, because they all inherit the same
	// narrow provenance.
	limitations = "One symbol, one venue, one eight-minute window, on the most " +
		"liquid crypto pair there is. It establishes that the model form does not " +
		"transfer to THIS market; it is not evidence about equities, about other " +
		"pairs, or about other times of day. Nothing here is a directional claim " +
		"about price."

	// poissonDispersion is what the model's likelihood assumes: Var/Mean = 1.
	poissonDispersion = 1.0
)

// FixturePath returns the committed capture the diagnostics run against.
//
// The segment is committed rather than re-recorded so these numbers regenerate in
// CI without a network — which is also the replay harness PLAN.md's Spike 3.3 asks
// for ("capture a live segment, replay it in CI"). Re-recording would make every
// claim here move on every run and pin nothing.
func FixturePath() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("crypto: cannot locate this package's source path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..",
		"testdata", "btcusdt_depth.log")
}

// Available reports whether the fixture is present.
//
// It is absent by default: the fixture cannot be committed, because the provider's
// licence does not permit redistributing their data or data derived from it
// (README.md quotes it). So these diagnostics are verifiable by anyone who
// obtains the source data, and enforced by nobody automatically — which is the
// honest consequence, and is why they do not appear in the generated CLAIMS.md.
func Available() bool {
	_, err := os.Stat(FixturePath())
	return err == nil
}

// ObservedBehaviour measures the Spike 2.2 diagnostics.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		panic("crypto: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	segment, _, err := diagnostics.LoadSegment(FixturePath())
	if err != nil {
		return nil, err
	}
	binding := claims.Binding{
		TestName: "TestCryptoResidualDiagnostics",
		TestFile: "pkg/crypto/behaviour_test.go",
	}

	depth := segment.Column(feed.IdxDepthStart)
	limit := segment.Column(feed.IdxLimit)
	cancel := segment.Column(feed.IdxCancel)
	market := segment.Column(feed.IdxMarket)

	return []claims.Claim{
		{
			ID: "crypto_cancellation_flow_does_not_scale_with_resting_depth",
			Statement: "The model's cancellation intensity is cancel_rate x resting " +
				"depth, so cancellations must rise with depth. On real data they do not: " +
				"the correlation is approximately zero and slightly negative. This is the " +
				"coupling the whole synthetic identification result rests on, and it is " +
				"absent — so the parameterisation does not transfer to this market, and " +
				"no cancellation rate is fitted from it.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation with resting depth at the start of each second",
			Limitations: limitations + " The pre-committed consequence of this " +
				"failing was that Phase 2 stops rather than being tuned, so it is acted " +
				"on rather than reported around. Measured on one symbol over one window; " +
				"a cross-segment check is pre-registered to establish whether it is specific to either.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 0.2, RefLabel: "+0.2"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs cancellations", Value: diagnostics.Correlation(depth, cancel)},
				{Label: "depth vs arrivals", Value: diagnostics.Correlation(depth, limit)},
				{Label: "depth vs market orders", Value: diagnostics.Correlation(depth, market)},
			},
			Binding: binding,
		},
		{
			ID: "crypto_quote_arrivals_and_cancellations_move_together",
			Statement: "Arrivals and cancellations are almost perfectly correlated " +
				"second by second, which is quote churn: market makers pull and re-post " +
				"together. The model treats the two as independent Poisson streams, so " +
				"this is a missing mechanism rather than a mis-tuned rate — and it is the " +
				"most likely explanation for why cancellations track arrivals instead of " +
				"depth.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between per-second lot counts",
			Limitations: limitations + " The churn interpretation is inference from a " +
				"correlation, not a measurement of order intent — the recorded segment " +
				"aggregates per second and cannot attribute a cancellation to the " +
				"participant who then re-posted.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: 0.9, RefLabel: "+0.9"},
			},
			Observations: []claims.Observation{
				{Label: "arrivals vs cancellations", Value: diagnostics.Correlation(limit, cancel)},
			},
			Binding: binding,
		},
		{
			ID: "crypto_count_dispersion_far_exceeds_poisson",
			Statement: "The Poisson likelihood requires variance to equal the mean. In " +
				"the recorded flow the ratio is in the hundreds to thousands, so the " +
				"likelihood is badly misspecified and any interval it produces would be " +
				"far too narrow. Predicted in advance — but the predicted CAUSE was " +
				"wrong: attributing it to lot discretisation implied a larger lot would " +
				"cure it, and raising the lot tenfold made it worse. The dominant cause " +
				"is cross-level correlation, a whole ladder being added or pulled at " +
				"once, which no lot size can absorb.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "variance / mean of per-second lot counts (Poisson requires 1)",
			Limitations: limitations + " Dispersion is measured on the aggregate over " +
				"a 0-100bp band; a narrower band or a longer bucket would give different " +
				"numbers. The finding is the order of magnitude, not the value.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: 10 * poissonDispersion, RefLabel: "10x Poisson"},
				{ObsIndex: 1, GreaterThan: true, Ref: 10 * poissonDispersion, RefLabel: "10x Poisson"},
				{ObsIndex: 2, GreaterThan: true, Ref: 10 * poissonDispersion, RefLabel: "10x Poisson"},
			},
			Observations: []claims.Observation{
				{Label: "arrivals", Value: diagnostics.Dispersion(limit)},
				{Label: "cancellations", Value: diagnostics.Dispersion(cancel)},
				{Label: "market orders", Value: diagnostics.Dispersion(market)},
			},
			Binding: binding,
		},
	}, nil
}
