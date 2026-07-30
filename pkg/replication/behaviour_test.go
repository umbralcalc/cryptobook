package replication

import (
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestCrossSegmentReplication is the binding test named by every claim in behaviour.go —
// one subtest per claim, named by the claim's ID.
//
// It skips without the segments. They are Binance data, which the licence does not permit
// redistributing, so they are git-ignored and have to be re-recorded — which is the point
// of this package: anyone can regenerate them from public endpoints in eight minutes and
// check these bounds themselves.
func TestCrossSegmentReplication(t *testing.T) {
	if !Available() {
		t.Skip("segments absent (not redistributable); re-record all five with:\n" +
			"  for s in BTCUSDT ETHUSDT SOLUSDT XRPUSDT DOGEUSDT; do \\\n" +
			"    go run ./cmd/record-feed -symbol $s -duration 8m -out testdata/seg_$s.log & \\\n" +
			"  done; wait")
	}
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestEverySegmentIsScored guards the quantifier the predictions are stated with.
//
// J, K and L assert a bound on EVERY segment. If a symbol were dropped from the list the
// claims would keep passing while quietly meaning something weaker, so the count and the
// membership are pinned here rather than left implicit.
func TestEverySegmentIsScored(t *testing.T) {
	want := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT"}
	if len(segments) != len(want) {
		t.Fatalf("segment count changed: %d, want %d — the predictions were fixed over "+
			"five symbols and a different set does not score them", len(segments), len(want))
	}
	for i, w := range want {
		if segments[i].symbol != w {
			t.Errorf("segment %d is %s, want %s: the pre-registered set and its "+
				"volume ordering are fixed, and prediction M's target depends on the "+
				"ordering", i, segments[i].symbol, w)
		}
	}
	// Descending quote volume is what makes the last entry "the lowest-liquidity symbol".
	for i := 1; i < len(segments); i++ {
		if segments[i].quoteVolume >= segments[i-1].quoteVolume {
			t.Errorf("quote volume is not descending at %s: prediction M names the "+
				"lowest-volume symbol, so the ordering has to hold", segments[i].symbol)
		}
	}
}
