// Package baseline runs the Spike 2.2 residual diagnostics against the SYNTHETIC
// models, using the same measurement code as the real-market ones.
//
// # Why this is the control the real-market result needed
//
// pkg/crypto reports that real order flow has essentially no correlation between
// resting depth and cancellation flow (-0.12 on Binance BTCUSDT), where the model
// requires it strongly positive. On its own that is ambiguous: a
// correlation near zero could mean the coupling is absent from the market, or that
// the diagnostic cannot see a coupling that is there.
//
// These claims settle it. Both synthetic models have the coupling BY CONSTRUCTION —
// cancellations are drawn as poisson(cancel_rate * resting) — and the same three
// functions detect it at +0.37 and +0.64. So the measurement works, and the real
// market's near-zero reading is the absence of a coupling rather than the absence of a
// detector.
//
// They are also the one part of the Spike 2.2 evidence that CI can re-check, since
// no third-party data is involved.
//
// # And they answer whether prices fixed Phase 2
//
// cfg/lob_priced.yaml was built because Spike 2.2 sent the project back to the
// domain model. The obvious question is whether prices, book-walking and decaying
// arrival intensity move the model towards the real diagnostics. Measured, they do
// not:
//
//	                        depth coupling   churn        dispersion
//	minimal synthetic           +0.37        +0.04        ~1
//	priced synthetic            +0.64        -0.04        ~1-4
//	crypto (Binance)            -0.12        +0.98        92-4601
//
// The priced model's depth coupling is if anything STRONGER, taking it further from
// the market rather than closer, and its churn correlation stays near zero against
// crypto's +0.98. So prices were necessary for the stability outputs (three of
// four now answerable) but are NOT the missing mechanism for Phase 2's failure.
// Coupled arrival and cancellation — quote churn — remains the only candidate
// standing, and it is now the one standing alone.
// # RE-SCORED ON ENSEMBLES 2026-08-02 — and the numbers moved toward theory
//
// Every value is now a 32-member ensemble mean at 8000 steps rather than one seed, and the
// effect is a small vindication of the change. These models draw independent Poisson
// streams, so their dispersions should be exactly 1 and their arrival/cancellation
// correlation exactly 0 BY CONSTRUCTION. One seed gave 0.96, 0.98 and +/-0.04. The
// ensemble means give 1.00 and 0.00.
//
// The single-seed deviations were never findings; they were noise around values the
// construction fixes. That is worth recording because this package's whole job is to be
// the synthetic control the real-market diagnostics are read against — a control whose own
// numbers wander is a poor yardstick.
package baseline

import (
	"fmt"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — both generators in cfg/, measured with the same code as " +
		"the real-market diagnostics in pkg/crypto"

	// settleFrom discards the approach to stationarity.
	settleFrom = 100

	// couplingFloor is how strongly a model that HAS the depth coupling must show
	// it, for the diagnostic to count as able to detect one. The real markets read
	// -0.12 and -0.02 against this.
	couplingFloor = 0.3
	// churnCeiling is how weak the arrival/cancellation correlation must stay in a
	// model with independent streams. Binance BTCUSDT reads +0.98.
	churnCeiling = 0.2
	// dispersionBand brackets Poisson: these models draw Poisson counts, so their
	// variance/mean must sit near 1 or the measurement is wrong.
	dispersionFloor   = 0.5
	dispersionCeiling = 5.0
)

// Model names one synthetic generator and where its flow columns live.
type Model struct {
	Label     string
	Config    string
	Partition string
	Steps     string
	LongSteps string
	Limit     int
	Cancel    int
	Market    int
	Depth     int
}

var models = []Model{
	{"minimal", "lob_generator.yaml", "lob_flow", "max_steps: 200", "max_steps: 2000",
		6, 7, 8, 9},
	{"priced", "lob_priced.yaml", "lob_priced", "max_steps: 400", "max_steps: 2000",
		16, 17, 18, 19},
}

// The churn model is deliberately NOT here. It breaks the independence these
// claims assert, which is the entire point of it; its pre-registered predictions
// are scored in pkg/churn instead. Adding it to this list would turn a control
// into a contradiction.

// Measure runs one model and returns its three diagnostics.
// Measure runs m as a 32-member ensemble at 8000 steps and returns the mean of each
// quantity across members, with the across-member spread attached.
//
// ENSEMBLED 2026-08-02. This used to be one seed at m.LongSteps. The seed audit of that
// day measured a single member's depth correlation at a standard deviation of ~0.05, which
// is the scale several of this project's comparisons were decided at — so single-seed
// values could not support them. See cfgrun.DefaultSeeds for the sizing.
//
// m.LongSteps is now ignored in favour of cfgrun.DefaultSteps, so every model package
// sharing this helper is measured at one length and the numbers stay comparable across
// them. Model.LongSteps is kept on the struct because callers still name the substitution
// target with it.
func Measure(m Model) (
	coupling, churn cfgrun.EnsembleStat,
	dispersion [3]cfgrun.EnsembleStat,
	err error,
) {
	stores, err := cfgrun.RunEnsemble(m.Config, cfgrun.Subs{
		m.Steps: fmt.Sprintf("max_steps: %d", cfgrun.DefaultSteps),
	}, cfgrun.DefaultSeeds)
	if err != nil {
		return coupling, churn, dispersion, err
	}
	var couplings, churns, dispA, dispC, dispM []float64
	for _, storage := range stores {
		rows := storage.GetValues(m.Partition)
		if len(rows) <= settleFrom {
			return coupling, churn, dispersion,
				fmt.Errorf("baseline: %s produced too few rows", m.Label)
		}
		segment := diagnostics.Segment{Rows: rows[settleFrom:]}
		arrivals := segment.Column(m.Limit)
		cancels := segment.Column(m.Cancel)
		market := segment.Column(m.Market)
		depth := segment.Column(m.Depth)
		couplings = append(couplings, diagnostics.Correlation(depth, cancels))
		churns = append(churns, diagnostics.Correlation(arrivals, cancels))
		dispA = append(dispA, diagnostics.Dispersion(arrivals))
		dispC = append(dispC, diagnostics.Dispersion(cancels))
		dispM = append(dispM, diagnostics.Dispersion(market))
	}
	return cfgrun.Summarise(couplings), cfgrun.Summarise(churns),
		[3]cfgrun.EnsembleStat{
			cfgrun.Summarise(dispA),
			cfgrun.Summarise(dispC),
			cfgrun.Summarise(dispM),
		}, nil
}

