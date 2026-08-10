package stability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestDepthRecoveryExpectedBehaviour is the binding test named by every claim in
// behaviour.go — one subtest per claim, named by the claim's ID.
func TestDepthRecoveryExpectedBehaviour(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func readConfig(t *testing.T, name string) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(source)
}

// TestShockConfigMatchesTheGenerator guards what makes the recovery measurement a
// statement about the model rather than about a second, subtly different model.
//
// cfg/lob_depth_recovery.yaml must differ from cfg/lob_generator.yaml in the shock
// and nothing else. If a rate or a binding drifted between them, the recovery
// numbers would describe dynamics the rest of the repo never validated.
func TestShockConfigMatchesTheGenerator(t *testing.T) {
	shock := readConfig(t, configName)
	base := readConfig(t, "lob_generator.yaml")
	for _, shared := range []string{
		"limit_rate: [1.2]", "cancel_rate: [0.15]", "market_rate: [0.8]",
		`{name: arrivals, expr: "iid(6, poisson(limit_rate * dt))"}`,
		`{name: cancels, expr: "min(q, poisson(cancel_rate * q * dt))"}`,
		`{name: takes, expr: "min(resting, mkt_bid * touch_bid + mkt_ask * touch_ask)"}`,
		"init_state_values: [8.0, 8.0, 8.0, 8.0, 8.0, 8.0, 0.0, 0.0, 0.0, 48.0]",
	} {
		if !strings.Contains(shock, shared) {
			t.Errorf("%s has drifted from the generator: missing %q", configName, shared)
		}
		if !strings.Contains(base, shared) {
			t.Errorf("lob_generator.yaml is missing %q", shared)
		}
	}
	// ...and the shock itself must be present, or the config is just the generator
	// and "recovery time" would be measuring nothing at all.
	if !strings.Contains(shock, "where(step == shock_step") {
		t.Error("the shock config has no shock; recovery time would be meaningless")
	}
}

// TestShockConstantsMatchTheConfig pins the step and thresholds this package reads
// against the config that produces them. A shock at a different step than the one
// the measurement slices around would silently report the wrong recovery time.
func TestShockConstantsMatchTheConfig(t *testing.T) {
	source := readConfig(t, configName)
	if !strings.Contains(source, "shock_step: [200.0]") {
		t.Errorf("shockStep = %d assumes the config shocks at step 200", shockStep)
	}
	if !strings.Contains(source, "survive_fraction: [0.1]") {
		t.Error("the claims describe a 90% liquidity removal; the config must match")
	}
	if baselineTo > shockStep {
		t.Errorf("the baseline window [%d,%d) must end before the shock at %d, or the "+
			"pre-shock equilibrium is contaminated by the event itself",
			baselineFrom, baselineTo, shockStep)
	}
}

// TestUnanswerableOutputsAreNotFaked is the audit's teeth.
//
// there are four stability outputs and says unanswerable ones must be MARKED AS
// SUCH rather than approximated. Three are structurally impossible here, so nothing
// in this package may quietly start reporting them — and the reason each is
// impossible is checkable against the model itself rather than taken on trust.
func TestUnanswerableOutputsAreNotFaked(t *testing.T) {
	generator := readConfig(t, "lob_generator.yaml")

	t.Run("the model has no prices, so no spread output can exist", func(t *testing.T) {
		// The ladder is addressed by 0/1 masks, not price levels.
		if !strings.Contains(generator, "touch_bid: [1.0, 0.0, 0.0, 0.0, 0.0, 0.0]") {
			t.Fatal("the ladder is no longer mask-addressed; re-audit whether a spread " +
				"output has become possible")
		}
		for _, priceish := range []string{"tick_size", "mid_price", "price:"} {
			if strings.Contains(generator, priceish) {
				t.Errorf("the generator now mentions %q — if the model has gained prices, "+
					"the spread and queue-position outputs must be re-audited", priceish)
			}
		}
	})

	t.Run("market orders cannot sweep, so no survival-fraction output can exist", func(t *testing.T) {
		// Consumption is clipped to the touch, so a "large" marketable order removes
		// at most what is resting there.
		if !strings.Contains(generator,
			`{name: takes, expr: "min(resting, mkt_bid * touch_bid + mkt_ask * touch_ask)"}`) {
			t.Error("market-order consumption has changed; re-audit whether a " +
				"liquidity-survival output has become possible")
		}
	})

	t.Run("only the answerable output is claimed", func(t *testing.T) {
		for _, claim := range ObservedBehaviour() {
			for _, forbidden := range []string{"spread", "queue_position", "tick_regime"} {
				if strings.Contains(claim.ID, forbidden) {
					t.Errorf("claim %q reports %q, which this model cannot support",
						claim.ID, forbidden)
				}
			}
		}
	})
}
