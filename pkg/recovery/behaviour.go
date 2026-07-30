// Package recovery holds the Phase 1 / Spike 1.2 result: can the inference stack
// recover known parameters from order flow it generated itself?
//
// The answer measured here is *partly*, and the mechanism matters more than the
// verdict. Three findings, each a claim below:
//
//  1. All three parameters ARE identified — the log-likelihood surface peaks at the
//     true value of each, including the cancellation rate, which is identifiable
//     only through its coupling to queue depth. So the parameterisation is sound
//     and PLAN.md's "recovery fails generally → the parameterisation is wrong"
//     branch is ruled out by evidence rather than by argument.
//
//  2. Importance sampling nevertheless has an effective sample size of ~1. The
//     log-likelihood is a windowed cumulative over 100 steps and 3 count streams,
//     so nearby proposals differ by tens of nats and the weights are a hard argmax.
//
//  3. The consequence is specific, not diffuse. With ESS ≈ 1 the posterior mean is
//     the best single sample ever drawn, and the proposal is centred on it — so the
//     procedure is a stochastic hill-climb. Coordinates that dominate the
//     log-likelihood (the arrival and cancellation rates) climb to within 9% of
//     truth across every setting tested. The weakly-identified coordinate does not
//     climb at all: it inherits whatever value the winning sample happened to carry.
//
// That last point is why this matters beyond Phase 1. Phase 2's deliverable is
// calibrated parameters WITH uncertainty, and a posterior whose ESS is 1 has no
// usable uncertainty at all — its variance is the spread of one point. This is
// PLAN.md's documented switch signal, and per the plan it is escalated rather than
// worked around. See DECISIONS.md.
package recovery

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

const (
	surfaceConfig  = "lob_likelihood_surface.yaml"
	recoveryConfig = "lob_recovery.yaml"

	phase   = "1 — Synthetic parameter recovery"
	dataset = "synthetic — the model's own generated order flow, no market data"

	// tolerance is the PRE-REGISTERED recovery tolerance: 25% relative error on
	// each parameter. Fixed before any tuning run, per PLAN.md ("define the
	// tolerance before running; do not tune it to the result"). It has not moved
	// since — the weakly-identified parameter missed it and stayed missed rather
	// than the bound being widened to admit it.
	tolerance = 0.25

	// weakAdvantageCeiling separates "strongly identified" from "weakly identified"
	// in nats of log-likelihood advantage. The two strong parameters clear 70 nats;
	// the weak one does not reach 25. The gap is a factor of several, so the exact
	// cutoff is not load-bearing.
	weakAdvantageCeiling = 25.0

	// essProposals is how many draws the ESS measurement uses, and essCeiling is
	// the value below which the sampler is degenerate. ESS measures ~1.0, so a
	// ceiling of 2 out of 16 is a wide margin around a stark result.
	essProposals = 16
	essCeiling   = 2.0
)

// trueRates is the recovery target — it MUST equal cfg/lob_recovery.yaml's
// lob_flow params. TestTruthMatchesTheConfig asserts that mechanically, so this
// cannot drift from the config it describes.
var trueRates = [3]float64{1.2, 0.15, 0.8}

// priorRates is the off-truth starting point, ~1/3 of every true value, so
// convergence cannot be an artifact of starting at the answer.
var priorRates = [3]float64{0.4, 0.05, 0.3}

// proposalVariance is the diagonal of the config's default_covariance. The ESS
// measurement has to draw from the same distribution the config samples from, so
// these numbers are duplicated from the YAML — and TestProposalMatchesTheConfig
// asserts the two agree, by looking for these exact literals in the config.
//
// The VARIANCE is stored rather than the standard deviation on purpose: the config
// states variances, so storing those keeps the drift check an exact string match.
// Deriving them from sd would reintroduce float noise (0.4*0.4 is
// 0.16000000000000003, which no config would ever spell).
var proposalVariance = [3]float64{0.36, 0.0064, 0.16}

// rateNames label the three parameters in claim statements and observations.
var rateNames = [3]string{"limit arrival rate", "cancellation rate", "market order rate"}

