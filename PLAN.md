# LOB Microsimulation — Implementation Plan

**Domain:** Limit order book microsimulation, calibrated against real market microstructure data.
**Framing:** Market-stability methodology, not price prediction. Every output must be a counterfactual about market state (spread response, depth recovery, queue dynamics), never a directional claim.
**Relationship to existing work:** Replaces synthetic parameters in `umbralcalc/lobsim` with fitted ones. The domain model largely exists; this project is calibration, ingress/egress, and validation.

---

## How to read this document

Each **spike** is a small, time-boxed piece of exploratory work whose *only* deliverable is a resolved decision. A spike is not complete when code runs — it is complete when the decision it gates has been made and recorded, with the evidence that forced it.

Each **decision gate** states the branches explicitly. If a spike result doesn't clearly select a branch, escalate rather than picking one; an ambiguous result is itself information about the design.

**Marked `⚠️ UNVERIFIED`** — assumptions about the stochadex codebase that the plan author could not confirm. An agent must verify these before building on them, and must report back rather than working around a mismatch. A workaround here silently changes the architecture.

---

## Phase 0 — Trust foundation (blocking)

Nothing in this plan is meaningful without CI. A calibration result that can't be re-run on every engine change is a screenshot, not a claim.

**Prerequisites, in order:**

