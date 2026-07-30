package recovery

// The SMC half of Spike 1.2: the answer to the escalation.
//
// cfg/lob_recovery.yaml identified all three parameters and then failed to
// estimate one of them, because its importance weights were a hard argmax (ESS
// 1.00 of 16). cfg/lob_recovery_smc.yaml runs the SAME model against the SAME
// observables scored by the SAME intensity model, changing only the inference
// layer, so the comparison isolates the sampler.
//
// What changes, mechanically: uniform priors bound the search to positive rates
// (removing the negative-proposal failure mode structurally rather than clamping
// it), and the proposal CONTRACTS across rounds — round 1 draws from the priors,
// each later round from a Gaussian fitted to the previous round's weighted
// posterior. Degeneracy is a property of proposal width relative to the
// likelihood's peak, so a narrowing proposal addresses the cause; a fixed-width
// proposal centred on a drifting mean never stops being too wide.
//
// It works, and the honest version has three parts:
//
//   - Recovery: all three rates within the pre-registered tolerance at all three
//     settings. Worst error 7.6%, against 113% before.
//   - Uncertainty: the truth lies within 1.8 posterior sd everywhere. This is the
//     part Phase 2 actually needs and the part the old sampler could not supply
//     at all.
//   - ESS: rises from 1.00 to 9–22 in the two well-behaved settings, but only to
//     ~4 of 160 near the boundary. Better, not solved.
//
// And one finding that matters more than it looks: raising the round count makes
// the near-boundary posterior OVERCONFIDENT. See
// raisingRoundsClaim below — the guard exists because "more rounds" is the
// obvious thing a future reader will try.

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

const (
	smcConfig = "lob_recovery_smc.yaml"

	// smcRounds is the config's round count, and smcRaisedRounds is the value the
	// calibration guard compares against. Both are pinned to the config by
	// TestSmcConstantsMatchTheConfig.
	smcRounds       = 5
	smcRaisedRounds = 9
	smcParticles    = 160

	// calibrationCeiling is how many posterior standard deviations the truth may sit
	// from the posterior mean before the uncertainty stops being usable. Two is the
	// conventional reading of a ~95% interval, and it is the property Phase 2 needs:
	// a posterior whose interval does not cover the truth is worse than no interval,
	// because it invites confident wrong conclusions.
	calibrationCeiling = 2.0
)

// smcResult is one SMC run's output.
type smcResult struct {
	mean [3]float64
	sd   [3]float64
	// essPerRound is the importance-sampling ESS of each round's particle weights,
	// computed from the run's OWN particle log-likelihoods rather than from proposals
	// drawn separately in Go — the evaluation partition's state row is exactly one
	// log-likelihood per particle, so this is the sampler's real ESS, not a proxy.
	essPerRound []float64
}

// worstRelativeError returns the largest relative error, as a percentage.
func (r smcResult) worstRelativeError(truth [3]float64) float64 {
	worst := 0.0
	for i := range r.mean {
		worst = math.Max(worst, math.Abs(r.mean[i]-truth[i])/truth[i]*100)
	}
	return worst
}

// worstCalibrationSigma returns how many posterior standard deviations the truth
// sits from the posterior mean, for the worst-covered parameter.
func (r smcResult) worstCalibrationSigma(truth [3]float64) float64 {
	worst := 0.0
	for i := range r.mean {
		if r.sd[i] <= 0 {
			return math.Inf(1) // a zero-width posterior covers nothing
		}
		worst = math.Max(worst, math.Abs(r.mean[i]-truth[i])/r.sd[i])
	}
	return worst
}

// peakESS returns the best ESS any round achieved.
func (r smcResult) peakESS() float64 {
	peak := 0.0
	for _, ess := range r.essPerRound {
		peak = math.Max(peak, ess)
	}
	return peak
}

