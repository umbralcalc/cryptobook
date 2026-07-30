// Command record-feed records a segment of an exchange's order-book feed as a
// newline-delimited JSON log that cfg/lob_calibrate_from_log.yaml can calibrate.
//
//	go run ./cmd/record-feed -symbol BTCUSDT -duration 8m -out dat/btcusdt.log
//
// It is deliberately thin. Everything that decides whether the data is trustworthy
// — the sequence contract, the cancellation/execution split, the suspect flag —
// lives in pkg/feed, which needs no network and is tested exhaustively. This file
// is a websocket, a REST call, and a clock, so that reading it is enough to see
// that it adds no judgement of its own.
//
// # Offline only, on purpose
//
// This records; it does not calibrate. PLAN.md's Gate 3.4 (who owns a live
// calibration loop) is open and reserved for the maintainer, and record-then-replay
// sits inside every branch of it — including the deferred one, which scopes Phase 3
// to "offline calibration on recorded streams only". Nothing here should grow a
// calibration step without that gate being resolved first.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/umbralcalc/cryptobook/pkg/feed"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "exchange symbol to record")
	duration := flag.Duration("duration", 8*time.Minute, "how long to record")
	bucket := flag.Duration("bucket", time.Second, "one state row per this interval")
	lotSize := flag.Float64("lot", feed.DefaultLotSize, "volume treated as one unit event")
	out := flag.String("out", "dat/feed.log", "output json_log path")
	flag.Parse()

	if err := record(*symbol, *duration, *bucket, *lotSize, *out); err != nil {
		log.Fatalf("record-feed: %v", err)
	}
}

func record(symbol string, duration, bucket time.Duration, lotSize float64, out string) error {
	file, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("creating %s: %w", out, err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)

	client := &http.Client{Timeout: 30 * time.Second}
	recorder := feed.NewRecorder(bucket, lotSize, feed.DefaultBandEdgesBP)

	// Connect BEFORE snapshotting. The documented procedure requires buffering
	// events while the snapshot is fetched — snapshot first and the events covering
	// the gap between them are simply gone, which surfaces as ErrStaleSnapshot
	// rather than as corruption, but wastes a reconnect every time.
	connection, _, err := websocket.DefaultDialer.Dial(feed.StreamURL(symbol), nil)
	if err != nil {
		return fmt.Errorf("dialling the stream: %w", err)
	}
	defer connection.Close()

	messages := make(chan feed.Message, 4096)
	readErrors := make(chan error, 1)
	go readLoop(connection, messages, readErrors)

	if err := resync(client, symbol, recorder); err != nil {
		return err
	}

	// Ctrl-C writes out what has been recorded so far rather than discarding it.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(bucket / 10)
	defer ticker.Stop()
	deadline := time.Now().Add(duration)
	recorder.Tick(time.Now())

	rows, suspect := 0, 0
	for {
		select {
		case err := <-readErrors:
			return fmt.Errorf("stream read: %w", err)

		case <-interrupt:
			log.Printf("interrupted; wrote %d rows (%d suspect)", rows, suspect)
			return nil

		case message := <-messages:
			switch {
			case message.Depth != nil:
				if recorder.OnDepth(*message.Depth) {
					// A gap. Resynchronise immediately; the buckets spanning this stay
					// marked suspect either way.
					log.Printf("sequence gap (%d so far) — resynchronising", recorder.Gaps())
					if err := resync(client, symbol, recorder); err != nil {
						return err
					}
				}
			case message.Trade != nil:
				recorder.OnTrade(*message.Trade)
			}

		case now := <-ticker.C:
			row, ok := recorder.Tick(now)
			if ok {
				if err := encoder.Encode(simulator.JsonLogEntry{
					PartitionName:       "lob_flow",
					State:               row,
					CumulativeTimesteps: float64(rows),
					// Time is the ROW INDEX, not a wall-clock stamp. The model's dt is one
					// bucket, and cfg/lob_calibrate_from_log.yaml drives the inner
					// simulation from these timestamps — feeding it epoch milliseconds
					// would make dt about 1e12 and every rate meaningless.
				}); err != nil {
					return fmt.Errorf("writing row: %w", err)
				}
				rows++
				if row[feed.IdxSuspect] != 0 {
					suspect++
				}
				if rows%60 == 0 {
					log.Printf("%d rows (%d suspect, %d gaps)", rows, suspect, recorder.Gaps())
				}
			}
			if now.After(deadline) {
				log.Printf("done: %d rows, %d suspect, %d gaps", rows, suspect, recorder.Gaps())
				if rows == 0 {
					return fmt.Errorf("recorded no rows")
				}
				return nil
			}
		}
	}
}

// readLoop decodes frames until the connection fails.
func readLoop(connection *websocket.Conn, messages chan<- feed.Message, errs chan<- error) {
	for {
		_, raw, err := connection.ReadMessage()
		if err != nil {
			errs <- err
			return
		}
		message, err := feed.ParseMessage(raw)
		if err != nil {
			// A malformed frame of a type we claim to handle is not survivable: skipping
			// it drops real updates, which is exactly the invisible corruption the
			// sequence check exists to prevent.
			errs <- err
			return
		}
		if message.Depth == nil && message.Trade == nil {
			continue
		}
		select {
		case messages <- message:
		default:
			// A full buffer means frames are being dropped, which breaks the sequence
			// chain. Fail rather than silently losing updates.
			errs <- fmt.Errorf("message buffer overflow — updates would be dropped")
			return
		}
	}
}

// resync fetches the deepest available snapshot and hands it to the recorder.
func resync(client *http.Client, symbol string, recorder *feed.Recorder) error {
	snapshot, err := feed.FetchSnapshot(client, symbol, 5000)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	recorder.Resync(snapshot)
	return nil
}
