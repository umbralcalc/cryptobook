package oospool

import (
	"fmt"
	"math"
	"testing"
)

// TestOccasions scores every recorded occasion against the frozen CD-CG bounds. It skips an
// absent occasion rather than inventing a result. oos2 is a REGRESSION check: it must
// reproduce the BS-BU numbers scored in PREREGISTRATION.md (occasion mean -0.1267 / -0.0293
// / +0.9529). oos3 is the live second occasion.
func TestOccasions(t *testing.T) {
	for _, occ := range []string{"oos2", "oos3"} {
		if !Available(occ) {
			t.Logf("%s: segments absent, skipping", occ)
			continue
		}
		r, err := Score(occ)
		if err != nil {
			t.Fatalf("%s: %v", occ, err)
		}
		fmt.Printf("\n══ %s (%d usable windows)\n", occ, r.usable)
		for _, w := range r.windows {
			ex := ""
			if w.excluded {
				ex = " EXCLUDED: " + w.reason
			}
			fmt.Printf("   window %d (%d rows): d/arr %+.4f  d/can %+.4f  arr/can %+.4f%s\n",
				w.window, w.rows, w.arrival, w.cancel, w.coMovement, ex)
		}
		fmt.Printf("   OCCASION MEAN: d/arr %+.4f  d/can %+.4f  arr/can %+.4f\n",
			r.arrival, r.cancel, r.coMovement)
		for _, v := range r.Verdicts() {
			pass := "PASS"
			if !v.pass() {
				pass = "FAIL"
			}
			fmt.Printf("   %-16s |%.4f - %.4f| = %.4f  vs %.3f  %s (%.2f SD)\n",
				v.name, v.model, v.occ, v.gap(), v.bound, pass, v.sigma())
		}
	}
}

// TestOccasion2Regression pins that oos2 still reproduces the BS-BU occasion means, so a
// change to the pooling code shows up here rather than silently altering a published result.
func TestOccasion2Regression(t *testing.T) {
	if !Available("oos2") {
		t.Skip("oos2 segments absent")
	}
	r, err := Score("oos2")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"d/arr", r.arrival, -0.1267},
		{"d/can", r.cancel, -0.0293},
		{"arr/can", r.coMovement, 0.9529},
	} {
		if math.Abs(c.got-c.want) > 0.001 {
			t.Errorf("oos2 %s = %.4f, BS-BU scored %.4f — the pooling changed", c.name, c.got, c.want)
		}
	}
}
