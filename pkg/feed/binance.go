package feed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// This file is the Binance-specific shell: wire formats and the REST snapshot.
// Everything that decides whether the data is trustworthy lives in book.go and
// bucket.go, which know nothing about Binance and need no network to test.
//
// The split is deliberate. Spike 3.3 warns that a live feed makes -race
// failures probabilistic rather than reproducible, so "a clean run proves less than
// it appears to". Keeping the sequence contract and the aggregation in pure,
// network-free code means the risky logic is tested deterministically and this file
// stays thin enough to inspect by reading.

// Endpoints. Spot, public, no authentication — this is read-only market data.
const (
	binanceREST   = "https://api.binance.com/api/v3/depth"
	binanceStream = "wss://stream.binance.com:9443/stream"
)

// StreamURL returns the combined-stream URL for a symbol's diff-depth and trade
// feeds. Both are needed: depth alone cannot separate a cancellation from a fill
// (see decision 3 in bucket.go).
func StreamURL(symbol string) string {
	lower := lowerASCII(symbol)
	return fmt.Sprintf("%s?streams=%s@depth@100ms/%s@trade", binanceStream, lower, lower)
}

// lowerASCII lowercases a symbol without pulling in locale-dependent casing.
func lowerASCII(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 32
		}
	}
	return string(out)
}

// wireLevel is Binance's ["price", "quantity"] pair, both as decimal strings.
type wireLevel [2]string

func (w wireLevel) level() (Level, error) {
	price, err := strconv.ParseFloat(w[0], 64)
	if err != nil {
		return Level{}, fmt.Errorf("feed: bad price %q: %w", w[0], err)
	}
	size, err := strconv.ParseFloat(w[1], 64)
	if err != nil {
		return Level{}, fmt.Errorf("feed: bad size %q: %w", w[1], err)
	}
	return Level{Price: price, Size: size}, nil
}

func levels(wire []wireLevel) ([]Level, error) {
	out := make([]Level, 0, len(wire))
	for _, w := range wire {
		level, err := w.level()
		if err != nil {
			return nil, err
		}
		out = append(out, level)
	}
	return out, nil
}

// Snapshot is a REST order-book snapshot.
type Snapshot struct {
	LastUpdateID int64
	Bids         []Level
	Asks         []Level
}

// FetchSnapshot retrieves a depth snapshot. limit must be one of Binance's
// accepted values; 5000 is the deepest and is what a resync should use, since a
// shallow snapshot can be overtaken before it is applied.
func FetchSnapshot(client *http.Client, symbol string, limit int) (Snapshot, error) {
	url := fmt.Sprintf("%s?symbol=%s&limit=%d", binanceREST, symbol, limit)
	response, err := client.Get(url)
	if err != nil {
		return Snapshot{}, fmt.Errorf("feed: snapshot request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf(
			"feed: snapshot returned HTTP %d", response.StatusCode)
	}
	var body struct {
		LastUpdateID int64       `json:"lastUpdateId"`
		Bids         []wireLevel `json:"bids"`
		Asks         []wireLevel `json:"asks"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return Snapshot{}, fmt.Errorf("feed: decoding snapshot: %w", err)
	}
	bids, err := levels(body.Bids)
	if err != nil {
		return Snapshot{}, err
	}
	asks, err := levels(body.Asks)
	if err != nil {
		return Snapshot{}, err
	}
	if body.LastUpdateID == 0 {
		return Snapshot{}, fmt.Errorf("feed: snapshot carried no lastUpdateId")
	}
	return Snapshot{LastUpdateID: body.LastUpdateID, Bids: bids, Asks: asks}, nil
}

// Trade is one executed trade.
type Trade struct {
	Price float64
	Size  float64
	Time  time.Time
}

// Message is one decoded combined-stream frame: exactly one of Depth or Trade is
// non-nil.
type Message struct {
	Depth *DepthUpdate
	Trade *Trade
}

// combinedFrame is the envelope the combined-stream endpoint wraps events in.
type combinedFrame struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

// ParseMessage decodes one combined-stream frame.
//
// An unrecognised event type returns a zero Message and no error: Binance sends
// housekeeping frames, and treating them as failures would make an ordinary feed
// look broken. A MALFORMED frame of a type we do claim to handle is an error,
// because silently skipping it would drop real updates and produce exactly the
// invisible corruption the sequence check exists to prevent.
func ParseMessage(raw []byte) (Message, error) {
	var frame combinedFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return Message{}, fmt.Errorf("feed: decoding frame: %w", err)
	}
	if len(frame.Data) == 0 {
		return Message{}, nil
	}
	// EventTime is declared even though it is unused. Go's JSON decoder falls back
	// to CASE-INSENSITIVE field matching, so the frame's "E" (event time, a number)
	// also matches an "e"-tagged field — decoding fails with a message that blames
	// "e" while pointing at valid data. Declaring "E" gives it an exact match to
	// land in. Every Binance key that differs from one of ours only by case needs
	// the same treatment; see the trade case below, where the collision is silent
	// rather than loud.
	var header struct {
		Event     string `json:"e"`
		EventTime int64  `json:"E"`
	}
	if err := json.Unmarshal(frame.Data, &header); err != nil {
		return Message{}, fmt.Errorf("feed: decoding event header: %w", err)
	}
	switch header.Event {
	case "depthUpdate":
		var body struct {
			FirstID int64       `json:"U"`
			FinalID int64       `json:"u"`
			Bids    []wireLevel `json:"b"`
			Asks    []wireLevel `json:"a"`
		}
		if err := json.Unmarshal(frame.Data, &body); err != nil {
			return Message{}, fmt.Errorf("feed: decoding depthUpdate: %w", err)
		}
		bids, err := levels(body.Bids)
		if err != nil {
			return Message{}, err
		}
		asks, err := levels(body.Asks)
		if err != nil {
			return Message{}, err
		}
		return Message{Depth: &DepthUpdate{
			FirstID: body.FirstID, FinalID: body.FinalID, Bids: bids, Asks: asks,
		}}, nil
	case "trade":
		// TradeID exists for the same reason, and it is the more dangerous case: "t"
		// (trade id) and "T" (trade time) are BOTH numbers, so without an exact field
		// for "t" it would land in Time with no error at all — a silently wrong
		// timestamp rather than a failed decode.
		var body struct {
			Price   string `json:"p"`
			Size    string `json:"q"`
			Time    int64  `json:"T"`
			TradeID int64  `json:"t"`
		}
		if err := json.Unmarshal(frame.Data, &body); err != nil {
			return Message{}, fmt.Errorf("feed: decoding trade: %w", err)
		}
		price, err := strconv.ParseFloat(body.Price, 64)
		if err != nil {
			return Message{}, fmt.Errorf("feed: bad trade price %q: %w", body.Price, err)
		}
		size, err := strconv.ParseFloat(body.Size, 64)
		if err != nil {
			return Message{}, fmt.Errorf("feed: bad trade size %q: %w", body.Size, err)
		}
		return Message{Trade: &Trade{
			Price: price, Size: size, Time: time.UnixMilli(body.Time),
		}}, nil
	default:
		return Message{}, nil
	}
}
