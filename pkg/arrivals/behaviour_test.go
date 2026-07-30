package arrivals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestDepthDependentArrivals is the binding test named by every claim in
// behaviour.go — one subtest per claim, named by the claim's ID.
func TestDepthDependentArrivals(t *testing.T) {
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

// TestCancellationHasNoDepthTerm guards the premise the whole result rests on.
//
// Prediction H's content is that depth was stabilised WITHOUT a depth-proportional
// cancellation term. If one crept back in — even at a small rate — the claim would
// become circular and its number would no longer mean what it says.
func TestCancellationHasNoDepthTerm(t *testing.T) {
	source := readConfig(t)
	if !strings.Contains(source, "cancel_rate: [0.0]") {
		t.Error("cancel_rate must be exactly zero: prediction H asserts the coupling " +
			"stays broken with NO depth-proportional cancellation, and any non-zero " +
			"rate reintroduces exactly the term the result is about")
	}
	// The churn term must still be activity-driven rather than depth-driven.
	if !strings.Contains(source, "poisson(churn_rate * exp(-arrival_decay * i) * activity * dt)") {
		t.Error("the churn term is no longer purely activity-driven; if cancellation " +
			"has gained any depth dependence the J claim must be re-measured")
	}
}

// TestTheStabiliserIsOnTheArrivalSide pins the mechanism itself: the damping must be
// per level and on arrivals, which is what distinguishes this model from the
// attrition one it replaces.
func TestTheStabiliserIsOnTheArrivalSide(t *testing.T) {
	source := readConfig(t)
	for _, side := range []string{"slice(bid, i, 1)", "slice(ask, i, 1)"} {
		if !strings.Contains(source, side) {
			t.Errorf("arrival damping no longer reads %s — the stabiliser must be per "+
				"level, since a trader joins the queue in front of them rather than "+
				"responding to total book depth", side)
		}
	}
	if !strings.Contains(source, "arrival_scale") {
		t.Error("the arrival damping parameter is gone; without it there is no stabiliser")
	}
}

// TestTheParametersAreArbitrary pins the values the G/H/I scores were measured at, in
// the place someone changing them would look.
//
// They are chosen, not fitted. Nothing in this package is calibrated against market
// data, and a comment claiming otherwise would make the claims say more than they can.
func TestTheParametersAreArbitrary(t *testing.T) {
	source := readConfig(t)
	if !strings.Contains(source, "arrival_scale: [19.0]") {
		t.Error("arrival_scale 19.0 is the arbitrary value predictions E, F and G were " +
			"scored against. Changing it invalidates those scores, and fitting it to " +
			"anything would need its own pre-registration")
	}
	if strings.Contains(source, "fitted to mean depth") ||
		strings.Contains(source, "fitted to a market") {
		t.Error("the config describes a parameter as fitted to market data. Nothing in " +
			"this package is: the values are chosen, and saying otherwise would make " +
			"the claims assert a calibration that was never done")
	}
}
