// Package feed reconstructs a local order book from an exchange's diff-update
// stream, and refuses to do so silently when the stream has a hole in it.
//
// # Why this is the riskiest code in the project
//
// PLAN.md calls Spike 3.1 the highest-risk item, and the reason is worth restating
// where the code lives: a dropped update corrupts the book, and the corruption is
// INVISIBLE IN AGGREGATE STATISTICS. Spread and depth summaries look entirely
// normal while the book state is wrong, so a calibration on corrupted state
// produces plausible parameters that mean nothing. Nothing downstream can detect
// it. It has to be caught here or not at all.
//
// So the sequence check below is not defensive tidiness — it is the only thing
// standing between a gap and a confident wrong number.
//
// # The contract
//
// Binance spot publishes each depth event with U (first update id) and u (final
// update id), against a REST snapshot carrying lastUpdateId. The documented
// procedure, implemented here:
//
//  1. Buffer events while fetching the snapshot.
//  2. Discard events with u <= lastUpdateId — they predate it.
//  3. The first applied event must satisfy U <= lastUpdateId+1 <= u.
//  4. Every subsequent event must satisfy U == previous u + 1.
//
// Step 4 is exact. There is no tolerance and no "close enough": either the ids
// chain or a message was missed.
//
// # What happens on a gap
//
// PLAN.md's branches were: resnapshot and mark the interval suspect (preferred);
// hard fail (acceptable early, unworkable long-running); tolerate silently
// (unacceptable). This implements the preferred one, and the marking is the part
// that matters — Apply returns ErrSequenceGap so the caller must handle it, and
// the collector records a suspect flag on every bucket the gap touched. That flag
// travels into the recorded segment as a column, so calibration can exclude
// suspect intervals rather than trusting them.
//
// A gap is therefore never absorbed. It is either handled by the caller or it
// stops the run.
package feed

import (
	"errors"
	"fmt"
	"sort"
)

// ErrSequenceGap reports a hole in the update stream: the ids did not chain, so
// at least one message was missed and the book can no longer be trusted.
//
// Callers must resynchronise from a fresh snapshot and mark the affected interval
// suspect. Ignoring this error silently corrupts every number downstream of it.
var ErrSequenceGap = errors.New("feed: sequence gap in the depth stream")

// ErrStaleSnapshot reports that the buffered events do not reach the snapshot —
// the first usable event starts after lastUpdateId+1, so the window between them
// is missing entirely. It is the same class of failure as a gap and is treated the
// same way, but named separately because the fix differs: fetch a newer snapshot
// rather than resume from this one.
var ErrStaleSnapshot = errors.New("feed: snapshot is older than the buffered events")

// Level is one price level's resting size. Size zero means the level was removed.
type Level struct {
	Price float64
	Size  float64
}

// DepthUpdate is one diff event from the stream.
type DepthUpdate struct {
	// FirstID and FinalID are Binance's U and u.
	FirstID int64
	FinalID int64
	Bids    []Level
	Asks    []Level
}

// Book is a local order book maintained from a snapshot plus diff updates.
//
// It is deliberately not concurrency-safe. The collector owns one Book on one
// goroutine; sharing it would require a lock on the hot path of every update, and
// there is no reason to.
type Book struct {
	bids map[float64]float64
	asks map[float64]float64
	// lastID is the final update id of the last applied event, or the snapshot's
	// lastUpdateId before any event has been applied.
	lastID int64
	// synced reports whether a snapshot has been installed. Applying an update to
	// an unsynced book is a programming error, not a data error.
	synced bool
}

// NewBook returns an empty, unsynchronised book.
func NewBook() *Book {
	return &Book{
		bids: make(map[float64]float64),
		asks: make(map[float64]float64),
	}
}

// Reset installs a snapshot, discarding any previous state. This is what a caller
// does after a gap: fetch a fresh snapshot and start again from it.
func (b *Book) Reset(lastUpdateID int64, bids, asks []Level) {
	b.bids = make(map[float64]float64, len(bids))
	b.asks = make(map[float64]float64, len(asks))
	for _, level := range bids {
		if level.Size > 0 {
			b.bids[level.Price] = level.Size
		}
	}
	for _, level := range asks {
		if level.Size > 0 {
			b.asks[level.Price] = level.Size
		}
	}
	b.lastID = lastUpdateID
	b.synced = true
}

