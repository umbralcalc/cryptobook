// This file adds CI: does offline dispersion recovery survive the arrival DAMPING — the
// second and last confound the full cfg/lob_counts.yaml adds over the clean case? It
// measures the two flow streams' empirical dispersions directly from the shipped model.
// See PREREGISTRATION.md CI, fixed before this ran.
package offline

import (
	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
	"sync"
)

const (
	// The persistent driver's marginal dispersion, the value a clean stream would show if a
	// finite window sampled it perfectly.
	driverPhiMarginal = 0.729
	// lob_counts summed-count observable columns.
	idxCountLimit  = 16
	idxCountCancel = 17
	dampSettle     = 200
	dampSteps      = 8000
)

// dispersion is phi = (Var - mean) / mean^2, the negative-binomial dispersion the offline
// calibration estimates.
func dispersion(x []float64) float64 {
	m := diagnostics.Mean(x)
	v := 0.0
	for _, q := range x {
		v += (q - m) * (q - m)
	}
	v /= float64(len(x) - 1)
	return (v - m) / (m * m)
}

type dampResult struct {
	arrivalPhi, cancelPhi float64
}

func measureDampingUncached() (dampResult, error) {
	stores, err := cfgrun.RunEnsemble("lob_counts.yaml", cfgrun.Subs{
		"max_steps: 400": "max_steps: 8000",
	}, cfgrun.DefaultSeeds)
	if err != nil {
		return dampResult{}, err
	}
	var arr, can []float64
	for _, storage := range stores {
		rows := storage.GetValues("lob_counts")[dampSettle:]
		seg := diagnostics.Segment{Rows: rows}
		arr = append(arr, dispersion(seg.Column(idxCountLimit)))
		can = append(can, dispersion(seg.Column(idxCountCancel)))
	}
	return dampResult{arrivalPhi: mean(arr), cancelPhi: mean(can)}, nil
}

var (
	dampOnce   sync.Once
	dampCached dampResult
	dampErr    error
)

func measureDamping() (dampResult, error) {
	dampOnce.Do(func() { dampCached, dampErr = measureDampingUncached() })
	return dampCached, dampErr
}

// dampingClaims pins CI: the two confounds stack, and the dispersion the offline
// calibration would see is ordered arrival < cancellation < driver.
func dampingClaims() []claims.Claim {
	r, err := measureDamping()
	if err != nil {
		panic("offline: measuring damping: " + err.Error())
	}
	binding := claims.Binding{TestName: "TestOfflineBehaviour", TestFile: "pkg/offline/behaviour_test.go"}
	return []claims.Claim{
		{
			ID: "the_two_confounds_stack_arrival_below_cancellation_below_driver",
			Statement: "In the full cfg/lob_counts.yaml the empirical dispersion an offline " +
				"calibration would see is ordered: arrival phi < cancellation phi < the " +
				"driver's marginal 0.729. Two distinct effects stack. PERSISTENCE (CH) pulls " +
				"BOTH streams below the driver's marginal on a finite window, because an AR(1) " +
				"driver under-explores its own dispersion — so even the clean churn " +
				"cancellation stream, linear in the driver, sits below 0.729. The arrival " +
				"DAMPING (CI) then pulls arrivals below cancellations, because the driver in " +
				"the damping denominator makes the arrival response sublinear and so transmits " +
				"less of the driver's variance. A per-step negative-binomial calibration " +
				"assumes ONE dispersion shared across streams; it cannot fit two that differ " +
				"by construction, and neither equals the truth.",
			Gate:  "3.4",
			Phase: phase,
			Data:  dataset,
			Unit:  "negative-binomial dispersion phi = (Var-mean)/mean^2 of each summed flow stream, 32-member ensemble",
			Limitations: "NOT PRE-REGISTERED as bounds — the pre-registered CI-1/CI-2 predictions " +
				"(cancellation within 25% of 0.729; arrival below 0.8x cancellation) both passed " +
				"but marginally (23%, ratio 0.780), and are scored in PREREGISTRATION.md; this " +
				"claim pins the ROBUST ordering with margin, as a regression guard. It is " +
				"model-internal and synthetic: it characterises what an offline calibration of " +
				"THIS model would face, not what real order flow shows, and it does not itself " +
				"run a calibration. The conclusion it supports — that the full model needs a " +
				"state-space likelihood, not per-step negative binomial — is a modelling " +
				"boundary named, not a result about markets.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 0.52, RefLabel: "0.52 (arrival, damped, below cancellation)"},
				{ObsIndex: 1, GreaterThan: true, Ref: 0.50, RefLabel: "0.50 (cancellation, above arrival)"},
				{ObsIndex: 1, GreaterThan: false, Ref: driverPhiMarginal, RefLabel: "0.729 (cancellation below the driver's marginal)"},
			},
			Observations: []claims.Observation{
				{Label: "arrival phi (damped)", Value: r.arrivalPhi},
				{Label: "cancellation phi (clean churn)", Value: r.cancelPhi},
			},
			Binding: binding,
		},
	}
}
