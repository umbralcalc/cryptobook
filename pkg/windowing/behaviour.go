// Package windowing measures how calibration behaves as the window it runs over
// gets longer. It exists because of Gate 3.4.
//
// The gate asks whether a continuously-running streaming calibration forces
// inference into the engine. The architecture that says no is windowed re-runs: a
// collector writes a segment, the engine reads a FINISHED window through the
// ordinary `data:` tier, calibration runs, repeat. cfg/lob_calibrate_from_log.yaml
// is that shape, and it needs no engine change at all.
//
// The obvious objection is throughput — can repeated re-runs keep up with arriving
// data? Measured, that is not close: ~1100 rows per compute-second, essentially
// flat in window size. The measurement is recorded in DECISIONS.md rather than as
// a claim here, because a wall-clock number is machine-dependent and would change
// on every run, which is precisely what the claim mechanism forbids.
//
// What this package pins instead is the thing the throughput measurement turned up
// on the way, which matters more:
//
//	rows    peak ESS    worst z
//	 200       26.0       0.73
//	 400       13.6       1.68
//	 800        7.8       3.00
//	1600        4.9       7.34
//
// ESS HALVES AS THE WINDOW DOUBLES, and the posterior's overconfidence tracks it.
// More data sharpens the likelihood, which widens the log-likelihood gaps between
// particles, which degenerates the weights — and SMC then fits its proposal
// covariance to those degenerate weights, so the posterior narrows faster than the
// mean converges. The point estimates stay fine throughout (2–5%); it is the
// uncertainty that rots.
//
// This is the same pathology as
// raising_smc_rounds_trades_calibration_for_point_accuracy, in its general form:
// ESS is the governing quantity, and anything that sharpens the likelihood — more
// rows, more rounds — degrades it. Two consequences worth carrying forward:
//
//   - For Gate 3.4, short windows are not a compromise forced by streaming. For
//     this sampler they are BETTER CALIBRATED than one long batch run, so the
//     windowed architecture is favoured on statistical grounds and not merely
//     permitted on architectural ones.
//   - For Phase 2, calibrating a long real-data window in one pass would land in the
//     overconfident regime. Either keep windows short or raise the particle count;
//     do not simply feed it more data and trust the interval.
package windowing

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

const (
	generatorConfig   = "lob_generator.yaml"
	calibrationConfig = "lob_calibrate_from_log.yaml"

	phase   = "1 — Synthetic parameter recovery"
	dataset = "synthetic — a recorded segment of the model's own generated order " +
		"flow, read back through the engine's json_log source"

	// calibrationCeiling mirrors pkg/recovery's: the truth must lie within this many
	// posterior standard deviations of the posterior mean for the interval to be
	// usable.
	calibrationCeiling = 2.0
)

// trueRates must match cfg/lob_generator.yaml's parameters — pinned by
// TestWindowingTruthMatchesTheConfig.
var trueRates = [3]float64{1.2, 0.15, 0.8}

// windowRows are the window lengths measured. Doubling each time makes the
// halving of ESS legible directly from the recorded numbers.
var windowRows = []int{200, 400, 800, 1600}

// recordSegment generates a segment of the given length and writes it as a
// newline-delimited JSON log, so the calibration reads it back through the engine's
// ordinary file source rather than from memory. That round trip is the point: it is
// the same path a collector-written segment would take.
func recordSegment(dir string, rows int) (string, error) {
	storage, err := cfgrun.Run(generatorConfig, cfgrun.Subs{
		"max_steps: 200": fmt.Sprintf("max_steps: %d", rows),
	})
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("segment_%d.log", rows))
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	times := storage.GetTimes()
	encoder := json.NewEncoder(file)
	for i, state := range storage.GetValues("lob_flow") {
		if err := encoder.Encode(simulator.JsonLogEntry{
			PartitionName:       "lob_flow",
			State:               state,
			CumulativeTimesteps: times[i],
		}); err != nil {
			return "", err
		}
	}
	return path, nil
}

// windowResult is one window length's outcome.
type windowResult struct {
	peakESS float64
	// worstSigma is the largest distance from posterior mean to truth, in posterior
	// standard deviations, over the three rates.
	worstSigma float64
}

// calibrateWindow runs the recorded-segment calibration over one window.
func calibrateWindow(segment string) (windowResult, error) {
	storage, err := cfgrun.Run(calibrationConfig, cfgrun.Subs{
		"json_log: {path: RECORDED_SEGMENT_PATH}": fmt.Sprintf(
			"json_log: {path: %s}", segment),
	})
	if err != nil {
		return windowResult{}, err
	}
	posterior, err := cfgrun.LastRow(storage, "smc_posterior")
	if err != nil {
		return windowResult{}, err
	}
	const d = 3
	if want := d + d*d + 1; len(posterior) != want {
		return windowResult{}, fmt.Errorf(
			"windowing: posterior width %d, expected %d", len(posterior), want)
	}
	result := windowResult{}
	for i := range d {
		variance := posterior[d+i*d+i]
		if variance <= 0 {
			return windowResult{}, fmt.Errorf(
				"windowing: posterior variance %d is not positive (%g)", i, variance)
		}
		sigma := math.Abs(posterior[i]-trueRates[i]) / math.Sqrt(variance)
		result.worstSigma = math.Max(result.worstSigma, sigma)
	}
	for _, row := range storage.GetValues("smc_particles") {
		if ess, ok := effectiveSampleSizeOf(row); ok {
			result.peakESS = math.Max(result.peakESS, ess)
		}
	}
	if result.peakESS == 0 {
		return windowResult{}, fmt.Errorf("windowing: no scored SMC rounds found")
	}
	return result, nil
}

