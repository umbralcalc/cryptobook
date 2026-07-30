package feed

import (
	"errors"
	"testing"
)

// snapshot installs a small two-level-per-side book at id 100.
func snapshot(t *testing.T) *Book {
	t.Helper()
	book := NewBook()
	book.Reset(100,
		[]Level{{Price: 99, Size: 5}, {Price: 98, Size: 7}},
		[]Level{{Price: 101, Size: 4}, {Price: 102, Size: 6}},
	)
	return book
}

// apply is a shorthand for a non-first update covering exactly one id.
func apply(t *testing.T, book *Book, id int64, bids, asks []Level) error {
	t.Helper()
	_, err := book.Apply(
		DepthUpdate{FirstID: id, FinalID: id, Bids: bids, Asks: asks}, false)
	return err
}

func TestSequenceContract(t *testing.T) {
	// These are the whole point of the package. A gap that is not caught here is
	// not caught anywhere: the resulting book looks entirely normal in aggregate,
	// so no downstream check can find it.
	t.Run("a contiguous update is applied", func(t *testing.T) {
		book := snapshot(t)
		if err := apply(t, book, 101, []Level{{Price: 99, Size: 9}}, nil); err != nil {
			t.Fatalf("expected a contiguous update to apply, got %v", err)
		}
		if got := book.SizeAt(99, true); got != 9 {
			t.Errorf("bid at 99 = %v, want 9", got)
		}
		if book.LastID() != 101 {
			t.Errorf("lastID = %d, want 101", book.LastID())
		}
	})

	t.Run("a one-message hole is a gap", func(t *testing.T) {
		book := snapshot(t)
		// 101 is missing; this event starts at 102.
		err := apply(t, book, 102, []Level{{Price: 99, Size: 9}}, nil)
		if !errors.Is(err, ErrSequenceGap) {
			t.Fatalf("expected ErrSequenceGap, got %v", err)
		}
	})

	t.Run("a gap leaves the book untouched rather than corrupted", func(t *testing.T) {
		// A caller that wrongly ignores the error should get a STALE book, not a
		// corrupted one. Staleness shows up in the numbers; corruption does not.
		book := snapshot(t)
		_ = apply(t, book, 102, []Level{{Price: 99, Size: 999}}, nil)
		if got := book.SizeAt(99, true); got != 5 {
			t.Errorf("bid at 99 = %v, want the pre-gap 5 — a rejected update must "+
				"not be partially applied", got)
		}
		if book.LastID() != 100 {
			t.Errorf("lastID = %d, want the pre-gap 100", book.LastID())
		}
	})

	t.Run("a replayed update is a gap, not a no-op", func(t *testing.T) {
		// Going backwards is as broken as skipping forwards, and a tolerant
		// implementation that silently ignored stale ids would hide a reordered or
		// duplicated stream.
		book := snapshot(t)
		if err := apply(t, book, 101, nil, nil); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := apply(t, book, 101, nil, nil); !errors.Is(err, ErrSequenceGap) {
			t.Fatalf("expected a replayed id to be rejected, got %v", err)
		}
	})

	t.Run("a multi-id event chains from its final id", func(t *testing.T) {
		book := snapshot(t)
		if _, err := book.Apply(
			DepthUpdate{FirstID: 101, FinalID: 105}, false); err != nil {
			t.Fatalf("expected a multi-id event to apply, got %v", err)
		}
		if book.LastID() != 105 {
			t.Fatalf("lastID = %d, want 105", book.LastID())
		}
		// The next event must continue from 106, not from 102.
		if err := apply(t, book, 106, nil, nil); err != nil {
			t.Errorf("expected 106 to follow 105, got %v", err)
		}
	})

	t.Run("applying before a snapshot is refused", func(t *testing.T) {
		if _, err := NewBook().Apply(DepthUpdate{FirstID: 1, FinalID: 1}, false); err == nil {
			t.Fatal("expected Apply on an unsynchronised book to fail")
		}
	})
}