// surfaceSubs renders the substitution that sets the fixed parameter vector the
// likelihood surface is evaluated at.
func surfaceSubs(at [3]float64) cfgrun.Subs {
	return cfgrun.Subs{
		"sampled: [1.2, 0.15, 0.8]": fmt.Sprintf(
			"sampled: [%g, %g, %g]", at[0], at[1], at[2]),
	}
}

// loglikeAt evaluates the windowed log-likelihood with the parameters held fixed at
// `at`. No sampler is involved, so this measures identification alone.
func loglikeAt(at [3]float64) (float64, error) {
	storage, err := cfgrun.Run(surfaceConfig, surfaceSubs(at))
	if err != nil {
		return 0, err
	}
	row, err := cfgrun.LastRow(storage, "lob_likelihood")
	if err != nil {
		return 0, err
	}
	// The comparison partition's embedded state ends with the cumulative loglike.
	return row[len(row)-1], nil
}

// advantage returns how many nats the true value of parameter i beats a value
// scaled by `factor`, holding the other two at truth. Positive means the surface
// prefers the truth, which is what identification means.
func advantage(i int, factor float64) (float64, error) {
	atTruth, err := loglikeAt(trueRates)
	if err != nil {
		return 0, err
	}
	off := trueRates
	off[i] *= factor
	atOff, err := loglikeAt(off)
	if err != nil {
		return 0, err
	}
	return atTruth - atOff, nil
}

// effectiveSampleSize draws proposals from the config's own proposal distribution,
// evaluates the log-likelihood at each, and returns the importance-sampling ESS
// (Σw)²/Σw² together with how far behind the runner-up is, in nats.
//
// The gap is reported alongside because it is the mechanism: ESS ≈ 1 is the
// symptom, and "the second-best draw is tens of nats behind" is the cause.
func effectiveSampleSize() (ess float64, runnerUpGap float64, err error) {
	// Fixed seed: this is a measurement that has to be reproducible in CI, and the
	// claim's recorded number would otherwise move on every run.
	random := rand.New(rand.NewSource(7))
	loglikes := make([]float64, 0, essProposals)
	for range essProposals {
		var at [3]float64
		for i := range at {
			at[i] = random.NormFloat64()*math.Sqrt(proposalVariance[i]) + priorRates[i]
		}
		loglike, err := loglikeAt(at)
		if err != nil {
			return 0, 0, err
		}
		loglikes = append(loglikes, loglike)
	}
	best, second := math.Inf(-1), math.Inf(-1)
	for _, loglike := range loglikes {
		if loglike > best {
			best, second = loglike, best
		} else if loglike > second {
			second = loglike
		}
	}
	// Weights normalised by the max before exponentiating — the raw log-likelihoods
	// are around -600, so exp() of them underflows to zero and the ESS would come
	// out as 0/0.
	sum, sumSquares := 0.0, 0.0
	for _, loglike := range loglikes {
		weight := math.Exp(loglike - best)
		sum += weight
		sumSquares += weight * weight
	}
	return sum * sum / sumSquares, best - second, nil
}

// setting is one (truth, prior) pair the recovery is run at.
type setting struct {
	label string
	truth [3]float64
	prior [3]float64
	// levelDepth is the stationary per-level resting volume, limit_rate/cancel_rate,
	// so the run starts at its own fixed point and needs no burn-in.
	levelDepth float64
}

// settings are the three true-parameter settings PLAN.md's third success criterion
// requires, including one near a range boundary. In every case the prior is placed
// well off truth — and in the boundary case ABOVE it, so the estimator has to come
// down rather than up, which is the harder direction when a rate cannot go negative.
var settings = []setting{
	{"nominal", [3]float64{1.2, 0.15, 0.8}, [3]float64{0.4, 0.05, 0.3}, 8},
	{"high rates", [3]float64{2.0, 0.25, 1.5}, [3]float64{0.67, 0.083, 0.5}, 8},
	{"near lower boundary", [3]float64{0.5, 0.05, 0.3}, [3]float64{1.5, 0.15, 0.9}, 10},
}

