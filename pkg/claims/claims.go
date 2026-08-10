// Package claims is this repo's claim↔test↔result bond: the Phase 0 trust
// foundation that every later phase hangs off.
//
// The problem it solves is the one the project opens with — "a calibration result
// that can't be re-run on every engine change is a screenshot, not a claim". So a
// behavioural claim here is not prose in a README. It is a value with a stable ID
// that (a) names the test subtest enforcing it, (b) carries the numbers that test
// produced, and (c) states what it does NOT support. CI regenerates CLAIMS.md from
// the same values the tests assert, so the three cannot drift: break a claim's
// assertion and its binding test fails; move a number without regenerating and
// TestClaimsUpToDate fails.
//
// It builds on the engine's own claim type (models/cardgen.Claim) so the bond has
// the same shape as the stochadex domain-models catalogue — Verify is the engine's,
// unchanged. What this package adds are the fields the standing constraints
// need and cardgen has no reason to carry:
//
//   - Gate — which spike or decision gate the claim discharges, so a resolved gate
//     is traceable to evidence rather than to a note in a commit message.
//   - Data — the dataset the claim was verified against. Phase 2 (crypto spot)
//     and Phase 3 (crypto) are two calibrations of different markets, not one
//     pipeline; naming the dataset per claim is what stops a reader merging them.
//   - Limitations — what the claim does not support. Required, not optional: the
//     "honest limitation reporting" constraint is a mechanical field here rather
//     than a discipline someone has to remember at writeup time.
//   - Binding — per claim, because claims here come from many tests across phases
//     (cardgen assumes one behaviour suite per model, so its single-Binding
//     renderer does not fit; Markdown below replaces it).
//
// Adding a claim: expose ObservedBehaviour() []claims.Claim from a non-test file in
// the phase package, consume it from a test that runs one subtest per claim ID,
// register the provider in internal/claimset, then `go run ./cmd/gen-claims`.
package claims

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/models/cardgen"
)

// Observation and Threshold are re-exported so a phase package states its claims
// without importing the engine's models catalogue directly — the dependency is an
// implementation detail of this package, not of every caller.
type (
	Observation = cardgen.Observation
	Threshold   = cardgen.Threshold
)

// Binding names the test that enforces a claim: the test function whose subtests
// are claim IDs, and the file it lives in as a repo-relative path so the link
// resolves on GitHub.
type Binding struct {
	TestName string // e.g. "TestSyntheticRecovery"
	TestFile string // e.g. "pkg/recovery/recovery_test.go"
}

// Claim is one behavioural claim with everything needed to re-verify it and to
// state it honestly. Assertions are cardgen's: Monotone for a directional
// response, Thresholds for a sign/level bound, at least one of the two.
type Claim struct {
	// ID is the claim's contract and the binding subtest's name: lower_snake_case,
	// stable across refactors. Renaming an ID is a new claim, not an edit.
	ID string
	// Statement is the claim in plain language, as a sentence a reader can disagree
	// with. Must be a counterfactual about market state, never a directional price
	// claim (the design framing discipline) — this package cannot check that, but the
	// review of a new claim should.
	Statement string
	// Gate is the spike or decision gate this claim discharges, e.g. "1.2".
	Gate string
	// Phase groups claims in CLAIMS.md, e.g. "1 — Synthetic parameter recovery".
	Phase string
	// Data names the dataset verified against, e.g. "synthetic (self-generated
	// order flow)" or "Binance BTCUSDT spot, recorded segment". Never merge markets under
	// one label.
	Data string
	// Unit annotates the observed values, e.g. "posterior mean, 500-step run".
	Unit string
	// Limitations states what this claim does NOT support. Required.
	Limitations string
	// Monotone, Thresholds and Observations are the assertion and its evidence,
	// verified by cardgen.Verify.
	Monotone     int
	Thresholds   []Threshold
	Observations []Observation
	// Binding is the test that enforces the assertion.
	Binding Binding
}

// idPattern is the ID contract: lower_snake_case, starting with a letter. Stable
// IDs are the point — an ID that can be reformatted is an ID that can drift.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// Verify checks one claim: the repo's required fields, then the engine's assertion
// semantics. Every field it requires exists because its absence is how a claim
// quietly becomes unfalsifiable — an unbound claim cannot be re-run, an
// unattributed one cannot be scoped to a market, and one without limitations
// invites the reader to assume it generalises.
func Verify(c Claim) error {
	if !idPattern.MatchString(c.ID) {
		return fmt.Errorf(
			"claim ID %q must be lower_snake_case starting with a letter", c.ID)
	}
	for field, value := range map[string]string{
		"Statement":   c.Statement,
		"Gate":        c.Gate,
		"Phase":       c.Phase,
		"Data":        c.Data,
		"Limitations": c.Limitations,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("claim %q: %s is required", c.ID, field)
		}
	}
	if c.Binding.TestName == "" || c.Binding.TestFile == "" {
		return fmt.Errorf(
			"claim %q: Binding needs both TestName and TestFile so the claim is "+
				"re-runnable", c.ID)
	}
	if len(c.Observations) == 0 {
		return fmt.Errorf("claim %q: no observations recorded", c.ID)
	}
	return cardgen.Verify(cardgen.Claim{
		ID:           c.ID,
		Statement:    c.Statement,
		Unit:         c.Unit,
		Monotone:     c.Monotone,
		Thresholds:   c.Thresholds,
		Observations: c.Observations,
	})
}

