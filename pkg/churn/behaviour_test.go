package churn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestChurnPredictions is the binding test named by every claim in behaviour.go —
// one subtest per claim, named by the claim's ID.
func TestChurnPredictions(t *testing.T) {
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

// TestTheBoundsMatchWhatWasPreRegistered is the point of the whole exercise.
//
// A pre-registered threshold that can be edited after seeing the result is not a
// pre-registration. These three numbers were committed in PREREGISTRATION.md before
// cfg/lob_churn.yaml existed, and this test fails if the code drifts from the
// document — in either direction.
func TestTheBoundsMatchWhatWasPreRegistered(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "..", "PREREGISTRATION.md"))
	if err != nil {
		t.Fatalf("reading PREREGISTRATION.md: %v", err)
	}
	text := string(source)
	for _, bound := range []struct {
		spelling string
		what     string
	}{
		{"> +0.5", "prediction A's floor"},
		{"< +0.2", "prediction B's ceiling"},
		{"> 1.5", "prediction C's floor"},
	} {
		if !strings.Contains(text, bound.spelling) {
			t.Errorf("PREREGISTRATION.md no longer states %s as %q — a bound that can be "+
				"edited after seeing the result is not a pre-registration",
				bound.what, bound.spelling)
		}
	}
	if churnFloor != 0.5 || couplingCeiling != 0.2 || dispersionFloor != 1.5 {
		t.Errorf("the scored bounds (%v, %v, %v) have drifted from the pre-registered "+
			"0.5 / 0.2 / 1.5", churnFloor, couplingCeiling, dispersionFloor)
	}
}

// TestTheChurnMechanismIsActuallyShared checks the premise the scoring rests on:
// that one activity draw reaches both streams. If they were drawn separately the
// model would not be a churn model at all, and prediction A would be scoring
// something else entirely.
func TestTheChurnMechanismIsActuallyShared(t *testing.T) {
	source := readConfig(t, "lob_churn.yaml")
	if !strings.Contains(source, "{name: activity, expr: \"shared(gamma(") {
		t.Fatal("the churn model must draw ONE shared activity factor per step")
	}
	for _, driven := range []string{
		"poisson(limit_rate * exp(-arrival_decay * i) * activity * dt)",
		"poisson(churn_rate * exp(-arrival_decay * i) * activity * dt)",
	} {
		if !strings.Contains(source, driven) {
			t.Errorf("both streams must be scaled by the shared activity factor; missing %q",
				driven)
		}
	}
}

// TestTheDepthDependentPathIsStillThere pins the diagnosis for prediction B's
// failure, so that a later change which removes those terms cannot quietly
// invalidate the recorded explanation while leaving the claim in place.
func TestTheDepthDependentPathIsStillThere(t *testing.T) {
	source := readConfig(t, "lob_churn.yaml")
	if !strings.Contains(source, "poisson(cancel_rate * bid * dt)") {
		t.Error("the residual depth-proportional attrition term is gone; prediction B's " +
			"recorded diagnosis names it as a cause and must be re-checked")
	}
	if !strings.Contains(source, "min(bid, churn_bid +") {
		t.Error("the clip is gone; prediction B's recorded diagnosis names it as a cause " +
			"and must be re-checked")
	}
}
