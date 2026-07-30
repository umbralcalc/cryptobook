package baseline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestSyntheticDiagnosticBaseline is the binding test named by every claim in
// behaviour.go — one subtest per claim, named by the claim's ID.
func TestSyntheticDiagnosticBaseline(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestSameMeasurementAsTheRealMarkets is what makes this a control rather than a
// separate analysis that happens to agree.
//
// The whole argument is that the diagnostic detecting a coupling here licenses
// reading its near-zero real-market result as a real absence. That only follows if
// the SAME code produced both numbers.
func TestSameMeasurementAsTheRealMarkets(t *testing.T) {
	for _, path := range []string{"behaviour.go", "../crypto/behaviour.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, call := range []string{"diagnostics.Correlation(", "diagnostics.Dispersion("} {
			if !strings.Contains(string(source), call) {
				t.Errorf("%s does not use %s — the synthetic control and the real-market "+
					"diagnostics must share one measurement or the control proves nothing",
					path, call)
			}
		}
	}
}

// TestTheCouplingIsActuallyInBothModels checks the premise the control rests on.
//
// The claim is "the diagnostic finds a coupling that is there by construction". If
// a model stopped drawing cancellations against resting depth, the diagnostic would
// correctly report no coupling and the control would silently become circular —
// asserting only that the measurement agrees with itself.
func TestTheCouplingIsActuallyInBothModels(t *testing.T) {
	for _, m := range models {
		source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), m.Config))
		if err != nil {
			t.Fatalf("reading %s: %v", m.Config, err)
		}
		text := string(source)
		// Cancellations must be drawn with resting volume inside the Poisson rate.
		if !strings.Contains(text, "poisson(cancel_rate * bid * dt)") &&
			!strings.Contains(text, "poisson(cancel_rate * q * dt)") {
			t.Errorf("%s no longer draws cancellations against resting depth; the "+
				"synthetic control assumes the coupling is present by construction",
				m.Config)
		}
	}
}

// TestArrivalsAndCancellationsAreIndependent pins the other premise: the churn
// claim asserts these models have no churn mechanism, which is only meaningful if
// their two streams really are drawn separately.
func TestArrivalsAndCancellationsAreIndependent(t *testing.T) {
	for _, m := range models {
		source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), m.Config))
		if err != nil {
			t.Fatalf("reading %s: %v", m.Config, err)
		}
		text := string(source)
		arrivals := strings.Contains(text, "poisson(limit_rate") ||
			strings.Contains(text, "poisson(limit_rate * exp(")
		if !arrivals {
			t.Errorf("%s no longer draws arrivals from their own Poisson; if the two "+
				"streams have been coupled, the churn claim must be re-measured rather "+
				"than left asserting independence", m.Config)
		}
	}
}
