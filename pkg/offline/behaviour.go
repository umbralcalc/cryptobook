// Package offline holds the Phase 3 result: can a churn model's parameters be recovered
// OFFLINE — from a recorded segment read back through the engine's json_log source, the
// architecture Gate 3.4 selected — and in particular can the parameter that makes it a
// churn model be recovered at all?
//
// # Why this package exists
//
// Spike 2.2 established that the independent-Poisson model does not transfer to real order
// flow: cancellations do not scale with depth, arrivals and cancellations co-move at +0.98,
// and the counts are overdispersed by three orders of magnitude. The churn model
// (cfg/lob_counts.yaml) answered that by giving both streams a shared latent driver. Its
// identifying parameter is the driver's DISPERSION — not a mean, a second moment — and the
// question Phase 3 has to face before calibrating anything against real data is whether an
// offline calibration can recover a dispersion at all, and with which likelihood.
//
// # The experiment (pre-registered CA-CC)
//
// A minimal shared-driver churn generator (cfg/lob_churn_flow.yaml) emits three count
// streams whose dispersion is known by construction: one gamma driver, mean 4 and variance
// 105, scales arrivals and cancellations, so phi = Var(act)/E(act)^2 = 6.5625 through
// Var(n) = E(n) + phi*E(n)^2. A segment is recorded to disk and read back through json_log,
// then the SAME three-parameter SMC (limit_rate, churn_rate, phi) is run twice, changing
// only the likelihood.
//
//	              limit_rate   churn_rate   phi (truth 6.5625)   peak ESS
//	poisson          ~12%         ~12%          ~120%  (blind)      ~27
//	negative_binom   ~11%          ~7%           ~29%  (found)      ~78
//
// A Poisson likelihood forces Var = mean, so phi never enters it and its posterior lands
// near the prior mean: the parameter that makes the churn model a churn model is invisible
// to the family 2.2 used. negative_binomial takes (mean, variance), represents phi, and
// recovers it — biased ~30% low, because a 400-step window under-samples the heavy gamma
// tail that carries the dispersion.
//
// # What this is and is not
//
// Synthetic throughout: the truth is the generator's own parameters, so this measures
// IDENTIFIABILITY and the offline machinery, not agreement with any market. It says that IF
// real cancellations had this driver, an offline negative-binomial SMC could recover its
// dispersion from a recorded segment. Whether they do is a market question that needs
// recorded segments and lives in DECISIONS.md. The generator is also the clean gamma-Poisson
// form, without cfg/lob_counts.yaml's arrival damping — recovering phi through the full
// damped model, where the nonlinearity distorts it, is the honest follow-on.
package offline

