package offline

import (
	"os"
	"sync"
)

// result is the ensemble outcome for one likelihood: per-parameter mean relative error and
// mean posterior-SD-to-prior-SD ratio, plus the mean peak ESS, averaged over segments.
type result struct {
	relErr  []float64 // per parameter, mean over segments
	sdRatio []float64 // per parameter, mean posterior SD / prior SD
	peakESS float64
	nSeg    int
}

func aggregate(likelihood string, segments []string) (result, error) {
	relErr := make([]float64, nParams)
	sdRatio := make([]float64, nParams)
	essSum := 0.0
	for _, seg := range segments {
		f, err := calibrate(seg, likelihood)
		if err != nil {
			return result{}, err
		}
		for i := 0; i < nParams; i++ {
			relErr[i] += absRel(f.mean[i], truth[i])
			sdRatio[i] += f.sd[i] / priorSD[i]
		}
		essSum += f.peakESS
	}
	n := float64(len(segments))
	for i := 0; i < nParams; i++ {
		relErr[i] /= n
		sdRatio[i] /= n
	}
	return result{relErr: relErr, sdRatio: sdRatio, peakESS: essSum / n, nSeg: len(segments)}, nil
}
func absRel(got, want float64) float64 {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d / want
}

// measured is both likelihoods' results over one shared set of recorded segments.
type measured struct {
	poisson, negbin result
}

func measureUncached(dir string) (measured, error) {
	if dir == "" {
		// So gen-claims (which calls ObservedBehaviour with no dir) writes segments to a
		// temp location rather than the repo root.
		var err error
		if dir, err = os.MkdirTemp("", "offline-seg-"); err != nil {
			return measured{}, err
		}
	}
	segments := make([]string, 0, len(genSeeds))
	for _, seed := range genSeeds {
		path, err := recordSegment(dir, seed)
		if err != nil {
			return measured{}, err
		}
		segments = append(segments, path)
	}
	pois, err := aggregate("poisson", segments)
	if err != nil {
		return measured{}, err
	}
	neg, err := aggregate("negative_binomial", segments)
	if err != nil {
		return measured{}, err
	}
	return measured{poisson: pois, negbin: neg}, nil
}

var (
	measureOnce   sync.Once
	measureCached measured
	measureErr    error
	measureDir    string
)

func measureAll() (measured, error) {
	measureOnce.Do(func() { measureCached, measureErr = measureUncached(measureDir) })
	return measureCached, measureErr
}
