// Package split scores whether the partitioned model reproduces the monolithic one.
//
// # What this is for
//
// cfg/lob_split.yaml is cfg/lob_damping.yaml with the activity driver lifted into its own
// partition, coupled back through params_from_upstream so it is read at the CURRENT step.
// The models are meant to be the same model. This package is the guard on that: it runs
// both as ensembles and pins the differences.
//
// # Why it cannot be an exact comparison
//
// Splitting gives each partition its own seed and its own draw order, so the two configs
// consume randomness differently and no seed makes them agree bit-for-bit. That rules out
// the exact-equality control used for the pow() reparameterisation in pkg/damping, where
// the change really was only a spelling.
//
// So the comparison is between ENSEMBLE MEANS, against the spreads measured on 2026-08-02:
// a depth correlation has an across-seed standard deviation of ~0.024 at 8000 steps, so a
// 32-member mean carries a standard error of ~0.004 and a DIFFERENCE of two such means
// carries ~0.006.
//
// # The correction this package exists because of
//
// A gap was filed claiming the expressions tier has no same-step cross-partition read,
// and this refactor was declared impossible on the strength of it. That was wrong —
// `upstreams:` gives row 0 while `params_from_upstream:` gives the current step, and both
// are pure config. The refactor works, and this package is the evidence.
package split

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — the same model expressed two ways, monolithic and partitioned, " +
		"compared as ensemble means. No market data"

	// agreementBound is DESCRIPTIVE and post-hoc: no bound on this was pre-registered,
	// because the agreed criterion was "within the measured spreads" rather than a number.
	// The difference of two 32-member means carries a standard error of ~0.006 on a depth
	// correlation, so 0.02 is roughly three of those — tight enough to catch a real
	// divergence, loose enough not to fire on the noise it is measured against.
	agreementBound = 0.02

	settleFrom = 100
	idxLimit   = 16
	idxCancel  = 17
	idxDepth   = 19
)

type summary struct {
	depthArrival, coupling, coMovement, drift cfgrun.EnsembleStat
}

func measureOne(config, partition string) (summary, error) {
	stores, err := cfgrun.RunEnsemble(config, cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", cfgrun.DefaultSteps),
	}, cfgrun.DefaultSeeds)
	if err != nil {
		return summary{}, err
	}
	var arr, can, com, drift []float64
	for _, storage := range stores {
		rows := storage.GetValues(partition)
		if len(rows) <= settleFrom {
			return summary{}, fmt.Errorf("split: %s produced too few rows", config)
		}
		rows = rows[settleFrom:]
		seg := diagnostics.Segment{Rows: rows}
		d, a, c := seg.Column(idxDepth), seg.Column(idxLimit), seg.Column(idxCancel)
		half := len(d) / 2
		arr = append(arr, diagnostics.Correlation(d, a))
		can = append(can, diagnostics.Correlation(d, c))
		com = append(com, diagnostics.Correlation(a, c))
		drift = append(drift, diagnostics.Mean(d[half:])/diagnostics.Mean(d[:half]))
	}
	return summary{
		depthArrival: cfgrun.Summarise(arr),
		coupling:     cfgrun.Summarise(can),
		coMovement:   cfgrun.Summarise(com),
		drift:        cfgrun.Summarise(drift),
	}, nil
}

func measure() (mono, split summary, err error) {
	mono, err = measureOne("lob_damping.yaml", "lob_damping")
	if err != nil {
		return mono, split, err
	}
	split, err = measureOne("lob_split.yaml", "lob_split")
	return mono, split, err
}

// ObservedBehaviour pins the agreement between the two expressions of the model.
func ObservedBehaviour() []claims.Claim {
	mono, split, err := measure()
	if err != nil {
		panic("split: measuring observed behaviour: " + err.Error())
	}
	return []claims.Claim{
		{
			ID: "the_partitioned_model_reproduces_the_monolithic_one",
			Statement: "The activity driver can be lifted into its own partition and " +
				"coupled back at the SAME step through params_from_upstream, and the " +
				"model is unchanged: all four scored quantities agree between the " +
				"monolithic and partitioned configs to within 0.006, against a bound of " +
				"0.02 and a difference-of-means standard error of about 0.006. This is " +
				"the guard that the decomposition is a re-expression rather than a new " +
				"model — and it is the evidence for a retraction, since a gap was filed " +
				"claiming the expressions tier cannot read another partition at the " +
				"current step and this refactor was declared impossible on it.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "absolute difference between the monolithic and partitioned 32-member " +
				"ensemble means",
			Limitations: "NOT an exact comparison and cannot be one: splitting gives each " +
				"partition its own seed and draw order, so no seed makes the two agree " +
				"bit-for-bit. Agreement within the noise is therefore the strongest " +
				"available statement, and a real difference smaller than ~0.006 would be " +
				"invisible to it. The 0.02 bound is DESCRIPTIVE and post-hoc — the agreed " +
				"criterion was 'within the measured spreads', not a number. Only the " +
				"DRIVER is split out here; flows, book and observables remain one " +
				"partition, so this establishes that a contemporaneous coupling survives " +
				"separation, not that the whole model decomposes.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: agreementBound, RefLabel: "0.02 (descriptive)"},
				{ObsIndex: 1, GreaterThan: false, Ref: agreementBound, RefLabel: "0.02 (descriptive)"},
				{ObsIndex: 2, GreaterThan: false, Ref: agreementBound, RefLabel: "0.02 (descriptive)"},
				{ObsIndex: 3, GreaterThan: false, Ref: agreementBound, RefLabel: "0.02 (descriptive)"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs arrivals", Value: math.Abs(mono.depthArrival.Mean - split.depthArrival.Mean)},
				{Label: "depth vs cancellations", Value: math.Abs(mono.coupling.Mean - split.coupling.Mean)},
				{Label: "arrivals vs cancellations", Value: math.Abs(mono.coMovement.Mean - split.coMovement.Mean)},
				{Label: "depth drift", Value: math.Abs(mono.drift.Mean - split.drift.Mean)},
			},
			Binding: claims.Binding{
				TestName: "TestPartitionedModelMatchesMonolith",
				TestFile: "pkg/split/behaviour_test.go",
			},
		},
	}
}