import (
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

const (
	phase   = "3 — Offline calibration"
	dataset = "synthetic — a recorded shared-driver churn segment read back through the " +
		"engine's json_log source. Model-internal: measures identifiability, no market data"
)

// ObservedBehaviour pins CA-CC.
func ObservedBehaviour() []claims.Claim {
	m, err := measureAll()
	if err != nil {
		panic("offline: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestOfflineCalibration",
		TestFile: "pkg/offline/behaviour_test.go",
	}
	return []claims.Claim{
		{
			ID: "the_activity_scaled_rates_recover_offline_under_both_likelihoods",
			Statement: "Read back through the json_log source that Gate 3.4 selected as the " +
				"offline architecture, an SMC recovers both activity-scaled rates — the " +
				"arrival rate and the churn (cancellation) rate — to within 25% of truth, " +
				"whether the likelihood is Poisson or negative binomial. Both read the mean " +
				"and the rates set the mean, so the recorded-segment round trip costs the " +
				"rate estimates nothing. This is the control the dispersion result rests on: " +
				"the plumbing recovers what it should.",
			Gate:  "3.4",
			Phase: phase,
			Data:  dataset,
			Unit: "relative error of the posterior-mean rate, averaged over 6 recorded " +
				"segments, for each (likelihood, rate) pair",
			Limitations: "NOT PRE-REGISTERED as a package; the CA prediction was fixed in " +
				"PREREGISTRATION.md but the 25% bar is the same one Phase 1's SMC used and " +
				"was not re-derived for the offline path. Synthetic: the segment is the " +
				"generator's own flow, so this is recovery of a known truth, not agreement " +
				"with a market. The market rate is held fixed at truth as a nuisance " +
				"parameter and is not among the three estimated.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 0.25, RefLabel: "25% (poisson, arrivals)"},
				{ObsIndex: 1, GreaterThan: false, Ref: 0.25, RefLabel: "25% (poisson, churn)"},
				{ObsIndex: 2, GreaterThan: false, Ref: 0.25, RefLabel: "25% (neg-binom, arrivals)"},
				{ObsIndex: 3, GreaterThan: false, Ref: 0.25, RefLabel: "25% (neg-binom, churn)"},
			},
			Observations: []claims.Observation{
				{Label: "poisson limit_rate", Value: m.poisson.relErr[0]},
				{Label: "poisson churn_rate", Value: m.poisson.relErr[1]},
				{Label: "neg-binom limit_rate", Value: m.negbin.relErr[0]},
				{Label: "neg-binom churn_rate", Value: m.negbin.relErr[1]},
			},
			Binding: binding,
		},
		{
			ID: "the_driver_dispersion_is_recovered_offline_only_by_the_dispersion_aware_likelihood",
			Statement: "The driver's dispersion phi — the parameter that makes the churn " +
				"model a churn model, and the one the independent-Poisson model of Spike 2.2 " +
				"structurally lacked — is recovered offline by a negative-binomial likelihood " +
				"(within 40% of truth) and NOT by a Poisson one (posterior mean more than " +
				"100% off, landing near the prior mean). A Poisson likelihood forces variance " +
				"to equal the mean, so phi never enters it; negative binomial takes a separate " +
				"variance and so can see it. The contrast is the finding: offline calibration " +
				"of the churn mechanism is possible, but only with a likelihood the inference " +
				"tier does not use by default.",
			Gate:  "3.4",
			Phase: phase,
			Data:  dataset,
			Unit: "relative error of the posterior-mean dispersion phi, averaged over 6 " +
				"segments, under each likelihood",
			Limitations: "The PRE-REGISTERED test of Poisson non-identification FAILED as " +
				"literally stated and is reported not rewidened: PREREGISTRATION.md CB " +
				"predicted Poisson's phi posterior SD would stay within 10% of the prior's, " +
				"and it contracted to 17%. The adaptive proposal narrows an unidentified " +
				"coordinate's proposal even with no likelihood signal, so posterior SD is the " +
				"WRONG discriminator; the point estimate landing near the prior mean (120% " +
				"off) is the right one, and it is what is pinned here. Two further honest " +
				"limits: even negative binomial recovers phi biased ~30% LOW, because a " +
				"400-step window under-samples the heavy gamma tail (shape 0.15) that carries " +
				"the dispersion — so this shows phi is identifiable, not that it is unbiased " +
				"on short segments. And it is a designed synthetic recovery: it says nothing " +
				"about whether real cancellations carry this driver, only that if they did, " +
				"the offline path could find it.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 0.40,
					RefLabel: "40% (negative binomial recovers it)"},
				{ObsIndex: 1, GreaterThan: true, Ref: 1.0,
					RefLabel: "100% (poisson cannot — near the prior mean)"},
			},
			Observations: []claims.Observation{
				{Label: "neg-binom phi", Value: m.negbin.relErr[2]},
				{Label: "poisson phi", Value: m.poisson.relErr[2]},
			},
			Binding: binding,
		},
		{
			ID: "the_offline_path_mixes_as_well_as_the_in_memory_smc",
			Statement: "Peak effective sample size recovers across rounds for the offline " +
				"calibration just as it does for the in-memory SMC of cfg/lob_recovery_smc.yaml " +
				"(whose peak was ~26): the json_log round trip changes only where the data " +
				"comes from, not how the inference mixes. So whatever the dispersion result " +
				"shows is a property of the likelihood, not of the offline plumbing — the " +
				"control that lets CB be read as being about the likelihood at all. The " +
				"correctly-specified negative-binomial run mixes markedly better than the " +
				"misspecified Poisson one, which is itself the expected direction.",
			Gate:  "3.4",
			Phase: phase,
			Data:  dataset,
			Unit:  "peak effective sample size out of 160 particles, averaged over 6 segments",
			Limitations: "NOT PRE-REGISTERED numerically; CC was fixed as a qualitative " +
				"control (ESS recovers as the in-memory SMC does) and the >10 floor is set " +
				"well below both observed values rather than derived. Peak ESS across rounds " +
				"is a coarse health check — it says the sampler did not degenerate in its " +
				"best round, not that every round was healthy or that the posterior is " +
				"well-explored.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: 10.0, RefLabel: "10 (poisson mixes)"},
				{ObsIndex: 1, GreaterThan: true, Ref: 10.0, RefLabel: "10 (neg-binom mixes)"},
			},
			Observations: []claims.Observation{
				{Label: "poisson peak ESS", Value: m.poisson.peakESS},
				{Label: "neg-binom peak ESS", Value: m.negbin.peakESS},
			},
			Binding: binding,
		},
	}
}
