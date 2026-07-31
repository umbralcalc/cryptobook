// Package recycled scores the depth-neutral churn model — cancellations that recycle
// what was just posted, rather than a fraction of what is resting.
//
// # The question it answers
//
// Every model before this one put its whole depth-stabilising brake on ONE flow.
// cfg/lob_arrivals.yaml reads corr(depth, arrivals) -0.116 against corr(depth, cancels)
// -0.002; the attrition variants put +0.6 on cancellations, with the wrong sign. On
// every Binance segment recorded, BOTH flows are mildly anti-correlated with depth,
// arrivals the stronger. So: can a model produce two comparable mild anti-correlations
// with depth WITHOUT losing the contemporaneous co-movement it already reproduces?
//
// # The mechanism
//
// The arrival side is untouched. Cancellation gains a recycled term alongside the
// existing same-step churn:
//
//	can_i(t) = min( q_i , recycle * arr_i(t-1) + Poisson(churn_rate * decay_i * activity * dt) )
//
// The recycled term is depth-neutral by construction — it depends on what was posted at
// that level last step, not on what is resting there now. The route to T is therefore
// INDIRECT, which is what made it uncertain: no depth term is written into cancellation
// anywhere, but last step's arrivals were themselves damped by last step's depth, and
// depth is autocorrelated.
//
// # The answer is no, and the reason is an identity rather than a rate
//
// T, U and V all FAILED; only the survival check W passed. T failed in the direction the
// pre-registered outcome table did not contain — the coupling came back POSITIVE at
// +0.458, stronger than the +0.37 of the minimal model this whole line of work started
// from.
//
// The mechanism is worth stating plainly because it generalises past this config.
// Cancellation was made proportional to arr(t-1). But arr(t-1) is precisely what is
// resting at t — a book is an accumulator of its own recent arrivals — so cancellation
// and depth were handed a shared term, and a positive correlation followed by
// construction. **Depth-neutral in the RATE is not depth-neutral in the CORRELATION.**
//
// That kills the family, not just the instance: ANY cancellation rule keyed to recent
// arrivals inherits this coupling, whatever the lag or the coefficient. The reasoning
// that picked a half-weight mixture over a pure lag was sound as far as it went and did
// not go far enough — it examined what lagging costs (V, correctly predicted in
// direction, badly underestimated in size: +0.897 to +0.436) without examining what
// keying to arrivals buys.
//
// # What is scored here, and what is not
//
// Predictions T, U, V and W were fixed in PREREGISTRATION.md and committed in 480992d,
// before cfg/lob_churn_recycled.yaml existed. One value moved after that commit and
// only one: churn_rate, re-set from the inherited 1.15 to 0.55 on MEAN DEPTH alone, by
// a sweep that computed no correlation. That adjustment is the one the pre-registration
// permits, and the sweep is recorded in the config and in DECISIONS.md rather than
// folded into the result.
//
// Dispersion is deliberately not scored. The shared activity driver already produces it
// and this mechanism does not change it; claiming it here would be re-claiming the
// previous step's result.
package recycled

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	configName = "lob_churn_recycled.yaml"
	partition  = "lob_recycled"

	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — depth-neutral-churn generator at an arbitrary " +
		"parameterisation. Model-internal only: no real-market comparison is made here"

	// The pre-registered bounds, fixed in 480992d before the config existed.
	//
	// T is two-sided on purpose: the point was to land inside the band the Binance
	// segments occupy, not merely to move off zero, so overshooting past the floor is
	// a different failure from not moving at all.
	couplingBandFloor   = -0.30
	couplingBandCeiling = -0.02
	brakeCeiling        = -0.05
	coMovementFloor     = 0.7
	driftCeiling        = 1.3
	spreadSDFloor       = 0.1

	// pinnedCouplingFloor is NOT a pre-registered bound. T's band is recorded above
	// exactly as it was fixed and is deliberately left unused by any threshold — the
	// measured +0.458 does not fail that band narrowly, it fails it by sign, so
	// asserting against the band would report the failure as a near miss. This floor
	// pins the failure that actually happened, at a value far below it, so a future
	// change that fixed the coupling breaks the claim loudly instead of passing.
	pinnedCouplingFloor = 0.2

	settleFrom  = 100
	emptySpread = 99.0

	// Indices 0-20 are identical to cfg/lob_arrivals.yaml's row by construction — the
	// promoted per-level arrivals are APPENDED at 21-36 — so these are the same
	// constants that package reads, and the two models stay directly comparable.
	idxLimit  = 16
	idxCancel = 17
	idxDepth  = 19
	idxSpread = 20
)