// Synced reports whether a snapshot has been installed.
func (b *Book) Synced() bool { return b.synced }

// LastID returns the final update id of the last applied event.
func (b *Book) LastID() int64 { return b.lastID }

// Applied reports whether an update is the FIRST one usable against the snapshot,
// per the documented rule U <= lastUpdateId+1 <= u. Events entirely at or before
// the snapshot are skippable; events starting after the window are a stale
// snapshot.
//
// Split out from Apply because the first event follows a different rule from every
// later one, and conflating the two is how a resync quietly starts from the wrong
// place.
func (b *Book) firstUsable(update DepthUpdate) (skip bool, err error) {
	switch {
	case update.FinalID <= b.lastID:
		return true, nil // predates the snapshot entirely
	case update.FirstID <= b.lastID+1 && b.lastID+1 <= update.FinalID:
		return false, nil // straddles the snapshot: this is the one to start from
	default:
		return false, fmt.Errorf(
			"%w: snapshot lastUpdateId %d, first buffered event covers [%d, %d]",
			ErrStaleSnapshot, b.lastID, update.FirstID, update.FinalID)
	}
}

// Apply applies one diff update, enforcing the sequence contract.
//
// The bool reports whether the update was skipped as predating the snapshot. An
// ErrSequenceGap return means the book is no longer trustworthy and the caller
// must resynchronise; the book's contents are left untouched so a caller that
// wrongly ignores the error gets a stale book rather than a corrupted one — the
// less dangerous of the two, since staleness at least shows up in the numbers.
func (b *Book) Apply(update DepthUpdate, first bool) (skipped bool, err error) {
	if !b.synced {
		return false, errors.New("feed: Apply called before a snapshot was installed")
	}
	if first {
		skip, err := b.firstUsable(update)
		if err != nil || skip {
			return skip, err
		}
	} else if update.FirstID != b.lastID+1 {
		return false, fmt.Errorf(
			"%w: expected first update id %d, got %d (missed %d update(s))",
			ErrSequenceGap, b.lastID+1, update.FirstID, update.FirstID-b.lastID-1)
	}
	applyLevels(b.bids, update.Bids)
	applyLevels(b.asks, update.Asks)
	b.lastID = update.FinalID
	return false, nil
}

// applyLevels writes one side's changes. A size of zero deletes the level, which
// is how the exchange spells "this level is gone" — storing it as a zero-size
// entry instead would leave the level counted as present but empty.
func applyLevels(side map[float64]float64, levels []Level) {
	for _, level := range levels {
		if level.Size <= 0 {
			delete(side, level.Price)
			continue
		}
		side[level.Price] = level.Size
	}
}

// TopBids returns the n best bids, highest price first, padded with zero-size
// levels if the book is thinner than n.
func (b *Book) TopBids(n int) []Level { return top(b.bids, n, true) }

// TopAsks returns the n best asks, lowest price first, padded with zero-size
// levels if the book is thinner than n.
func (b *Book) TopAsks(n int) []Level { return top(b.asks, n, false) }

// top sorts one side and takes the n levels nearest the touch. Padding rather than
// returning a short slice keeps the recorded row a fixed width, which the
// downstream config requires — a variable-width state would be a different model
// every step.
func top(side map[float64]float64, n int, descending bool) []Level {
	levels := make([]Level, 0, len(side))
	for price, size := range side {
		levels = append(levels, Level{Price: price, Size: size})
	}
	sort.Slice(levels, func(i, j int) bool {
		if descending {
			return levels[i].Price > levels[j].Price
		}
		return levels[i].Price < levels[j].Price
	})
	if len(levels) > n {
		levels = levels[:n]
	}
	for len(levels) < n {
		levels = append(levels, Level{})
	}
	return levels
}

// ForEach visits every resting level on one side. Used to total a price band that
// is far wider than the few levels TopBids/TopAsks return.
func (b *Book) ForEach(bid bool, visit func(price, size float64)) {
	side := b.asks
	if bid {
		side = b.bids
	}
	for price, size := range side {
		visit(price, size)
	}
}

// SizeAt returns the resting size at a price on the given side, or zero.
func (b *Book) SizeAt(price float64, bid bool) float64 {
	if bid {
		return b.bids[price]
	}
	return b.asks[price]
}
