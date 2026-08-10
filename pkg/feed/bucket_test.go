package feed

import "testing"

// testEdges put exactly one of standardBook's price levels in each band. The mid
// is 100, so 150/250/350 bp are distances of 1.5/2.5/3.5 price units: 99 and 101
// land in band 0, 98 and 102 in band 1, 97 and 103 in band 2. Real edges are
// DefaultBandEdgesBP, which are geometric and much wider.
var testEdges = [LevelsPerSide]float64{150, 250, 350}

// bookAt builds a synced book with the given bid and ask levels.
func bookAt(bids, asks []Level) *Book {
	book := NewBook()
	book.Reset(0, bids, asks)
	return book
}

// standardBook is three levels a side, one unit each, so lot arithmetic is legible.
func standardBook() *Book {
	return bookAt(
		[]Level{{Price: 99, Size: 3}, {Price: 98, Size: 2}, {Price: 97, Size: 1}},
		[]Level{{Price: 101, Size: 4}, {Price: 102, Size: 5}, {Price: 103, Size: 6}},
	)
}

// step applies an update to the aggregator and then the book, in the order the
// collector must use.
func step(a *Aggregator, book *Book, update DepthUpdate) {
	a.ObserveUpdate(book, update)
	applyLevels(book.bids, update.Bids)
	applyLevels(book.asks, update.Asks)
}

func TestBucketOpeningState(t *testing.T) {
	t.Run("opening levels and depth are recorded touch-first", func(t *testing.T) {
		agg := NewAggregator(1, testEdges)
		agg.OpenBucket(standardBook())
		row := agg.CloseBucket()
		// One level per band, nearest the mid first: bids 99/98/97, asks 101/102/103.
		want := []float64{3, 2, 1, 4, 5, 6}
		for i, w := range want {
			if row[i] != w {
				t.Errorf("row[%d] = %v, want %v (bids touch-first, then asks)", i, row[i], w)
			}
		}
		if row[IdxDepthStart] != 21 {
			t.Errorf("depth = %v, want 21", row[IdxDepthStart])
		}
	})

	t.Run("an unsynced book makes the bucket suspect", func(t *testing.T) {
		agg := NewAggregator(1, testEdges)
		agg.OpenBucket(NewBook())
		if !agg.Suspect() {
			t.Error("a bucket opened on an unsynchronised book must be suspect")
		}
		if agg.CloseBucket()[IdxSuspect] != 1 {
			t.Error("the suspect flag must reach the row")
		}
	})

	t.Run("the row keeps the generator's index meanings", func(t *testing.T) {
		// This is what lets cfg/lob_calibrate_from_log.yaml read real data unchanged.
		if IdxLimit != 6 || IdxCancel != 7 || IdxMarket != 8 || IdxDepthStart != 9 {
			t.Fatal("indices 6..9 must match cfg/lob_generator.yaml's row layout")
		}
		if got := len(NewAggregator(1, testEdges).CloseBucket()); got != RowWidth {
			t.Errorf("row width = %d, want %d", got, RowWidth)
		}
	})
}

