# stochadex gaps

Capability gaps in the [stochadex](https://github.com/umbralcalc/stochadex) engine
found while building this project, recorded as they are hit.

**Why a file rather than issues.** This repo is a downstream application, and
stochadex's own `models/CONVENTIONS.md` states the test this file exists to serve:
a domain model is also written as data, and *"whether that twin can be written is
the test: if it can, the bespoke Go is a convenience and promotion is optional; if
it cannot, the engine has a real capability gap and one model is enough to prove
it."* Each entry below is one model proving one gap.

**What belongs here.** Only gaps **verified against the code or by a failing
example**, with what they blocked in this project. Not wishes, and not things that
turned out to be my misunderstanding — those get recorded as corrections in
`DECISIONS.md` instead. An entry that cannot name what it blocked is not a gap, it
is a preference.

Checked against **v0.14.0**.

## Re-verified in full, then CLOSED upstream in v0.14.0

Every open entry was re-checked before being taken upstream, and the method is written
down because two claims in this project were wrong this week for the same reason: a
mechanism assumed absent because it had not been exercised. A gap entry asserting the
engine *cannot* do something deserves the same adversarial check as a claim that a model
*can*.

**Both actionable entries were then fixed upstream in v0.14.0 and are moved to Resolved
below.** What remains open is entries 3 and 4, neither of which should become an issue.
The verification table is kept because it is the evidence the fixes were made against.

| entry | verdict | how it was checked |
|---|---|---|
| **1. no scan across `each` lanes** | **confirmed**, three independent ways | the DSL registry has 30 functions and none is a fold (`scan`/`fold`/`cumsum`/`prod` all absent); `out` is a local Go slice never bound into the lane environment (read at `pkg/general/expression.go`, the `each` case); and the prefix-sum workaround the entry says is *not* blocked returns exactly the documented `[3 2 1 1 0 0]` when run |
| **2. `slice` rejects a zero width** | **confirmed**, reproduced | running `sum(slice(v, 0, 0))` gives `panic: expression: slice's width must be at least 1` |
| **3. no streaming/growing `data:` source** | **confirmed as described** | every source returns `(*simulator.StateTimeStorage, error)`, and so does `RegisterDataSource`'s `build` signature — there is no growing variant to register. The engine websocket sets `OutputFunction` and never reads a message, so it is output-only. `arrow` and `s3` are contributed by the CLI, not in-engine, as the entry says |
| **4. no data-agreement layer** | **confirmed as described** | zero matches anywhere in `pkg/` for schema negotiation or data agreement; the Postgres table is a fixed `CREATE TABLE IF NOT EXISTS (partition_name, time, state)` |

**Only entries 1 and 2 were actionable upstream, and both are now closed.** Entry 3 is a
deliberate NON-gap — Gate 3.4 selected the architecture that makes the blocking-source path
correct rather than a workaround — and entry 4 exists so the next reader does not go
looking for a layer that was never designed. Neither should become an issue.

**Entry 1 was a real gap, but I overstated what it blocked — corrected 2026-08-08.** The
gap itself stands exactly as verified: there is no fold across `each` lanes, confirmed three
independent ways, and `scan` closes it. What was wrong is the CONSEQUENCE this file claimed
for it: that "order identity was unsayable, so the queue-position stability output
could not be answered".

`cfg/lob_queue.yaml` now answers that output and **uses no `scan` anywhere**. Order identity
needs COMPACTION, not allocation-to-free-slots — when a mid-queue order cancels the orders
behind it move up rather than leaving a hole for a newcomer — and compaction's only
non-trivial ingredient is an exclusive prefix sum, which the lazy-`where` idiom in this same
file's own notes could always express. So order identity was sayable all along; I reached
for the allocation framing and recorded its difficulty as an engine limitation.

`scan` remains worth having on its own merits — it is O(n) where the prefix-sum idiom is
O(n²), which is why `cfg/lob_ages.yaml` uses it at 128 slots — and the vector-accumulator
case is genuinely inexpressible without it. But **no output was ever blocked on it**, and
this file should not have said one was. The lesson is the one DECISIONS.md keeps recording:
a capability claimed absent because the first formulation was awkward deserves the same
adversarial check as a claim that a model works.

### One entry was filed wrongly and withdrawn, which is why this section exists

A high-severity entry 1b — "the expressions tier has no same-step cross-partition read" —
was filed and then retracted the same day. `upstreams:` gives row 0, but
`params_from_upstream:` gives the current step, and both are pure config. A four-partition
version of this project's best model now uses both, including **both pointed at the same
upstream partition at once**. See DECISIONS.md. The letter `1b` is left unused rather than
recycled, so no entry number ever means two things.

---

## 3. No streaming or growing `data:` source

**Severity: medium — it shapes an architectural decision rather than blocking one.**

Every `data:` source builds a **complete** `StateTimeStorage` and returns: `csv`,
`json_log` and `postgres` in-engine (`pkg/api/macros_data.go`), plus `arrow` and
`s3` registered by the CLI. The macros tier consumes a finished storage; nothing
anywhere consumes a growing one. The websocket in `pkg/api/socket.go` is output
only — there is no ingress.

Phase 3 assumed a "streaming source stanza" exists. It does not.

**Not necessarily worth closing.** `RegisterDataSource`'s contract is
`build(fields) (*StateTimeStorage, error)`, so a downstream source may block on a
live feed and return once a window is full — streaming ingress with no engine change
and no change to the macro tier's completeness assumption. This project's Gate 3.4
evidence measured that path at ~1100 rows per compute-second, which is ample. The
gap is only real if genuinely *continuous* calibration is wanted, and that would
mean changing the analysis tier's core assumption that storage is complete before
anything consumes it — a much deeper change than adding a source type.

**Settled 2026-07-31: do not close this.** Gate 3.4 selected branch 1 — inference stays
downstream — so the blocking-source path above *is* the chosen architecture rather than a
workaround for a missing feature. Continuous calibration is out of scope, and the
windowing evidence argues it should stay out: ESS halves as the window doubles, so
growing storage would buy the long-window regime that degrades calibration. Kept here as
a recorded non-gap, so a future reader does not close it thinking it was an oversight.

---

## 4. No data-agreement or schema-negotiation layer

**Severity: low — the plan assumed one; the engine's approach is simpler.**

Spike 3.2 set out to "exercise the column-mapping validation and schema
negotiation path". Grepping the engine finds no such layer. The Postgres table shape
is fixed by the engine at `(partition_name, time, state)` and is written by
`CREATE TABLE IF NOT EXISTS` (`pkg/analysis/postgres.go`), the same three columns
for both source and sink, so it already serves the dual role without negotiation.

Recorded not as something to build but so the next reader does not go looking for
it. The `csv` source's `state_columns` map is the only column mapping that exists,
and it is unvalidated beyond index bounds.

---

## 5. The config's `run:` ensemble block is unreachable from Go without printing

**Severity: low — it decides which layer owns the seeds, and forces the wrong one.**

The config schema declares ensemble parameterisation as data. `RunModeConfig`
(`pkg/api/program.go`) is a `run:` block carrying `mode: ensemble`, `seeds:` and
`concurrency:`, and the CLI honours it: `api.Run` switches on `config.Run.Mode` and, for
ensemble mode, runs one member per configured seed. So *in principle* an ensemble's seeds
and size live in YAML, next to the model they parameterise.

But `api.Run(config, socket)` returns nothing — it **prints** every member to stdout via
`printEnsemble` and exits. The function that does the work and returns the storages,
`ensembleRuns(config, resolvedSim) ([]simulator.EnsembleRun, error)`, is explicitly the
"testable core ... performs no output" — and it is **unexported**, and it additionally
reads `config.sourcePath`, an unexported `yaml:"-"` field. So a downstream Go harness that
wants a config's declared ensemble AND the resulting storages (to compute a statistic
rather than print rows) has no exported path to it.

