// This file adds CH: does the offline dispersion recovery of CA-CC survive an AR(1)
// PERSISTENT driver — the first confound the full cfg/lob_counts.yaml adds over the clean
// IID case? See PREREGISTRATION.md CH, fixed before this ran.
package offline

import (
	"encoding/json"
	"fmt"
	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"os"
	"path/filepath"
	"sync"
)

const (
	// Both generators are matched to this marginal dispersion analytically. Persistent:
	// (1-p)/(1+p)*Var(gamma)/E^2 = 0.2/1.8*105/16. IID-matched: gamma(1.3714, 0.3428) has
	// variance 11.667, so the same 11.667/16.
	persistPhiTruth = 0.7292
	// A phi prior that brackets 0.7292 without putting it at the edge (CA-CC's [0.5,20]
	// would sit the truth on the boundary).
	persistPhiPrior = "{type: uniform, lo: 0.05, hi: 3.0}"
	persistPhiOrig  = "{type: uniform, lo: 0.5, hi: 20.0}"
)

func recordFrom(dir, config, seed string, extra cfgrun.Subs) (string, error) {
	subs := cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", segLen),
		"seed: 20260830": "seed: " + seed,
	}
	for k, v := range extra {
		subs[k] = v
	}
	storage, err := cfgrun.Run(config, subs)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "persist_"+config+"_"+seed+".log")
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

// calibratePhi runs the negative-binomial SMC with the wider phi prior and returns the
// posterior-mean phi (the third coordinate).
func calibratePhi(segment string) (float64, error) {
	storage, err := cfgrun.Run(calibration, cfgrun.Subs{
		"json_log: {path: RECORDED_SEGMENT_PATH}": fmt.Sprintf("json_log: {path: %s}", segment),
		"{type: LIKELIHOOD_TYPE}":                 "{type: negative_binomial}",
		persistPhiOrig:                            persistPhiPrior,
	})
	if err != nil {
		return 0, err
	}
	post, err := cfgrun.LastRow(storage, "smc_posterior")
	if err != nil {
		return 0, err
	}
	return post[2], nil
}

// persistResult is the mean recovered phi for one generator over the seed ensemble.
type persistResult struct {
	meanPhi, relErr float64
}

func measurePersistUncached(dir string) (persistent, matched persistResult, err error) {
	if dir == "" {
		if dir, err = os.MkdirTemp("", "persist-seg-"); err != nil {
			return persistent, matched, err
		}
	}
	iidMatched := cfgrun.Subs{
		"activity_shape: [0.152367]": "activity_shape: [1.3714]",
		"activity_rate: [0.038092]":  "activity_rate: [0.3428]",
	}
	var pPhi, mPhi []float64
	for _, seed := range genSeeds {
		pSeg, e := recordFrom(dir, "lob_churn_persist.yaml", seed, nil)
		if e != nil {
			return persistent, matched, e
		}
		mSeg, e := recordFrom(dir, generator, seed, iidMatched)
		if e != nil {
			return persistent, matched, e
		}
		p, e := calibratePhi(pSeg)
		if e != nil {
			return persistent, matched, e
		}
		m, e := calibratePhi(mSeg)
		if e != nil {
			return persistent, matched, e
		}
		pPhi = append(pPhi, p)
		mPhi = append(mPhi, m)
	}
	persistent = persistResult{meanPhi: mean(pPhi), relErr: absRel(mean(pPhi), persistPhiTruth)}
	matched = persistResult{meanPhi: mean(mPhi), relErr: absRel(mean(mPhi), persistPhiTruth)}
	return persistent, matched, nil
}

var (
	persistOnce    sync.Once
	persistCachedP persistResult
	persistCachedM persistResult
	persistErr     error
)

func measurePersist() (persistent, matched persistResult, err error) {
	persistOnce.Do(func() {
		persistCachedP, persistCachedM, persistErr = measurePersistUncached(persistDir)
	})
	return persistCachedP, persistCachedM, persistErr
}

var persistDir string
