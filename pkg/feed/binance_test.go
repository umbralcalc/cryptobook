package feed

import "testing"

// Real frames captured from wss://stream.binance.com:9443/stream on 2026-07-28.
// Captured rather than hand-written on purpose: a fixture I invented would only
// prove the parser matches my reading of the format, and the bug these pin was
// invisible to exactly that kind of reasoning.
const (
	realDepthFrame = `{"stream":"btcusdt@depth@100ms","data":{"e":"depthUpdate",` +
		`"E":1785272250314,"s":"BTCUSDT","U":97959308357,"u":97959308363,` +
		`"b":[["57539.63000000","0.00420000"],["51146.00000000","0.00000000"]],` +
		`"a":[["63932.93000000","2.91574000"]]}}`
	realTradeFrame = `{"stream":"btcusdt@trade","data":{"e":"trade","E":1785272250500,` +
		`"s":"BTCUSDT","t":5432109876,"p":"63932.93000000","q":"0.00250000",` +
		`"T":1785272250499,"m":true,"M":true}}`
)

// TestCaseInsensitiveKeyCollisions is a regression test for a decoding trap that
// cost a debugging cycle and would have caused silent corruption if it had not.
//
// Go's encoding/json falls back to CASE-INSENSITIVE field matching when no exact
// tag matches a key. Binance frames carry both "e" (event type, a string) and "E"
// (event time, a number), so a struct declaring only `json:"e"` also receives "E"
// — and fails with a message blaming "e" while pointing at entirely valid data.
//
// The trade frame is the dangerous version: "t" (trade id) and "T" (trade time)
// are BOTH numbers, so without an exact field for "t" it lands in the timestamp
// with no error whatsoever. That is a wrong number that looks like a right one,
// which is the failure mode this whole package exists to prevent.
func TestCaseInsensitiveKeyCollisions(t *testing.T) {
	t.Run("a depth frame with both e and E decodes", func(t *testing.T) {
		message, err := ParseMessage([]byte(realDepthFrame))
		if err != nil {
			t.Fatalf("decoding a valid depth frame failed: %v", err)
		}
		if message.Depth == nil {
			t.Fatal("expected a depth update")
		}
		if message.Depth.FirstID != 97959308357 || message.Depth.FinalID != 97959308363 {
			t.Errorf("ids = [%d, %d], want [97959308357, 97959308363]",
				message.Depth.FirstID, message.Depth.FinalID)
		}
	})

	t.Run("a trade frame takes T, not t, as the timestamp", func(t *testing.T) {
		message, err := ParseMessage([]byte(realTradeFrame))
		if err != nil {
			t.Fatalf("decoding a valid trade frame failed: %v", err)
		}
		if message.Trade == nil {
			t.Fatal("expected a trade")
		}
		// 1785272250499 is "T". 5432109876 is "t", the trade id — if it landed here
		// the timestamp would be somewhere in 1970 and nothing would have complained.
		if got := message.Trade.Time.UnixMilli(); got != 1785272250499 {
			t.Errorf("trade time = %d, want 1785272250499 — the trade id must not "+
				"land in the timestamp via case-insensitive matching", got)
		}
		if message.Trade.Price != 63932.93 || message.Trade.Size != 0.0025 {
			t.Errorf("price/size = %v/%v, want 63932.93/0.0025",
				message.Trade.Price, message.Trade.Size)
		}
	})
}

func TestParseMessageTolerance(t *testing.T) {
	t.Run("housekeeping frames are not errors", func(t *testing.T) {
		// Treating a subscription acknowledgement as a failure would make an ordinary
		// feed look broken.
		message, err := ParseMessage([]byte(`{"result":null,"id":1}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if message.Depth != nil || message.Trade != nil {
			t.Error("expected an empty message")
		}
	})

	t.Run("an unknown event type is ignored", func(t *testing.T) {
		message, err := ParseMessage(
			[]byte(`{"stream":"x","data":{"e":"kline","E":1,"k":{}}}`))
		if err != nil || message.Depth != nil || message.Trade != nil {
			t.Errorf("expected an unknown event to be ignored, got %+v / %v", message, err)
		}
	})

	t.Run("a malformed price in a handled type is an error", func(t *testing.T) {
		// The opposite policy — skipping it — would drop real updates and break the
		// sequence chain silently.
		_, err := ParseMessage([]byte(`{"stream":"x","data":{"e":"depthUpdate",` +
			`"E":1,"U":1,"u":1,"b":[["not-a-price","1"]],"a":[]}}`))
		if err == nil {
			t.Error("expected a malformed level to be rejected, not skipped")
		}
	})
}

func TestStreamURL(t *testing.T) {
	// Both streams are required: depth alone cannot separate a cancellation from a
	// fill, which is decision 3 of the state spine.
	got := StreamURL("BTCUSDT")
	want := "wss://stream.binance.com:9443/stream?streams=btcusdt@depth@100ms/btcusdt@trade"
	if got != want {
		t.Errorf("StreamURL = %q, want %q", got, want)
	}
}
