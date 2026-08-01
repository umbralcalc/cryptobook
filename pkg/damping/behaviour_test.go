package damping

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestDampingCalibration is the binding test named by every claim in behaviour.go.
func TestDampingCalibration(t *testing.T) {
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

// TestGammaOneReproducesThePersistentModel is the control that makes the whole sweep
// legitimate. gamma = 1 must be cfg/lob_persistent.yaml exactly, or the
// reparameterisation changed the model rather than only its spelling — and the sweep
// would then be exploring a different family from the one whose result motivated it.
func TestGammaOneReproducesThePersistentModel(t *testing.T) {
	p, err := measureAt("1.0", "1.05")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		label     string
		got, want float64
	}{
		{"depth vs arrivals", p.depthArrival, -0.4172},
		{"depth vs cancellations", p.coupling, -0.2856},
		{"arrivals vs cancellations", p.coMovement, 0.8218},
	} {
		if diff := c.got - c.want; diff > 1e-3 || diff < -1e-3 {
			t.Errorf("%s at gamma=1: got %.4f, cfg/lob_persistent.yaml gives %.4f — the "+
				"pow() reparameterisation is not exact", c.label, c.got, c.want)
		}
	}
}

// TestTheSelectionRuleIsTheOnePreRegistered re-derives the choice mechanically rather
// than trusting the constant. If the rule and the shipped gamma ever disagree, the
// fitted parameter was chosen by something other than the stated rule — which is the
// single thing that would turn this calibration back into a fit.
func TestTheSelectionRuleIsTheOnePreRegistered(t *testing.T) {
	all, _, err := measureAll()
	if err != nil {
		t.Fatal(err)
	}
	best, bestGamma := 1e9, ""
	for i, g := range sweep {
		d := all[i].depthArrival - fitTarget
		if d < 0 {
			d = -d
		}
		// Ties go to the LARGER gamma; the grid is ascending, so >= takes the later one.
		if d <= best {
			best, bestGamma = d, g.gamma
		}
	}
	if bestGamma != selectedGamma {
		t.Errorf("the pre-registered rule selects gamma=%s, but the package ships %s",
			bestGamma, selectedGamma)
	}
	if best > fitTolerance {
		t.Errorf("closest grid point is %.4f from the target, beyond the %.2f at which "+
			"the pre-registration declares the fit to have failed outright", best, fitTolerance)
	}
	if !strings.Contains(readConfig(t), "damping_gamma: ["+selectedGamma+"]") {
		t.Errorf("%s does not ship damping_gamma %s", configName, selectedGamma)
	}
}

// TestOnlyTheDampingExponentDistinguishesThisFromPersistent pins what was allowed to
// move. gamma is the fitted parameter and churn_rate is the depth control; everything
// else must be inherited, or the calibration has more free parameters than it declares.
func TestOnlyTheDampingExponentDistinguishesThisFromPersistent(t *testing.T) {
	source := readConfig(t)
	previous, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_persistent.yaml"))
	if err != nil {
		t.Fatalf("reading lob_persistent.yaml: %v", err)
	}
	for _, inherited := range []string{
		"limit_rate: [2.0]", "arrival_decay: [0.35]", "arrival_scale: [19.0]",
		"cancel_rate: [0.0]", "market_rate: [1.2]", "persistence: [0.8]",
		"activity_shape: [0.222222]", "activity_rate: [0.0555556]", "act_ref: [4.0]",
	} {
		if !strings.Contains(source, inherited) {
			t.Errorf("%s must inherit %s unchanged", configName, inherited)
		}
		if !strings.Contains(string(previous), inherited) {
			t.Errorf("lob_persistent.yaml no longer has %s — the two models have diverged "+
				"on something other than the damping exponent", inherited)
		}
	}
	if !strings.Contains(source, "pow(activity / act_ref, damping_gamma)") {
		t.Errorf("%s must express the damping as (act/act_ref)^gamma", configName)
	}
}