1. CI running the full test suite with `-race`.
2. Postgres service container in CI (needed from Phase 3; establish now so the schema work isn't blocked later).
3. Claim-ID mechanism in place: each behavioural claim gets a stable identifier, cross-referenced to the test that verifies it and the observed result.

**Gate:** Do not begin Phase 1 until (1) and (3) are live. (2) may lag until Phase 3 begins, but not later.

---

## Phase 1 — Synthetic parameter recovery

**Purpose:** Verify the inference stack can recover known parameters from its own generative process. This is the only phase where failure is unambiguous, which makes it the anchor for everything downstream.

**Why first:** If the importance sampler cannot recover parameters from order flow it generated itself, no result against real data means anything. This phase is cheap and its failure mode is decisive.

### Spike 1.1 — Parameterisation inventory

Enumerate the free parameters in the existing `lobsim` model. For each: name, physical meaning, plausible range, and whether it's identifiable from message-level data alone.

**Decision gated:** Which parameter subset goes into the Phase 1 recovery target.

**Branches:**
- **≤5 identifiable parameters** → proceed with the full set; `NewPosteriorEstimationPartitions` is in its sweet spot.
- **>5 identifiable parameters** → select a reduced subset for Phase 1 and record which parameters were held fixed and why. The reduced set must still include at least one arrival-rate and one cancellation-rate parameter, since those carry the stability questions.
- **Parameters not identifiable from message data at all** (e.g. anything about hidden liquidity) → exclude and document. These become a known limitation in the writeup.

⚠️ **UNVERIFIED:** The plan assumes `lobsim`'s parameters are exposed in a form the analysis layer can vary. If they're compiled-in constants rather than config-driven, this spike expands considerably — report before proceeding.

### Spike 1.2 — Sampler wiring

Wire `NewPosteriorEstimationPartitions` against synthetic flow with parameters set to known values. Run recovery.

**Decision gated:** Is the inference stack usable for this domain as-is?

**Success criteria — all three must hold:**
- Posterior mass concentrates on true values (define the tolerance *before* running; do not tune it to the result).
- ESS (`1 / Σ wᵢ²`) stays above threshold across the run — record the trajectory, not just the final value.
- Recovery is stable across at least 3 different true-parameter settings, including one near a range boundary.

**Branches:**
- **All three hold** → proceed to Phase 2 with confidence in the sampler.
- **Recovery works but ESS collapses** → this is the documented switch signal. Escalate: the EnKF/SMC question arrives earlier than planned and is a methodology decision, not an implementation one.
- **Recovery fails on boundary settings only** → narrow the prior ranges and document the restriction.
- **Recovery fails generally** → stop. Do not proceed to real data. Debug the sampler or the parameterisation; a failure here invalidates the whole project.

⚠️ **UNVERIFIED:** `NewPosteriorEstimationPartitions` returns `[]*simulator.PartitionConfig` like the other eight constructors in `pkg/analysis`, but the exact settings block it expects is unconfirmed. Verify the signature from the codebase before writing config.

**Deliverable:** CI-enforced recovery test with a stable claim ID. This test is the trust bond for every later claim.

---

## Phase 2 — crypto spot calibration

**Purpose:** Calibrate against real market data. First contact with reality.

**Data:** Binance public spot depth-diff and trade streams, recorded from unauthenticated endpoints. Depth snapshot paired with the diff sequence, bucketed into one state row per second.

**Known constraint:** Recorded segments cover minutes, not days, on a handful of symbols. Sufficient for calibration and validation; insufficient for any claim about generality across market conditions. State this limitation explicitly in outputs rather than letting a reader infer breadth.

### Spike 2.1 — Message format to state spine

Map the provider’s message schema onto the model's event types. Handle the documented edge cases: dummy fill values for unoccupied levels (`-9999999999` / `9999999999`, volume 0), and trading-halt messages (type 7).

**Decision gated:** How halts and dummy levels are represented in the state.

**Branches:**
- **Halts as an explicit event type in the model** → richer, but expands the parameterisation and may push past the sampler's dimension sweet spot.
- **Halt periods excluded from the calibration window** → simpler and defensible; document as a scope boundary.
- Recommended default: **exclude for Phase 2**, revisit only if halt dynamics turn out to matter for the stability outputs in Phase 4.

### Spike 2.2 — Held-out residual diagnostics

Calibrate on part of the sample, evaluate on held-out message data. Report residuals against the specific quantities the stability framing depends on: inter-arrival distributions by event type, cancellation timing relative to queue position, size distributions.

**Decision gated:** Does the parametric form suffice, or is a learned component justified?

This is the decision that determines whether Phase 5 happens at all.

**Branches:**
- **Residuals acceptable across all diagnostic quantities** → parametric form suffices. **Defer ONNX to a different domain.** Do not add a learned component to a model that doesn't need one; it produces an unconvincing artifact and weakens the ecosystem claim rather than strengthening it.
- **Residuals acceptable except in one identifiable place** (most likely: size distribution tails or inter-arrival clustering) → Phase 5 proceeds, scoped to exactly that component. Record which quantity failed and by how much — this is the motivation Phase 5 must point back to.
- **Residuals bad across the board** → the model form is wrong, not the parameters. Return to the domain model before proceeding to Phase 3.

**Deliverable:** Calibration result plus residual diagnostics, both CI-enforced. Honest reporting of where the fit is weak is more valuable here than a clean summary.

---

## Phase 3 — Streaming ingress

**Purpose:** The phase that earns this project's place in the ecosystem. Exercises the streaming source stanza, the data-agreement layer, and Postgres as both observation source and simulation output sink under real conditions.

**Data:** Crypto exchange WebSocket depth stream (Binance, Coinbase, or OKX — all publish free real-time diff updates suitable for local book reconstruction). Take the WebSocket path, not REST: REST endpoints give periodic snapshots, and reconstructing a continuous order-level record from snapshots is a window-walking and gap-filling project that produces a worse dataset for more work.

**⚠️ Scope discipline:** Phase 2 and Phase 3 fit *different markets with different microstructure* — spot and perpetuals. These are two calibrations, not one pipeline. The writeup must be explicit about this or the honest-methodology framing frays — a reader who thinks spot and perpetuals are being treated as one dataset will discount everything.

### Spike 3.1 — Reconnect and gap handling

Build book reconstruction from diff updates. Deliberately induce disconnects and verify behaviour.

**Decision gated:** What happens when a sequence gap is detected?

**This is the highest-risk item in the plan.** Silently dropped updates corrupt the book, and the corruption is *invisible in aggregate statistics* — spread and depth summaries look entirely normal while the book state is wrong. A calibration on corrupted book state produces plausible parameters that mean nothing.

**Required:** Explicit sequence-number gap detection with a loud failure mode. Not a warning log — a hard stop or an explicitly-marked data-quality flag that propagates into the calibration.

**Branches:**
- **Gap detected → resnapshot and resume, marking the interval as suspect** → preferred. Calibration excludes suspect intervals.
- **Gap detected → hard fail the run** → acceptable for early work, unworkable for long-running collection.
- **Gap tolerated silently** → unacceptable. If this is where an implementation lands by default, that is a finding to report, not a state to accept.

### Spike 3.2 — Postgres schema and data agreement

Design the schema serving both roles: observation source and simulation output sink. Exercise the column-mapping validation and schema negotiation path.

**Decision gated:** Does the existing data-agreement layer handle the dual-role case, or does it need extension?

⚠️ **UNVERIFIED:** The engine supports Postgres and S3 source types, but the skill documentation only surfaces `csv` and `json_log`. Field spellings for the Postgres source stanza must be read from the codebase, not guessed. Whatever this spike learns should feed directly into the pending SKILL.md documentation edit — that edit is blocked on exactly this information.

### Spike 3.3 — Race conditions under live feed

Run the streaming path under `-race` against a live feed.

**Decision gated:** Is the streaming path race-clean, and can it be tested deterministically?

**Hazard:** A live feed makes `-race` failures probabilistic rather than reproducible. A clean run proves less than it appears to.

**Required:** A recorded-feed replay harness that makes race testing deterministic. Capture a live segment, replay it in CI. Without this, the streaming path cannot be regression-tested and does not meet the trust standard the rest of the ecosystem holds.

### ⚠️ Decision gate 3.4 — Invariant A boundary (architectural, not implementational)

**This gate requires the maintainer, not the agent.**

A continuously-running streaming calibration is inference-as-forward-simulation in its most literal form. All nine exported constructors in `pkg/analysis` return `[]*simulator.PartitionConfig`, making inference structurally identical to aggregation. Phase 3 walks directly into this boundary.

**The decision gets made in Phase 3 whether or not it is made deliberately.** If the agent implements streaming calibration without this being resolved, the boundary is resolved by fait accompli — which is precisely the outcome previously identified as worth avoiding.

**Branches:**
- **Inference stays downstream** → the streaming calibration lives in the domain repo; the engine sees only forward simulation. Requires an explicit interface at the boundary.
- **Inference-as-forward-simulation is admitted into the engine** → Invariant A is restated, not violated. Requires deliberate reformulation, documented.
- **Deferred** → then Phase 3 must be scoped to *offline* calibration on recorded streams only, with online calibration explicitly out of scope until the boundary is resolved.

**Agent instruction:** Halt at this gate and escalate. Do not select a branch.

> **RESOLVED 2026-07-31 — branch 1, inference stays downstream. Selected by the
> maintainer.** The agent halted here as instructed, gathered evidence, and presented
> the branches priced; the choice was the maintainer's. Full reasoning and the evidence
> it was made against are in [DECISIONS.md](DECISIONS.md) under "Gate 3.4".
>
> Two corrections to this gate's framing, both established while gathering that
> evidence and both worth reading before trusting the text above. **The gate's headline
> question was already answered upstream:** stochadex has since restated Invariant A for
> the config surface — inference-as-forward-simulation is *in scope for the engine*,
> while the dataset, the calibration loop and the decision layer stay downstream. And
> **branch 2's cost is misidentified above:** admitting streaming would not mean
> admitting inference (settled, in scope) but admitting *growing storage*, which breaks
> the analysis tier's assumption that a `StateTimeStorage` is complete before macros
> consume it.
>
> Three of this phase's premises also do not hold against v0.13.1 — there is no
> streaming source stanza, no data-agreement layer, and the Postgres schema is fixed by
> the engine rather than negotiated. Phase 3's shape is consequently: collector →
> Postgres → existing source → windowed calibration, with the source contributed
> downstream via `RegisterDataSource` and **no engine change**.

---

## Phase 4 — Arrow egress and stability outputs

**Purpose:** Make the stability framing legible rather than asserted. The outputs are the framing — if the dashboards show spread-resilience curves rather than price paths, nobody reads it as a trading claim, because it isn't shaped like one.

### Spike 4.1 — ArrowStateTimeStorage on the output path

Replace per-row heap allocation with contiguous builder-append. Deliver Arrow IPC output consumable by DuckDB/Polars/pandas.

**Decision gated:** Is the Arrow path performance-neutral-or-better on the hot loop? (Invariant B.)

**Branches:**
- **Neutral or better** → proceed; this was already identified as a genuine improvement over `StateTimeStorage`.
- **Regression on the hot loop** → Arrow moves strictly to the egress boundary with a copy at the edge. The state spine stays dense row-oriented `[]float64`. Do not compromise the hot loop for interchange convenience.

**Measurement required:** Allocation profile, not just wall-clock. The invariant is about allocations in the hot path.

### Spike 4.2 — Counterfactual output suite

Implement the stability outputs:
- Spread response to a shock in order arrival intensity
- Fraction of resting liquidity surviving a large marketable order
- Depth recovery time following a liquidity event
- Queue-position distribution under varying tick regimes

**Decision gated:** Which of these are actually answerable given the calibrated parameterisation?

Some may not be, depending on what Phase 1 and 2 excluded. **Mark unanswerable outputs as such rather than approximating them** — an honestly absent output is worth more than a plausible one that the calibration doesn't support.

---

## Phase 5 — ONNX (conditional)

**Only proceeds if Spike 2.2 identified a specific failure in the parametric form.**

Scope is exactly the component that failed — a learned inter-arrival or size distribution, embedded in the transition, not a classifier bolted alongside.

### Spike 5.1 — ONNX runtime allocation profile

**Decision gated:** Does ONNX inference satisfy Invariant B?

**Branches:**
- **No per-step allocation** → proceed; this is the demonstration that a Python-trained model runs in the hot loop without violating the invariant.
- **Allocates per step** → the whole ONNX axis needs redesigning. Better discovered on one model than eleven. Report and stop; do not work around it with a caching layer that hides the allocation.

**This spike is the reason ONNX was sequenced first among the ecosystem axes.** Its failure is informative for every future domain, which is why it's worth running even on a single vertical.

---

## Out of sequence — Dagster

Dagster is a distribution question, not an engine question. It waits until Phase 3 has a stable Postgres schema to orchestrate against.

The uninteresting version demonstrates that stochadex runs as a job under an orchestrator — true of any binary, proves nothing. The interesting version wraps the config-only path with Postgres as both source and sink, giving the schema-negotiation story a real user.

**Do not start before Phase 3 completes.**

---

## Decision summary

| Gate | Resolves | Owner |
|---|---|---|
| 1.1 | Parameter subset for recovery | Agent |
| 1.2 | Sampler viability; EnKF/SMC escalation | Agent → escalate on ESS collapse |
| 2.1 | Halt and dummy-level representation | Agent |
| 2.2 | Whether Phase 5 happens at all | Agent |
| 3.1 | Gap handling semantics | Agent |
| 3.2 | Data-agreement extension needs | Agent → feeds SKILL.md edit |
| 3.3 | Deterministic race testing | Agent |
| **3.4** | **Invariant A boundary** | **Maintainer — RESOLVED 2026-07-31, branch 1: inference stays downstream** |
| 4.1 | Arrow placement vs. Invariant B | Agent |
| 4.2 | Which stability outputs are supportable | Agent |
| 5.1 | ONNX viability across the ecosystem | Agent |

---

## Standing constraints

- **Invariant A:** Engine owns forward simulation; downstream repos own inference, calibration, and decision layers. Gate 3.4 tests this directly.
- **Invariant B:** A capability enters the engine core only if performance-neutral-or-better on the hot loop, with no allocations in the hot path. Gates 4.1 and 5.1 test this.
- **Framing discipline:** Every output is a counterfactual about market state. No directional claims, no price paths in dashboards.
- **Honest limitation reporting:** Small sample coverage, excluded parameters, unanswerable outputs, and weak residuals all get stated plainly. The methodology claim survives honest limitations; it does not survive an oversold result.