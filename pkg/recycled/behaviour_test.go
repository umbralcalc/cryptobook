package recycled

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestDepthNeutralChurn is the binding test named by every claim in behaviour.go —
// one subtest per claim, named by the claim's ID.
func TestDepthNeutralChurn(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func readConfig(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), configName))
	if err != nil {
		t.Fatalf("reading %s: %v", configName, err)
	}
	return string(source)
}

// TestCancellationHasNoDepthTerm guards what makes T meaningful at all. The whole
// point of the mechanism is that cancellation is depth-NEUTRAL: any correlation it
// picks up must arrive indirectly through last step's arrivals. A depth-proportional
// attrition term creeping back would produce the same correlation for a completely
// different and uninteresting reason, and T would silently stop testing anything.
func TestCancellationHasNoDepthTerm(t *testing.T) {
	source := readConfig(t)
	if !strings.Contains(source, "cancel_rate: [0.0]") {
		t.Error("cancel_rate must stay a named zero — a depth-proportional " +
			"cancellation term would make T pass for the wrong reason")
	}
	for _, forbidden := range []string{
		"poisson(cancel_rate * bid * dt) +", "churn_rate * bid", "churn_rate * ask",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("%s contains %q — cancellation must not scale with resting volume",
				configName, forbidden)
		}
	}
}

// TestTheRecycledTermReadsExactlyOneStepBack pins the mechanism to the lag that
// defines it, and guards a mistake that was actually made here.
//
// A BARE field name is already row 0 — the previous committed step — so `posted_bid` is
// arr(t-1) and lag(posted_bid, 1) is arr(t-2). The first version of this config used
// the lag form and therefore ran a two-step recycling model against a pre-registration
// that said one step. Same-step recycling (which no spelling here produces, since a
// binding cannot see itself) would be a different mechanism again, and would trivially
// inflate the co-movement V measures.
func TestTheRecycledTermReadsExactlyOneStepBack(t *testing.T) {
	source := readConfig(t)
	for _, required := range []string{
		`{name: recycled_bid, expr: "recycle * posted_bid"}`,
		`{name: recycled_ask, expr: "recycle * posted_ask"}`,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("%s is missing %s", configName, required)
		}
	}
	if strings.Contains(source, `expr: "recycle * lag(posted_`) {
		t.Errorf("%s reads the posted volumes through lag(), which reaches TWO steps "+
			"back and is not the pre-registered mechanism", configName)
	}
}

// TestTheArrivalSideIsInheritedUnchanged is what makes U's ordering a real comparison.
// If the arrival damping drifted from cfg/lob_arrivals.yaml, the two models would
// differ on both sides at once and neither the ordering nor the -0.116 baseline this
// package is measured against would mean anything.
func TestTheArrivalSideIsInheritedUnchanged(t *testing.T) {
	source := readConfig(t)
	previous, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_arrivals.yaml"))
	if err != nil {
		t.Fatalf("reading lob_arrivals.yaml: %v", err)
	}
	for _, binding := range []string{
		`{name: arr_bid, expr: "each(8, i, poisson(limit_rate * exp(-arrival_decay * i) * activity * dt / (1 + slice(bid, i, 1) / arrival_scale)))"}`,
		`{name: arr_ask, expr: "each(8, i, poisson(limit_rate * exp(-arrival_decay * i) * activity * dt / (1 + slice(ask, i, 1) / arrival_scale)))"}`,
		"limit_rate: [2.0]",
		"arrival_decay: [0.35]",
		"arrival_scale: [19.0]",
	} {
		if !strings.Contains(source, binding) {
			t.Errorf("%s is missing inherited arrival-side element %s", configName, binding)
		}
		if !strings.Contains(string(previous), binding) {
			t.Errorf("lob_arrivals.yaml no longer contains %s — the two models have "+
				"diverged on the arrival side and U's ordering is no longer like-for-like",
				binding)
		}
	}
}

// TestOnlyChurnRateMoved pins the single permitted adjustment. The pre-registration
// allows churn_rate to be re-set once on mean depth; anything else moving would mean a
// parameter was changed after the predictions were fixed.
func TestOnlyChurnRateMoved(t *testing.T) {
	source := readConfig(t)
	if !strings.Contains(source, "churn_rate: [0.55]") {
		t.Error("churn_rate must be 0.55 — the value the depth sweep selected and the " +
			"scores were measured at")
	}
	if !strings.Contains(source, "recycle: [0.5]") {
		t.Error("recycle must stay at its pre-registered 0.5; moving it after seeing T " +
			"would need a fresh pre-registration")
	}
}
