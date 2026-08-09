// Package oospool scores the pooled out-of-sample test for the frozen best model,
// cfg/lob_counts.yaml, across whole occasions — the BS-BV / CD-CG protocol.
//
// # What it does, and why it is not in CLAIMS.md
//
// Each occasion is three 8-minute windows, five symbols concurrently, recorded as
// dat/<occasion>w<window>_<symbol>.log. The occasion's value for a correlation is the mean
// over its windows of the five-symbol mean; a window with any suspect or gapped row in any
// symbol is excluded whole. The frozen model's three reference values and the three
// tolerances are fixed in PREREGISTRATION.md before each occasion is recorded, and this
// package only re-applies them.
//
// The segments are Binance data the licence does not permit redistributing, so they are
// git-ignored. This package is therefore NOT registered in internal/claimset and its numbers
// live in DECISIONS.md, exactly as pkg/oos does — anyone can record their own occasion with
// cmd/record-feed and re-run TestOccasions. It SKIPS when the segments are absent rather than
// inventing a pass.
package oospool

import (
	"fmt"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
	"github.com/umbralcalc/cryptobook/pkg/feed"
	"math"
	"os"
	"path/filepath"
	"runtime"
)

// The frozen model reference values and 1.5-between-occasion-SD tolerances, fixed at BS-BV
// (2026-08-08) and re-used verbatim for every later occasion. Not re-estimated — see the
// CD-CG pre-registration.
const (
	modelArrival = -0.1832
	modelCancel  = -0.0752
	modelCoMove  = 0.9154
	boundArrival = 0.109
	boundCancel  = 0.122
	boundCoMove  = 0.046
)

var symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT"}
var windows = []int{1, 2, 3}

func datDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("oospool: cannot locate this package's source path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "dat")
}
func segmentPath(occasion string, window int, symbol string) string {
	return filepath.Join(datDir(), fmt.Sprintf("%sw%d_%s.log", occasion, window, symbol))
}

// Available reports whether every segment of an occasion is present. All or nothing: a
// partial occasion would silently change which windows the mean is taken over.
func Available(occasion string) bool {
	for _, w := range windows {
		for _, s := range symbols {
			if _, err := os.Stat(segmentPath(occasion, w, s)); err != nil {
				return false
			}
		}
	}
	return true
}

type windowResult struct {
	window                      int
	arrival, cancel, coMovement float64
	rows                        int
	excluded                    bool
	reason                      string
}

// OccasionResult is one occasion's pooled outcome.
type OccasionResult struct {
	occasion                    string
	windows                     []windowResult
	arrival, cancel, coMovement float64 // occasion means over usable windows
	usable                      int
}

// measureWindow computes one window's five-symbol mean of each correlation, and whether the
// window is clean. A window is excluded whole if any symbol dropped a suspect or gapped row.
func measureWindow(occasion string, window int) (windowResult, error) {
	wr := windowResult{window: window}
	var arr, can, com []float64
	minRows := math.MaxInt32
	for _, s := range symbols {
		seg, dropped, err := diagnostics.LoadSegment(segmentPath(occasion, window, s))
		if err != nil {
			return wr, fmt.Errorf("%s window %d %s: %w", occasion, window, s, err)
		}
		if dropped > 0 {
			wr.excluded = true
			wr.reason = fmt.Sprintf("%s dropped %d suspect/gapped rows", s, dropped)
		}
		depth := seg.Column(feed.IdxDepthStart)
		limit := seg.Column(feed.IdxLimit)
		cancel := seg.Column(feed.IdxCancel)
		arr = append(arr, diagnostics.Correlation(depth, limit))
		can = append(can, diagnostics.Correlation(depth, cancel))
		com = append(com, diagnostics.Correlation(limit, cancel))
		if len(depth) < minRows {
			minRows = len(depth)
		}
	}
	wr.arrival = diagnostics.Mean(arr)
	wr.cancel = diagnostics.Mean(can)
	wr.coMovement = diagnostics.Mean(com)
	wr.rows = minRows
	return wr, nil
}

// Score pools an occasion per the protocol: the occasion value is the mean over USABLE
// windows of the five-symbol mean.
func Score(occasion string) (OccasionResult, error) {
	r := OccasionResult{occasion: occasion}
	var arr, can, com []float64
	for _, w := range windows {
		wr, err := measureWindow(occasion, w)
		if err != nil {
			return r, err
		}
		r.windows = append(r.windows, wr)
		if wr.excluded {
			continue
		}
		arr = append(arr, wr.arrival)
		can = append(can, wr.cancel)
		com = append(com, wr.coMovement)
	}
	r.usable = len(arr)
	if r.usable == 0 {
		return r, fmt.Errorf("oospool: %s has no usable windows", occasion)
	}
	r.arrival = diagnostics.Mean(arr)
	r.cancel = diagnostics.Mean(can)
	r.coMovement = diagnostics.Mean(com)
	return r, nil
}

// verdict is the pass/fail against the frozen bounds, with the distance in tolerance units.
type verdict struct {
	name              string
	model, occ, bound float64
}

func (v verdict) gap() float64   { return math.Abs(v.model - v.occ) }
func (v verdict) pass() bool     { return v.gap() < v.bound }
func (v verdict) sigma() float64 { return v.gap() / (v.bound / 1.5) }

// Verdicts returns CD/CE/CF for an occasion result.
func (r OccasionResult) Verdicts() []verdict {
	return []verdict{
		{"arrival (CD)", modelArrival, r.arrival, boundArrival},
		{"cancel (CE)", modelCancel, r.cancel, boundCancel},
		{"co-movement (CF)", modelCoMove, r.coMovement, boundCoMove},
	}
}
