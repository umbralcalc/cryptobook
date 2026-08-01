package oos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestOutOfSampleWindow is the binding test named by every claim in behaviour.go.
//
// It skips without the fresh segments. They are Binance data, which the licence does not
// permit redistributing, so they are git-ignored — which is why this package is NOT
// registered in internal/claimset and why these numbers live in DECISIONS.md instead of
// CLAIMS.md. Anyone can record their own window and re-run this.
func TestOutOfSampleWindow(t *testing.T) {
	if !Available() {
		t.Skip("fresh segments absent (not redistributable); record a window with:\n" +
			"  for s in BTCUSDT ETHUSDT SOLUSDT XRPUSDT DOGEUSDT; do \\\n" +
			"    go run ./cmd/record-feed -symbol $s -duration 8m -out dat/oos_$s.log & \\\n" +
			"  done; wait\n" +
			"NOTE: a different window will give different numbers — that is the finding.")
	}
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestTheModelIsFrozen is the guard that makes this test out-of-sample at all. If the
// damping exponent were ever re-fitted after this window was seen, the only out-of-sample
// evidence this project has would be destroyed and nothing would say so.
func TestTheModelIsFrozen(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), dampingConfig))
	if err != nil {
		t.Fatalf("reading %s: %v", dampingConfig, err)
	}
	for _, pinned := range []string{"damping_gamma: [0.6]", "churn_rate: [1.075]"} {
		if !strings.Contains(string(source), pinned) {
			t.Errorf("%s no longer has %s. If this changed because the model was re-fitted "+
				"after the 2026-08-01 window was recorded, the out-of-sample test is void "+
				"and AH-AK must be withdrawn rather than re-scored.", dampingConfig, pinned)
		}
	}
}

// TestTheWeakGateIsReported guards the honesty device rather than a number: a fresh
// window that had not moved would make AH-AK vacuous, and the gate has to be evaluated
// and reported whichever way it falls.
func TestTheWeakGateIsReported(t *testing.T) {
	if !Available() {
		t.Skip("fresh segments absent")
	}
	r, err := measure()
	if err != nil {
		t.Fatal(err)
	}
	if r.Weak() {
		t.Errorf("the market moved only %.4f between windows, under the pre-registered "+
			"%.2f gate — AH-AK carry no verdict and must be recorded WEAK BY "+
			"CONSTRUCTION rather than scored", r.driftArrival, weakGate)
	}
}