// Check verifies a whole claim set and enforces ID uniqueness across it. Duplicate
// IDs would make the claim↔test bond ambiguous — two claims pointing at one
// subtest name means neither is pinned.
func Check(set []Claim) error {
	seen := make(map[string]string, len(set))
	for _, c := range set {
		if err := Verify(c); err != nil {
			return err
		}
		if where, dup := seen[c.ID]; dup {
			return fmt.Errorf(
				"duplicate claim ID %q (already bound to %s)", c.ID, where)
		}
		seen[c.ID] = c.Binding.TestName
	}
	return nil
}

// observed renders a claim's numbers and the bound each is checked against, so the
// assertion is visible on the page and not only in the test.
func observed(c Claim) string {
	parts := make([]string, len(c.Observations))
	for i, o := range c.Observations {
		parts[i] = fmt.Sprintf("%s %s", o.Label, cardgen.FormatValue(o.Value))
	}
	body := strings.Join(parts, " · ")
	if c.Unit != "" {
		body = fmt.Sprintf("%s — %s", c.Unit, body)
	}
	asserts := make([]string, 0, len(c.Thresholds)+1)
	switch c.Monotone {
	case 1:
		asserts = append(asserts, "values increase in order")
	case -1:
		asserts = append(asserts, "values decrease in order")
	}
	for _, th := range c.Thresholds {
		rel := "<"
		if th.GreaterThan {
			rel = ">"
		}
		asserts = append(asserts, fmt.Sprintf("%s %s %s",
			c.Observations[th.ObsIndex].Label, rel, th.RefLabel))
	}
	if len(asserts) > 0 {
		body = fmt.Sprintf("%s (asserts %s)", body, strings.Join(asserts, ", "))
	}
	return body
}

// Markdown renders the whole claim set as CLAIMS.md. Claims are grouped by phase
// and ordered by ID within a phase, so the file is a function of the set alone —
// reordering providers must not produce a diff, or the up-to-date test becomes
// noise everyone learns to ignore.
func Markdown(set []Claim) string {
	var b strings.Builder
	b.WriteString("# Verified claims\n\n")
	b.WriteString(
		"Every claim below is a *bound* object: a stable ID, the test subtest that " +
			"enforces it, the numbers that test produced, and what the claim does not " +
			"support. Nothing here is hand-written — this file is generated by " +
			"`go run ./cmd/gen-claims` from the same values the tests assert, and CI " +
			"fails if it is stale or if any assertion breaks (the binding test). A " +
			"claim cannot reach this page without a test, and a number cannot change " +
			"here without the code changing.\n\n")
	// Stated on the page rather than only in a design document, because a reader
	// counting claims would otherwise have no way to know something was left out.
	b.WriteString(
		"**What is deliberately absent.** This page carries only claims anyone can " +
			"re-derive from this repository alone. Measurements that need third-party " +
			"data which cannot be redistributed are excluded — their tests still " +
			"enforce them for anyone holding that data, but nothing can re-check them " +
			"automatically, so publishing their numbers here would imply a guarantee " +
			"that does not exist. Any such result is recorded in `DECISIONS.md` " +
			"instead, with its provenance.\n\n")
	if len(set) == 0 {
		b.WriteString(
			"**No verified claims yet.** The mechanism is live and CI-enforced; " +
				"Phase 1 lands the first claim (synthetic parameter recovery). An empty " +
				"page is the honest state before then — it is not a placeholder for " +
				"results that exist elsewhere.\n")
		return b.String()
	}
	ordered := make([]Claim, len(set))
	copy(ordered, set)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Phase != ordered[j].Phase {
			return ordered[i].Phase < ordered[j].Phase
		}
		return ordered[i].ID < ordered[j].ID
	})
	phase := ""
	for _, c := range ordered {
		if c.Phase != phase {
			phase = c.Phase
			fmt.Fprintf(&b, "## Phase %s\n\n", phase)
		}
		fmt.Fprintf(&b, "### `%s`\n\n", c.ID)
		fmt.Fprintf(&b, "> %s\n\n", c.Statement)
		fmt.Fprintf(&b, "- **Discharges gate:** %s\n", c.Gate)
		fmt.Fprintf(&b, "- **Data:** %s\n", c.Data)
		fmt.Fprintf(&b, "- **Enforced by:** [`%s/%s`](%s)\n",
			c.Binding.TestName, c.ID, c.Binding.TestFile)
		fmt.Fprintf(&b, "- **Observed:** %s\n", observed(c))
		fmt.Fprintf(&b, "- **Does not support:** %s\n\n", c.Limitations)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
