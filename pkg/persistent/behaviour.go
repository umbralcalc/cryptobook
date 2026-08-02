// Package persistent scores the persistent-driver model — an AR(1) activity process
// with a damping scale that shrinks as activity rises.
//
// # Why persistence was necessary before anything else could be seen
//
// Depth is a slow accumulator and every earlier model drew its driver IID PER STEP.
// depth_start(t) then depends on activity only up to t-1 while cancellation depends on
// activity at t, so the two are independent by construction and corr(depth, cancels)
// sits near zero WHATEVER mechanism is present. Every near-zero cancellation-side
// reading in this project was therefore partly structural — which is why prediction H's
// uncertainty claim was withdrawn. Persistence makes the depth response observable.
//
// # The homogeneity trap
//
// One driver scaling every flow cannot work: the system is then homogeneous in act, and
// act cancels out of the equilibrium entirely. Activity would rescale time, not depth.
// The damping scale is what breaks it here — s_eff = arrival_scale * act_ref / act, so
// q* is proportional to 1/act.
//
// # The result: the closest any model here has come, and it still misses
//
// X, Z and AB pass; Y and AA fail. This is the FIRST model in the project to put both
// flows negative against depth with arrivals the stronger — the ordering every Binance
// segment shows and no previous model produced. The failures are of magnitude, not of
// direction:
//
//	                        this model    Binance range        pre-registered band
//	corr(depth, cancels)      -0.286     -0.015 .. -0.246       [-0.30, -0.01]  in
//	corr(depth, arrivals)     -0.417     -0.121 .. -0.339       [-0.40, -0.05]  OUT
//	corr(arrivals, cancels)   +0.822     +0.940 .. +0.980       > +0.85         OUT
//
// Both failures plausibly share one cause, and it is a single parameter rather than the
// mechanism: the damping's activity dependence is too strong at its pre-registered
// full strength. It overshoots the depth correlations, and it costs co-movement because
// arrivals now SATURATE in activity — arr is proportional to act/(1 + q*act/(s*ref)),
// whose denominator grows with act — while cancellation stays proportional to it, so the
// two flows track each other less closely than when both were proportional.
//
// That is a continuation, not a rescue, and it needs its own pre-registration: sweeping
// the strength after seeing which way these missed is exactly the move PREREGISTRATION.md
// exists to prevent.
package persistent

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	configName = "lob_persistent.yaml"
	partition  = "lob_persistent"

	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — persistent-driver generator at an arbitrary parameterisation. " +
		"Model-internal only: no real-market comparison is made here"

	// The pre-registered bounds, fixed in 5c1081e before the config existed.
	activityCeiling  = -0.05
	cancelBandFloor  = -0.30
	cancelBandCeil   = -0.01
	arrivalBandFloor = -0.40
	arrivalBandCeil  = -0.05
	coMovementFloor  = 0.85
	driftCeiling     = 1.3
	spreadSDFloor    = 0.1

	// clipCeiling is the VALIDITY PRECONDITION, not a prediction: above this, a binding
	// clip ties cancellation to depth mechanically and Y and Z would be scoring the clip
	// rather than the mechanism. Pre-registered at 5% of level-steps because that fault
	// is exactly what made the churn block's prediction B inconclusive.
	clipCeiling = 5.0

	settleFrom  = 100
	emptySpread = 99.0
	levels      = 16

	// 0-20 are identical to cfg/lob_arrivals.yaml by construction; 21 and 22 are
	// appended, so the models stay directly comparable.
	idxLimit    = 16
	idxCancel   = 17
	idxDepth    = 19
	idxSpread   = 20
	idxActivity = 21
	idxClip     = 22
)

// measured is this model's ensemble summary. Every field is a mean over members with the
// across-member spread attached — see cfgrun.DefaultSeeds for why 32 and why 8000 steps.
type measured struct {
	depthActivity, coupling, depthArrival, coMovement cfgrun.EnsembleStat
	drift, spreadSD, meanDepth, clipRate              cfgrun.EnsembleStat
}

