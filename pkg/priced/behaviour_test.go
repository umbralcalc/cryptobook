package priced

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestPricedBookExpectedBehaviour is the binding test named by every claim in
// behaviour.go — one subtest per claim, named by the claim's ID.
func TestPricedBookExpectedBehaviour(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func readConfig(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), configName))
	if err != nil {
		t.Fatalf("reading %s: %v", configName, err)
	}
	return string(source)
}

// TestTheModelIsStillPureConfig guards the property that has held for every model
// in this repo: the dynamics live in YAML, not in Go. If order identity is ever
// added as a bespoke iteration, this is where that decision becomes visible rather
// than arriving silently.
func TestTheModelIsStillPureConfig(t *testing.T) {
	source := readConfig(t)
	for _, required := range []string{"expressions:", "bindings:", "outputs:"} {
		if !strings.Contains(source, required) {
			t.Errorf("%s no longer states its dynamics as expressions", configName)
		}
	}
	if strings.Contains(source, "iteration:") {
		t.Errorf("%s names a Go iteration; the model must stay pure config, and a "+
			"change here is an Invariant A decision rather than an implementation one",
			configName)
	}
}

// TestTheBaseModelIsUnshocked matters because the spread claims measure ORDINARY
// dynamics. If the shipped config carried a live shock, those numbers would be
// describing a perturbed book while claiming to describe a steady state.
func TestTheBaseModelIsUnshocked(t *testing.T) {
	source := readConfig(t)
	if !strings.Contains(source, "shock_step: [-1.0]") {
		t.Error("the shipped config must default to a shock step that never fires")
	}
	if !strings.Contains(source, "shock_size: [0.0]") {
		t.Error("the shipped config must default to a zero shock size")
	}
}

// TestSweepAbsorbsInFrontFirst pins the book-walking arithmetic directly, rather
// than only through the survival claim.
//
// The sweep is a prefix-sum-plus-clamp reformulation, chosen because a lane in `each`
// cannot read an earlier lane's value (the "PREFIX SUM" idiom documented in
// cfg/lob_priced.yaml). It is the least obvious part of the model, and
// an error in it would show up as a plausible-looking survival number rather than
// as a failure — so it gets checked against a case whose answer is arithmetic.
func TestSweepAbsorbsInFrontFirst(t *testing.T) {
	// A shock far larger than the book must clear the swept side outright, and the
	// spread must then report the one-sided sentinel rather than a tight book.
	storage, err := cfgrun.Run(configName, cfgrun.Subs{
		"shock_step: [-1.0]": "shock_step: [200.0]",
		"shock_size: [0.0]":  "shock_size: [500.0]",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	rows := storage.GetValues("lob_priced")
	if got := sideDepth(rows[shockStep], askFrom); got != 0 {
		t.Errorf("ask depth after an overwhelming order = %v, want 0 — the order must "+
			"walk every level, not stop at the touch", got)
	}
	if got := sideDepth(rows[shockStep], bidFrom); got <= 0 {
		t.Error("a marketable BUY must not consume the bid side")
	}
	if got := rows[shockStep][idxSpread]; got < emptySpread {
		t.Errorf("spread = %v with an empty ask side; a one-sided book must report the "+
			"sentinel rather than a number that reads as a tight market", got)
	}
}

// TestQueuePositionIsStillNotClaimed keeps the Spike 4.2 audit honest as the model
// grows. Prices and book-walking made two more outputs answerable; order identity
// did not arrive with them, and nothing here may quietly start reporting it.
func TestQueuePositionIsStillNotClaimed(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		for _, forbidden := range []string{"queue_position", "tick_regime", "fifo"} {
			if strings.Contains(claim.ID, forbidden) {
				t.Errorf("claim %q reports %q, which needs per-order identity the model "+
					"does not have", claim.ID, forbidden)
			}
		}
	}
	if strings.Contains(readConfig(t), "order_id") {
		t.Error("the config mentions order identity; if it has genuinely gained it, " +
			"the queue-position output must be re-audited rather than left absent")
	}
}