// smcSubs sets the truth (and the matching stationary ladder) for one setting, plus
// an optional round-count override. Unlike the posterior_estimation config there is
// no prior to move: the same wide priors cover all three settings, which makes the
// comparison across them cleaner.
func smcSubs(s setting, rounds int) cfgrun.Subs {
	depth := s.levelDepth
	subs := cfgrun.Subs{
		"limit_rate: [1.2]":   fmt.Sprintf("limit_rate: [%g]", s.truth[0]),
		"cancel_rate: [0.15]": fmt.Sprintf("cancel_rate: [%g]", s.truth[1]),
		"market_rate: [0.8]":  fmt.Sprintf("market_rate: [%g]", s.truth[2]),
		"[8.0, 8.0, 8.0, 8.0, 8.0, 8.0, 0.0, 0.0, 0.0, 48.0]": fmt.Sprintf(
			"[%g, %g, %g, %g, %g, %g, 0.0, 0.0, 0.0, %g]",
			depth, depth, depth, depth, depth, depth, 6*depth),
	}
	if rounds != smcRounds {
		subs[fmt.Sprintf("num_rounds: %d", smcRounds)] =
			fmt.Sprintf("num_rounds: %d", rounds)
	}
	return subs
}

// runSMC runs the SMC config for one setting and reads back the posterior and the
// per-round ESS.
func runSMC(s setting, rounds int) (smcResult, error) {
	storage, err := cfgrun.Run(smcConfig, smcSubs(s, rounds))
	if err != nil {
		return smcResult{}, err
	}
	posterior, err := cfgrun.LastRow(storage, "smc_posterior")
	if err != nil {
		return smcResult{}, err
	}
	// SMCPosteriorIteration's layout is [mean(d) | covariance(d*d) | logMarginalLik].
	const d = 3
	if want := d + d*d + 1; len(posterior) != want {
		return smcResult{}, fmt.Errorf(
			"recovery: smc posterior has width %d, expected %d", len(posterior), want)
	}
	result := smcResult{}
	for i := range d {
		result.mean[i] = posterior[i]
		variance := posterior[d+i*d+i] // the covariance diagonal
		if variance < 0 {
			return smcResult{}, fmt.Errorf(
				"recovery: smc posterior variance %d is negative (%g)", i, variance)
		}
		result.sd[i] = math.Sqrt(variance)
	}
	for _, row := range storage.GetValues("smc_particles") {
		if ess, ok := effectiveSampleSizeOf(row); ok {
			result.essPerRound = append(result.essPerRound, ess)
		}
	}
	if len(result.essPerRound) == 0 {
		return smcResult{}, fmt.Errorf("recovery: no scored SMC rounds found")
	}
	return result, nil
}

// effectiveSampleSizeOf computes (sum w)^2 / sum w^2 for one round's particle
// log-likelihoods. Reports false for the pre-first-round row, whose loglikes are
// all still the partition's zero init values and would give a meaningless ESS of N.
func effectiveSampleSizeOf(loglikes []float64) (float64, bool) {
	scored := make([]float64, 0, len(loglikes))
	allZero := true
	for _, loglike := range loglikes {
		if math.IsNaN(loglike) || math.IsInf(loglike, -1) {
			continue // a particle whose model produced nothing carries no weight
		}
		if loglike != 0 {
			allZero = false
		}
		scored = append(scored, loglike)
	}
	if len(scored) == 0 || allZero {
		return 0, false
	}
	// Normalised by the max before exponentiating: these loglikes are around -2300,
	// so exp() of them underflows to zero and the ESS would come out as 0/0.
	best := scored[0]
	for _, loglike := range scored {
		best = math.Max(best, loglike)
	}
	sum, sumSquares := 0.0, 0.0
	for _, loglike := range scored {
		weight := math.Exp(loglike - best)
		sum += weight
		sumSquares += weight * weight
	}
	return sum * sum / sumSquares, true
}

