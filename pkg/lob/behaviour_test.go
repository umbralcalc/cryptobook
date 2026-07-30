package lob

import (
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestLobGeneratorExpectedBehaviour is the binding test named by every claim in
// behaviour.go. One subtest per claim, named by the claim's ID, so a failure names
// the behaviour that broke rather than a line number.
func TestLobGeneratorExpectedBehaviour(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestGeneratorMatchesItsIntensityModel checks the generator's own moments against
// the closed-form intensities that cfg/lob_recovery.yaml scores against.
//
// This is the test that makes Spike 1.2 interpretable. Without it, a recovery
// failure has two candidate causes — the sampler, or an intensity model that does
// not describe the generator — and no way to tell them apart. Pinning the moments
// here rules out the second, so pkg/recovery's results are about inference alone.
func TestGeneratorMatchesItsIntensityModel(t *testing.T) {
	const (
		limitRate  = 1.2
		cancelRate = 0.15
		marketRate = 0.8
		numLevels  = 6.0

		colLimit  = 6
		colCancel = 7
		colMarket = 8

		// 3% covers Monte Carlo error over this many steps and seeds, and is far
		// tighter than any mismatch that would matter — a wrong intensity model would
		// be out by tens of percent, not by three.
		tolerance = 0.03
	)

	depth, err := meanOver(nil, colDepthStart)
	if err != nil {
		t.Fatalf("measuring depth: %v", err)
	}

	for _, check := range []struct {
		name   string
		column int
		expect float64
		// why records what would be wrong if this one failed.
		why string
	}{
		{
			name:   "limit arrivals match limit_rate x levels",
			column: colLimit,
			expect: limitRate * numLevels,
			why:    "arrivals are not one independent Poisson stream per level",
		},
		{
			name:   "cancellations match cancel_rate x resting depth",
			column: colCancel,
			expect: cancelRate * depth,
			why: "cancellations are not proportional to resting depth — which is the " +
				"coupling cancel_rate's identifiability depends on entirely",
		},
		{
			name:   "market arrivals match market_rate",
			column: colMarket,
			expect: marketRate,
			why:    "market-order arrivals are not Poisson at market_rate",
		},
	} {
		t.Run(check.name, func(t *testing.T) {
			got, err := meanOver(nil, check.column)
			if err != nil {
				t.Fatalf("measuring column %d: %v", check.column, err)
			}
			relative := (got - check.expect) / check.expect
			if relative < -tolerance || relative > tolerance {
				t.Errorf(
					"mean = %.4f, expected %.4f (%.1f%% off, tolerance %.0f%%); "+
						"this would mean %s",
					got, check.expect, relative*100, tolerance*100, check.why)
			}
		})
	}

	// Note on the cancellation check above: min(q, poisson(...)) can only clip
	// downwards, so the measured mean is biased low relative to cancel_rate x depth.
	// At these parameters the bias is far below the Monte Carlo error — stationary
	// depth is ~8 per level, so a level is almost never thin enough to clip — which
	// is why a two-sided tolerance is the right test here. It would NOT be at a much
	// higher cancel_rate or a much lower limit_rate, where clipping starts to bite;
	// if this test is ever retuned to such a regime it needs a one-sided bound.
}

// TestSubstitutionIsNotSilent guards the harness property the behaviour claims
// depend on: a substitution whose target is absent must fail, not quietly measure
// the unmodified config. Without this, a reformatted config would leave every sweep
// above measuring the same parameter value — and all of them would still pass,
// because a flat response is monotone in neither direction only if the values are
// actually equal, which floating-point means they would be.
func TestSubstitutionIsNotSilent(t *testing.T) {
	_, err := cfgrun.Run(configName, cfgrun.Subs{
		"this_key_does_not_exist: [0.0]": "whatever: [1.0]",
	})
	if err == nil {
		t.Fatal("expected a missing substitution target to be an error")
	}
}
