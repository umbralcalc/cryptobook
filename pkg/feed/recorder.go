package feed

import (
	"errors"
	"time"
)

// Recorder drives a Book and an Aggregator from a message stream and emits one
// state row per bucket. It performs no I/O.
//
// That is the point. PLAN.md's Spike 3.3 warns that a live feed makes -race
// failures probabilistic — "a clean run proves less than it appears to" — and
// requires a recorded-feed replay harness so race testing is deterministic. This
// type IS that harness: feed it messages and clock ticks and it behaves
// identically every time, so the concurrency-free logic can be tested exhaustively
// and the network shell in cmd/record-feed stays thin enough to read.
//
// # The gap contract, end to end
//
// Spike 3.1 requires more than detection: the marking must PROPAGATE INTO THE
// CALIBRATION. Here is the whole path.
//
//	depth update arrives
//	  -> Book.Apply enforces U == previous u + 1
//	  -> a gap returns ErrSequenceGap
//	  -> the Recorder marks the CURRENT bucket suspect and desynchronises
//	  -> every bucket stays suspect until a snapshot resynchronises the book
//	  -> the flag is written as column 10 of each affected row
//	  -> calibration filters on that column
//
// Nothing in that chain absorbs a gap quietly, and the interval a gap touched is
// distinguishable from the intervals it did not — which is what stops one reconnect
// from either corrupting a segment or condemning all of it.
type Recorder struct {
	book   *Book
	agg    *Aggregator
	bucket time.Duration

	// bucketEnd is when the open bucket closes. Zero means no bucket is open.
	bucketEnd time.Time
	// awaitingFirst reports that the next depth update is the first against a fresh
	// snapshot, and so follows the straddling rule rather than the chaining rule.
	awaitingFirst bool
	// desynced reports that the book cannot be trusted until a snapshot arrives.
	desynced bool

	// gaps counts sequence gaps seen, for reporting.
	gaps int
}

// NewRecorder returns a recorder emitting one row per bucket of the given
// duration, discretising volume by lotSize and banding by edgesBP.
func NewRecorder(
	bucket time.Duration, lotSize float64, edgesBP [LevelsPerSide]float64,
) *Recorder {
	if bucket <= 0 {
		panic("feed: bucket duration must be positive")
	}
	return &Recorder{
		book:     NewBook(),
		agg:      NewAggregator(lotSize, edgesBP),
		bucket:   bucket,
		desynced: true,
	}
}

// Gaps returns how many sequence gaps have been seen.
func (r *Recorder) Gaps() int { return r.gaps }

// Desynced reports whether the recorder is waiting for a snapshot.
func (r *Recorder) Desynced() bool { return r.desynced }

// Resync installs a snapshot and resumes. Any bucket open at the time stays
// suspect, because part of it was recorded against a book that had a hole in it.
func (r *Recorder) Resync(snapshot Snapshot) {
	r.book.Reset(snapshot.LastUpdateID, snapshot.Bids, snapshot.Asks)
	r.awaitingFirst = true
	r.desynced = false
	if !r.bucketEnd.IsZero() {
		r.agg.MarkSuspect()
	}
}

// OnDepth applies one depth update.
//
// It returns true when the caller must fetch a fresh snapshot and call Resync: a
// sequence gap, or a snapshot too old for the buffered events. The recorder keeps
// emitting rows in the meantime, all marked suspect, rather than stalling — a
// long-running collection should not lose the intervals around a reconnect, it
// should be able to tell which ones they were.
func (r *Recorder) OnDepth(update DepthUpdate) (resyncNeeded bool) {
	if r.desynced {
		r.agg.MarkSuspect()
		return true
	}
	// The aggregator must see the update BEFORE the book applies it, since deltas
	// are computed against the pre-update sizes.
	r.agg.ObserveUpdate(r.book, update)
	skipped, err := r.book.Apply(update, r.awaitingFirst)
	switch {
	case err == nil:
		if !skipped {
			r.awaitingFirst = false
		}
		return false
	case errors.Is(err, ErrSequenceGap), errors.Is(err, ErrStaleSnapshot):
		r.gaps++
		r.desynced = true
		r.agg.MarkSuspect()
		return true
	default:
		// Applying to an unsynchronised book, which Resync has not happened for.
		r.desynced = true
		r.agg.MarkSuspect()
		return true
	}
}

// OnTrade records one trade against the open bucket.
func (r *Recorder) OnTrade(trade Trade) {
	r.agg.ObserveTrade(trade.Price, trade.Size)
}

// Tick advances the clock, returning a completed row when a bucket closes.
//
// The first call opens the first bucket and returns no row. Buckets are aligned to
// the recorder's start rather than to wall-clock seconds, which keeps every bucket
// exactly one interval long — an alignment to real seconds would make the first
// bucket a partial one and quietly bias its counts downwards.
func (r *Recorder) Tick(now time.Time) (row []float64, ok bool) {
	if r.bucketEnd.IsZero() {
		r.agg.OpenBucket(r.book)
		if r.desynced {
			r.agg.MarkSuspect()
		}
		r.bucketEnd = now.Add(r.bucket)
		return nil, false
	}
	if now.Before(r.bucketEnd) {
		return nil, false
	}
	row = r.agg.CloseBucket()
	r.agg.OpenBucket(r.book)
	if r.desynced {
		r.agg.MarkSuspect()
	}
	r.bucketEnd = r.bucketEnd.Add(r.bucket)
	// A long stall (a reconnect, a suspended process) can leave the deadline in the
	// past. Skip forward rather than emitting a burst of empty buckets that would
	// read as a period of zero market activity.
	if now.After(r.bucketEnd) {
		r.bucketEnd = now.Add(r.bucket)
		r.agg.MarkSuspect()
	}
	return row, true
}
