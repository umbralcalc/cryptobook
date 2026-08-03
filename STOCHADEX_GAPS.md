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

**Entry 1 was the one that mattered**, and it is the reason this file exists in the form it
does: it was the only gap that ever forced a modelling decision. Order identity was
unsayable, so PLAN.md's queue-position stability output could not be answered. `scan`
closes it, and Gate 3.4's resolution is what placed the fix in the engine rather than in
downstream Go — a `scan` primitive concerns the expressiveness of forward simulation.

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

PLAN.md's Phase 3 assumed a "streaming source stanza" exists. It does not.

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

PLAN.md's Spike 3.2 set out to "exercise the column-mapping validation and schema
negotiation path". Grepping the engine finds no such layer. The Postgres table shape
is fixed by the engine at `(partition_name, time, state)` and is written by
`CREATE TABLE IF NOT EXISTS` (`pkg/analysis/postgres.go`), the same three columns
for both source and sink, so it already serves the dual role without negotiation.

Recorded not as something to build but so the next reader does not go looking for
it. The `csv` source's `state_columns` map is the only column mapping that exists,
and it is unvalidated beyond index bounds.

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
PLAN.md's queue-position stability output, which Spike 4.2 recorded as *not answerable*.

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
