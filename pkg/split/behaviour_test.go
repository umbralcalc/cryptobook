package split

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestPartitionedModelMatchesMonolith is the binding test named by this package's claim.
func TestPartitionedModelMatchesMonolith(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestTheDriverIsCoupledAtTheSameStep guards the one thing that makes the split a
// re-expression rather than a different model.
//
// `upstreams: {alias: partition}` would give the driver's PREVIOUS step, and a one-step
// lag on a coupled flow is not a small change here — it is what cost
// cfg/lob_churn_recycled.yaml its co-movement, +0.897 to +0.432. If this config ever
// switches mechanism, the numbers would move and this test says why before anyone hunts.
func TestTheDriverIsCoupledAtTheSameStep(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_split.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "params_from_upstream:") ||
		!strings.Contains(text, "activity: {upstream: activity}") {
		t.Error("the driver must reach the book partition through params_from_upstream, " +
			"which is the CURRENT step")
	}
	if strings.Contains(text, "upstreams: {act") || strings.Contains(text, "upstreams: {activity") {
		t.Error("an upstreams alias gives the PREVIOUS step and would lag the driver " +
			"coupling by one step, which is a different model")
	}
}

// TestTheMonolithIsUnchanged pins the comparison's other side. The scored AC-AG claims are
// measured against cfg/lob_damping.yaml, so if the refactor ever edited it instead of
// adding alongside it, this package would be comparing a model to itself.
func TestTheMonolithIsUnchanged(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_damping.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"damping_gamma: [0.6]", "churn_rate: [1.075]", "persistence: [0.8]",
		"- name: lob_damping",
	} {
		if !strings.Contains(string(source), required) {
			t.Errorf("cfg/lob_damping.yaml no longer has %q — the split must be a NEW "+
				"config, not an edit to the one the scored claims rest on", required)
		}
	}
}
