package offline

import (
	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOfflineCalibration is the binding test named by every claim in behaviour.go.
func TestOfflineCalibration(t *testing.T) {
	// Skipped under -short, which the nightly -race job uses. This package runs 24 SMC
	// calibrations, and SMC under the race detector is pathologically slow — it times out
	// the per-package limit on a two-core runner. The skip loses no race coverage: this
	// package adds no concurrency of its own (record and calibrate are sequential Go), and
	// the engine's SMC concurrency it exercises is the same code pkg/recovery and
	// pkg/windowing run under -race. The claims themselves are fully verified in the
	// non-race merge gate. See DECISIONS.md, "Splitting -race off the merge gate".
	if testing.Short() {
		t.Skip("skipping the 24-calibration re-measurement under -short (nightly -race)")
	}
	measureDir = t.TempDir()
	persistDir = t.TempDir()
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestTheGeneratorTruthIsWhatTheClaimsAssume pins the driver the whole experiment rests on:
// its dispersion phi is the truth CB is scored against, and the shared draw is the coupling.
// If cfg/lob_churn_flow.yaml changes either, the truth in behaviour.go is stale.
func TestTheGeneratorTruthIsWhatTheClaimsAssume(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), generator))
	if err != nil {
		t.Fatal(err)
	}
	src := string(source)
	// mean 4 (shape/rate) and variance 105 (shape/rate^2) give phi = 6.5625.
	for _, want := range []string{
		"activity_shape: [0.152367]", "activity_rate: [0.038092]",
		"limit_rate: [3.381]", "churn_rate: [1.900]",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("%s no longer contains %q; the truth in behaviour.go assumes it", generator, want)
		}
	}
	// The single shared draw reused across the two scaled streams IS the churn coupling.
	if !strings.Contains(src, "shared(gamma(activity_shape, activity_rate))") {
		t.Errorf("%s no longer draws one shared driver; without it arrivals and cancels do "+
			"not co-move and the model is not a churn model", generator)
	}
}

// TestThePersistentGeneratorIsWhatCHAssumes pins the AR(1) driver CH rests on: its params,
// the recursion on the bare `driver` field (row 0 = previous step), and the shared coupling.
func TestThePersistentGeneratorIsWhatCHAssumes(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_churn_persist.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(source)
	for _, want := range []string{
		"persistence: [0.8]", "activity_shape: [0.152367]", "activity_rate: [0.038092]",
		"limit_rate: [3.381]", "churn_rate: [1.900]",
		"persistence * driver + (1 - persistence) * gamma(activity_shape, activity_rate)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("lob_churn_persist.yaml no longer contains %q; CH's truth phi_marginal "+
				"0.7292 and its AR(1) structure assume it", want)
		}
	}
}
