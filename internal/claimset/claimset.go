// Package claimset assembles this repo's full claim set from the phase packages
// that provide it. It exists so pkg/claims stays dependency-free — the phase
// packages import pkg/claims, and only this package imports them, which keeps the
// bond acyclic as phases are added.
//
// To register a phase's claims: add its ObservedBehaviour() to providers below.
// Nothing else changes — cmd/gen-claims picks the claims up from All().
//
// # This package has no tests, deliberately
//
// It used to carry two: one checking the whole set validates, one checking
// CLAIMS.md was not stale. Both duplicated what CI already does in its
// "CLAIMS.md is generated, not hand-written" step — cmd/gen-claims calls
// claims.Check and exits non-zero on an invalid set, and `git diff --exit-code`
// catches a stale page.
//
// The duplication was not free. Every provider runs simulations to produce its
// numbers, `go test` gives each package its own process (so the cache below cannot
// span them), and the two tests shared one measurement that cost ~277s under
// -race — making this package the critical path of the whole suite. Deleting both
// took the suite from 4:37 to ~4:01; deleting only one would have saved nothing,
// since the surviving test would still have paid for the same measurement.
//
// What this trades away: a bare local `go test ./...` no longer notices a stale
// CLAIMS.md. Run `go run ./cmd/gen-claims` before pushing, or let CI say so. The
// phase binding tests still catch a broken claim assertion, which is the half that
// matters most.
package claimset

import (
	"sync"

	"github.com/umbralcalc/cryptobook/pkg/arrivals"
	"github.com/umbralcalc/cryptobook/pkg/baseline"
	"github.com/umbralcalc/cryptobook/pkg/ceiling"
	"github.com/umbralcalc/cryptobook/pkg/churn"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/conservation"
	"github.com/umbralcalc/cryptobook/pkg/damping"
	"github.com/umbralcalc/cryptobook/pkg/lob"
	"github.com/umbralcalc/cryptobook/pkg/persistent"
	"github.com/umbralcalc/cryptobook/pkg/priced"
	"github.com/umbralcalc/cryptobook/pkg/recovery"
	"github.com/umbralcalc/cryptobook/pkg/recycled"
	"github.com/umbralcalc/cryptobook/pkg/split"
	"github.com/umbralcalc/cryptobook/pkg/stability"
	"github.com/umbralcalc/cryptobook/pkg/windowing"
)

// providers is every phase package's claim provider. Order is irrelevant —
// claims.Markdown sorts — so a provider can be appended without reshuffling.
var providers = []func() []claims.Claim{
	lob.ObservedBehaviour,          // the minimal LOB generator's responses
	baseline.ObservedBehaviour,     // Spike 2.2: the synthetic control for the real-market diagnostics
	churn.ObservedBehaviour,        // Spike 2.2: the churn model's pre-registered predictions, scored
	arrivals.ObservedBehaviour,     // Spike 2.2: depth-dependent arrivals — the mechanism that holds
	recycled.ObservedBehaviour,     // Spike 2.2: depth-neutral churn — the mechanism that does not
	persistent.ObservedBehaviour,   // Spike 2.2: a persistent driver — the closest miss so far
	damping.ObservedBehaviour,      // Spike 2.2: the first calibration — one parameter fitted, two held out
	split.ObservedBehaviour,        // Step 3: the partitioned model reproduces the monolith
	ceiling.ObservedBehaviour,      // The co-movement ceiling account, and the matched pair that identified it
	conservation.ObservedBehaviour, // Volume accounting in the age models, after two defects
	recovery.ObservedBehaviour,     // Spike 1.2: identification, ESS, recovery
	windowing.ObservedBehaviour,    // Gate 3.4: how calibration degrades with window length
	stability.ObservedBehaviour,    // Spike 4.2: the one output the minimal model supports
	priced.ObservedBehaviour,       // Spike 4.2: two more, unlocked by prices and book-walking
}

// NOT registered: pkg/crypto or pkg/replication.
//
// Their Spike 2.2 diagnostics are real and their tests enforce them — but only for
// someone holding the recorded segments, which cannot be committed because Binance's
// licence does not permit redistribution (README.md quotes it). Registering them would
// make CLAIMS.md depend on files a fresh clone does not have, so the page would differ
// between CI and a machine with the data and TestClaimsUpToDate would fail for whoever
// had less.
//
// This is the project's central structural weakness, and narrowing to crypto did not fix
// it: the domain's own data is still non-redistributable, so the results that matter most
// are the ones CI cannot re-check. What crypto changes is that anyone can REGENERATE the
// input from public endpoints in eight minutes, which is why pkg/replication states its
// findings as bounds that any fresh recording must satisfy rather than as point values.

// All returns the repo's complete claim set.
//
// Measured once per process and then reused. Every provider runs simulations to
// produce its numbers — the recovery claims run three full posterior estimations —
// and both tests here plus cmd/gen-claims ask for the set. Without the cache the
// suite pays for the same measurements repeatedly, which under -race is the
// difference between one minute and two. The claims are deterministic (fixed seeds
// throughout), so caching changes no result.
var All = sync.OnceValue(func() []claims.Claim {
	set := make([]claims.Claim, 0)
	for _, provider := range providers {
		set = append(set, provider()...)
	}
	return set
})
