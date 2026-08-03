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

// TestTheWiringDirectionsAreRight guards the one thing that would make the split a
// different model rather than a re-expression, silently.
//
// Three edges, and two different mechanisms:
//
//	activity -> flows   CURRENT step (params_from_upstream). An alias would lag the driver
//	                    coupling, and a one-step lag on a coupled flow is what cost
//	                    cfg/lob_churn_recycled.yaml its co-movement, +0.897 to +0.432.
//	book     -> flows   PREVIOUS step (upstreams alias). The monolith already damps
//	                    arrivals by the previous step's depth — its `bid` is its own row 0
//	                    — so this is not a concession, it is the original semantics. It is
//	                    also what breaks the apparent flows<->book cycle.
//	flows    -> book    CURRENT step (params_from_upstream). Lagging it would delay every
//	                    arrival by a step.
//
// Getting any of these backwards produces a plausible model with different numbers, which
// is exactly the failure a config test can catch and a results check cannot.
func TestTheWiringDirectionsAreRight(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_split.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"activity: {upstream: activity}", // driver -> flows, current
		"flow: {upstream: flows}",        // flows -> book, current
		"upstreams: {book_prev: book}",   // book -> flows, previous
	} {
		if !strings.Contains(text, required) {
			t.Errorf("cfg/lob_split.yaml is missing the wiring %q", required)
		}
	}
	// The two edges that must NOT swap mechanism.
	if strings.Contains(text, "upstreams: {activity") || strings.Contains(text, "upstreams: {flow") {
		t.Error("the driver and flows must reach their consumers at the CURRENT step; an " +
			"upstreams alias gives the previous one and is a different model")
	}
	if strings.Contains(text, "book_prev: {upstream: book}") {
		t.Error("flows must read the book at the PREVIOUS step via an upstreams alias — " +
			"params_from_upstream would make it current and close a cycle")
	}
}

// TestTheBookDrawsNoRandomness pins why the flows/book boundary is free.
//
// The book is deterministic given the flows, which is why the three-way split reproduces
// a two-way driver-only split to four decimals: separating a partition that consumes no
// randomness cannot change a result. If a draw ever appears in the book partition that
// stops being true, and the agreement claim would start to move for a reason nobody
// intended.
func TestTheDeterministicPartitionsDrawNoRandomness(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), "lob_split.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	deterministic := string(source)[strings.Index(string(source), "- partition: book"):]
	for _, draw := range []string{"poisson(", "gamma(", "binomial(", "normal(", "uniform(", "exponential("} {
		if strings.Contains(deterministic, draw) {
			t.Errorf("the book/observables partitions contain %q — both must stay deterministic given "+
				"the flows, which is what makes this boundary cost nothing", draw)
		}
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
