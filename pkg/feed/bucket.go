package feed

import "math"

// This file is the state spine: it turns an exchange's diff-update and trade
// streams into the same row shape cfg/lob_generator.yaml produces, so the existing
// calibration config consumes real market data with no change.
//
// It is the crypto form of Spike 2.1 ("message format to state spine"),
// and the mapping decisions live here rather than in a document because each one is
// a modelling choice the fitted numbers depend on.
//
// ── DECISION 1: flow is measured at ABSOLUTE PRICES, never at offsets ─────────
//
// The tempting alternative is to track "the queue at the best bid", since the model
// has a static ladder and the market does not. It is wrong, and wrong invisibly.
// When the best bid ticks up, the old offset-0 queue becomes offset-1 and a
// different queue appears at offset-0 — which reads as an enormous arrival and an
// enormous cancellation, neither of which happened. Every price movement would be
// laundered into order flow and the fitted rates would be measuring volatility.
//
// The band is therefore re-derived at each bucket open and then FROZEN: within a
// bucket every delta is against a fixed absolute price.
//
// ── DECISION 2: volume is discretised into LOTS, and lots stand in for counts ──
//
// The model's observables are Poisson counts. A diff stream cannot supply them: it
// reports net size change per level, so one 5-lot order and five 1-lot orders are
// indistinguishable. Rather than change the likelihood — discarding Phase 1's
// identification result with it — volume is divided by a lot size and the integer
// treated as the count. This is the unit-order-size idealisation the Santa Fe model
// makes, and therefore the one umbralcalc/lobsim inherits.
//
// CHOOSING THE LOT IS A MODELLING DECISION, NOT A FORMAT DETAIL. For a compound
// Poisson process (a Poisson number of orders, each of iid size S), the dispersion
// of total volume is Var/Mean = E[S²]/E[S] in lot units — so there is exactly one
// lot size at which lot-counts are Poisson-dispersed, namely E[size²]/E[size].
// Measured over 1000 BTCUSDT trades that is 0.0141 BTC. See DefaultLotSize for why
// the shipped value is round rather than that.
//
// ── DECISION 3: cancellations and executions are split by the trade stream ────
//
// A size decrease at a price is either a cancellation or a fill, and the depth
// stream cannot tell them apart. Within a bucket a decrease is attributed to
// execution up to the volume traded at that price, and the remainder to
// cancellation — the direct parallel of an order-level feed's cancel/delete messages
// against its executions.
//
// ── DECISION 4: deltas accumulate PER UPDATE, not net per bucket ──────────────
//
// Netting a bucket's open against its close erases churn: volume added and removed
// inside the same bucket cancels out, understating both arrivals and cancellations,
// and understating them MORE the busier the market is.
//
// ── DECISION 5 (REVISED): buckets are PRICE BANDS, not adjacent price levels ───
//
// The first version took the top 3 price levels per side, to match the model's six
// ladder slots one-for-one. Real data killed it immediately: BTCUSDT quotes every
// cent, so three price levels span about three cents, and the measured book was
//
//	bid0 1693.65   bid1 1.02   bid2 0.00   ask0 3130.65   ask1 0.94   ask2 0.00
//
// — the touch holding everything and four of the six slots holding dust. The ladder
// structure the model depends on simply was not there, and a calibration on it
// would have been fitting a two-slot book while claiming six.
//
// A "level" in the model is a queue at a distance from the mid. In a book quoting
// every cent, the analogue is a price BAND, not an individual tick. Buckets are
// therefore geometric bands measured in basis points of the mid — the standard way
// to coarse-grain a dense book — with the default 0–5, 5–25, 25–100 bp chosen from
// a measured snapshot so each carries comparable mass:
//
//	band (bp)     bid BTC    ask BTC
//	0-5            56.94      38.42
//	5-25           99.86     120.70
//	25-100         96.58     152.63
//
// ── WHAT THIS DOES NOT FIX ────────────────────────────────────────────────────
//
// The model assumes arrivals are UNIFORM across levels; real books concentrate them
// near the touch. That misspecification is left in deliberately: Spike 2.2 exists to
// measure where the parametric form fails, and reshaping the model beforehand to
// match what I expect would be fitting it to my expectations rather than to the data.
// The expected failures are written down in PREREGISTRATION.md instead.

