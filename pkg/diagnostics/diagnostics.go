// Package diagnostics holds the Spike 2.2 residual measurements, shared by every
// market they are run against.
//
// It exists so results measured on different segments are LIKE-FOR-LIKE by construction
// rather than by inspection. The two markets differ in how their rows are produced
// — pkg/feed infers counts from depth diffs and discretises volume into lots,
// an order-level feed would count messages directly — but once a row exists the
// measurement applied to it is literally the same code. A difference in the numbers
// is then a difference in the markets, not in two hand-written analyses that drifted.
package diagnostics

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/umbralcalc/cryptobook/pkg/feed"
)

// Segment is a recorded feed segment loaded into columns.
type Segment struct {
	Rows [][]float64
}

// LoadSegment reads a recorded segment, keeping only trustworthy rows.
//
// SUSPECT ROWS ARE DROPPED HERE. This is the far end of Spike 3.1's requirement
// that a sequence gap "propagate into the calibration" rather than being logged and
// forgotten: the collector marks the interval, the marking rides in column 10, and
// this is where it changes what gets analysed. A gap therefore costs the intervals
// it touched and nothing else.
func LoadSegment(path string) (Segment, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return Segment{}, 0, fmt.Errorf("diagnostics: opening segment: %w", err)
	}
	defer file.Close()

	segment := Segment{}
	dropped := 0
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var entry struct {
			State []float64 `json:"state"`
		}
		if err := decoder.Decode(&entry); err != nil {
			return Segment{}, 0, fmt.Errorf("diagnostics: decoding segment: %w", err)
		}
		if len(entry.State) != feed.RowWidth {
			return Segment{}, 0, fmt.Errorf(
				"diagnostics: row width %d, expected %d", len(entry.State), feed.RowWidth)
		}
		if entry.State[feed.IdxSuspect] != 0 {
			dropped++
			continue
		}
		segment.Rows = append(segment.Rows, entry.State)
	}
	if len(segment.Rows) == 0 {
		return Segment{}, dropped, fmt.Errorf("diagnostics: segment has no trustworthy rows")
	}
	return segment, dropped, nil
}

// column extracts one state index across the segment.
func (s Segment) Column(index int) []float64 {
	out := make([]float64, len(s.Rows))
	for i, row := range s.Rows {
		out[i] = row[index]
	}
	return out
}

func Mean(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

// dispersion returns Var/Mean, which a Poisson observable must have equal to 1.
func Dispersion(values []float64) float64 {
	m := Mean(values)
	if m == 0 {
		return 0
	}
	variance := 0.0
	for _, v := range values {
		variance += (v - m) * (v - m)
	}
	return variance / float64(len(values)) / m
}

// correlation returns Pearson's r.
func Correlation(x, y []float64) float64 {
	mx, my := Mean(x), Mean(y)
	var sxy, sxx, syy float64
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}
