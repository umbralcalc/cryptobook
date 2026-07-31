package persistent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestPersistentDriver is the binding test named by every claim in behaviour.go — one
// subtest per claim, named by the claim's ID.
func TestPersistentDriver(t *testing.T) {
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

// TestTheDriverReadsExactlyOneStepBack guards the error that was actually made in
// cfg/lob_churn_recycled.yaml and caught here. A BARE field name is already row 0 — the
// previous committed step — so lag(prev_activity, 1) reaches TWO steps back and builds
// an AR(2) whose lag-1 autocorrelation is zero. The marginal moments come out right
// either way, so this does not show up in a mean or a variance check.
func TestTheDriverReadsExactlyOneStepBack(t *testing.T) {
	source := readConfig(t)
	if !strings.Contains(source, "persistence * prev_activity + (1 - persistence)") {
		t.Error("the driver must read prev_activity BARE; anything else is not AR(1)")
	}
	if strings.Contains(source, "lag(prev_activity") {
		t.Error("lag(prev_activity, ...) reaches two steps back and makes this an AR(2) " +
			"with a zero lag-1 autocorrelation")
	}
}

// TestTheDriverMarginalIsHeldFixed is the control that makes X interpretable. If the
// innovation moments drifted, the driver would differ from the incumbent's in mean or
// spread as well as in autocorrelation, and any measured effect could be attributed to
// a busier or burstier market rather than to persistence.
//
// act(t) = p*act(t-1) + (1-p)*innovation has stationary mean = innovation mean and
// stationary variance = (1-p)/(1+p) * Var(innovation). At p = 0.8 that is Var/9, so
// matching the incumbent's gamma(2, 0.5) — mean 4, variance 8 — needs an innovation of
// mean 4 and variance 72, i.e. gamma(4/18, 1/18).
func TestTheDriverMarginalIsHeldFixed(t *testing.T) {
	source := readConfig(t)
	for _, required := range []string{
		"persistence: [0.8]",
		"activity_shape: [0.222222]",
		"activity_rate: [0.0555556]",
		"act_ref: [4.0]",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("%s is missing %s — the driver's marginal is no longer pinned to "+
				"the incumbent's and X stops being attributable to persistence",
				configName, required)
		}
	}
	previous, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_arrivals.yaml"))
	if err != nil {
		t.Fatalf("reading lob_arrivals.yaml: %v", err)
	}
	for _, incumbent := range []string{"activity_shape: [2.0]", "activity_rate: [0.5]"} {
		if !strings.Contains(string(previous), incumbent) {
			t.Errorf("lob_arrivals.yaml no longer has %s — the moment-matching above "+
				"targets a driver that has moved", incumbent)
		}
	}
}

// TestTheHomogeneityIsBroken pins the change that lets depth respond to the driver at
// all. If the damping lost its activity term, the system would be homogeneous in
// activity — which would cancel out of the equilibrium entirely — and X would be
// measuring a variance effect rather than a mean one.
func TestTheHomogeneityIsBroken(t *testing.T) {
	source := readConfig(t)
	for _, required := range []string{
		"slice(bid, i, 1) * activity / (arrival_scale * act_ref)",
		"slice(ask, i, 1) * activity / (arrival_scale * act_ref)",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("%s is missing %s — without the activity term in the damping the "+
				"model is homogeneous in activity and depth cannot respond to it",
				configName, required)
		}
	}
	// Marketable flow must stay UNSCALED. Scaling it would remove the competing effect
	// declared before running, and X would stop being the test it was registered as.
	if !strings.Contains(source, `{name: mkt, expr: "shared(poisson(market_rate * dt))"}`) {
		t.Error("marketable flow must stay unscaled by activity — the pre-registration " +
			"declares the constant-consumption drag as the effect competing with X")
	}
}

// TestOnlyChurnRateMoved pins the single permitted adjustment.
func TestOnlyChurnRateMoved(t *testing.T) {
	source := readConfig(t)
	if !strings.Contains(source, "churn_rate: [1.05]") {
		t.Error("churn_rate must be 1.05 — the value the mean-depth sweep selected and " +
			"the scores were measured at")
	}
	for _, inherited := range []string{
		"limit_rate: [2.0]", "arrival_decay: [0.35]", "arrival_scale: [19.0]",
		"cancel_rate: [0.0]", "market_rate: [1.2]",
	} {
		if !strings.Contains(source, inherited) {
			t.Errorf("%s must inherit %s unchanged; churn_rate is the only value the "+
				"pre-registration permits to move", configName, inherited)
		}
	}
}