// Row indices in the emitted state row. Indices 0..9 carry exactly the meanings
// cfg/lob_generator.yaml gives them, which is what lets the calibration config
// consume real data unchanged. Suspect is appended rather than inserted, for the
// same reason.
const (
	IdxBidTouch   = 0 // 0..2 are bid bands, nearest the mid first
	IdxAskTouch   = 3 // 3..5 are ask bands, nearest the mid first
	IdxLimit      = 6
	IdxCancel     = 7
	IdxMarket     = 8
	IdxDepthStart = 9
	IdxSuspect    = 10
	RowWidth      = 11

	// LevelsPerSide matches the model's ladder.
	LevelsPerSide = 3
)

// DefaultBandEdgesBP are the outer edges of the three price bands, in basis points
// of the mid. Geometric, and chosen from a measured BTCUSDT snapshot so each band
// carries comparable resting size.
var DefaultBandEdgesBP = [LevelsPerSide]float64{5, 25, 100}

// DefaultLotSize is the volume treated as one unit event, in BTC.
//
// The dispersion-matched value measured over 1000 BTCUSDT trades is 0.0141 BTC
// (see decision 2). The shipped default is the round 0.01, whose predicted
// dispersion is 1.41 rather than 1.00, because with a heavy-tailed size
// distribution E[size²] is dominated by the largest few observations — quoting four
// significant figures would be precision the estimate does not have.
const DefaultLotSize = 0.01

// Aggregator accumulates one bucket's order flow into a state row.
//
// Single-goroutine by design, like Book — the collector owns one and drives it.
type Aggregator struct {
	lotSize  float64
	edgesBP  [LevelsPerSide]float64
	rounding func(float64) float64

	// mid and edges are the band geometry, fixed at bucket open (decision 1).
	// edges holds absolute price distances from the mid.
	mid   float64
	edges [LevelsPerSide]float64

	// openLevels is resting volume per band at bucket open, in row slot order.
	openLevels [LevelsPerSide * 2]float64
	openDepth  float64

	added          float64
	removedByPrice map[float64]float64
	tradedByPrice  map[float64]float64

	suspect bool
	open    bool
}

// NewAggregator returns an aggregator using the given lot size and band edges.
func NewAggregator(lotSize float64, edgesBP [LevelsPerSide]float64) *Aggregator {
	if lotSize <= 0 {
		panic("feed: lot size must be positive")
	}
	for i := 1; i < len(edgesBP); i++ {
		if edgesBP[i] <= edgesBP[i-1] {
			panic("feed: band edges must be strictly increasing")
		}
	}
	if edgesBP[0] <= 0 {
		panic("feed: band edges must be positive")
	}
	return &Aggregator{
		lotSize:        lotSize,
		edgesBP:        edgesBP,
		removedByPrice: make(map[float64]float64),
		tradedByPrice:  make(map[float64]float64),
	}
}

// OpenBucket fixes the band geometry from the book's current touch and records the
// opening state. Any previously accumulated flow is discarded.
func (a *Aggregator) OpenBucket(book *Book) {
	a.removedByPrice = make(map[float64]float64)
	a.tradedByPrice = make(map[float64]float64)
	a.added = 0
	a.openDepth = 0
	a.openLevels = [LevelsPerSide * 2]float64{}
	a.suspect = !book.Synced()
	a.open = true

	bestBid, bestAsk := book.TopBids(1)[0], book.TopAsks(1)[0]
	if bestBid.Size <= 0 || bestAsk.Size <= 0 {
		// With no two-sided touch there is no mid, so no band can be defined. Mark the
		// bucket suspect rather than inventing a reference price.
		a.mid = 0
		a.suspect = true
		return
	}
	a.mid = (bestBid.Price + bestAsk.Price) / 2
	for i, bp := range a.edgesBP {
		a.edges[i] = bp * a.mid / 1e4
	}

	book.ForEach(true, func(price, size float64) {
		if band := a.bandOf(price); band >= 0 {
			a.openLevels[IdxBidTouch+band] += size
			a.openDepth += size
		}
	})
	book.ForEach(false, func(price, size float64) {
		if band := a.bandOf(price); band >= 0 {
			a.openLevels[IdxAskTouch+band] += size
			a.openDepth += size
		}
	})
}