func TestFlowAccumulation(t *testing.T) {
	t.Run("an increase is an arrival", func(t *testing.T) {
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 99, Size: 10}}})
		if row := agg.CloseBucket(); row[IdxLimit] != 7 {
			t.Errorf("arrivals = %v, want 7 (3 -> 10)", row[IdxLimit])
		}
	})

	t.Run("a decrease with no trade is a cancellation", func(t *testing.T) {
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 99, Size: 1}}})
		row := agg.CloseBucket()
		if row[IdxCancel] != 2 {
			t.Errorf("cancellations = %v, want 2 (3 -> 1)", row[IdxCancel])
		}
		if row[IdxMarket] != 0 {
			t.Errorf("market = %v, want 0", row[IdxMarket])
		}
	})

	t.Run("a decrease matched by a trade is an execution, not a cancellation", func(t *testing.T) {
		// Decision 3: the split is what makes cancel_rate mean the same thing here as
		// in the synthetic model. Getting it backwards would inflate cancellations by
		// exactly the traded volume.
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		agg.ObserveTrade(99, 2)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 99, Size: 1}}})
		row := agg.CloseBucket()
		if row[IdxCancel] != 0 {
			t.Errorf("cancellations = %v, want 0 — the whole decrease was traded", row[IdxCancel])
		}
		if row[IdxMarket] != 2 {
			t.Errorf("market = %v, want 2", row[IdxMarket])
		}
	})

	t.Run("a decrease larger than the trade splits into both", func(t *testing.T) {
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		agg.ObserveTrade(101, 1)
		step(agg, book, DepthUpdate{Asks: []Level{{Price: 101, Size: 1}}}) // 4 -> 1
		row := agg.CloseBucket()
		if row[IdxMarket] != 1 || row[IdxCancel] != 2 {
			t.Errorf("market=%v cancel=%v, want 1 and 2 (3 removed, 1 traded)",
				row[IdxMarket], row[IdxCancel])
		}
	})

	t.Run("churn within a bucket is not netted away", func(t *testing.T) {
		// Decision 4. Netting open against close would report no flow at all here,
		// and would understate the busiest markets the most.
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 99, Size: 8}}}) // +5
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 99, Size: 3}}}) // -5
		row := agg.CloseBucket()
		if row[IdxLimit] != 5 || row[IdxCancel] != 5 {
			t.Errorf("arrivals=%v cancellations=%v, want 5 and 5 — add-then-remove "+
				"inside a bucket must not cancel out", row[IdxLimit], row[IdxCancel])
		}
	})

	t.Run("activity outside the band is ignored", func(t *testing.T) {
		// Decision 5: the counts must describe the same slice of the book as the
		// depth does, or the fitted rates describe neither.
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 50, Size: 100}}})
		agg.ObserveTrade(50, 100)
		row := agg.CloseBucket()
		if row[IdxLimit] != 0 || row[IdxMarket] != 0 {
			t.Errorf("arrivals=%v market=%v, want 0 and 0 for out-of-band activity",
				row[IdxLimit], row[IdxMarket])
		}
	})

	t.Run("a new level inside the frozen band is genuine flow", func(t *testing.T) {
		// Bands are price RANGES, so liquidity appearing at a new price within the
		// range is a real arrival — unlike the earlier top-N-price-levels version,
		// which could only see prices that already existed.
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 99.5, Size: 50}}})
		if row := agg.CloseBucket(); row[IdxLimit] != 50 {
			t.Errorf("arrivals = %v, want 50 — a new price inside the band is flow",
				row[IdxLimit])
		}
	})

	t.Run("the band is frozen at open and does not follow the price", func(t *testing.T) {
		// Decision 1, and the reason it matters. The band is derived from the OPENING
		// mid and does not move, so when the market runs away mid-bucket the new
		// quotes fall outside it rather than being counted as a flood of arrivals.
		// Without the freeze, every price move would be laundered into order flow.
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 105, Size: 40}}})
		step(agg, book, DepthUpdate{Asks: []Level{{Price: 106, Size: 40}}})
		if row := agg.CloseBucket(); row[IdxLimit] != 0 {
			t.Errorf("arrivals = %v, want 0 — quotes beyond the opening band are a "+
				"price move, not order flow", row[IdxLimit])
		}
	})

	t.Run("a book with no two-sided touch is suspect, not banded at zero", func(t *testing.T) {
		// Without both sides there is no mid, so no band can be defined. Inventing a
		// reference price would silently band everything against a fiction.
		agg := NewAggregator(1, testEdges)
		agg.OpenBucket(bookAt([]Level{{Price: 99, Size: 5}}, nil))
		row := agg.CloseBucket()
		if row[IdxSuspect] != 1 {
			t.Error("a one-sided book must mark the bucket suspect")
		}
		if row[IdxDepthStart] != 0 {
			t.Errorf("depth = %v, want 0 when no band could be defined", row[IdxDepthStart])
		}
	})
}

func TestLotDiscretisation(t *testing.T) {
	t.Run("volumes convert to whole lots", func(t *testing.T) {
		agg, book := NewAggregator(0.5, testEdges), standardBook()
		agg.OpenBucket(book)
		step(agg, book, DepthUpdate{Bids: []Level{{Price: 99, Size: 5}}}) // +2 volume
		row := agg.CloseBucket()
		if row[IdxLimit] != 4 {
			t.Errorf("arrivals = %v lots, want 4 (2 volume at 0.5 per lot)", row[IdxLimit])
		}
		if row[IdxBidTouch] != 6 {
			t.Errorf("touch = %v lots, want 6 (3 volume at 0.5 per lot)", row[IdxBidTouch])
		}
	})

	t.Run("rounding happens once at close, not per update", func(t *testing.T) {
		// Rounding each delta as it arrived would bias the total by an amount
		// proportional to the number of updates — worst exactly when the market is
		// busiest. Ten arrivals of 0.4 lots are 4 lots, not 0.
		agg, book := NewAggregator(1, testEdges), standardBook()
		agg.OpenBucket(book)
		size := 3.0
		for range 10 {
			size += 0.4
			step(agg, book, DepthUpdate{Bids: []Level{{Price: 99, Size: size}}})
		}
		if row := agg.CloseBucket(); row[IdxLimit] != 4 {
			t.Errorf("arrivals = %v, want 4", row[IdxLimit])
		}
	})

	t.Run("counts are never negative", func(t *testing.T) {
		agg := NewAggregator(1, testEdges)
		agg.OpenBucket(bookAt(nil, nil))
		row := agg.CloseBucket()
		for i, v := range row {
			if v < 0 {
				t.Errorf("row[%d] = %v, must not be negative", i, v)
			}
		}
	})
}

func TestSuspectPropagation(t *testing.T) {
	// the marking must to reach the calibration rather than a log line.
	t.Run("marking reaches the row", func(t *testing.T) {
		agg := NewAggregator(1, testEdges)
		agg.OpenBucket(standardBook())
		if agg.CloseBucket()[IdxSuspect] != 0 {
			t.Fatal("a clean bucket must not be flagged")
		}
		agg.OpenBucket(standardBook())
		agg.MarkSuspect()
		if agg.CloseBucket()[IdxSuspect] != 1 {
			t.Error("a marked bucket must carry the flag into the row")
		}
	})

	t.Run("the flag does not leak into the next bucket", func(t *testing.T) {
		// A gap must taint the interval it touched, not everything after it, or the
		// whole segment becomes unusable after a single reconnect.
		agg := NewAggregator(1, testEdges)
		agg.OpenBucket(standardBook())
		agg.MarkSuspect()
		agg.CloseBucket()
		agg.OpenBucket(standardBook())
		if agg.CloseBucket()[IdxSuspect] != 0 {
			t.Error("the suspect flag must reset when a new bucket opens")
		}
	})
}