func measure() (measured, error) {
	stores, err := cfgrun.RunEnsemble(configName, cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", cfgrun.DefaultSteps),
	}, cfgrun.DefaultSeeds)
	if err != nil {
		return measured{}, err
	}
	var act, can, arr, com, drift, sprSD, depth, clip []float64
	for _, storage := range stores {
		rows := storage.GetValues(partition)
		if len(rows) <= settleFrom {
			return measured{}, fmt.Errorf("persistent: a member produced too few rows")
		}
		rows = rows[settleFrom:]
		segment := diagnostics.Segment{Rows: rows}
		arrival := segment.Column(idxLimit)
		cancel := segment.Column(idxCancel)
		d := segment.Column(idxDepth)
		activity := segment.Column(idxActivity)
		half := len(d) / 2

		act = append(act, diagnostics.Correlation(d, activity))
		can = append(can, diagnostics.Correlation(d, cancel))
		arr = append(arr, diagnostics.Correlation(d, arrival))
		com = append(com, diagnostics.Correlation(arrival, cancel))
		drift = append(drift, diagnostics.Mean(d[half:])/diagnostics.Mean(d[:half]))
		depth = append(depth, diagnostics.Mean(d))

		binds := 0.0
		observed := make([]float64, 0, len(rows))
		for _, row := range rows {
			binds += row[idxClip]
			if row[idxSpread] < emptySpread {
				observed = append(observed, row[idxSpread])
			}
		}
		if len(observed) == 0 {
			return measured{}, fmt.Errorf("persistent: a member was one-sided at every step")
		}
		clip = append(clip, 100*binds/float64(len(rows)*levels))
		mean := diagnostics.Mean(observed)
		variance := 0.0
		for _, x := range observed {
			variance += (x - mean) * (x - mean)
		}
		sprSD = append(sprSD, math.Sqrt(variance/float64(len(observed))))
	}
	return measured{
		depthActivity: cfgrun.Summarise(act),
		coupling:      cfgrun.Summarise(can),
		depthArrival:  cfgrun.Summarise(arr),
		coMovement:    cfgrun.Summarise(com),
		drift:         cfgrun.Summarise(drift),
		spreadSD:      cfgrun.Summarise(sprSD),
		meanDepth:     cfgrun.Summarise(depth),
		clipRate:      cfgrun.Summarise(clip),
	}, nil
}