// measured holds everything the four predictions are scored on.
type measured struct {
	coupling, depthArrival, coMovement float64
	drift, spread, spreadSD, meanDepth float64
}

func measure() (measured, error) {
	storage, err := cfgrun.Run(configName, cfgrun.Subs{"max_steps: 400": "max_steps: 2000"})
	if err != nil {
		return measured{}, err
	}
	rows := storage.GetValues(partition)
	if len(rows) <= settleFrom {
		return measured{}, fmt.Errorf("recycled: run produced too few rows")
	}
	rows = rows[settleFrom:]
	segment := diagnostics.Segment{Rows: rows}
	arrival := segment.Column(idxLimit)
	cancel := segment.Column(idxCancel)
	depth := segment.Column(idxDepth)

	// Stationarity by halves rather than a fitted trend, matching pkg/arrivals so W is
	// measured the same way its predecessor was.
	half := len(depth) / 2
	m := measured{
		coupling:     diagnostics.Correlation(depth, cancel),
		depthArrival: diagnostics.Correlation(depth, arrival),
		coMovement:   diagnostics.Correlation(arrival, cancel),
		drift:        diagnostics.Mean(depth[half:]) / diagnostics.Mean(depth[:half]),
		meanDepth:    diagnostics.Mean(depth),
	}

	// One-sided steps carry the sentinel rather than a spread, so they are excluded —
	// averaging a sentinel would turn "the book broke" into "the spread was wide".
	observed := make([]float64, 0, len(rows))
	for _, row := range rows {
		if row[idxSpread] < emptySpread {
			observed = append(observed, row[idxSpread])
		}
	}
	if len(observed) == 0 {
		return measured{}, fmt.Errorf("recycled: every step was one-sided")
	}
	m.spread = diagnostics.Mean(observed)
	variance := 0.0
	for _, x := range observed {
		variance += (x - m.spread) * (x - m.spread)
	}
	m.spreadSD = math.Sqrt(variance / float64(len(observed)))
	return m, nil
}

