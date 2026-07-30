package windowing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestWindowLengthExpectedBehaviour is the binding test named by every claim in
// behaviour.go — one subtest per claim, named by the claim's ID.
func TestWindowLengthExpectedBehaviour(t *testing.T) {
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

// TestWindowingTruthMatchesTheConfig pins this package's truth to the generator's,
// since every sigma above is measured against it.
func TestWindowingTruthMatchesTheConfig(t *testing.T) {
	source := readConfig(t, generatorConfig)
	for _, spelling := range []string{
		fmt.Sprintf("limit_rate: [%g]", trueRates[0]),
		fmt.Sprintf("cancel_rate: [%g]", trueRates[1]),
		fmt.Sprintf("market_rate: [%g]", trueRates[2]),
	} {
		if !strings.Contains(source, spelling) {
			t.Errorf("%s does not contain %q", generatorConfig, spelling)
		}
	}
}

// TestCalibrationConfigReadsARecordedSegment guards what makes this evidence for
// Gate 3.4 rather than a repeat of pkg/recovery.
//
// The whole argument is that the engine reads a FINISHED dataset through the
// ordinary data: tier and never sees a stream. If this config grew its own
// generating sub-simulation, it would stop demonstrating the recorded-segment path
// and the throughput and window-length numbers would describe something else.
func TestCalibrationConfigReadsARecordedSegment(t *testing.T) {
	source := readConfig(t, calibrationConfig)
	if !strings.Contains(source, "source:") ||
		!strings.Contains(source, "json_log: {path:") {
		t.Error("the calibration config must read its data through a data: source")
	}
	// Matched as whole lines at the data: tier's indentation, not as substrings: a
	// bare Contains for "steps:" also fires on the model's burn_in_steps, which
	// would fail this test for a config that is entirely correct.
	for _, forbidden := range []string{"expressions:", "partitions:", "steps:"} {
		if hasTopLevelDataKey(source, forbidden) {
			t.Errorf(
				"the calibration config declares %q in its data: tier — it must read a "+
					"recorded segment, not generate its own flow, or it stops being "+
					"evidence that the engine only ever sees a finished dataset", forbidden)
		}
	}
}

// TestCalibrationMatchesTheSmcConfig keeps the window-length numbers comparable to
// pkg/recovery's accuracy numbers. If the samplers diverged, the claims here would
// describe a different estimator than the one the project selected at Gate 1.2.
func TestCalibrationMatchesTheSmcConfig(t *testing.T) {
	fromLog := readConfig(t, calibrationConfig)
	smc := readConfig(t, "lob_recovery_smc.yaml")
	for _, shared := range []string{
		"num_particles: 160",
		"num_rounds: 5",
		"{type: uniform, lo: 0.1,  hi: 3.0}",
		"{type: uniform, lo: 0.01, hi: 0.5}",
		"{type: uniform, lo: 0.05, hi: 2.5}",
		"latest_data_values: {upstream: lob_flow_observed, indices: [6, 7, 8]}",
		"rates[1] * observed[9] * dt",
	} {
		if !strings.Contains(fromLog, shared) {
			t.Errorf("%s is missing %q", calibrationConfig, shared)
		}
		if !strings.Contains(smc, shared) {
			t.Errorf("lob_recovery_smc.yaml is missing %q", shared)
		}
	}
}

// hasTopLevelDataKey reports whether the config declares the given key at the
// data: tier's own indentation (two spaces), which is where a generating
// sub-simulation would be stated.
func hasTopLevelDataKey(source, key string) bool {
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(line, "  "+key) {
			return true
		}
	}
	return false
}
