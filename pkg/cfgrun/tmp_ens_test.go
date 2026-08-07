package cfgrun

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

func TestEnsSweep(t *testing.T) {
	for _, h := range []string{"0.017", "0.016", "0.015", "0.014"} {
		stores, err := RunEnsemble("lob_ages12.yaml", Subs{
			"max_steps: 400": fmt.Sprintf("max_steps: %d", DefaultSteps),
			"haz0: [0.18]":   fmt.Sprintf("haz0: [%s]", h),
		}, DefaultSeeds)
		if err != nil {
			t.Fatal(err)
		}
		var d, o []float64
		for _, s := range stores {
			rows := s.GetValues("lob_ages12")[100:]
			m := diagnostics.Segment{Rows: rows}
			d = append(d, diagnostics.Mean(m.Column(195)))
			o = append(o, diagnostics.Mean(m.Column(198)))
		}
		sd, so := Summarise(d), Summarise(o)
		t.Logf("haz0 %-6s -> depth %6.1f (SE %.1f)   oldest share %.4f (SE %.4f)   [band 227.8-235.9, precondition >=0.05]",
			h, sd.Mean, sd.StdError(32), so.Mean, so.StdError(32))
	}
}