// ObservedBehaviour scores the pre-registered predictions T, U, V and W.
//
// Three of the four FAILED, and T failed in the direction nobody wrote down. The claim
// IDs below say so: a claim that reported these as anything other than failures would
// be the exact dishonesty PREREGISTRATION.md exists to make impossible.
func ObservedBehaviour() []claims.Claim {
	m, err := measure()
	if err != nil {
		panic("recycled: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestDepthNeutralChurn",
		TestFile: "pkg/recycled/behaviour_test.go",
	}

	return []claims.Claim{
		{
			ID: "prediction_t_recycling_reintroduces_the_depth_coupling_through_the_book_identity",
			Statement: "Prediction T, FAILED, and failed in the direction the outcome " +
				"table did not contain. The pre-registered band was [-0.30, -0.02] and " +
				"the measured correlation is +0.458 — not a weak version of the target " +
				"but the OPPOSITE SIGN, and stronger than the +0.37 the minimal model " +
				"had. The reason is an accounting identity rather than a rate: " +
				"cancellation was made proportional to arr(t-1), and arr(t-1) is what is " +
				"resting at t, so cancellation and depth were given a shared term. " +
				"DEPTH-NEUTRAL IN THE RATE IS NOT DEPTH-NEUTRAL IN THE CORRELATION, " +
				"because a book is an accumulator of its own recent arrivals.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between resting depth and cancellation flow",
			Limitations: "This is pinned as a FLOOR so a future change that quietly " +
				"fixed it would break the claim rather than pass. It generalises further " +
				"than one config and that is the point: any cancellation rule keyed to " +
				"RECENT ARRIVALS inherits a positive depth coupling by the same identity, " +
				"whatever the lag or the coefficient. It says nothing about rules keyed " +
				"to something other than arrivals, which is where the mechanism hunt now " +
				"has to go.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: pinnedCouplingFloor,
					RefLabel: "+0.2 (the pre-registered band it had to fall far below)"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs cancellations", Value: m.coupling},
			},
			Binding: binding,
		},
		{
			ID: "prediction_u_the_brake_ordering_inverts_when_cancellation_tracks_recent_arrivals",
			Statement: "Prediction U, FAILED on the half that was actually being tested. " +
				"Its forced half held — the inherited arrival damping still reads -0.110 " +
				"— but the ORDERING inverted: the cancellation side now carries +0.458, " +
				"four times the arrival side's magnitude and with the wrong sign, so the " +
				"margin is -0.348 where U required it positive. Every Binance segment has " +
				"arrivals as the stronger brake; this model now has cancellation as the " +
				"stronger ANTI-brake, which is further from the market than the model it " +
				"was built to improve on.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlation between resting depth and each flow; and the " +
				"margin by which the arrival correlation is the more negative",
			Limitations: "The forced half passing establishes only that the arrival side " +
				"was inherited intact, which the config test asserts directly and more " +
				"cheaply. The failure is the informative part, and it is a restatement of " +
				"[[prediction_t_recycling_reintroduces_the_depth_coupling_through_the_book_identity]] " +
				"rather than independent evidence — the same shared term produces both.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: brakeCeiling,
					RefLabel: "-0.05 (pre-registered, and the forced half)"},
				{ObsIndex: 2, GreaterThan: false, Ref: 0,
					RefLabel: "0 (U required this POSITIVE; it is not)"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs arrivals", Value: m.depthArrival},
				{Label: "depth vs cancellations", Value: m.coupling},
				{Label: "margin", Value: math.Abs(m.depthArrival) - math.Abs(m.coupling)},
			},
			Binding: binding,
		},
		{
			ID: "prediction_v_recycled_churn_halves_the_contemporaneous_co_movement",
			Statement: "Prediction V, FAILED, and it is the failure that was reasoned " +
				"out in advance — just not far enough. PREREGISTRATION.md argued a PURE " +
				"lag would drive contemporaneous co-movement to about zero, because the " +
				"activity driver is iid per step, and chose a half-weight mixture to " +
				"avoid that. Half was already too much: +0.897 fell to +0.436, well " +
				"under the +0.7 floor. So the cost of lagging scales faster than its " +
				"weight, and the model sold its best-matched signature (real: +0.98) to " +
				"buy a depth signature it did not get.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "Pearson correlation between per-step arrival and cancellation counts",
			Limitations: "Pinned as a CEILING so a future fix breaks it loudly. One " +
				"mixture weight at one parameterisation — it establishes that 0.5 is too " +
				"much, not where the boundary lies, and the shape of the trade-off " +
				"between recycling weight and co-movement is unmeasured. Finding that " +
				"boundary by sweeping the weight would need its own pre-registration, " +
				"and per T it would be sweeping toward a mechanism that fails anyway.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: coMovementFloor,
					RefLabel: "+0.7 (the pre-registered floor it had to clear)"},
			},
			Observations: []claims.Observation{
				{Label: "arrivals vs cancellations", Value: m.coMovement},
			},
			Binding: binding,
		},
		{
			ID: "prediction_w_the_book_survives_recycled_churn",
			Statement: "Prediction W, PASSED, and it is the only one that did. The book " +
				"stays conserved at a drift of 1.066 and the spread keeps a live " +
				"distribution at 0.579 ticks of standard deviation. Recycling removes " +
				"cancellation volume that no longer scales with what is resting, leaving " +
				"the arrival-side brake to hold the book alone, and it holds — so this " +
				"mechanism fails on correlations rather than on survival, unlike the " +
				"attrition-free variant which failed on both.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "mean resting depth over the second half of the scored window divided " +
				"by the first half (a conserved book gives ~1); spread sd in ticks over " +
				"two-sided steps; and mean depth in lots",
			Limitations: "Mean depth is NOT a result — churn_rate was re-set to 0.55 on " +
				"exactly this quantity, the one adjustment the pre-registration permits, " +
				"so it is reported as provenance rather than as a finding. A passing cost " +
				"check on a mechanism whose three substantive predictions failed is not a " +
				"partial success: it says the failure is clean, not that anything works.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: driftCeiling,
					RefLabel: "1.3 (pre-registered)"},
				{ObsIndex: 1, GreaterThan: true, Ref: spreadSDFloor,
					RefLabel: "0.1 (pre-registered)"},
			},
			Observations: []claims.Observation{
				{Label: "second half / first half", Value: m.drift},
				{Label: "spread sd", Value: m.spreadSD},
				{Label: "mean depth", Value: m.meanDepth},
			},
			Binding: binding,
		},
	}
}
