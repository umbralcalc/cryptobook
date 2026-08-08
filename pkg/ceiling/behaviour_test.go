package ceiling

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestCeilingAccount is the binding test named by every claim in behaviour.go.
func TestCeilingAccount(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestTheTwoRoutesReachAMatchedCeiling is what makes the pair a comparison rather than
// two runs. If the ceilings drift apart, the penalty difference could be a ceiling
// difference and the claim would be measuring nothing.
func TestTheTwoRoutesReachAMatchedCeiling(t *testing.T) {
	m, err := measureAll()
	if err != nil {
		t.Fatal(err)
	}
	burst, counts := m["via variance"].ceiling.Mean, m["via counts"].ceiling.Mean
	if diff := math.Abs(burst - counts); diff > 0.01 {
		t.Errorf("the two routes are at ceilings %.4f and %.4f, %.4f apart — the matched "+
			"pair only isolates the driver's spread while these agree", burst, counts, diff)
	}
}

// TestTheDriverMeanIsFourEverywhere pins the constant the ceiling formula assumes.
// N*Var(A)/(E[A]^2 + N*Var(A)) has E[A] baked in at 4; if a config's driver mean drifted,
// every ceiling in this package would be computed against the wrong denominator and the
// error would look like a change in the saturation penalty.
func TestTheDriverMeanIsFourEverywhere(t *testing.T) {
	for _, r := range routes {
		source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), r.config))
		if err != nil {
			t.Fatalf("reading %s: %v", r.config, err)
		}
		if !strings.Contains(string(source), "act_ref: [4.0]") {
			t.Errorf("%s no longer declares act_ref 4.0; the ceiling formula in this "+
				"package assumes a driver mean of 4", r.config)
		}
	}
}

// TestTheCountsModelIsFrozen guards the model the out-of-sample test BS-BU was scored
// against. That test is only out-of-sample while these values are the ones that were
// shipped before the 2026-08-08 window was recorded; if they change, the result must be
// withdrawn rather than re-scored, and this says so at the point of breakage.
func TestTheCountsModelIsFrozen(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_counts.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pinned := range []string{
		"limit_rate: [3.381]", "churn_rate: [1.900]", "damping_gamma: [0.45]",
		"activity_shape: [0.152367]", "activity_rate: [0.038092]",
	} {
		if !strings.Contains(string(source), pinned) {
			t.Errorf("cfg/lob_counts.yaml no longer has %s. If this changed after the "+
				"2026-08-08 recording, the BS-BU out-of-sample result is void and must be "+
				"WITHDRAWN rather than re-scored.", pinned)
		}
	}
}