// effectiveSampleSizeOf computes (sum w)^2 / sum w^2 for one round's particle
// log-likelihoods, reporting false for the all-zero pre-first-round row.
func effectiveSampleSizeOf(loglikes []float64) (float64, bool) {
	scored := make([]float64, 0, len(loglikes))
	allZero := true
	for _, loglike := range loglikes {
		if math.IsNaN(loglike) || math.IsInf(loglike, -1) {
			continue
		}
		if loglike != 0 {
			allZero = false
		}
		scored = append(scored, loglike)
	}
	if len(scored) == 0 || allZero {
		return 0, false
	}
	// Normalised by the max before exponentiating: these loglikes reach the
	// thousands, so exp() of them underflows and the ESS would come out 0/0.
	best := scored[0]
	for _, loglike := range scored {
		best = math.Max(best, loglike)
	}
	sum, sumSquares := 0.0, 0.0
	for _, loglike := range scored {
		weight := math.Exp(loglike - best)
		sum += weight
		sumSquares += weight * weight
	}
	return sum * sum / sumSquares, true
}

// ObservedBehaviour measures the window-length claims.
func ObservedBehaviour() []claims.Claim {
	set, err := observedBehaviour()
	if err != nil {
		panic("windowing: measuring observed behaviour: " + err.Error())
	}
	return set
}

func observedBehaviour() ([]claims.Claim, error) {
	dir, err := os.MkdirTemp("", "cryptobook-windowing-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	binding := claims.Binding{
		TestName: "TestWindowLengthExpectedBehaviour",
		TestFile: "pkg/windowing/behaviour_test.go",
	}
	essObs := make([]claims.Observation, 0, len(windowRows))
	sigmaObs := make([]claims.Observation, 0, len(windowRows))
	for _, rows := range windowRows {
		segment, err := recordSegment(dir, rows)
		if err != nil {
			return nil, err
		}
		result, err := calibrateWindow(segment)
		if err != nil {
			return nil, err
		}
		label := fmt.Sprintf("%d rows", rows)
		essObs = append(essObs,
			claims.Observation{Label: label, Value: result.peakESS})
		sigmaObs = append(sigmaObs,
			claims.Observation{Label: label, Value: result.worstSigma})
	}

	last := len(windowRows) - 1
	return []claims.Claim{
		{
			ID: "effective_sample_size_falls_as_the_calibration_window_lengthens",
			Statement: fmt.Sprintf(
				"Effective sample size roughly halves each time the calibration window "+
					"doubles, from %d rows to %d. More data sharpens the likelihood, which "+
					"widens the log-likelihood gaps between particles, which degenerates "+
					"the weights — so the sampler gets worse at exactly the point more "+
					"evidence should be making it better.",
				windowRows[0], windowRows[last]),
			Gate:  "3.4",
			Phase: phase,
			Data:  dataset,
			Unit: "best per-round effective sample size out of 160 particles, " +
				"calibrating a recorded segment of the stated length",
			Limitations: "Measured on synthetic flow with one seed per window length, " +
				"so the halving is a clear trend rather than a fitted rate. It says " +
				"nothing about how ESS would behave against real message data, where the " +
				"likelihood is misspecified and may not sharpen the same way.",
			Monotone:     -1,
			Observations: essObs,
			Binding:      binding,
		},
		{
			ID: "posterior_overconfidence_grows_as_the_calibration_window_lengthens",
			Statement: fmt.Sprintf(
				"Because SMC fits its proposal covariance to the degenerating weights, "+
					"the posterior narrows faster than the mean converges — so a LONGER "+
					"window produces a more confident and less correct interval. The truth "+
					"sits comfortably inside %.0f posterior sd at %d rows and far outside "+
					"it at %d. The point estimates stay good throughout (2-5%%); it is "+
					"only the uncertainty that rots.",
				calibrationCeiling, windowRows[0], windowRows[last]),
			Gate:  "3.4",
			Phase: phase,
			Data:  dataset,
			Unit: "distance from posterior mean to truth in posterior standard " +
				"deviations, worst of the three rates",
			Limitations: "A pinned warning, not a result to build on. Two things follow " +
				"and neither is optional: Phase 2 must not calibrate one long window and " +
				"trust the interval, and " +
				"[[smc_posterior_uncertainty_is_calibrated]] holds at the window length " +
				"it was measured at, not in general. If a change makes long windows safe, " +
				"this claim's assertion breaks and it must be retired.",
			Monotone: 1,
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: calibrationCeiling,
					RefLabel: fmt.Sprintf("%.0f sd", calibrationCeiling)},
				{ObsIndex: last, GreaterThan: true, Ref: 2 * calibrationCeiling,
					RefLabel: fmt.Sprintf("%.0f sd", 2*calibrationCeiling)},
			},
			Observations: sigmaObs,
			Binding:      binding,
		},
	}, nil
}
