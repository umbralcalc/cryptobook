// Package ceiling pins the account of WHY the co-movement signature is missed, and the
// model that comes closest to it.
//
// # Why this package exists
//
// Six configs were built between cfg/lob_var.yaml and cfg/lob_counts.yaml and none of
// them had a scoring package: their results lived only in PREREGISTRATION.md and
// DECISIONS.md prose. That suspends this repo's own rule — no number without a test — so
// the model-internal findings are pinned here. What is NOT pinned here is any comparison
// against market data: that needs recorded Binance segments, cannot be redistributed, and
// stays in DECISIONS.md.
//
// # The account
//
// Arrivals and cancellations share a latent driver A, and each carries independent Poisson
// noise. That caps how correlated they can be:
//
//	ceiling = N * Var(A) / (E[A]^2 + N * Var(A))
//
// where N is the per-step count. Counts and variance enter ONLY through their product, so
// the ceiling cannot distinguish them — but the achieved co-movement falls short of the
// ceiling by a saturation penalty, and that penalty CAN.
//
// # The matched pair, which is the finding
//
// cfg/lob_burst.yaml reaches N*V ~ 515 by raising Var(A); cfg/lob_counts.yaml reaches
// N*V ~ 550+ by raising counts. Same ceiling, different route. The penalty grows on the
// variance route and does not on the counts route, because raising Var(A) changes the
// driver's distribution and so how the arrival damping's denominator behaves, while
// raising counts at fixed variance leaves the driver untouched.
//
// So the penalty is a property of the DRIVER'S SPREAD, not of the ceiling — established by
// construction in a designed comparison rather than inferred from a sequence of runs.
package ceiling

import (
	"fmt"
	"math"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — three configs at matched Poisson ceilings reached by different " +
		"routes. Model-internal only: no market comparison is made here"

	// Every model in this project reads its observables at these columns.
	idxLimit  = 16
	idxCancel = 17
	idxDepth  = 19
	idxSpread = 20
	idxAct    = 21

	settleFrom  = 100
	emptySpread = 99.0

	// meanActivity is the driver's stationary mean, held at 4 across every config here so
	// the ceiling depends only on counts and variance.
	meanActivity = 4.0
)

// route is one config and the partition it writes.
type route struct{ label, config, partition string }

var routes = []route{
	{"baseline", "lob_var.yaml", "lob_var"},
	{"via variance", "lob_burst.yaml", "lob_burst"},
	{"via counts", "lob_counts.yaml", "lob_counts"},
}

type measured struct {
	counts, driverVar, ceiling, coMovement, gap cfgrun.EnsembleStat
	depthArrival, coupling, drift, spreadSD     cfgrun.EnsembleStat
}

func measureRoute(r route) (measured, error) {
	stores, err := cfgrun.RunEnsemble(r.config, cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", cfgrun.DefaultSteps),
	}, cfgrun.DefaultSeeds)
	if err != nil {
		return measured{}, err
	}
	var n, v, ceil, com, gap, arr, can, drift, sprSD []float64
	for _, storage := range stores {
		rows := storage.GetValues(r.partition)
		if len(rows) <= settleFrom {
			return measured{}, fmt.Errorf("ceiling: %s produced too few rows", r.config)
		}
		rows = rows[settleFrom:]
		seg := diagnostics.Segment{Rows: rows}
		a, c, d := seg.Column(idxLimit), seg.Column(idxCancel), seg.Column(idxDepth)
		half := len(d) / 2

		count := diagnostics.Mean(a)
		act := seg.Column(idxAct)
		mu := diagnostics.Mean(act)
		variance := 0.0
		for _, x := range act {
			variance += (x - mu) * (x - mu)
		}
		variance /= float64(len(act) - 1)
		product := count * variance
		theCeiling := product / (meanActivity*meanActivity + product)
		theCoMovement := diagnostics.Correlation(a, c)

		n = append(n, count)
		v = append(v, variance)
		ceil = append(ceil, theCeiling)
		com = append(com, theCoMovement)
		gap = append(gap, theCeiling-theCoMovement)
		arr = append(arr, diagnostics.Correlation(d, a))
		can = append(can, diagnostics.Correlation(d, c))
		drift = append(drift, diagnostics.Mean(d[half:])/diagnostics.Mean(d[:half]))

		observed := make([]float64, 0, len(rows))
		for _, row := range rows {
			if row[idxSpread] < emptySpread {
				observed = append(observed, row[idxSpread])
			}
		}
		if len(observed) == 0 {
			return measured{}, fmt.Errorf("ceiling: %s was one-sided at every step", r.config)
		}
		m2 := diagnostics.Mean(observed)
		v2 := 0.0
		for _, x := range observed {
			v2 += (x - m2) * (x - m2)
		}
		sprSD = append(sprSD, math.Sqrt(v2/float64(len(observed))))
	}
	return measured{
		counts: cfgrun.Summarise(n), driverVar: cfgrun.Summarise(v),
		ceiling: cfgrun.Summarise(ceil), coMovement: cfgrun.Summarise(com),
		gap: cfgrun.Summarise(gap), depthArrival: cfgrun.Summarise(arr),
		coupling: cfgrun.Summarise(can), drift: cfgrun.Summarise(drift),
		spreadSD: cfgrun.Summarise(sprSD),
	}, nil
}