// recoverAt runs the full posterior estimation for one setting and returns the
// final posterior mean.
func recoverAt(s setting) ([3]float64, error) {
	depth := s.levelDepth
	subs := cfgrun.Subs{
		"limit_rate: [1.2]":   fmt.Sprintf("limit_rate: [%g]", s.truth[0]),
		"cancel_rate: [0.15]": fmt.Sprintf("cancel_rate: [%g]", s.truth[1]),
		"market_rate: [0.8]":  fmt.Sprintf("market_rate: [%g]", s.truth[2]),
		// One key, three sites: the posterior mean's default, the sampler's default
		// and the inner intensity placeholder all carry the prior, and all three must
		// move together.
		"[0.4, 0.05, 0.3]": fmt.Sprintf("[%g, %g, %g]", s.prior[0], s.prior[1], s.prior[2]),
		"[8.0, 8.0, 8.0, 8.0, 8.0, 8.0, 0.0, 0.0, 0.0, 48.0]": fmt.Sprintf(
			"[%g, %g, %g, %g, %g, %g, 0.0, 0.0, 0.0, %g]",
			depth, depth, depth, depth, depth, depth, 6*depth),
	}
	storage, err := cfgrun.Run(recoveryConfig, subs)
	if err != nil {
		return [3]float64{}, err
	}
	row, err := cfgrun.LastRow(storage, "lob_post_mean")
	if err != nil {
		return [3]float64{}, err
	}
	if len(row) != 3 {
		return [3]float64{}, fmt.Errorf(
			"recovery: posterior mean has width %d, expected 3", len(row))
	}
	return [3]float64{row[0], row[1], row[2]}, nil
}

// relativeErrors returns the per-parameter relative error, as a percentage.
func relativeErrors(got, truth [3]float64) [3]float64 {
	var errors [3]float64
	for i := range got {
		errors[i] = math.Abs(got[i]-truth[i]) / truth[i] * 100
	}
	return errors
}

