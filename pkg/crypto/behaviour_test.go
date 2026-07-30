package crypto

import (
	"os"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
	"github.com/umbralcalc/cryptobook/pkg/feed"
)

// TestCryptoResidualDiagnostics is the binding test named by every claim in
// behaviour.go — one subtest per claim, named by the claim's ID.
func TestCryptoResidualDiagnostics(t *testing.T) {
	requireFixture(t)
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestFixtureIsCommitted guards the property that makes these claims reproducible.
//
// If the fixture went missing the claims would not fail — they would panic at
// generation time, or worse, a future edit could point them at a freshly recorded
// file whose numbers move on every run, pinning nothing.
func TestFixtureIsPresent(t *testing.T) {
	requireFixture(t)
	info, err := os.Stat(FixturePath())
	if err != nil {
		t.Fatalf("the recorded segment must be committed: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("the recorded segment is empty")
	}
}

// TestSuspectRowsAreExcluded is the far end of Spike 3.1.
//
// The requirement was never "detect a gap" — it was that the marking propagate INTO
// the calibration. The collector marks the interval, the flag rides in column 10,
// and LoadSegment is where it changes what gets analysed. A test that only checked
// the flag existed would miss the half that matters.
func TestSuspectRowsAreExcluded(t *testing.T) {
	requireFixture(t)
	segment, dropped, err := diagnostics.LoadSegment(FixturePath())
	if err != nil {
		t.Fatalf("loading the segment: %v", err)
	}
	for i, row := range segment.Rows {
		if row[feed.IdxSuspect] != 0 {
			t.Fatalf("row %d survived the filter with the suspect flag set", i)
		}
	}
	t.Logf("kept %d rows, dropped %d suspect", len(segment.Rows), dropped)
}

// TestDiagnosticsAreArithmeticallySound checks the statistics against hand-worked
// cases, so a claim's headline number cannot be an artifact of a broken helper.
func TestDiagnosticsAreArithmeticallySound(t *testing.T) {
	t.Run("dispersion of a constant is zero", func(t *testing.T) {
		if got := diagnostics.Dispersion([]float64{5, 5, 5, 5}); got != 0 {
			t.Errorf("dispersion = %v, want 0", got)
		}
	})

	t.Run("dispersion of a known sample", func(t *testing.T) {
		// mean 2, population variance 2, so the ratio is exactly 1 — a sample that
		// IS Poisson-dispersed, which is what the claim's threshold is measured against.
		got := diagnostics.Dispersion([]float64{0, 1, 2, 3, 4})
		if got < 0.99 || got > 1.01 {
			t.Errorf("dispersion = %v, want 1", got)
		}
	})

	t.Run("correlation is exact at the extremes", func(t *testing.T) {
		up := []float64{1, 2, 3, 4}
		down := []float64{4, 3, 2, 1}
		if got := diagnostics.Correlation(up, up); got < 0.999 {
			t.Errorf("self-correlation = %v, want 1", got)
		}
		if got := diagnostics.Correlation(up, down); got > -0.999 {
			t.Errorf("anti-correlation = %v, want -1", got)
		}
	})

	t.Run("a flat series correlates with nothing rather than dividing by zero", func(t *testing.T) {
		if got := diagnostics.Correlation([]float64{1, 2, 3}, []float64{7, 7, 7}); got != 0 {
			t.Errorf("correlation with a constant = %v, want 0", got)
		}
	})
}

// TestSegmentRowsAreWellFormed checks the fixture itself, so a malformed capture
// cannot quietly become a finding about the market.
func TestSegmentRowsAreWellFormed(t *testing.T) {
	requireFixture(t)
	segment, _, err := diagnostics.LoadSegment(FixturePath())
	if err != nil {
		t.Fatalf("loading the segment: %v", err)
	}
	if len(segment.Rows) < 100 {
		t.Errorf("segment has %d usable rows; too short to say anything",
			len(segment.Rows))
	}
	for i, row := range segment.Rows {
		for j, value := range row {
			if value < 0 {
				t.Fatalf("row %d index %d is negative (%v) — counts and depths cannot be",
					i, j, value)
			}
		}
		// Every band must carry mass. This is what failed on the first recording and
		// is the reason decision 5 was revised: with adjacent price levels, four of
		// the six slots were empty and the ladder the model needs was not there.
		if row[feed.IdxDepthStart] == 0 {
			t.Fatalf("row %d has zero depth across all six bands", i)
		}
	}
}

// requireFixture skips when the market-data fixture is absent, which is the normal
// state of a fresh clone: the provider's licence does not permit redistributing their
// data, so it is not committed (README.md quotes the licence).
//
// Skipping rather than failing is deliberate. A failure would say the repo is
// broken; a skip says this measurement needs data the repo may not lawfully carry,
// and names how to get it.
func requireFixture(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("market-data fixture absent (not redistributable); regenerate with:\n  go run ./cmd/record-feed -symbol BTCUSDT -duration 8m -out testdata/btcusdt_depth.log")
	}
}
