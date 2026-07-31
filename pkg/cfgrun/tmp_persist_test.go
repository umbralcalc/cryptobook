package cfgrun

import (
	"math"
	"testing"
)

// Construction check + the ONE permitted adjustment (mean depth). Deliberately
// computes no correlation between depth and either flow.
func TestTmpPersist(t *testing.T) {
	for _, churn := range []string{"1.15"} {
		s, err := Run("lob_persistent.yaml", Subs{
			"max_steps: 400":     "max_steps: 2000",
			"churn_rate: [1.15]": "churn_rate: [" + churn + "]",
		})
		if err != nil {
			t.Fatalf("%s: %v", churn, err)
		}
		rows := s.GetValues("lob_persistent")[100:]
		act := make([]float64, len(rows))
		for i, r := range rows {
			act[i] = r[21]
		}
		mean := 0.0
		for _, a := range act {
			mean += a
		}
		mean /= float64(len(act))
		v, ac := 0.0, 0.0
		for i, a := range act {
			v += (a - mean) * (a - mean)
			if i > 0 {
				ac += (a - mean) * (act[i-1] - mean)
			}
		}
		v /= float64(len(act))
		ac /= float64(len(act)-1) * v
		depth, err := MeanColumn(s, "lob_persistent", 19, 100)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("churn %-5s | driver mean %.3f (want 4) var %.2f (want 8) sd %.2f | lag-1 autocorr %.3f (want ~0.8) | MEAN DEPTH %.1f",
			churn, mean, v, math.Sqrt(v), ac, depth)
	}
}
