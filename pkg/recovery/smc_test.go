package recovery

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestSmcRecoveryExpectedBehaviour is the binding test named by every claim in
// smc.go — one subtest per claim, named by the claim's ID.
func TestSmcRecoveryExpectedBehaviour(t *testing.T) {
	set, err := smcBehaviour()
	if err != nil {
		t.Fatalf("measuring SMC behaviour: %v", err)
	}
	for _, claim := range set {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestSmcConstantsMatchTheConfig pins this package's round and particle counts to
// the config's. They appear in claim statements and units, and the round-count
// guard compares against them, so a config edit that left them behind would leave
// every SMC claim describing a run that never happened.
func TestSmcConstantsMatchTheConfig(t *testing.T) {
	source := readConfig(t, smcConfig)
	for _, spelling := range []string{
		fmt.Sprintf("num_rounds: %d", smcRounds),
		fmt.Sprintf("num_particles: %d", smcParticles),
	} {
		if !strings.Contains(source, spelling) {
			t.Errorf("%s does not contain %q", smcConfig, spelling)
		}
	}
	// The raised count must NOT be what the config ships, or the calibration guard
	// would be comparing a value against itself and could never fail.
	if smcRaisedRounds == smcRounds {
		t.Error("smcRaisedRounds must differ from smcRounds for the guard to mean anything")
	}
}

// TestSmcComparesLikeForLike guards what makes the two samplers comparable at all.
//
// The SMC claims are only evidence for a switch if both configs generate the same
// data from the same truth and score it with the same intensity model. If the SMC
// config drifted to an easier model — different rates, a different coupling, more
// data — it would look better for reasons that have nothing to do with the sampler.
func TestSmcComparesLikeForLike(t *testing.T) {
	smc := readConfig(t, smcConfig)
	other := readConfig(t, recoveryConfig)

	t.Run("same truth", func(t *testing.T) {
		for i, spelling := range []string{
			fmt.Sprintf("limit_rate: [%g]", trueRates[0]),
			fmt.Sprintf("cancel_rate: [%g]", trueRates[1]),
			fmt.Sprintf("market_rate: [%g]", trueRates[2]),
		} {
			if !strings.Contains(smc, spelling) {
				t.Errorf("%s does not generate with %q (%s)",
					smcConfig, spelling, rateNames[i])
			}
		}
	})

	t.Run("same generator expression", func(t *testing.T) {
		// The bindings are the model. Comparing them verbatim is cruder than parsing
		// but catches the thing that matters: a divergence in the dynamics being
		// inferred.
		for _, binding := range []string{
			`{name: arrivals, expr: "iid(6, poisson(limit_rate * dt))"}`,
			`{name: cancels, expr: "min(q, poisson(cancel_rate * q * dt))"}`,
			`{name: mkt, expr: "shared(poisson(market_rate * dt))"}`,
			`{name: takes, expr: "min(resting, mkt_bid * touch_bid + mkt_ask * touch_ask)"}`,
		} {
			if !strings.Contains(smc, binding) {
				t.Errorf("%s is missing generator binding %s", smcConfig, binding)
			}
			if !strings.Contains(other, binding) {
				t.Errorf("%s is missing generator binding %s", recoveryConfig, binding)
			}
		}
	})

	t.Run("same intensity model and same scored counts", func(t *testing.T) {
		// Both must score exactly n_limit, n_cancel and n_market...
		if !strings.Contains(smc, "indices: [6, 7, 8]") {
			t.Errorf("%s does not score the three counts at indices [6, 7, 8]", smcConfig)
		}
		if !strings.Contains(other, "value_indices: [6, 7, 8]") {
			t.Errorf("%s does not score the three counts at indices [6, 7, 8]", recoveryConfig)
		}
		// ...and both must read queue depth at index 9 for the cancellation intensity,
		// which is the coupling the whole spike turns on. The param name differs
		// (`rates` vs `sampled`) because one is forwarded per particle and the other
		// injected from a sampler, so only the depth term is compared.
		for _, name := range []string{smcConfig, recoveryConfig} {
			source := smc
			if name == recoveryConfig {
				source = other
			}
			if !strings.Contains(source, "observed[9] * dt") {
				t.Errorf(
					"%s does not couple the cancellation intensity to observed[9] "+
						"(queue depth); without that the spike's central claim is untested",
					name)
			}
		}
	})

	t.Run("SMC is not given more data", func(t *testing.T) {
		// A larger dataset would improve SMC's numbers for reasons unrelated to the
		// sampler. It has fewer steps here (400 vs 2000), so if this ever flips the
		// comparison has stopped being conservative.
		// Compared as integers, not strings: "400" > "2000" lexicographically, which
		// would make this guard pass in exactly the case it exists to catch.
		smcSteps, err := intAfter(smc, "steps: ")
		if err != nil {
			t.Fatalf("finding SMC steps: %v", err)
		}
		otherSteps, err := intAfter(other, "steps: ")
		if err != nil {
			t.Fatalf("finding steps: %v", err)
		}
		if smcSteps > otherSteps {
			t.Errorf(
				"SMC uses %d data steps against %d — it must not be handed more data "+
					"than the sampler it is being compared with",
				smcSteps, otherSteps)
		}
	})
}

// intAfter returns the integer following the first occurrence of a key.
func intAfter(source, key string) (int, error) {
	raw, err := scalarAfter(source, key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(raw)
}