func measureAll() (map[string]measured, error) {
	out := make(map[string]measured, len(routes))
	for _, r := range routes {
		m, err := measureRoute(r)
		if err != nil {
			return nil, err
		}
		out[r.label] = m
	}
	return out, nil
}

// ObservedBehaviour pins the ceiling account and the matched-pair result.
func ObservedBehaviour() []claims.Claim {
	m, err := measureAll()
	if err != nil {
		panic("ceiling: measuring observed behaviour: " + err.Error())
	}
	base, burst, counts := m["baseline"], m["via variance"], m["via counts"]
	binding := claims.Binding{
		TestName: "TestCeilingAccount",
		TestFile: "pkg/ceiling/behaviour_test.go",
	}

	return []claims.Claim{
		{
			ID: "the_saturation_penalty_tracks_the_drivers_spread_not_the_poisson_ceiling",
			Statement: "Arrivals and cancellations share a driver and carry independent " +
				"Poisson noise, which caps their correlation at N*Var(A)/(E[A]^2 + " +
				"N*Var(A)). Counts and variance enter ONLY through their product, so the " +
				"ceiling cannot tell them apart — but the shortfall BELOW the ceiling " +
				"can. Two configs reach essentially the same ceiling by different routes: " +
				"raising Var(A) drives the shortfall UP, while raising counts at fixed " +
				"variance leaves it where it was. The penalty is therefore a property of " +
				"the DRIVER'S SPREAD interacting with the arrival damping, not of the " +
				"ceiling — raising Var(A) changes how the damping denominator behaves, " +
				"raising counts does not touch the driver at all.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Poisson ceiling minus achieved co-movement, at three (counts, driver " +
				"variance) settings",
			Limitations: "NOT PRE-REGISTERED. Both bounds were set after the " +
				"measurements, so this is a regression guard on an already-observed " +
				"result and carries none of the evidential weight of the pre-registered " +
				"blocks in PREREGISTRATION.md. It is a designed comparison at two " +
				"matched ceilings, not a sweep, so it establishes the DIRECTION of the " +
				"effect rather than its functional form — how the penalty scales with " +
				"driver spread is unmeasured. Model " +
				"internal: no market number appears in this package, and the comparison " +
				"that motivated it lives in DECISIONS.md because it needs recorded " +
				"segments. The account was also stated over the wrong variable twice " +
				"before it was stated over the right one; it is constant in COUNTS, not " +
				"in the ceiling, and two earlier blocks read it the other way.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 1, GreaterThan: true, Ref: 0.070,
					RefLabel: "0.070 (the variance route inflates the penalty)"},
				{ObsIndex: 2, GreaterThan: false, Ref: 0.065,
					RefLabel: "0.065 (the counts route does not)"},
			},
			Observations: []claims.Observation{
				{Label: "baseline penalty", Value: base.gap.Mean},
				{Label: "penalty via variance", Value: burst.gap.Mean},
				{Label: "penalty via counts", Value: counts.gap.Mean},
			},
			Binding: binding,
		},
		{
			ID: "co_movement_stays_below_its_poisson_ceiling_at_every_setting",
			Statement: "The ceiling is an upper bound and behaves like one: at all three " +
				"settings the achieved co-movement sits strictly below N*Var(A)/(E[A]^2 " +
				"+ N*Var(A)). This is the half of the account that is forced — shared " +
				"driver plus independent Poisson noise cannot produce more correlation " +
				"than the shared part supports — and it is claimed so that a change " +
				"which appeared to beat the bound would break loudly rather than be " +
				"read as an improvement.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "achieved co-movement minus its Poisson ceiling, at three settings",
			Limitations: "Near-forced by construction and claimed as a guard rather than " +
				"a finding. It does not establish that the ceiling formula is the RIGHT " +
				"bound, only that it is not violated — a looser but still valid bound " +
				"would pass this equally well.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 0, RefLabel: "0 (strictly below)"},
				{ObsIndex: 1, GreaterThan: false, Ref: 0, RefLabel: "0 (strictly below)"},
				{ObsIndex: 2, GreaterThan: false, Ref: 0, RefLabel: "0 (strictly below)"},
			},
			Observations: []claims.Observation{
				{Label: "baseline", Value: base.coMovement.Mean - base.ceiling.Mean},
				{Label: "via variance", Value: burst.coMovement.Mean - burst.ceiling.Mean},
				{Label: "via counts", Value: counts.coMovement.Mean - counts.ceiling.Mean},
			},
			Binding: binding,
		},
		{
			ID: "the_counts_route_holds_all_three_correlation_signatures_at_once",
			Statement: "cfg/lob_counts.yaml is the first model in this project whose " +
				"three correlation signatures sit simultaneously where the pooled " +
				"Binance measurements do. Model-internal here: it reads corr(depth, " +
				"arrivals), corr(depth, cancels) and corr(arrivals, cancels) together, " +
				"having reached them through a driver whose variance and counts were " +
				"computed from the ceiling algebra rather than fitted, with only " +
				"churn_rate adjusted and only on mean depth.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "Pearson correlations of the depth/arrival, depth/cancellation and " +
				"arrival/cancellation pairs; and the book-survival checks",
			Limitations: "NOT PRE-REGISTERED as a package — the underlying prediction " +
				"WAS pre-registered as block BO and the bands here are the ones it was " +
				"scored against, unwidened, but the reader should check " +
				"PREREGISTRATION.md rather than take that on trust. THE MARKET " +
				"COMPARISON IS NOT HERE and cannot be — it needs " +
				"recorded segments that cannot be redistributed, so DECISIONS.md carries " +
				"it. What this pins is that the three quantities take these values " +
				"together, not that they match anything. The bands they were judged " +
				"against are 1.5 SD of a between-occasion spread estimated from four " +
				"occasions on one venue within ten days, and the co-movement's agreement " +
				"depends on which of two defensible spread estimates is used. Read " +
				"DECISIONS.md before treating this as agreement with a market.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: -0.062, RefLabel: "-0.062"},
				{ObsIndex: 0, GreaterThan: true, Ref: -0.217, RefLabel: "-0.217"},
				{ObsIndex: 1, GreaterThan: false, Ref: 0.030, RefLabel: "+0.030"},
				{ObsIndex: 1, GreaterThan: true, Ref: -0.146, RefLabel: "-0.146"},
				{ObsIndex: 2, GreaterThan: true, Ref: 0.9134, RefLabel: "+0.9134"},
				{ObsIndex: 3, GreaterThan: false, Ref: 1.3, RefLabel: "1.3"},
				{ObsIndex: 4, GreaterThan: true, Ref: 0.1, RefLabel: "0.1"},
			},
			Observations: []claims.Observation{
				{Label: "depth vs arrivals", Value: counts.depthArrival.Mean},
				{Label: "depth vs cancellations", Value: counts.coupling.Mean},
				{Label: "arrivals vs cancellations", Value: counts.coMovement.Mean},
				{Label: "depth drift", Value: counts.drift.Mean},
				{Label: "spread sd", Value: counts.spreadSD.Mean},
			},
			Binding: binding,
		},
	}
}