**What it blocks, concretely.** This project scores every claim on an ensemble, so
`pkg/cfgrun.RunEnsemble` calls the one exported ensembler, `simulator.RunSeededEnsemble`,
with a hand-built `build` closure and **seeds passed as a Go argument** (`DefaultSeeds`).
That works — it is not a blocker — but it means the ensemble parameterisation lives in Go,
not in the config. The `run:` block that the schema provides for exactly this is dead
weight from a programmatic caller: none of this repo's configs carry one, because nothing
here could consume it. For a project whose standing principle is that the model is pure
config, having the *seeds and member count* forced into Go is the gap — small, but real,
and squarely on the layer boundary this file is meant to police.

**The fix is narrow.** Export a storage-returning ensemble entry point — essentially
`ensembleRuns` under a public name, e.g. `api.RunEnsembleToStorage(config *ApiRunConfig)
([]simulator.EnsembleRun, error)`, honouring the YAML `run:` block — or have `api.Run`
optionally accept a sink instead of always printing. Either collapses
`cfgrun.RunEnsemble` to a thin call with `seeds:`/`concurrency:` living in the config. The
existing constraints on `ensembleRuns` would carry over and should be documented on the
exported version: it requires a file-loaded config (members are rebuilt by re-loading the
path), rejects embedded runs, and requires data-only partitions.

