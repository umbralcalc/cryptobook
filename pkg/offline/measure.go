package offline

import (
	"encoding/json"
	"fmt"
	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"math"
	"os"
	"path/filepath"
)

const (
	generator   = "lob_churn_flow.yaml"
	calibration = "lob_churn_calibrate.yaml"
	partition   = "lob_flow"
	segLen      = 400
	nParams     = 3
)

// truth is the generator's parameters. phi = Var(act)/E(act)^2 = 105/16.
var (
	truth      = []float64{3.381, 1.900, 6.5625}
	paramNames = []string{"limit_rate", "churn_rate", "phi"}
	// priorSD of each uniform prior, for the "did the data move it" comparison.
	priorSD = []float64{
		(6.0 - 0.5) / math.Sqrt(12), (4.0 - 0.3) / math.Sqrt(12), (20.0 - 0.5) / math.Sqrt(12),
	}
	// genSeeds is the ensemble of recorded segments. Fixed list, varied only in the
	// generator seed, so each is an independent draw of the same process.
	genSeeds = []string{"20260830", "20260901", "20260902", "20260903", "20260904", "20260905"}
)

// fit is one calibration's posterior for one segment.
type fit struct {
	mean, sd []float64
	peakESS  float64
}

func recordSegment(dir, seed string) (string, error) {
	storage, err := cfgrun.Run(generator, cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", segLen),
		"seed: 20260830": "seed: " + seed,
	})
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "seg_"+seed+".log")
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	times := storage.GetTimes()
	encoder := json.NewEncoder(file)
	for i, state := range storage.GetValues(partition) {
		if err := encoder.Encode(simulator.JsonLogEntry{
			PartitionName: partition, State: state, CumulativeTimesteps: times[i],
		}); err != nil {
			return "", err
		}
	}
	return path, nil
}
func calibrate(segment, likelihood string) (fit, error) {
	storage, err := cfgrun.Run(calibration, cfgrun.Subs{
		"json_log: {path: RECORDED_SEGMENT_PATH}": fmt.Sprintf("json_log: {path: %s}", segment),
		"{type: LIKELIHOOD_TYPE}":                 fmt.Sprintf("{type: %s}", likelihood),
	})
	if err != nil {
		return fit{}, err
	}
	post, err := cfgrun.LastRow(storage, "smc_posterior")
	if err != nil {
		return fit{}, err
	}
	if want := nParams + nParams*nParams + 1; len(post) != want {
		return fit{}, fmt.Errorf("offline: posterior width %d, expected %d", len(post), want)
	}
	f := fit{mean: make([]float64, nParams), sd: make([]float64, nParams)}
	for i := 0; i < nParams; i++ {
		f.mean[i] = post[i]
		v := post[nParams+i*nParams+i]
		if v <= 0 {
			return fit{}, fmt.Errorf("offline: posterior variance %d not positive (%g)", i, v)
		}
		f.sd[i] = math.Sqrt(v)
	}
	for _, row := range storage.GetValues("smc_particles") {
		if ess, ok := effectiveSampleSize(row); ok {
			f.peakESS = math.Max(f.peakESS, ess)
		}
	}
	return f, nil
}

// effectiveSampleSize is (sum w)^2 / sum w^2 over one round's particle loglikes, with the
// max-normalisation windowing uses (loglikes reach the thousands and exp() underflows).
func effectiveSampleSize(loglikes []float64) (float64, bool) {
	scored := make([]float64, 0, len(loglikes))
	allZero := true
	for _, l := range loglikes {
		if math.IsNaN(l) || math.IsInf(l, -1) {
			continue
		}
		if l != 0 {
			allZero = false
		}
		scored = append(scored, l)
	}
	if len(scored) == 0 || allZero {
		return 0, false
	}
	best := scored[0]
	for _, l := range scored {
		best = math.Max(best, l)
	}
	sum, sumSq := 0.0, 0.0
	for _, l := range scored {
		w := math.Exp(l - best)
		sum += w
		sumSq += w * w
	}
	if sumSq == 0 {
		return 0, false
	}
	return sum * sum / sumSq, true
}
func mean(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}