func TestFirstEventAgainstSnapshot(t *testing.T) {
	// The first usable event follows a different rule from every later one
	// (U <= lastUpdateId+1 <= u), and conflating the two is how a resync quietly
	// starts from the wrong place.
	t.Run("an event predating the snapshot is skipped", func(t *testing.T) {
		book := snapshot(t)
		skipped, err := book.Apply(DepthUpdate{FirstID: 90, FinalID: 95}, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !skipped {
			t.Error("expected an event entirely before the snapshot to be skipped")
		}
		if book.LastID() != 100 {
			t.Errorf("a skipped event must not move lastID, got %d", book.LastID())
		}
	})

	t.Run("an event straddling the snapshot is the one to start from", func(t *testing.T) {
		book := snapshot(t)
		skipped, err := book.Apply(
			DepthUpdate{FirstID: 95, FinalID: 105, Bids: []Level{{Price: 99, Size: 3}}},
			true)
		if err != nil || skipped {
			t.Fatalf("expected the straddling event to apply, got skipped=%v err=%v",
				skipped, err)
		}
		if book.LastID() != 105 {
			t.Errorf("lastID = %d, want 105", book.LastID())
		}
	})

	t.Run("an event starting after the snapshot window is a stale snapshot", func(t *testing.T) {
		// The snapshot is older than the buffered events, so the interval between
		// them was never seen. Same class of failure as a gap, different fix.
		book := snapshot(t)
		_, err := book.Apply(DepthUpdate{FirstID: 110, FinalID: 115}, true)
		if !errors.Is(err, ErrStaleSnapshot) {
			t.Fatalf("expected ErrStaleSnapshot, got %v", err)
		}
	})

	t.Run("resetting after a gap resynchronises cleanly", func(t *testing.T) {
		// The preferred branch of PLAN.md's Spike 3.1: resnapshot and resume.
		book := snapshot(t)
		if err := apply(t, book, 105, nil, nil); !errors.Is(err, ErrSequenceGap) {
			t.Fatalf("setup: expected a gap, got %v", err)
		}
		book.Reset(200, []Level{{Price: 99, Size: 1}}, []Level{{Price: 101, Size: 2}})
		if err := apply(t, book, 201, nil, nil); err != nil {
			t.Errorf("expected the stream to resume after a resnapshot, got %v", err)
		}
	})
}

func TestLevelBookkeeping(t *testing.T) {
	t.Run("a zero size deletes the level", func(t *testing.T) {
		// The exchange spells "this level is gone" as size zero. Storing it as a
		// zero-size entry would leave it counted as present but empty, which shifts
		// every level below it in TopBids/TopAsks.
		book := snapshot(t)
		if err := apply(t, book, 101, []Level{{Price: 99, Size: 0}}, nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if got := book.TopBids(1)[0]; got.Price != 98 {
			t.Errorf("best bid = %v, want the level below the deleted one (98)", got.Price)
		}
	})

	t.Run("top levels are ordered from the touch outwards", func(t *testing.T) {
		book := snapshot(t)
		bids, asks := book.TopBids(2), book.TopAsks(2)
		if bids[0].Price != 99 || bids[1].Price != 98 {
			t.Errorf("bids = %v, want 99 then 98 (descending from the touch)", bids)
		}
		if asks[0].Price != 101 || asks[1].Price != 102 {
			t.Errorf("asks = %v, want 101 then 102 (ascending from the touch)", asks)
		}
	})

	t.Run("a thin book is padded to a fixed width", func(t *testing.T) {
		// The recorded row must be a fixed width or the downstream config is
		// describing a different model every step.
		book := NewBook()
		book.Reset(1, []Level{{Price: 99, Size: 5}}, nil)
		if got := len(book.TopBids(3)); got != 3 {
			t.Errorf("TopBids(3) returned %d levels, want 3", got)
		}
		if got := book.TopAsks(3); len(got) != 3 || got[0].Size != 0 {
			t.Errorf("TopAsks(3) on an empty side = %v, want three zero levels", got)
		}
	})

	t.Run("a snapshot discards previous state", func(t *testing.T) {
		book := snapshot(t)
		book.Reset(200, []Level{{Price: 50, Size: 1}}, nil)
		if got := book.SizeAt(99, true); got != 0 {
			t.Errorf("bid at 99 = %v after a reset, want 0 — stale levels must not "+
				"survive a resynchronisation", got)
		}
	})
}