// smcBehaviour measures the SMC claims.
func smcBehaviour() ([]claims.Claim, error) {
	binding := claims.Binding{
		TestName: "TestSmcRecoveryExpectedBehaviour",
		TestFile: "pkg/recovery/smc_test.go",
	}

	results := make([]smcResult, len(settings))
	errorObs := make([]claims.Observation, 0, len(settings))
	sigmaObs := make([]claims.Observation, 0, len(settings))
	for i, s := range settings {
		result, err := runSMC(s, smcRounds)
		if err != nil {
			return nil, err
		}
		results[i] = result
		errorObs = append(errorObs, claims.Observation{
			Label: s.label, Value: result.worstRelativeError(s.truth)})
		sigmaObs = append(sigmaObs, claims.Observation{
			Label: s.label, Value: result.worstCalibrationSigma(s.truth)})
	}

	// The near-boundary setting is the weak one for ESS, and the one whose
	// calibration degrades when the round count is raised.
	const boundary = 2
	raised, err := runSMC(settings[boundary], smcRaisedRounds)
	if err != nil {
		return nil, err
	}

	thresholdsBelow := func(ref float64, label string, n int) []claims.Threshold {
		bounds := make([]claims.Threshold, n)
		for i := range bounds {
			bounds[i] = claims.Threshold{
				ObsIndex: i, GreaterThan: false, Ref: ref, RefLabel: label}
		}
		return bounds
	}

	return []claims.Claim{
		{
			ID: "smc_recovers_every_rate_at_every_setting",
			Statement: fmt.Sprintf(
				"Sequential Monte Carlo recovers all three rates — including the weakly "+
					"identified market-order rate — to within the pre-registered %.0f%% "+
					"tolerance at every true-parameter setting, from wide uniform priors "+
					"that are not centred on the truth. Each figure is the worst of the "+
					"three rates for that setting.", tolerance*100),
			Gate:  "1.2",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("worst relative error of the posterior mean over the three "+
				"rates, percent, %d particles x %d rounds", smcParticles, smcRounds),
			Limitations: "Recovery from the model's OWN generated flow, which is the " +
				"easiest possible case — it bounds nothing about real message data, where " +
				"the model is misspecified rather than exact. The comparison against " +
				"[[weakly_identified_rate_fails_to_recover_at_every_setting]] is like-for-" +
				"like in the model and the observables, but the two samplers are given " +
				"different starting information: wide priors here, an off-truth point " +
				"prior there. That difference is part of what SMC buys, not a confound to " +
				"correct for.",
			Thresholds:   thresholdsBelow(tolerance*100, "25%", len(errorObs)),
			Observations: errorObs,
			Binding:      binding,
		},
		{
			ID: "smc_posterior_uncertainty_is_calibrated",
			Statement: fmt.Sprintf(
				"The SMC posterior's uncertainty is usable, not decorative: the true "+
					"value lies within %.0f posterior standard deviations of the posterior "+
					"mean for every rate at every setting. This is the property Phase 2's "+
					"deliverable depends on, and the one the importance-sampled posterior "+
					"could not supply at all — with ESS ~1 its variance was the spread of "+
					"a single point.", calibrationCeiling),
			Gate:  "1.2",
			Phase: phase,
			Data:  dataset,
			Unit: "distance from posterior mean to truth in posterior standard " +
				"deviations, worst of the three rates",
			Limitations: "Coverage checked at three settings on synthetic data, which is " +
				"a sanity check and not a calibration study — three points cannot " +
				"establish an interval's frequentist coverage. More importantly it holds " +
				"AT THE WINDOW LENGTH AND ROUND COUNT MEASURED, not in general, and it " +
				"degrades in the dangerous direction on both axes: see " +
				"[[raising_smc_rounds_trades_calibration_for_point_accuracy]] and " +
				"[[posterior_overconfidence_grows_as_the_calibration_window_lengthens]]. " +
				"At 1600 rows instead of 400 the truth sits over 7 posterior sd from the " +
				"mean. Do not read this claim as \"the SMC posterior is calibrated\".",
			Thresholds: thresholdsBelow(
				calibrationCeiling,
				fmt.Sprintf("%.0f sd", calibrationCeiling), len(sigmaObs)),
			Observations: sigmaObs,
			Binding:      binding,
		},
		{
			ID: "smc_effective_sample_size_recovers_but_stays_modest",
			Statement: fmt.Sprintf(
				"SMC's first round is just as degenerate as single-proposal importance "+
					"sampling — its particles are spread across the whole prior — and the "+
					"ESS then recovers as the proposal contracts onto the likelihood's "+
					"peak. That confirms proposal width, not the algorithm's identity, was "+
					"the cause. It is an improvement rather than a solution: the peak is "+
					"still a small fraction of the %d particles, and worst near a "+
					"parameter-range boundary.", smcParticles),
			Gate:  "1.2",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("effective sample size of one round's particle weights, "+
				"out of %d particles", smcParticles),
			Limitations: "An ESS of ~13 of 160 is around 8%, which is low by any " +
				"conventional standard — the posterior means and intervals above are " +
				"trustworthy on this evidence, but the sampler has little margin. Raising " +
				"the particle count is the untried lever; raising rounds is not (see " +
				"[[raising_smc_rounds_trades_calibration_for_point_accuracy]]).",
			Monotone: 1,
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 2.0, RefLabel: "2 particles"},
				{ObsIndex: 1, GreaterThan: true, Ref: 2.0, RefLabel: "2 particles"},
				{ObsIndex: 2, GreaterThan: true, Ref: 5.0, RefLabel: "5 particles"},
				{ObsIndex: 2, GreaterThan: false, Ref: 40.0, RefLabel: "40 particles"},
			},
			Observations: []claims.Observation{
				{Label: "round 1, nominal", Value: results[0].essPerRound[0]},
				{Label: "best round, near boundary", Value: results[boundary].peakESS()},
				{Label: "best round, nominal", Value: results[0].peakESS()},
			},
			Binding: binding,
		},
		{
			ID: "raising_smc_rounds_trades_calibration_for_point_accuracy",
			Statement: fmt.Sprintf(
				"More rounds is not monotonically better, and it fails in the dangerous "+
					"direction. Going from %d to %d rounds in the near-boundary setting — "+
					"where ESS plateaus around 3 — keeps contracting the proposal faster "+
					"than the estimate converges, so the posterior gets tighter and its "+
					"interval stops covering the truth. The truth moves from inside %.0f "+
					"posterior sd to outside it.", smcRounds, smcRaisedRounds,
				calibrationCeiling),
			Gate:  "1.2",
			Phase: phase,
			Data:  dataset,
			Unit: "distance from posterior mean to truth in posterior standard " +
				"deviations, worst of the three rates, near-boundary setting",
			Limitations: "A pinned tripwire, not a result to build on. It says nothing " +
				"about where the optimum round count is, only that it is not 'as many as " +
				"possible' — and it is measured at one setting, the one where ESS is " +
				"weakest. If a change makes the round count safe to raise, this claim's " +
				"assertion breaks and it must be retired, which is the intent.",
			Monotone: 1,
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: calibrationCeiling,
					RefLabel: fmt.Sprintf("%.0f sd", calibrationCeiling)},
				{ObsIndex: 1, GreaterThan: true, Ref: calibrationCeiling,
					RefLabel: fmt.Sprintf("%.0f sd", calibrationCeiling)},
			},
			Observations: []claims.Observation{
				{Label: fmt.Sprintf("%d rounds", smcRounds),
					Value: results[boundary].worstCalibrationSigma(settings[boundary].truth)},
				{Label: fmt.Sprintf("%d rounds", smcRaisedRounds),
					Value: raised.worstCalibrationSigma(settings[boundary].truth)},
			},
			Binding: binding,
		},
	}, nil
}