// ObservedBehaviour scores the pre-registered predictions X, Y, Z, AA and AB.
//
// RE-SCORED ON ENSEMBLES 2026-08-02: X, Y, Z and AB pass; only AA fails. Y previously
// FAILED, and that change is explained in its limitations rather than quietly absorbed.
func ObservedBehaviour() []claims.Claim {
	m, err := measure()
	if err != nil {
		panic("persistent: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestPersistentDriver",
		TestFile: "pkg/persistent/behaviour_test.go",
	}
	validity := claims.Observation{Label: "clip-binding rate, percent", Value: m.clipRate.Mean}

	return []claims.Claim{
		{
			ID: "prediction_x_a_persistent_driver_makes_depth_respond_to_activity",
			Statement: "Prediction X, PASSED, and it is the one that unlocked the rest. " +
				"With an iid driver this correlation is structurally near zero — depth at " +
				"the start of a step cannot depend on activity drawn during it — so no " +
				"earlier model could have shown a depth response at all. Made AR(1) at " +
				"0.8, with the driver's stationary mean and variance held at the " +
				"incumbent's so only autocorrelation changes, depth reads -0.284 against " +
				"activity. The competing effect declared in advance, constant marketable " +
				"consumption dragging depth down in quiet stretches, did not dominate.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between resting depth and the latent activity driver",
			Limitations: "The SIGN is largely built in — the damping scale shrinks as " +
				"activity rises, so q* is proportional to 1/act by construction. What was " +
				"genuinely uncertain was whether it would survive the opposing " +
				"constant-consumption effect and whether persistence would make it " +
				"visible at all, and X answers those. Model-internal: no market number " +
				"appears in this package.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: activityCeiling,
					RefLabel: "-0.05 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs activity", Value: m.depthActivity.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_y_both_depth_correlations_land_in_the_observed_band",
			Statement: "Prediction Y, PASSED on ensemble means — and it FAILED when this " +
				"package measured one seed. Both flows are mildly negative against depth " +
				"and both are inside their pre-registered bands: cancellations -0.257 in " +
				"[-0.30, -0.01], arrivals -0.387 in [-0.40, -0.05]. Y required both at " +
				"once, which is the joint landing no earlier model in this project " +
				"achieved.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlation between resting depth and each flow; and the " +
				"clip-binding rate, which is the validity precondition rather than a result",
			Limitations: "VERDICT CHANGED 2026-08-02, FAIL TO PASS, and the reason is " +
				"recorded rather than absorbed. On one seed at 2000 steps the arrival " +
				"side read -0.417 and fell outside its band, so Y was scored a failure. " +
				"The seed audit then measured this quantity's across-seed spread at 0.116 " +
				"and recorded, BEFORE any re-scoring, that most seeds do not overshoot " +
				"and Y would have passed on them. The 32-member mean at 8000 steps is " +
				"-0.387, inside the band by 0.013 against a standard error of 0.004. The " +
				"pre-registered bands did not move; the measurement got finer. " +
				"That margin is thin — three standard errors — so this is a pass, not a " +
				"comfortable one. The bands themselves come from Binance segments whose " +
				"flows are both INFERRED from net depth changes, so the target carries " +
				"the standing confound. The clip bound on 4.2% of level-steps, clearing " +
				"the pre-registered 5%, but not by much.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: cancelBandCeil,
					RefLabel: "-0.01 (band ceiling)"},
				{ObsIndex: 0, GreaterThan: true, Ref: cancelBandFloor,
					RefLabel: "-0.30 (band floor)"},
				{ObsIndex: 1, GreaterThan: true, Ref: arrivalBandFloor,
					RefLabel: "-0.40 (band floor the single seed fell below)"},
				{ObsIndex: 1, GreaterThan: false, Ref: arrivalBandCeil,
					RefLabel: "-0.05 (band ceiling)"},
				{ObsIndex: 2, GreaterThan: false, Ref: clipCeiling,
					RefLabel: "5% (validity precondition)"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs cancellations", Value: m.coupling.Mean},
				{Label: "depth vs arrivals", Value: m.depthArrival.Mean},
				validity,
			},
			Binding: binding,
		},
		{
			ID: "prediction_z_the_brake_ordering_matches_the_market_for_the_first_time",
			Statement: "Prediction Z, PASSED. Arrivals are the stronger brake at -0.387 " +
				"against cancellations' -0.257, which is the ordering all six Binance " +
				"segments show and which no previous model here produced — the recycled " +
				"model inverted it, and every earlier one put the whole brake on one flow " +
				"with the other at zero or the wrong sign. Arrivals carry an extra " +
				"negative path, being damped by depth directly while cancellation reaches " +
				"depth only through the driver.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "the margin by which the arrival correlation is the more negative; and " +
				"the clip-binding rate",
			Limitations: "Declared the WEAKEST of this block's predictions in advance, " +
				"because the extra negative path makes the ordering close to expected. " +
				"The margin is 0.130 against a per-quantity standard error of ~0.004, so " +
				"unlike the single-seed scoring it is now comfortably resolved. Same " +
				"validity caveat as " +
				"[[prediction_y_both_depth_correlations_land_in_the_observed_band]].",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: 0,
					RefLabel: "0 (arrivals strictly the stronger)"},
				{ObsIndex: 1, GreaterThan: false, Ref: clipCeiling,
					RefLabel: "5% (validity precondition)"},
			},
			Observations: []claims.Observation{
				{Label: "margin", Value: math.Abs(m.depthArrival.Mean) - math.Abs(m.coupling.Mean)},
				validity,
			},
			Binding: binding,
		},
		{
			ID: "prediction_aa_activity_dependent_damping_costs_co_movement",
			Statement: "Prediction AA, FAILED, and it is now the ONLY failure in this " +
				"block. Co-movement is +0.816 against a pre-registered floor of +0.85 and " +
				"the previous model's +0.897. The floor was set below the incumbent " +
				"deliberately, to test that the mechanism did not break the co-movement " +
				"rather than that it beat it, and it was not cleared. The cause is that " +
				"arrivals now SATURATE in activity — arr is proportional to " +
				"act / (1 + q*act/(s*ref)), whose denominator grows with act — while " +
				"cancellation stays proportional to it, so the two flows track each other " +
				"less closely than when both were proportional.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between per-step arrival and cancellation counts",
			Limitations: "Pinned as a CEILING so a future fix breaks it loudly. On " +
				"ensemble means this failure is ROBUST — the mean sits about six standard " +
				"errors below the floor — where the single-seed scoring left it 0.015 " +
				"away and in doubt. Measured at one damping strength; it establishes that " +
				"full strength costs enough to miss the floor, not where the boundary " +
				"lies. The saturation account is a mechanism explanation consistent with " +
				"the number, not an independently tested claim.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: coMovementFloor,
					RefLabel: "+0.85 (the pre-registered floor it had to clear)"},
			},
			Observations: []claims.Observation{
				{Label: "arrivals vs cancellations", Value: m.coMovement.Mean},
			},
			Binding: binding,
		},
		{
			ID: "prediction_ab_the_book_survives_a_moving_equilibrium",
			Statement: "Prediction AB, PASSED. The equilibrium depth is proportional to " +
				"1/activity, so the book chases a target that moves with a persistent " +
				"driver rather than relaxing to a fixed one — conservation is less " +
				"obvious than in any previous variant. It holds: drift 0.996 and a live " +
				"spread distribution at 0.528 ticks of standard deviation.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "mean resting depth over the second half of the scored window divided " +
				"by the first half (a conserved book gives ~1); spread sd in ticks over " +
				"two-sided steps; and mean depth in lots",
			Limitations: "Mean depth is NOT a result — churn_rate was re-set to 1.05 on " +
				"exactly this quantity, the one adjustment the pre-registration permits, " +
				"so it is provenance. Ensembling also moved this: the single seed gave a " +
				"drift of 1.164, the highest of any surviving model, and the 32-member " +
				"mean is 0.996. The single seed was an unlucky draw rather than a sign " +
				"the moving equilibrium strains conservation, which is what its earlier " +
				"limitations suggested.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: driftCeiling,
					RefLabel: "1.3 (pre-registered)"},
				{ObsIndex: 1, GreaterThan: true, Ref: spreadSDFloor,
					RefLabel: "0.1 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "second half / first half", Value: m.drift.Mean},
				{Label: "spread sd", Value: m.spreadSD.Mean},
				{Label: "mean depth", Value: m.meanDepth.Mean},
			},
			Binding: binding,
		},
	}
}
