package claims

import (
	"strings"
	"testing"
)

// valid is a well-formed fixture claim. Tests mutate a copy to check that each
// required field is actually load-bearing — a validator nobody has tried to break
// is a validator that passes everything.
func valid() Claim {
	return Claim{
		ID:          "fixture_claim",
		Statement:   "A fixture claim used only to exercise the claim mechanism.",
		Gate:        "0",
		Phase:       "0 — Trust foundation",
		Data:        "none (fixture)",
		Unit:        "arbitrary units",
		Limitations: "Supports nothing about the domain; exercises validation only.",
		Monotone:    1,
		Observations: []Observation{
			{Label: "baseline", Value: 1.0},
			{Label: "perturbed", Value: 2.0},
		},
		Binding: Binding{
			TestName: "TestClaimValidation",
			TestFile: "pkg/claims/claims_test.go",
		},
	}
}

func TestClaimValidation(t *testing.T) {
	t.Run("a well-formed claim verifies", func(t *testing.T) {
		if err := Verify(valid()); err != nil {
			t.Fatalf("expected the fixture to verify, got %v", err)
		}
	})

	// Each of these fields exists because its absence is a specific way a claim
	// stops being falsifiable, so each must be rejected rather than defaulted.
	t.Run("required fields are rejected when missing", func(t *testing.T) {
		for name, break_ := range map[string]func(*Claim){
			"ID":          func(c *Claim) { c.ID = "" },
			"Statement":   func(c *Claim) { c.Statement = "" },
			"Gate":        func(c *Claim) { c.Gate = "" },
			"Phase":       func(c *Claim) { c.Phase = "" },
			"Data":        func(c *Claim) { c.Data = "" },
			"Limitations": func(c *Claim) { c.Limitations = "" },
			"TestName":    func(c *Claim) { c.Binding.TestName = "" },
			"TestFile":    func(c *Claim) { c.Binding.TestFile = "" },
			"assertion":   func(c *Claim) { c.Monotone = 0; c.Thresholds = nil },
			"observations": func(c *Claim) {
				c.Observations = nil
			},
			// Whitespace must not satisfy a required field: " " in Limitations renders
			// as an empty bullet on the page and reads as "no limitations".
			"whitespace-only Limitations": func(c *Claim) { c.Limitations = "   " },
		} {
			c := valid()
			break_(&c)
			if err := Verify(c); err == nil {
				t.Errorf("expected missing %s to be rejected", name)
			}
		}
	})

	t.Run("IDs must be lower_snake_case", func(t *testing.T) {
		for _, id := range []string{
			"CamelCase", "with-hyphen", "with space", "_leading", "trailing_",
			"double__underscore", "9leading_digit",
		} {
			c := valid()
			c.ID = id
			if err := Verify(c); err == nil {
				t.Errorf("expected ID %q to be rejected", id)
			}
		}
		for _, id := range []string{"a", "spread_recovers", "ess_above_0_2"} {
			c := valid()
			c.ID = id
			if err := Verify(c); err != nil {
				t.Errorf("expected ID %q to be accepted, got %v", id, err)
			}
		}
	})

	t.Run("a broken assertion fails", func(t *testing.T) {
		c := valid()
		c.Monotone = -1 // the observations increase, so this must not verify
		if err := Verify(c); err == nil {
			t.Fatal("expected a contradicted monotone direction to be rejected")
		}
	})
}

func TestClaimSetChecking(t *testing.T) {
	t.Run("duplicate IDs are rejected", func(t *testing.T) {
		a, b := valid(), valid()
		b.Binding.TestName = "TestSomethingElse"
		if err := Check([]Claim{a, b}); err == nil {
			t.Fatal("expected a duplicate claim ID to be rejected")
		}
	})

	t.Run("an empty set is valid", func(t *testing.T) {
		if err := Check(nil); err != nil {
			t.Fatalf("expected an empty set to be valid, got %v", err)
		}
	})

	t.Run("one bad claim fails the set", func(t *testing.T) {
		bad := valid()
		bad.ID = "second_claim"
		bad.Limitations = ""
		if err := Check([]Claim{valid(), bad}); err == nil {
			t.Fatal("expected the set to fail on its invalid member")
		}
	})
}

func TestMarkdownRendering(t *testing.T) {
	t.Run("the empty set says so plainly", func(t *testing.T) {
		out := Markdown(nil)
		if !strings.Contains(out, "No verified claims yet") {
			t.Fatalf("expected the empty state to be stated, got:\n%s", out)
		}
	})

	t.Run("a claim renders its whole bond", func(t *testing.T) {
		out := Markdown([]Claim{valid()})
		// Every element of the bond must appear: without the ID and test link the
		// claim is not re-runnable from the page, and without the limitations the
		// page oversells it.
		for _, want := range []string{
			"### `fixture_claim`",
			"A fixture claim used only to exercise the claim mechanism.",
			"**Discharges gate:** 0",
			"**Data:** none (fixture)",
			"[`TestClaimValidation/fixture_claim`](pkg/claims/claims_test.go)",
			"baseline 1.00 · perturbed 2.00",
			"asserts values increase in order",
			"**Does not support:**",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("rendered claim is missing %q; got:\n%s", want, out)
			}
		}
	})

	t.Run("thresholds render their bound", func(t *testing.T) {
		c := valid()
		c.Monotone = 0
		c.Thresholds = []Threshold{
			{ObsIndex: 0, GreaterThan: true, Ref: 0.5, RefLabel: "the floor"},
		}
		out := Markdown([]Claim{c})
		if !strings.Contains(out, "asserts baseline > the floor") {
			t.Fatalf("expected the threshold bound to render, got:\n%s", out)
		}
	})

	// The up-to-date test compares generated output against the committed file, so
	// rendering must depend on the set's content and not on the order providers
	// happen to be registered in — otherwise CI produces diffs nobody caused.
	t.Run("output is independent of input order", func(t *testing.T) {
		a := valid()
		b := valid()
		b.ID = "another_claim"
		if Markdown([]Claim{a, b}) != Markdown([]Claim{b, a}) {
			t.Fatal("expected rendering to be order-independent")
		}
	})

	t.Run("phases group and are headed once", func(t *testing.T) {
		a := valid()
		b := valid()
		b.ID = "another_claim"
		out := Markdown([]Claim{a, b})
		if got := strings.Count(out, "## Phase 0 — Trust foundation"); got != 1 {
			t.Fatalf("expected one heading for the shared phase, got %d", got)
		}
	})
}