// ObservedBehaviour measures and returns the Spike 1.2 claims.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		panic("recovery: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	binding := claims.Binding{
		TestName: "TestSyntheticRecoveryExpectedBehaviour",
		TestFile: "pkg/recovery/behaviour_test.go",
	}
	set := make([]claims.Claim, 0, 6)

	// ── Identification: does the surface peak at the truth? ────────────────────
	//
	// Evaluated at 0.6x and 1.67x each parameter — multiplicative and symmetric in
	// log space, so neither side is favoured by the choice of probe.
	advantages := make([][2]float64, 3)
	for i := range advantages {
		below, err := advantage(i, 0.6)
		if err != nil {
			return nil, err
		}
		above, err := advantage(i, 1.0/0.6)
		if err != nil {
			return nil, err
		}
		advantages[i] = [2]float64{below, above}
	}

	identification := []struct {
		id        string
		statement string
	}{
		{
			"limit_arrival_rate_is_identified_from_order_flow",
			"The windowed log-likelihood peaks at the true limit-order arrival rate: " +
				"the truth scores higher than either 0.6x or 1.67x it, by tens of nats. " +
				"This is a property of the likelihood, shared by both samplers.",
		},
		{
			"cancellation_rate_is_identified_through_queue_depth",
			"The cancellation rate is identified, and identified ONLY through its " +
				"coupling to queue depth: its expected count is cancel_rate x resting " +
				"depth, so nothing but the step-to-step variation in depth distinguishes " +
				"it. The log-likelihood peaks at the true value by tens of nats.",
		},
	}
	for i, claim := range identification {
		set = append(set, claims.Claim{
			ID:        claim.id,
			Statement: claim.statement,
			Gate:      "1.2",
			Phase:     phase,
			Data:      dataset,
			Unit: "log-likelihood advantage of the true value over an off-truth value, " +
				"in nats, on a 100-step window",
			Limitations: "Identification is not recovery. This says the information is " +
				"present in the observables, not that the sampler finds it — and it says " +
				"nothing about whether the same parameter is identifiable from real " +
				"message data, where the model is misspecified rather than exact.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: 0, RefLabel: "0 nats"},
				{ObsIndex: 1, GreaterThan: true, Ref: 0, RefLabel: "0 nats"},
			},
			Observations: []claims.Observation{
				{Label: "vs 0.6x", Value: advantages[i][0]},
				{Label: "vs 1.67x", Value: advantages[i][1]},
			},
			Binding: binding,
		})
	}

	// The market-order rate is identified too, but an order of magnitude more
	// weakly — and that quantitative gap is the whole explanation for which
	// parameters recover below. Asserting a CEILING as well as a floor is what makes
	// this a claim about weakness rather than just another identification claim.
	set = append(set, claims.Claim{
		ID: "market_order_rate_is_only_weakly_identified",
		Statement: "The market-order arrival rate is identified, but far more weakly " +
			"than the other two: its log-likelihood advantage is under 25 nats where " +
			"theirs exceed 70, because its expected count is ~0.8 per step against " +
			"~7 for both other streams.",
		Gate:  "1.2",
		Phase: phase,
		Data:  dataset,
		Unit: "log-likelihood advantage of the true value over an off-truth value, " +
			"in nats, on a 100-step window",
		Limitations: "The weakness is a property of this generator's chosen rates, not " +
			"a general fact about market-order rates. A market with more frequent " +
			"marketable flow would identify it better.",
		Thresholds: []claims.Threshold{
			{ObsIndex: 0, GreaterThan: true, Ref: 0, RefLabel: "0 nats"},
			{ObsIndex: 1, GreaterThan: true, Ref: 0, RefLabel: "0 nats"},
			{ObsIndex: 0, GreaterThan: false, Ref: weakAdvantageCeiling,
				RefLabel: "25 nats"},
			{ObsIndex: 1, GreaterThan: false, Ref: weakAdvantageCeiling,
				RefLabel: "25 nats"},
		},
		Observations: []claims.Observation{
			{Label: "vs 0.6x", Value: advantages[2][0]},
			{Label: "vs 1.67x", Value: advantages[2][1]},
		},
		Binding: binding,
	})

	// ── The sampler: ESS ──────────────────────────────────────────────────────
	ess, gap, err := effectiveSampleSize()
	if err != nil {
		return nil, err
	}
	set = append(set, claims.Claim{
		ID: "importance_sampling_effective_sample_size_collapses",
		Statement: fmt.Sprintf(
			"cfg/lob_recovery.yaml's single fixed-width proposal is fully degenerate: the "+
				"effective sample size is ~1 of %d draws, because the windowed "+
				"cumulative log-likelihood puts the runner-up tens of nats behind the "+
				"best draw. The posterior mean is therefore the single best sample, and "+
				"the posterior variance is not an uncertainty.", essProposals),
		Gate:  "1.2",
		Phase: phase,
		Data:  dataset,
		Unit: fmt.Sprintf("ESS = (sum w)^2 / sum w^2 over %d draws from the config's "+
			"own proposal; runner-up gap in nats", essProposals),
		Limitations: "Specific to cfg/lob_recovery.yaml's sampler, and kept as the " +
			"recorded reason Gate 1.2 selected SMC instead — deleting it would erase " +
			"the evidence for that decision. It is not a statement about this repo's " +
			"inference as a whole: see " +
			"[[smc_effective_sample_size_recovers_but_stays_modest]], where the same " +
			"model under a contracting proposal reaches an ESS of 9-22. Note that SMC's " +
			"FIRST round is equally degenerate, which is what identifies proposal width " +
			"rather than the algorithm as the cause.",
		Thresholds: []claims.Threshold{
			{ObsIndex: 0, GreaterThan: false, Ref: essCeiling,
				RefLabel: fmt.Sprintf("%.0f of %d", essCeiling, essProposals)},
			{ObsIndex: 1, GreaterThan: true, Ref: 10, RefLabel: "10 nats"},
		},
		Observations: []claims.Observation{
			{Label: "ESS", Value: ess},
			{Label: "runner-up gap", Value: gap},
		},
		Binding: binding,
	})

	// ── Recovery across settings ──────────────────────────────────────────────
	strongWorst := 0.0
	weakBest := math.Inf(1)
	strongObservations := make([]claims.Observation, 0, len(settings))
	weakObservations := make([]claims.Observation, 0, len(settings))
	for _, s := range settings {
		got, err := recoverAt(s)
		if err != nil {
			return nil, err
		}
		errors := relativeErrors(got, s.truth)
		// The worst of the two strongly-identified coordinates, per setting.
		worst := math.Max(errors[0], errors[1])
		strongWorst = math.Max(strongWorst, worst)
		weakBest = math.Min(weakBest, errors[2])
		strongObservations = append(strongObservations,
			claims.Observation{Label: s.label, Value: worst})
		weakObservations = append(weakObservations,
			claims.Observation{Label: s.label, Value: errors[2]})
	}

	set = append(set, claims.Claim{
		ID: "strongly_identified_rates_recover_across_parameter_settings",
		Statement: fmt.Sprintf(
			"Under cfg/lob_recovery.yaml's importance sampler, the limit-arrival and "+
				"cancellation rates recover to within the pre-registered %.0f%% tolerance "+
				"at all three true-parameter settings, including one near a range boundary "+
				"approached from above. Each figure is the worse of the two rates for that "+
				"setting.", tolerance*100),
		Gate:  "1.2",
		Phase: phase,
		Data:  dataset,
		Unit:  "relative error of the final posterior mean, percent",
		Limitations: "Point estimates only. Because ESS is ~1 (see " +
			"[[importance_sampling_effective_sample_size_collapses]]) these are the " +
			"best sample from a hill-climb, not posterior means, so they come with no " +
			"usable uncertainty — which is why they did not settle Gate 1.2 and " +
			"[[smc_posterior_uncertainty_is_calibrated]] did. Recovery here is from " +
			"the model's OWN generated flow, " +
			"which is the easiest possible case: it bounds nothing about real data.",
		Thresholds: []claims.Threshold{
			{ObsIndex: 0, GreaterThan: false, Ref: tolerance * 100, RefLabel: "25%"},
			{ObsIndex: 1, GreaterThan: false, Ref: tolerance * 100, RefLabel: "25%"},
			{ObsIndex: 2, GreaterThan: false, Ref: tolerance * 100, RefLabel: "25%"},
		},
		Observations: strongObservations,
		Binding:      binding,
	})

	set = append(set, claims.Claim{
		ID: "importance_sampled_weak_rate_fails_to_recover_at_every_setting",
		Statement: fmt.Sprintf(
			"Under cfg/lob_recovery.yaml's importance sampler the market-order rate "+
				"misses the pre-registered %.0f%% tolerance at all three settings, and "+
				"misses it worst at the boundary. With ESS ~1 the winning sample is chosen "+
				"on the two dominant coordinates' merits, so a weakly-identified third "+
				"coordinate is not estimated — it inherits whatever value that sample "+
				"happened to carry. SMC recovers the same rate to within 7.6%%: see "+
				"[[smc_recovers_every_rate_at_every_setting]].", tolerance*100),
		Gate:  "1.2",
		Phase: phase,
		Data:  dataset,
		Unit:  "relative error of the final posterior mean, percent",
		Limitations: "A pinned known failure of ONE sampler, kept because it is the " +
			"measured justification for Gate 1.2 selecting SMC. It asserts a floor " +
			"rather than being left out so a future fix to this sampler breaks it " +
			"loudly; it does not describe the path the project now takes.",
		Thresholds: []claims.Threshold{
			{ObsIndex: 0, GreaterThan: true, Ref: tolerance * 100, RefLabel: "25%"},
			{ObsIndex: 1, GreaterThan: true, Ref: tolerance * 100, RefLabel: "25%"},
			{ObsIndex: 2, GreaterThan: true, Ref: tolerance * 100, RefLabel: "25%"},
		},
		Observations: weakObservations,
		Binding:      binding,
	})

	smc, err := smcBehaviour()
	if err != nil {
		return nil, err
	}
	return append(set, smc...), nil
}
