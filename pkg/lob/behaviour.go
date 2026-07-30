// Package lob holds the expected-behaviour claims for the minimal LOB generator
// defined in cfg/lob_generator.yaml.
//
// The model itself is entirely in that config. Nothing here states any dynamics —
// this file runs the config at several parameter values and reports how the book
// responded, and behaviour_test.go asserts those responses hold.
//
// Every claim is a counterfactual about market state (resting depth, where
// liquidity sits in the ladder), never a directional claim about price. The model
// has no price process at all, which makes that easy to hold to here and is worth
// keeping in mind when Phase 4 adds outputs that could be read otherwise.
package lob

import (
	"fmt"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

const (
	// configName is the model under test. One config, one source of truth.
	configName = "lob_generator.yaml"

	// Column indices in the lob_flow state row — see cfg/lob_generator.yaml.
	colBidTouch   = 0
	colBidBehind  = 1
	colAskTouch   = 3
	colAskBehind  = 4
	colDepthStart = 9

	// runSteps is long enough that a mean over it is stable to well under the
	// response sizes these claims assert.
	runSteps = 1200
	// burnIn discards the approach to stationarity. The ladder starts at the
	// arrival/cancellation fixed point, so this only has to cover the market-order
	// depletion of the touch settling in.
	burnIn = 200
)

// seeds are averaged over for every measurement. These claims are about the
// model's response to a parameter, not about one realisation of it, so a
// single-seed number could carry sampling noise into the assertion.
var seeds = []int{11, 22, 33}

// phase labels this package's claims in CLAIMS.md.
const phase = "1 — Synthetic parameter recovery"

// dataset describes what these claims were verified against. They say nothing
// about any real market, and the wording is meant to make that impossible to miss.
const dataset = "synthetic — the model's own generated order flow, no market data"

// meanOver runs the config once per seed with the given substitutions and returns
// the seed-averaged mean of one state column.
func meanOver(subs cfgrun.Subs, column int) (float64, error) {
	total := 0.0
	for _, seed := range seeds {
		withSeed := cfgrun.Subs{
			"max_steps: 200": fmt.Sprintf("max_steps: %d", runSteps),
			"seed: 20260728": fmt.Sprintf("seed: %d", seed),
		}
		for key, value := range subs {
			withSeed[key] = value
		}
		storage, err := cfgrun.Run(configName, withSeed)
		if err != nil {
			return 0, err
		}
		mean, err := cfgrun.MeanColumn(storage, "lob_flow", column, burnIn)
		if err != nil {
			return 0, err
		}
		total += mean
	}
	return total / float64(len(seeds)), nil
}

// ObservedBehaviour runs the model and returns its verified response claims. It is
// shared by behaviour_test.go (which asserts them) and cmd/gen-claims (which
// renders them), so a claim's statement and its number have one source.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		// A claim set that cannot be measured is not a claim set. Failing here rather
		// than returning a partial one keeps gen-claims from writing a page that
		// silently omits a claim.
		panic("lob: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	binding := claims.Binding{
		TestName: "TestLobGeneratorExpectedBehaviour",
		TestFile: "pkg/lob/behaviour_test.go",
	}

	// Cancellations are drawn at rate cancel_rate per resting unit, so raising the
	// rate must thin the book. This is the stability-relevant direction: the whole
	// point of calibrating a cancellation rate is that it sets how much resting
	// liquidity survives.
	cancelDepths := make([]claims.Observation, 0, 3)
	for _, rate := range []struct {
		label string
		value string
	}{{"at 0.10", "0.10"}, {"at 0.15", "0.15"}, {"at 0.225", "0.225"}} {
		depth, err := meanOver(cfgrun.Subs{
			"cancel_rate: [0.15]": "cancel_rate: [" + rate.value + "]",
		}, colDepthStart)
		if err != nil {
			return nil, err
		}
		cancelDepths = append(cancelDepths,
			claims.Observation{Label: rate.label, Value: depth})
	}

	// The other side of the same balance: more limit arrivals must deepen the book.
	arrivalDepths := make([]claims.Observation, 0, 3)
	for _, rate := range []struct {
		label string
		value string
	}{{"at 0.8", "0.8"}, {"at 1.2", "1.2"}, {"at 1.8", "1.8"}} {
		depth, err := meanOver(cfgrun.Subs{
			"limit_rate: [1.2]": "limit_rate: [" + rate.value + "]",
		}, colDepthStart)
		if err != nil {
			return nil, err
		}
		arrivalDepths = append(arrivalDepths,
			claims.Observation{Label: rate.label, Value: depth})
	}

	// Market orders consume at the touch and nowhere else, so the touch must carry
	// less resting volume than the level behind it. This is a structural claim about
	// the model's mechanism rather than a response to a lever, and it is the one
	// that would catch the ladder wiring being wrong — a bug that would leave both
	// depth claims above still passing.
	touch, err := meanOver(nil, colBidTouch)
	if err != nil {
		return nil, err
	}
	askTouch, err := meanOver(nil, colAskTouch)
	if err != nil {
		return nil, err
	}
	behind, err := meanOver(nil, colBidBehind)
	if err != nil {
		return nil, err
	}
	askBehind, err := meanOver(nil, colAskBehind)
	if err != nil {
		return nil, err
	}

	return []claims.Claim{
		{
			ID: "resting_depth_falls_as_cancellation_rate_rises",
			Statement: "Raising the per-unit cancellation rate thins the book: total " +
				"resting depth falls monotonically as the rate goes from 0.10 to 0.225.",
			Gate:  "1.1",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("mean total resting depth over %d steps after a %d-step "+
				"burn-in, averaged over %d seeds", runSteps-burnIn, burnIn, len(seeds)),
			Limitations: "Says nothing about any real book. The ladder is static with no " +
				"price process, arrivals are uniform across levels, and market orders do " +
				"not walk the book — so this is the response of a deliberately minimal " +
				"generator, not a calibrated market.",
			Monotone:     -1,
			Observations: cancelDepths,
			Binding:      binding,
		},
		{
			ID: "resting_depth_rises_as_limit_arrival_rate_rises",
			Statement: "Raising the per-level limit-order arrival rate deepens the book: " +
				"total resting depth rises monotonically as the rate goes from 0.8 to 1.8.",
			Gate:  "1.1",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("mean total resting depth over %d steps after a %d-step "+
				"burn-in, averaged over %d seeds", runSteps-burnIn, burnIn, len(seeds)),
			Limitations: "As above: a minimal generator with no price process and no " +
				"book-walking. The proportionality to arrival rate is a property of the " +
				"model's independent per-level queues, which real books do not have.",
			Monotone:     1,
			Observations: arrivalDepths,
			Binding:      binding,
		},
		{
			ID: "the_touch_holds_less_resting_volume_than_the_level_behind_it",
			Statement: "Because market orders consume only at the touch, the touch level " +
				"holds less resting volume than the level immediately behind it, on both " +
				"sides of the book.",
			Gate:  "1.1",
			Phase: phase,
			Data:  dataset,
			Unit: fmt.Sprintf("mean resting volume per level over %d steps after a "+
				"%d-step burn-in, averaged over %d seeds and over the two sides",
				runSteps-burnIn, burnIn, len(seeds)),
			Limitations: "This is a consequence of market orders not walking the book, " +
				"which is a simplification, not a finding about real microstructure. Real " +
				"depth profiles are shaped mainly by arrival intensity decaying away from " +
				"the mid, which this model does not have.",
			Monotone: 1,
			Observations: []claims.Observation{
				{Label: "touch", Value: (touch + askTouch) / 2},
				{Label: "level behind", Value: (behind + askBehind) / 2},
			},
			Binding: binding,
		},
	}, nil
}
