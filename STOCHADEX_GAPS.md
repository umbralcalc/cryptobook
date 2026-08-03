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

Checked against **v0.13.1**.

## Re-verified in full, 2026-08-02 — method recorded, not just the verdict

Every open entry was re-checked before being taken upstream, and the method is written
down because two claims in this project were wrong this week for the same reason: a
mechanism assumed absent because it had not been exercised. A gap entry asserting the
engine *cannot* do something deserves the same adversarial check as a claim that a model
*can*.

| entry | verdict | how it was checked |
|---|---|---|
| **1. no scan across `each` lanes** | **confirmed**, three independent ways | the DSL registry has 30 functions and none is a fold (`scan`/`fold`/`cumsum`/`prod` all absent); `out` is a local Go slice never bound into the lane environment (read at `pkg/general/expression.go`, the `each` case); and the prefix-sum workaround the entry says is *not* blocked returns exactly the documented `[3 2 1 1 0 0]` when run |
| **2. `slice` rejects a zero width** | **confirmed**, reproduced | running `sum(slice(v, 0, 0))` gives `panic: expression: slice's width must be at least 1` |
| **3. no streaming/growing `data:` source** | **confirmed as described** | every source returns `(*simulator.StateTimeStorage, error)`, and so does `RegisterDataSource`'s `build` signature — there is no growing variant to register. The engine websocket sets `OutputFunction` and never reads a message, so it is output-only. `arrow` and `s3` are contributed by the CLI, not in-engine, as the entry says |
| **4. no data-agreement layer** | **confirmed as described** | zero matches anywhere in `pkg/` for schema negotiation or data agreement; the Postgres table is a fixed `CREATE TABLE IF NOT EXISTS (partition_name, time, state)` |

**Only entries 1 and 2 are actionable upstream.** Entry 3 is recorded as a deliberate
NON-gap — Gate 3.4 selected the architecture that makes the blocking-source path correct
rather than a workaround — and entry 4 exists so the next reader does not go looking for a
layer that was never designed. Neither should become an issue.

**Entry 1 is the one that matters.** It is the only gap that has forced a modelling
decision: order identity is unsayable, so PLAN.md's queue-position stability output cannot
be answered, and Gate 3.4's resolution places the fix in the engine because a `scan`
primitive concerns the expressiveness of forward simulation.

### One entry was filed wrongly and withdrawn, which is why this section exists

A high-severity entry 1b — "the expressions tier has no same-step cross-partition read" —
was filed and then retracted the same day. `upstreams:` gives row 0, but
`params_from_upstream:` gives the current step, and both are pure config. A four-partition
version of this project's best model now uses both, including **both pointed at the same
upstream partition at once**. See DECISIONS.md. The letter `1b` is left unused rather than
recycled, so no entry number ever means two things.

---

## 1. The expressions DSL has no scan across `each` lanes

**Severity: high — it is the one gap that has forced a modelling decision.**

`each(n, i, expr)` is the DSL's only non-elementwise construct. It binds a lane
index and runs lanes in order, but `out[i]` is never exposed to the inner
environment (`pkg/general/expression.go`), so **a lane cannot read earlier lanes of
its own comprehension**. There is no cumulative fold, and no other construct
provides one — `sum` and `dot` reduce to a scalar, and there is no `prod`.

This is a deliberate property, not an oversight: `each` is documented as "a bounded
comprehension with no assignment and no recursion, so an expression still always
terminates". Any fix has to preserve that.

**What it did NOT block.** A limit-order book sweep — a marketable order consuming
across price levels — looks like a scan but can be reformulated as prefix-sum plus
clamp:

```yaml
- {name: cumsum, expr: "each(6, i, where(i == 0, 0, sum(slice(q, 0, max(i, 1)))))"}
- {name: taken,  expr: "clamp(sweep_size - cumsum, 0, q)"}
```

Verified: `q = [3 2 1 4 5 6]` with a sweep of 7 gives `taken = [3 2 1 1 0 0]`. It is
O(n²) in slices rather than O(n), which is irrelevant at book depth.

**What it DID block: order-level identity.** Modelling a FIFO queue needs
per-order records and, critically, assigning *k* simultaneous arrivals to the first
*k* free slots. Each new order must see which slots the previous ones just took.
That cannot be reformulated as a prefix sum, because what is accumulated is *which
slots are now taken* rather than a running total — the classic allocation problem.

So this repo's model can have prices, a moving touch and book-walking as data, but
**not order identity**, and therefore cannot answer PLAN.md's queue-position
stability output. The alternative is bespoke Go, which would break the property
that every model here is pure config.

**What would close it.** A fold with a bounded accumulator, e.g.
`scan(n, i, acc, init, expr)` where lane `i` sees the previous lane's value. It
keeps termination (still bounded, still no recursion, still no assignment beyond the
threaded accumulator) and would make allocation, running maxima and true prefix
operations sayable in O(n).

**Where it belongs, settled 2026-07-31.** This entry used to pose its own resolution as
an open Invariant A question — bespoke Go downstream, or a primitive in the engine. Gate
3.4 answered it: the engine owns forward simulation and inference-as-forward-simulation,
downstream owns the dataset, the calibration loop and the decision layer. A `scan`
primitive is squarely about **the expressiveness of forward simulation**, so it is engine
work, and closing it downstream in Go would put domain code where the boundary says the
engine belongs. **This is the highest-value candidate for upstream release.**

---

## 2. `slice` rejects a zero width, which the natural prefix-sum spelling needs

**Severity: low — a sharp edge, with a working idiom.**

`slice(v, from, width)` panics when `width < 1` (`pkg/general/expression.go`). The
obvious prefix sum `each(n, i, sum(slice(q, 0, i)))` therefore dies on lane 0, where
the correct answer is simply 0.

It is workable because `where` is lazy inside `each`, so the guarded form never
evaluates the bad slice:

```yaml
each(6, i, where(i == 0, 0, sum(slice(q, 0, max(i, 1)))))
```

Both the `where` guard **and** the `max(i, 1)` are needed — the guard for
correctness, the `max` because the argument is still parsed and bounds-checked in
the general case. Worth a line in the DSL documentation, since prefix sums are the
main reason to reach for `each` at all.

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