// bandOf returns which band a price falls in, or -1 if it is outside the widest
// one. Distance is measured from the bucket's opening mid, so the answer cannot
// change part-way through a bucket.
func (a *Aggregator) bandOf(price float64) int {
	if a.mid <= 0 {
		return -1
	}
	distance := math.Abs(price - a.mid)
	for i, edge := range a.edges {
		if distance < edge {
			return i
		}
	}
	return -1
}

// ObserveUpdate accumulates one depth update's deltas against the band.
//
// It MUST be called before book.Apply for that same update, because it needs the
// pre-update sizes. Calling it afterwards would compare the new state against
// itself and record no flow at all — silently, since the row would still look
// well-formed.
func (a *Aggregator) ObserveUpdate(book *Book, update DepthUpdate) {
	if !a.open {
		return
	}
	a.observeSide(book, update.Bids, true)
	a.observeSide(book, update.Asks, false)
}

func (a *Aggregator) observeSide(book *Book, levels []Level, bid bool) {
	for _, level := range levels {
		if a.bandOf(level.Price) < 0 {
			continue
		}
		delta := level.Size - book.SizeAt(level.Price, bid)
		if delta > 0 {
			a.added += delta
			continue
		}
		if delta < 0 {
			a.removedByPrice[level.Price] -= delta
		}
	}
}

// ObserveTrade accumulates one trade against the band. Trades outside it are
// ignored, so the counts describe the same slice of the book as the depth flow.
func (a *Aggregator) ObserveTrade(price, size float64) {
	if !a.open || a.bandOf(price) < 0 {
		return
	}
	a.tradedByPrice[price] += size
}

// MarkSuspect flags this bucket's data as untrustworthy — a sequence gap, a
// resynchronisation, a stall, or a book with no two-sided touch.
//
// the marking must to propagate INTO the calibration rather than being
// logged and forgotten. CloseBucket writes it as a column so a calibration can
// exclude suspect intervals instead of silently trusting them.
func (a *Aggregator) MarkSuspect() { a.suspect = true }

// Suspect reports the current bucket's data-quality flag.
func (a *Aggregator) Suspect() bool { return a.suspect }

// CloseBucket emits the bucket's state row and ends the bucket.
//
// Volumes convert to lots only here, once. Rounding each delta as it arrived would
// accumulate a bias proportional to the number of updates — largest exactly when
// the market is busiest.
func (a *Aggregator) CloseBucket() []float64 {
	row := make([]float64, RowWidth)

	var cancelled float64
	for price, removed := range a.removedByPrice {
		executed := math.Min(removed, a.tradedByPrice[price])
		cancelled += removed - executed
	}
	var traded float64
	for _, size := range a.tradedByPrice {
		traded += size
	}

	for i, size := range a.openLevels {
		row[i] = a.lots(size)
	}
	row[IdxLimit] = a.lots(a.added)
	row[IdxCancel] = a.lots(cancelled)
	row[IdxMarket] = a.lots(traded)
	row[IdxDepthStart] = a.lots(a.openDepth)
	if a.suspect {
		row[IdxSuspect] = 1
	}
	a.open = false
	return row
}

// lots converts a volume to a non-negative whole number of lots.
func (a *Aggregator) lots(volume float64) float64 {
	if volume <= 0 {
		return 0
	}
	return math.Round(volume / a.lotSize)
}