// ObservedBehaviour measures the synthetic control for the Spike 2.2 diagnostics.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		panic("baseline: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	binding := claims.Binding{
		TestName: "TestSyntheticDiagnosticBaseline",
		TestFile: "pkg/baseline/behaviour_test.go",
	}

	coupling := make([]claims.Observation, 0, len(models))
	churn := make([]claims.Observation, 0, len(models))
	dispersion := make([]claims.Observation, 0, len(models)*3)
	for _, m := range models {
		c, k, d, err := Measure(m)
		if err != nil {
			return nil, err
		}
		coupling = append(coupling, claims.Observation{Label: m.Label, Value: c.Mean})
		churn = append(churn, claims.Observation{Label: m.Label, Value: k.Mean})
		dispersion = append(dispersion,
			claims.Observation{Label: m.Label + " arrivals", Value: d[0].Mean},
			claims.Observation{Label: m.Label + " cancellations", Value: d[1].Mean},
			claims.Observation{Label: m.Label + " market orders", Value: d[2].Mean})
	}

	above := func(n int, ref float64, label string) []claims.Threshold {
		out := make([]claims.Threshold, n)
		for i := range out {
			out[i] = claims.Threshold{ObsIndex: i, GreaterThan: true, Ref: ref, RefLabel: label}
		}
		return out
	}
	below := func(n int, ref float64, label string) []claims.Threshold {
		out := make([]claims.Threshold, n)
		for i := range out {
			out[i] = claims.Threshold{ObsIndex: i, GreaterThan: false, Ref: ref, RefLabel: label}
		}
		return out
	}
	band := make([]claims.Threshold, 0, len(dispersion)*2)
	for i := range dispersion {
		band = append(band,
			claims.Threshold{ObsIndex: i, GreaterThan: true, Ref: dispersionFloor,
				RefLabel: fmt.Sprintf("%.1f", dispersionFloor)},
			claims.Threshold{ObsIndex: i, GreaterThan: false, Ref: dispersionCeiling,
				RefLabel: fmt.Sprintf("%.1f", dispersionCeiling)})
	}

	return []claims.Claim{
		{
			ID: "the_depth_coupling_diagnostic_detects_the_coupling_when_it_is_present",
			Statement: "Both synthetic models draw cancellations as " +
				"poisson(cancel_rate x resting depth), so the coupling is there by " +
				"construction — and the diagnostic finds it, strongly positive in both. " +
				"This is the control the real-market result needed: a near-zero reading " +
				"on real data means the coupling is ABSENT FROM THE MARKET, not that the " +
				"measurement cannot see one.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between resting depth and cancellation flow",
			Limitations: "It establishes that the diagnostic has power, not how much. " +
				"A weak-but-real coupling in market data could still be missed, so this " +
				"licenses reading a near-zero result as 'no strong coupling' rather than " +
				"as 'no coupling at all'.",
			Thresholds: above(len(coupling), couplingFloor,
				fmt.Sprintf("+%.1f", couplingFloor)),
			Observations: coupling,
			Binding:      binding,
		},
		{
			ID: "prices_and_book_walking_do_not_introduce_quote_churn",
			Statement: "Adding prices, an emergent spread and marketable orders that " +
				"walk the book leaves arrivals and cancellations essentially " +
				"uncorrelated, exactly as in the minimal model — because both draw them " +
				"as independent streams. Binance BTCUSDT reads about +0.98. So the " +
				"domain-model step that unlocked the stability outputs did NOT supply " +
				"the mechanism Spike 2.2 found missing, and coupled arrival/cancellation " +
				"remains the only candidate standing.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between per-step arrival and cancellation counts",
			Limitations: "A negative result about a change that was made for a different " +
				"reason — prices were added for the Spike 4.2 outputs, and it would have " +
				"been surprising if they had also produced churn. It says the hypothesis " +
				"is untouched, not that churn is the right answer; nothing here tests " +
				"whether adding a churn mechanism would reproduce the real diagnostics.",
			Thresholds: below(len(churn), churnCeiling,
				fmt.Sprintf("+%.1f", churnCeiling)),
			Observations: churn,
			Binding:      binding,
		},
		{
			ID: "synthetic_counts_are_poisson_dispersed_as_constructed",
			Statement: "Every count in both synthetic models has a variance-to-mean " +
				"ratio near 1, which is what drawing Poisson counts means. Real order " +
				"flow does not: into the thousands on lot-discretised crypto. The " +
				"overdispersion Spike 2.2 reports is therefore a property of the data " +
				"rather than an artefact of how it is measured.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "variance / mean of per-step counts (Poisson requires exactly 1)",
			Limitations: "The priced model's market-order dispersion sits at the top of " +
				"the band because marketable orders arrive in fixed-size blocks there, " +
				"so its counts are a scaled Poisson rather than a Poisson. That is a " +
				"modelling choice, not a defect, but it means this claim brackets a band " +
				"rather than asserting exactly 1.",
			Thresholds:   band,
			Observations: dispersion,
			Binding:      binding,
		},
	}, nil
}
