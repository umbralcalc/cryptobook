package queue

import (
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestQueuePosition is the binding test named by every claim in behaviour.go.
func TestQueuePosition(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestOccupancyIsBinary guards the assumption every position statistic rests on. The
// position sums are computed in the config as sum(pos * occ), so a slot holding 2 would
// weight one position twice and every mean here would be quietly wrong rather than absent.
func TestOccupancyIsBinary(t *testing.T) {
	m, err := measureAll()
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range regimes {
		if m[i].nonBinary != 0 {
			t.Errorf("%s: %.0f slots hold something other than 0 or 1; every mean position "+
				"in this package assumes one order per slot", r.label, m[i].nonBinary)
		}
	}
}

// TestTheTickSweepActuallyVaries guards the substitution. Both tick claims compare three
// regimes, and cfgrun.Subs silently succeeds on a pattern that matches nothing — so a
// renamed param in the config would leave three IDENTICAL runs, and a monotone assertion
// over three equal values is not something to rely on failing.
func TestTheTickSweepActuallyVaries(t *testing.T) {
	m, err := measureAll()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(m); i++ {
		if m[i].queueLength == m[i-1].queueLength {
			t.Fatalf("%s and %s produced identical queue lengths (%.6f); the tick "+
				"substitution is not reaching cfg/lob_queue.yaml",
				regimes[i-1].label, regimes[i].label, m[i].queueLength)
		}
	}
}
