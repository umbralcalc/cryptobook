package cfgrun

import (
	"fmt"
	"testing"
)

func TestAX(t *testing.T) {
	for _, h := range []string{"0.020", "0.018", "0.017", "0.016"} {
		s, err := Run("lob_ages12.yaml", Subs{
			"max_steps: 400": "max_steps: 2000",
			"haz0: [0.18]":   fmt.Sprintf("haz0: [%s]", h),
		})
		if err != nil {
			t.Fatalf("haz0 %s: %v", h, err)
		}
		d, _ := MeanColumn(s, "lob_ages12", 195, 100)
		o, _ := MeanColumn(s, "lob_ages12", 198, 100)
		a, _ := MeanColumn(s, "lob_ages12", 192, 100)
		t.Logf("haz0 %-7s -> depth %6.1f  oldest %.3f  n_limit %.1f   [band 227.8-235.9, AX predicts ceiling ~256]", h, d, o, a)
	}
}
