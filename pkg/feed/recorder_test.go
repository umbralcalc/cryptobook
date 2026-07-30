package feed

import (
	"testing"
	"time"
)

// base is a fixed start instant so every test is deterministic.
var base = time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

// newSyncedRecorder returns a recorder already synchronised on a small book, with
// its first bucket open.
func newSyncedRecorder(t *testing.T) *Recorder {
	t.Helper()
	rec := NewRecorder(time.Second, 1, testEdges)
	rec.Resync(Snapshot{
		LastUpdateID: 100,
		Bids:         []Level{{Price: 99, Size: 3}, {Price: 98, Size: 2}, {Price: 97, Size: 1}},
		Asks:         []Level{{Price: 101, Size: 4}, {Price: 102, Size: 5}, {Price: 103, Size: 6}},
	})
	if row, ok := rec.Tick(base); ok || row != nil {
		t.Fatalf("the first tick must open a bucket without emitting a row")
	}
	return rec
}

// depth is a single-id depth update.
func depth(id int64, bids, asks []Level) DepthUpdate {
	return DepthUpdate{FirstID: id, FinalID: id, Bids: bids, Asks: asks}
}

func TestRecorderHappyPath(t *testing.T) {
	rec := newSyncedRecorder(t)
	if resync := rec.OnDepth(depth(101, []Level{{Price: 99, Size: 8}}, nil)); resync {
		t.Fatal("a contiguous update must not request a resync")
	}
	rec.OnTrade(Trade{Price: 101, Size: 1})
	rec.OnDepth(depth(102, nil, []Level{{Price: 101, Size: 3}}))

	row, ok := rec.Tick(base.Add(time.Second))
	if !ok {
		t.Fatal("expected a row when the bucket elapsed")
	}
	if row[IdxSuspect] != 0 {
		t.Errorf("a clean bucket must not be suspect")
	}
	if row[IdxLimit] != 5 {
		t.Errorf("arrivals = %v, want 5 (bid 3 -> 8)", row[IdxLimit])
	}
	if row[IdxMarket] != 1 {
		t.Errorf("market = %v, want 1", row[IdxMarket])
	}
	if row[IdxCancel] != 0 {
		t.Errorf("cancel = %v, want 0 — the ask decrease was entirely traded",
			row[IdxCancel])
	}
	if rec.Gaps() != 0 {
		t.Errorf("gaps = %d, want 0", rec.Gaps())
	}
}

func TestGapPropagatesToTheRow(t *testing.T) {
	// This is Spike 3.1's actual requirement. Detecting a gap is not enough; the
	// marking has to reach the calibration, and the only thing the calibration reads
	// is the row.
	rec := newSyncedRecorder(t)
	rec.OnDepth(depth(101, []Level{{Price: 99, Size: 8}}, nil))

	if resync := rec.OnDepth(depth(105, nil, nil)); !resync {
		t.Fatal("a gap must request a resync")
	}
	if rec.Gaps() != 1 {
		t.Errorf("gaps = %d, want 1", rec.Gaps())
	}
	row, ok := rec.Tick(base.Add(time.Second))
	if !ok {
		t.Fatal("expected a row")
	}
	if row[IdxSuspect] != 1 {
		t.Fatal("the bucket a gap touched must be marked suspect in the row")
	}
}

func TestSuspectSpansTheWholeDesyncedPeriod(t *testing.T) {
	// A gap taints every bucket until the book is trustworthy again, not just the
	// one the gap landed in — the intervening buckets were recorded against a book
	// with a hole in it.
	rec := newSyncedRecorder(t)
	rec.OnDepth(depth(105, nil, nil)) // gap

	for i := 1; i <= 3; i++ {
		row, ok := rec.Tick(base.Add(time.Duration(i) * time.Second))
		if !ok {
			t.Fatalf("expected a row at second %d", i)
		}
		if row[IdxSuspect] != 1 {
			t.Errorf("bucket %d: suspect = %v, want 1 while desynced", i, row[IdxSuspect])
		}
	}

	// Resynchronising during a bucket still leaves THAT bucket suspect, because
	// part of it was recorded before the book was trustworthy.
	rec.Resync(Snapshot{
		LastUpdateID: 200,
		Bids:         []Level{{Price: 99, Size: 3}},
		Asks:         []Level{{Price: 101, Size: 3}},
	})
	row, ok := rec.Tick(base.Add(4 * time.Second))
	if !ok {
		t.Fatal("expected a row")
	}
	if row[IdxSuspect] != 1 {
		t.Error("the bucket a resync happened in must stay suspect")
	}

	// The bucket AFTER a clean resync is trustworthy again. Without this a single
	// reconnect would condemn the rest of the segment.
	rec.OnDepth(depth(201, nil, nil))
	row, ok = rec.Tick(base.Add(5 * time.Second))
	if !ok {
		t.Fatal("expected a row")
	}
	if row[IdxSuspect] != 0 {
		t.Error("a bucket wholly after a clean resync must not be suspect")
	}
}

func TestUpdatesWhileDesyncedAreNotApplied(t *testing.T) {
	// The dangerous failure is applying updates to a book that has already lost
	// sync: the numbers would look ordinary and be wrong.
	rec := newSyncedRecorder(t)
	rec.OnDepth(depth(105, nil, nil)) // gap
	rec.OnDepth(depth(106, []Level{{Price: 99, Size: 999}}, nil))
	if got := rec.book.SizeAt(99, true); got != 3 {
		t.Errorf("bid at 99 = %v, want the pre-gap 3 — updates must not be applied "+
			"to a desynchronised book", got)
	}
	if !rec.Desynced() {
		t.Error("the recorder must stay desynced until a snapshot arrives")
	}
}

func TestBucketTiming(t *testing.T) {
	t.Run("no row before the interval elapses", func(t *testing.T) {
		rec := newSyncedRecorder(t)
		if _, ok := rec.Tick(base.Add(999 * time.Millisecond)); ok {
			t.Error("a bucket must not close early")
		}
	})

	t.Run("buckets are a fixed length, not wall-clock aligned", func(t *testing.T) {
		// Aligning to real seconds would make the first bucket partial and bias its
		// counts downwards.
		rec := NewRecorder(time.Second, 1, testEdges)
		rec.Resync(Snapshot{
			LastUpdateID: 1,
			Bids:         []Level{{Price: 99, Size: 1}},
			Asks:         []Level{{Price: 101, Size: 1}},
		})
		start := base.Add(300 * time.Millisecond)
		rec.Tick(start)
		if _, ok := rec.Tick(start.Add(999 * time.Millisecond)); ok {
			t.Error("bucket closed before a full interval had passed")
		}
		if _, ok := rec.Tick(start.Add(time.Second)); !ok {
			t.Error("bucket did not close after exactly one interval")
		}
	})

	t.Run("a long stall skips forward and marks suspect", func(t *testing.T) {
		// Emitting a burst of empty buckets for a stalled process would read as a
		// period of zero market activity, which is a fabricated observation.
		rec := newSyncedRecorder(t)
		row, ok := rec.Tick(base.Add(30 * time.Second))
		if !ok {
			t.Fatal("expected a row")
		}
		_ = row
		next, ok := rec.Tick(base.Add(31 * time.Second))
		if !ok {
			t.Fatal("expected the following bucket to close one interval later")
		}
		if next[IdxSuspect] != 1 {
			t.Error("the bucket spanning a stall must be marked suspect")
		}
	})
}