**The honest counter-case, recorded so the maintainer can dismiss this fairly.** Seeds and
member count are arguably a property of the *scoring harness*, not of the model — this
project sizes `DefaultSeeds` from a measured standard-error target, which is a decision
about how precisely to measure a claim, not about the dynamics. On that reading, seeds
belong in Go and this is a preference, not a gap. The reason it is filed anyway is that the
engine's *own schema* already took the other position by putting `seeds:` in the config;
the gap is the asymmetry between a schema that says "ensemble parameters are data" and a
programmatic surface that can only act on them by printing. If the answer upstream is "the
`run:` block is for the CLI only, embedders should pass seeds in Go", that is a legitimate
resolution — and it should be *stated*, because right now nothing says it and the dead
`run:` block invites a downstream author to expect a path that is not there.

**Not verified by a failing example, because there is nothing to fail** — the workaround is
clean and in use. Verified by reading the exported surface: across `pkg/`, the only exported
symbol returning ensemble storages is `simulator.RunSeededEnsemble` (seeds-as-argument), and
no exported function consumes an `ApiRunConfig.Run` block and returns storages.

---

## Resolved

### The expressions DSL had no scan across `each` lanes — entry 1

**Fixed in v0.14.0**, reported from this project. `scan(count, i, acc, init, expr)` threads
an accumulator through the lanes: lane `i` sees the previous lane's value, `init` at lane
0, and the call's value is the last lane's — a fold rather than a map. Crucially **a scan
lane may be any width**, which is what closes the case that asked for it: the thing carried
between lanes is *which slots are now taken*, a vector, not a running total.

Verified from config here, on exactly the allocation problem the entry named as blocked —
assigning *k* simultaneous arrivals to the first *k* free slots:

```yaml
scan(5, i, acc, concat(fill(5, 0), k),
     concat(slice(acc, 0, i),
            where(slice(free, i, 1) * slice(acc, 5, 1) > 0, 1, 0),
            slice(acc, i + 1, 4 - i),
            slice(acc, 5, 1) - where(slice(free, i, 1) * slice(acc, 5, 1) > 0, 1, 0)))
```

`free = [0 1 0 1 1]` with `k = 3` gives `[0 1 0 1 1 0]` — slots 1, 3 and 4 assigned, no
arrivals left over. **Order identity is therefore expressible as pure config**, and with it
the queue-position stability output, which Spike 4.2 recorded as *not answerable*.

Note the spelling needs the entry-2 fix too: the last lane's `slice(acc, 5, 0)` is
zero-width. The two fixes compose, and neither alone would have been enough.

### `slice` rejected a zero width — entry 2

**Fixed in v0.14.0**, reported from this project. A zero width now yields an empty value
rather than panicking, so the natural prefix-sum spelling `each(n, i, sum(slice(q, 0, i)))`
no longer dies on lane 0 — and, more importantly, slicing to the end of a vector inside a
`scan` no longer needs a guard at the final lane.

The guarded idiom the entry documented still works and is left in the shipped configs
unchanged; nothing was rewritten to use the new behaviour, because rewriting a working
model to exercise a fix is how a regression gets introduced for no result.

### `window_data_history_depth` larger than the window silently inverted the likelihood

Reported from this project, fixed in **v0.13.1**. It must *equal* `Window.Depth`,
not merely be `>=` it; anything larger anchors the window in zero-filled rows and
the likelihood is scored against zeros, which does not weaken the signal but
**inverts** it — a near-zero parameter outscores the truth and a posterior converges
onto its prior. Validation rejected only the too-shallow side. It now warns on the
dangerous side.

### `comparison.model` with no `params:` panicked on a nil map

Reported from this project, fixed in **v0.13.1**. `ParameterisedModel.Init`
allocated the two wiring maps but not `Params`, so the natural shape for a
likelihood taking its mean from upstream died with a bare `assignment to entry in
nil map`. Both model types now allocate it.

## Reusable iterations surveyed — not gaps, recorded so the driver's hand-rolling is a choice

The engine ships point-process and continuous iterations the models here approximate by hand:
`cox_process` (doubly-stochastic Poisson — a Poisson count with a stochastic intensity, which is
exactly what the flows are), `hawkes_process` (self-exciting intensity — the temporal clustering
the AR(1) driver produces), `ornstein_uhlenbeck`, `gamma`, and `poisson_process`.

None is a drop-in for the hand-rolled `activity` partition, and that is a deliberate choice, not a
gap: the driver is a discrete gamma-AR(1) (`act = ρ·prev + (1−ρ)·gamma(…)`) whose heavy *positive*
tail is the dispersion this project measures — `ornstein_uhlenbeck` is Gaussian and can go
negative, so swapping it would change the dynamics under study. The LOB partitions (flows, book,
observables) are inherently bespoke. Recorded because these iterations ARE the formal processes the
model stands in for, and are the natural building blocks for the state-space likelihood the Phase 3
offline-calibration boundary needs (see DECISIONS.md) — a future rebuild should reach for them.
