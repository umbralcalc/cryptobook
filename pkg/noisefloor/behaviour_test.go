package noisefloor

import (
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestNoiseFloor is the binding test named by every claim in behaviour.go.
//
// It skips without the six windows. They are Binance data, which the licence does not
// permit redistributing, so they are git-ignored — which is why this package is NOT
// registered in internal/claimset and why these numbers live in DECISIONS.md.
func TestNoiseFloor(t *testing.T) {
	if !Available() {
		t.Skip("windows absent (not redistributable); record five with:\n" +
			"  for w in 1 2 3 4 5; do\n" +
			"    for s in BTCUSDT ETHUSDT SOLUSDT XRPUSDT DOGEUSDT; do \\\n" +
			"      go run ./cmd/record-feed -symbol $s -duration 8m -out dat/nf${w}_$s.log & \\\n" +
			"    done; wait; sleep 120\n" +
			"  done\n" +
			"plus the dat/oos_*.log window. Different windows give different numbers — " +
			"measuring that is the point.")
	}
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestAllSixWindowsAreUsed guards the range these predictions are scored on. A partial
// set would silently narrow it — and a narrower range makes AL and AM easier to pass,
// which is the direction that flatters the result.
func TestAllSixWindowsAreUsed(t *testing.T) {
	if !Available() {
		t.Skip("windows absent")
	}
	r, err := measure()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.perWindow) != len(windows) {
		t.Errorf("scored %d windows, expected %d", len(r.perWindow), len(windows))
	}
	for _, w := range r.perWindow {
		if len(w.perSymbolCancel) != len(symbols) {
			t.Errorf("window %s has %d symbols, expected %d",
				w.label, len(w.perSymbolCancel), len(symbols))
		}
	}
}
