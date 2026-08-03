# Decision log

PLAN.md's rule: a spike is complete when the decision it gates has been made and
recorded, *with the evidence that forced it*. This file is that record. Each entry
names the gate, the branch selected, and the evidence — or states that the gate is
open and why.

Verified claims about model behaviour live in [CLAIMS.md](CLAIMS.md), which is
generated and CI-enforced. This file records *design* decisions, which have no
numbers to bind.

---

## Phase 0 — Trust foundation

**Status: complete.** All three prerequisites are live.

### (1) CI running the full test suite with `-race`

Resolved. [.github/workflows/ci.yml](.github/workflows/ci.yml) runs `go build`,
`go vet`, and `go test ./... -race -count=1` on every PR and push to `main`.

Evidence for `-race` being load-bearing rather than ceremonial: the engine runs a
goroutine per partition (stochadex `CLAUDE.md`, "How partitions fit together"), and
Phase 3 adds a feed goroutine alongside them. Modelled on `umbralcalc/stochadex`'s
own `ci.yml`.

**Note:** no other downstream repo in this ecosystem has CI at all
(`energy-balancer`, `antimicrobial-resistance`, `business-survival`, `trywizard`,
`openaction2outcome` were all checked — none has `.github/workflows/`). So this is
not a convention being followed; it is one being set. Worth knowing before treating
a sibling repo's layout as a template.

### (2) Postgres service container in CI — stood up, then removed

**Status: removed 2026-07-29.** It was live for Phase 0 as PLAN.md requires
("needed from Phase 3; establish now so the schema work isn't blocked later"), and
it never had a user.

The reasoning for standing it up early was that an unused service container is
cheap and a missing one is a phase-boundary stall. That was sound at the time and
wrong in the event, because the stall it was insuring against cannot happen: Gate
3.4 established that **Phase 3's premise does not hold**. The engine has no
data-agreement layer and no schema negotiation, its Postgres table shape is fixed
at `(partition_name, time, state)` rather than negotiated, and the source stanza's
field spellings were verified by reading `pkg/api/macros_data.go` — no live
database was needed for any of it.

So it was provisioning a database on every push that nothing ever connected to.

**Restoring it costs a few lines**, and the two details that were expensive to work
out are kept here rather than lost with the config:

- `pkg/analysis` builds its connection string from `user`/`password`/`dbname` with
  **no host**, so `lib/pq` only reaches a service container if `PGHOST` and `PGPORT`
  are set in the job's `env`.
- `PGHOST` must be `127.0.0.1`, **not** `localhost`. The service container publishes
  its port mapping on IPv4, and `localhost` resolves to `[::1]`.

```yaml
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: cryptobookuser
          POSTGRES_PASSWORD: cryptobookpassword
          POSTGRES_DB: cryptobookdb
        ports: ["5432:5432"]
        options: >-
          --health-cmd "pg_isready -U cryptobookuser -d cryptobookdb"
          --health-interval 10s --health-timeout 5s --health-retries 5
    env:
      PGHOST: 127.0.0.1
      PGPORT: "5432"
```

The general lesson is worth more than the container: **provisioning early against a
future requirement is only cheap if the requirement is real.** This one was taken
from the plan rather than verified, and the plan turned out to be wrong about the
engine. Nothing else in Phase 0 was provisioned that way — CI and the claim
mechanism both had immediate users.

### (3) Claim-ID mechanism

Resolved, and it reuses the ecosystem's existing mechanism rather than inventing a
parallel one.

stochadex's `models/CONVENTIONS.md` already specifies exactly this bond for its
domain-models catalogue: a `cardgen.Claim` carrying a stable `ID`, a
plain-language statement, and the observed numbers; a `behaviour_test.go` running
one subtest *named by the claim ID*; and a generator that renders claim → test →
observed into the card, guarded by a `TestCardsUpToDate`. `anglersim` is the
reference. `models/cardgen` is importable from here
(`github.com/umbralcalc/stochadex/models/cardgen` — `models/` is inside the engine
module).

[pkg/claims](pkg/claims/claims.go) therefore wraps `cardgen.Claim` and uses
`cardgen.Verify` unchanged, adding the four things PLAN.md's standing constraints
need and `cardgen` has no reason to carry:

| Field | Why it is required, not optional |
|---|---|
| `Gate` | A resolved gate must be traceable to evidence, not to a commit message. |
| `Data` | Phase 2 (crypto spot) and Phase 3 (crypto perpetuals) are **two calibrations of different markets**. Naming the dataset per claim is the mechanical guard against a reader merging them — the risk PLAN.md flags under "Scope discipline". |
| `Limitations` | "Honest limitation reporting" becomes a required field rather than a discipline someone must remember at writeup time. A claim without stated limits will not validate. |
| `Binding` | Per claim, not per file — claims here come from many tests across phases, so `cardgen`'s single-`Binding` renderer does not fit and is replaced. |

Two tests close the loop: the phase binding tests catch a broken assertion, and
`TestClaimsUpToDate` catches a number that moved without `CLAIMS.md` being
regenerated.

At the end of Phase 0 `CLAIMS.md` was **empty, and said so** — the mechanism was
live and CI-enforced before any claim existed, which is what the gate asked for. It
now carries twenty-one claims.

---

## Verification of PLAN.md's `⚠️ UNVERIFIED` assumptions

PLAN.md requires these to be checked before building on them, and mismatches
reported rather than worked around. Checked against `umbralcalc/stochadex` — first at
`v0.13.0`, now pinned at **`v0.13.1`** (see the engine findings below).

### Spike 1.2 — `NewPosteriorEstimationPartitions` location and signature

**Mismatch, benign.** The plan places it in `pkg/analysis` and describes "the other
eight constructors in `pkg/analysis`". It is in **`pkg/macros`**
(`pkg/macros/inference.go:72`), which is a newer tier layered *over* `pkg/analysis`
(`macros` depends on `analysis`, never the reverse). `pkg/analysis` is now the data
layer only: CSV/DataFrame/JSON-log/Postgres I/O, `DataRef` addressing, grouping,
plotting.

There are **twelve** exported constructors in `pkg/macros`, not nine.

Signature verified:

```go
func NewPosteriorEstimationPartitions(
    applied AppliedPosteriorEstimation,
    storage *simulator.StateTimeStorage,
) []*simulator.PartitionConfig
```

`AppliedPosteriorEstimation` is `{LogNorm, Mean, Covariance, Sampler, Comparison,
PastDiscount, MemoryDepth, Seed}`. Two constraints matter for Phase 1 and are
enforced by panics at construction, so they cannot be got wrong quietly:

1. **The comparison must read the sampler**, either via
   `Comparison.Model.ParamsFromUpstream` or as a window partition
   `OutsideUpstream`. Otherwise every sample scores identically, the weights go
   uniform, and the posterior mean random-walks away from the truth. The docstring
   names this "the classic silently-degenerate `posterior_estimation` config" — it
   is exactly the failure mode Phase 1 exists to rule out, and the engine already
   fails loud on it.
2. `MemoryDepth >= 1`; the embedded comparison defaults `burn_in_steps` to
   `Window.Depth`.

**It is also reachable without Go at all** — as a `macros:` entry
(`posterior_estimation`) over a `data:` source. So Phase 1 can be a YAML config
plus a test, not a Go program.

**Consequence for the plan's ESS-collapse branch:** `NewSMCInferencePartitions` /
the `smc_inference` macro already exists in `pkg/macros`. If Phase 1 hits ESS
collapse, switching to SMC is a config change against an existing, CI-tested
component — not the open methodology question the plan budgets for. The escalation
is still worth making, but it is much cheaper than "the EnKF/SMC question arrives
early".

### Spike 3.2 — Postgres source stanza field spellings

**Resolved.** Read from `pkg/api/macros_data.go:61`. The `data:` tier's
`source:` accepts exactly one of `csv`, `json_log`, `postgres`, plus any source
registered from above `pkg/api` via `RegisterDataSource` (the Arrow source is
contributed this way by the distributed CLI, which is why it is not in the struct).

```yaml
data:
  source:
    postgres:
      user: cryptobookuser
      password: cryptobookpassword
      dbname: cryptobookdb
      table: book_events
      partition_names: [best_bid, best_ask]
      start_time: 0.0
      end_time: 3600.0
```

The **sink** side is separate and has two spellings
(`pkg/api/registry_postgres_output.go:23`):

```yaml
output_function: {type: postgres, user: u, password: p, dbname: d, table: results}
output_function: {type: postgres, driver: pgx, dsn: "postgres://…", table: results}
```

The `driver`/`dsn` form goes through `database/sql`, so it reaches any
Postgres-wire-compatible database whose driver is compiled in — relevant if the
Phase 3 sink is not a local Postgres.

**Correction to how this was first reported here.** PLAN.md describes a pending
SKILL.md edit blocked on exactly this information, and this log initially said the
edit was now unblocked. That was wrong: the engine's own
`.claude/skills/stochadex-model/SKILL.md` **already documented the Postgres source
and both sink spellings as of v0.13.0**. What was stale was the locally *cached*
plugin copy of that skill (version 0.5.3), which surfaces only `csv` and `json_log`.

So there was nothing to unblock, and the field spellings above were confirmed from
the code rather than from either copy — which is the right habit regardless, and is
what PLAN.md asks for ("must be read from the codebase, not guessed"). The operational
lesson is narrower than a documentation gap: **a cached skill can lag its repo, so a
config surface that looks undocumented may simply be documented somewhere the cache
has not caught up with.** Worth checking the repo before concluding a capability is
undocumented.

One thing genuinely worth adding upstream: *why* the surfaced source list is shorter
than what the engine supports. Extra sources (Arrow, for one) are contributed from
above `pkg/api` via `RegisterDataSource` and live in opt-in modules outside the
engine's `go.mod`, so which sources a given binary accepts depends on how it was
built — `--version` reports the feature set.

### Spike 1.1 — are `lobsim`'s parameters exposed in a form the analysis layer can vary?

**Mismatch, and not a benign one: the model did not exist in a form this stack can
step. Escalated, and resolved by the maintainer — see below.**

---

## Phase 1 premise — escalated, resolved

### The finding

PLAN.md's framing says: *"Replaces synthetic parameters in `umbralcalc/lobsim` with
fitted ones. The domain model largely exists; this project is calibration, ingress/
egress, and validation."*

**`umbralcalc/lobsim` is a Python/Jupyter repository.** Its language breakdown is
Jupyter Notebook 565 KB, TeX 40 KB, HTML 27 KB, Python 16 KB, Shell 209 B — no Go.
The model is `src/lobsim.py` and `src/agents.py`, presented through
`LOB_simulation.ipynb`. Nothing in it is a `simulator.Iteration`.

There is also **no LOB entry in stochadex's `models/` catalogue** — its nine
entries (per `models/index.json`) are `anglersim`, `antimicrobial-resistance`,
`bathing-water-forecaster`, `business-survival`, `energy-balancer`, `floodrisk`,
`homark`, `measles-risk-forecaster`, `trywizard`. (`models/cardgen` is the shared
claim/card helper package, not a domain entry.)

So Spike 1.1's `⚠️ UNVERIFIED` note resolves worse than its own worst case. It asks
whether the parameters are "config-driven or compiled-in constants"; the actual
answer is that the generative model does not exist in a form this stack can step at
all. Phase 1 recovers parameters *from the model's own generative process*, which
presupposes that process runs as stochadex partitions.

This was reported rather than worked around, per PLAN.md's instruction that a
workaround here silently changes the architecture, and escalated to the maintainer
because each branch changes what the project is.

### Decision

**Selected: a deliberately minimal LOB generator, for Phase 1 only, written as pure
config and living in this repo — not ported into the engine's `models/` catalogue.**

Maintainer decision, 2026-07-28. Two parts:

1. *Which model.* Not a port of `lobsim`. A minimal generator whose only job is to
   make the recovery test decisive: Poisson limit arrivals per level, cancellations
   proportional to resting volume, market orders consuming at the touch. The
   consequence — Phase 2 calibrates a simpler model than the original framing
   implies — is accepted and must be stated plainly wherever results appear.
2. *Where it lives.* Here first, promoted to `models/` later if Phase 2 confirms the
   form. Building in the engine catalogue now would mean an engine PR before the
   model form is settled.

**Constraint added by the maintainer: the model is written as pure config.** No
bespoke Go iterations. It is [cfg/lob_generator.yaml](cfg/lob_generator.yaml) —
partitions, params, `expressions:` and wiring, all data, resolved in-process with no
toolchain. This turned out to be entirely achievable, including the parts that
looked like they would need Go:

- the 6-level ladder is one width-6 expression field, updated elementwise;
- "add at the touch" is a multiply by a mask param, not an index;
- `iid(6, poisson(limit_rate * dt))` gives one independent arrival stream per level
  (a bare `poisson` of a scalar is rejected — its width would be ambiguous);
- the whole inference layer is the `posterior_estimation` macro over a `data:`
  sub-simulation.

`pkg/lob` and `pkg/recovery` therefore contain **no model**. They run the configs,
read the output, and assert. [pkg/cfgrun](pkg/cfgrun/cfgrun.go) is the harness that
keeps it that way: it substitutes the one parameter a test is varying, and a
substitution whose target is missing is an error rather than a silent no-op — so a
reformatted config fails loudly instead of leaving every sweep measuring the same
value and agreeing with itself.

The scope boundary is recorded in the config's own header and in the `Limitations`
field of every claim it backs: static ladder, no price process, no spread, no queue
position, uniform arrival intensity, market orders that do not walk the book. Spike
4.2's spread-response and queue-position outputs are **not** answerable from this
model, and are marked as such rather than approximated.

---

## Spike 1.2 — sampler viability

**Result: identification is sound; the importance sampler was degenerate; SMC fixes
it. Gate 1.2 is RESOLVED — the escalation was raised as PLAN.md requires and then
settled on measurement.** Thirteen claims in [CLAIMS.md](CLAIMS.md) carry the
evidence; the tolerance was fixed in advance in
[PREREGISTRATION.md](PREREGISTRATION.md) and did not move.

### Against the three pre-committed success criteria

| Criterion | Result |
|---|---|
| Posterior mass concentrates on true values, within a tolerance fixed beforehand | **Partial.** `limit_rate` and `cancel_rate` land within 25% at every setting (worst 8.9%). `market_rate` misses at every setting: 29.8%, 31.3%, 113.3%. |
| ESS stays above threshold across the run | **Fails outright.** ESS = **1.00 of 16** draws. |
| Recovery stable across ≥3 true-parameter settings including a boundary | **Split the same way.** The two strong parameters are stable including at the boundary; the weak one is worst there. |

### Why this is the ESS branch and not "the parameterisation is wrong"

The two branches call for opposite responses, so they were separated with evidence
rather than argument. [cfg/lob_likelihood_surface.yaml](cfg/lob_likelihood_surface.yaml)
is the recovery config's comparison with the parameters held **fixed** and no sampler
anywhere, so evaluating it traces the likelihood surface directly. It peaks at the
true value of all three parameters:

| Parameter | Advantage of truth over 0.6× | over 1.67× |
|---|---|---|
| `limit_rate` | 93.6 nats | 98.4 nats |
| `cancel_rate` | 71.2 nats | 103.1 nats |
| `market_rate` | 14.5 nats | 6.9 nats |

`cancel_rate` clearing 70 nats matters most: its expected count is
`cancel_rate × resting depth`, so nothing but step-to-step variation in depth
distinguishes it. **The queue-state coupling that carries the stability question is
identified.** `TestGeneratorMatchesItsIntensityModel` independently pins the
generator's own moments to the intensities being scored (within 3%), ruling out
misspecification as well.

So the information is present and the model is right. The sampler cannot use it.

### The mechanism, and why the consequence is specific

Those same magnitudes are the problem. The log-likelihood is a cumulative over a
100-step window and three count streams, so nearby proposals differ by tens of nats
and the weights are a hard argmax — the runner-up of 16 draws sits **351 nats**
behind the best, giving ESS = 1.00.

What that does is sharper than "the posterior is noisy". With ESS ≈ 1 the posterior
mean *is* the single best sample drawn, and the proposal is centred on that mean — so
the procedure is a **stochastic hill-climb, not Bayesian updating**. Which yields the
exact pattern observed:

- Coordinates that dominate the log-likelihood climb, and climb reliably.
- A weakly-identified coordinate does **not** climb at all. The winning sample is
  chosen on the dominant coordinates' merits, so the weak one just inherits whatever
  value that sample happened to carry. At the boundary setting `market_rate` moved
  only ~30% of the way from prior to truth.

**And the posterior variance is meaningless** — it is the spread of one point. That
is the part that bites later: Phase 2's deliverable is calibrated parameters *with*
uncertainty, and this sampler cannot supply uncertainty on anything.

### Escalation, and its resolution

PLAN.md: *"Recovery works but ESS collapses → this is the documented switch signal.
Escalate: the EnKF/SMC question arrives earlier than planned and is a methodology
decision, not an implementation one."* Escalated as required, and then — on the
maintainer's instruction to settle it — decided on measurement rather than argument.

**Selected: switch to `smc_inference`.** Maintainer decision, 2026-07-28.

[cfg/lob_recovery_smc.yaml](cfg/lob_recovery_smc.yaml) runs the same model against
the same observables scored by the same intensity model, changing only the inference
layer, so the comparison isolates the sampler. `TestSmcComparesLikeForLike` enforces
that: same truth, same generator bindings verbatim, same scored count indices, the
same `observed[9]` depth coupling, and SMC given *no more* data (400 steps against
2000). Still pure config.

| | `posterior_estimation` | `smc_inference` |
|---|---|---|
| Worst error, nominal | 29.8% | **6.9%** |
| Worst error, high rates | 31.3% | **1.9%** |
| Worst error, near boundary | 113.3% | **7.6%** |
| Weakly-identified rate | not estimated at all | recovered |
| ESS | 1.00 of 16, flat | 1.00 → **9–22** of 160 (≈4 near the boundary) |
| Usable uncertainty | none — variance of one point | **truth within 1.8 posterior sd everywhere** |

The uncertainty row is the one that decided it. Point estimates were never the
blocker; Phase 2's deliverable is calibrated parameters *with* uncertainty, and the
importance sampler could not supply that at any tuning.

**Why it works, confirmed rather than assumed.** Two changes, and the measurement
separates them. Uniform priors bound the search to positive rates, which removes the
negative-proposal failure mode structurally instead of clamping it. But the decisive
change is that the proposal *contracts*: round 1 draws from the priors, each later
round from a Gaussian fitted to the previous round's weighted posterior.

The prediction was that round 1 should be just as degenerate as the old sampler, with
ESS recovering as the proposal narrows. That is exactly what happens — ESS by round,
nominal setting: **1.00, 1.08, 13.65, 12.82, 8.87**. So the cause was *proposal width
relative to the likelihood's peak*, not the algorithm's identity. Worth keeping: it
means any future estimator will be judged on the same axis, and the ESS is now read
from the run's own particle log-likelihoods rather than from a proxy.

**What is honestly still weak.** ESS peaks at ~13 of 160 (8%) nominally and only ~4
near the boundary. That is low by any conventional standard. The posterior means and
intervals hold up on this evidence, but the sampler has little margin, and the
untried lever is the particle count — not the round count, for the reason below.

**A finding that matters more than it looks: do not raise the round count.** Going
from 5 to 9 rounds improves the point estimates in the two easy settings and
*breaks calibration* in the boundary setting, where ESS plateaus around 3: the
posterior sd shrinks from 0.0015 to 0.0009 while the mean stops improving, moving the
truth from 1.79 sd out to 2.33 sd. The posterior gets tighter and wronger — the exact
failure a calibration exists to rule out. Pinned as
`raising_smc_rounds_trades_calibration_for_point_accuracy`, because "more rounds" is
the first thing a future reader will try.

**`cfg/lob_recovery.yaml` is kept, not deleted**, along with the two claims recording
its failure. It is the measured justification for this decision, and deleting it would
leave the switch looking like a preference. The claims' statements now name the
sampler they describe so no reader mistakes them for a statement about the project's
inference as a whole.

**EnKF was not evaluated.** SMC cleared the bar, and the plan's branch names them as
alternatives rather than requiring both. If the ESS margin later proves too thin —
the most likely trigger being real Phase 2 data, where the model is misspecified and
the likelihood surface is less clean — EnKF is the next thing to try, and this
comparison harness is what it should be measured in.

## Spike 3.1 — sequence-gap handling

**Branch selected: gap detected → resnapshot and resume, marking the interval
suspect.** PLAN.md's preferred branch, and the only one it considers workable for
long-running collection. The alternatives were hard-fail (acceptable early,
unworkable later) and silent tolerance (explicitly unacceptable).

**Status: the detection half is built and tested; the propagation half is not yet.**

### Why this got built before anything else in the phase

PLAN.md calls this the highest-risk item, and the reason survives restating: a
dropped update corrupts the book, and **the corruption is invisible in aggregate
statistics.** Spread and depth summaries look entirely normal while the state is
wrong, so a calibration on corrupted state produces plausible parameters that mean
nothing. Nothing downstream can detect it — it is caught at ingress or not at all.

### The contract, and that it is exact

Binance spot publishes each depth event with `U` (first update id) and `u` (final
update id) against a REST snapshot's `lastUpdateId`.
[pkg/feed](pkg/feed/book.go) implements the documented procedure: buffer while
snapshotting, discard events with `u <= lastUpdateId`, require the first applied
event to satisfy `U <= lastUpdateId+1 <= u`, and require every later event to
satisfy `U == previous u + 1`.

That last rule is exact — no tolerance, no "close enough". Either the ids chain or a
message was missed. `Apply` returns `ErrSequenceGap`, which the caller cannot ignore
without discarding a value.

Two design choices worth keeping:

- **A rejected update leaves the book untouched.** A caller that wrongly swallows
  the error gets a *stale* book rather than a corrupted one — the less dangerous
  failure, because staleness eventually shows up in the numbers and corruption does
  not.
- **A replayed or reordered id is a gap, not a no-op.** A tolerant implementation
  that ignored stale ids would silently hide a duplicated stream.

`ErrStaleSnapshot` is separated from `ErrSequenceGap` because the fix differs: fetch
a newer snapshot rather than resume from this one.

Fifteen subtests in [pkg/feed/book_test.go](pkg/feed/book_test.go) pin the contract,
including deliberately induced one-message holes, replays, multi-id events, and
recovery via resnapshot. They need no network, which is also the beginning of an
answer to Spike 3.3 — the risky logic is deterministic and testable off-line, so
`-race` against it proves something rather than being probabilistic.

### What remains

The suspect *flag* — PLAN.md requires the marking to "propagate into the
calibration", not merely to be detected. That needs the collector and the recorded
row shape, which are the next piece: every bucket a gap touches carries a
data-quality column, and calibration excludes suspect intervals rather than trusting
them. Until that exists, this gate is half-discharged and is recorded as such.

---

## Spike 2.1 (crypto form) — message format to state spine

**Decided before implementing, because every one of these is a modelling choice the
fitted numbers depend on.** The mapping lives in
[pkg/feed/bucket.go](pkg/feed/bucket.go), documented next to the code it constrains.

The goal is that real market data lands in the **same row shape**
`cfg/lob_generator.yaml` emits, so `cfg/lob_calibrate_from_log.yaml` consumes it
with no change. Indices 0–9 keep their meanings exactly; the data-quality flag is
appended at index 10 rather than inserted.

### 1. Flow is measured at absolute prices, never at offsets from the touch

The tempting alternative — track "the queue at the best bid", since the model has a
static ladder and the market does not — is wrong in a way that would not be visible
in the output. When the best bid ticks up, the old offset-0 queue becomes offset-1
and a different queue appears at offset-0. That reads as an enormous arrival and an
enormous cancellation, neither of which happened. **Every price movement would be
laundered into order flow, and the fitted rates would be measuring volatility.**

Per-price deltas have no such artifact. The band moves between buckets; within a
bucket every delta is against a fixed absolute price.

### 2. Volume is discretised into lots, and lots stand in for counts

A diff stream reports net size change per level, so one 5-lot order and five 1-lot
orders are indistinguishable — it cannot supply the event counts the Poisson
observables need. Rather than change the likelihood, which would discard Phase 1's
identification result with it, volume is divided by a lot size and the integer
treated as the count.

This is the unit-order-size idealisation the Santa Fe model makes, and therefore the
one `umbralcalc/lobsim` inherits — a named assumption rather than an improvised one.
Its cost is **predicted in advance** in [PREREGISTRATION.md](PREREGISTRATION.md):
treating one k-lot order as k independent arrivals must produce overdispersion.

### 3. Cancellations and executions are split by the trade stream

A size decrease is either a cancellation or a fill, and the depth stream cannot tell
them apart. Within a bucket, a decrease at a price is attributed to execution up to
the volume traded at that price, and the remainder to cancellation. This is the
direct parallel of the provider’s message types 2/3 against 4, which is what would make
the two describe the same quantities.

### 4. Deltas accumulate per update, not net per bucket

Netting a bucket's open against its close erases churn: volume added and removed
within the same bucket cancels out, understating both arrivals and cancellations —
and understating them *more* the busier the market is, which is precisely backwards.

### 5. The band is the top 3 price levels per side, re-derived each bucket

Matching the model's six ladder slots, and fixed within a bucket so every delta is
unambiguous.

### What is deliberately left broken

The model assumes arrivals are **uniform across levels**; real books concentrate
them at the touch. That misspecification stays. Spike 2.2 exists to measure where
the parametric form fails, and reshaping the model beforehand to match what I expect
would be fitting it to my expectations rather than to the data. The expected failure
is written down in PREREGISTRATION.md instead.

Also worth stating plainly: the synthetic claim
`the_touch_holds_less_resting_volume_than_the_level_behind_it` holds *because*
arrivals are uniform and market orders consume at the touch. It is not expected to
transfer to real data, and it is labelled synthetic in `CLAIMS.md` for that reason.

---

## Spike 2.2 — residual diagnostics: the model form does not transfer

**Result: NEGATIVE, and acted on rather than reported around.** Measured against
[testdata/btcusdt_depth.log](testdata/btcusdt_depth.log), a 480-second Binance
BTCUSDT spot capture. Three claims in [CLAIMS.md](CLAIMS.md) carry the numbers.

| diagnostic | measured | the model requires |
|---|---|---|
| `corr(depth, n_cancel)` | **−0.12** | strongly positive |
| `corr(n_limit, n_cancel)` | **+0.98** | no such mechanism exists |
| `Var/Mean`, arrivals | **3785** | exactly 1 |
| `Var/Mean`, cancellations | **4601** | exactly 1 |
| `Var/Mean`, market orders | **92** | exactly 1 |

### The line that stops Phase 2

The model's cancellation intensity is `cancel_rate × resting depth`.
[PREREGISTRATION.md](PREREGISTRATION.md) called that the most plausible of the
model's assumptions and the one Phase 1's identification rests on entirely, and
fixed the consequence before any market data existed:

> "If this one fails, the parameterisation does not transfer to real data at all
> and Phase 2 stops rather than being tuned."

It failed: cancellations are essentially uncorrelated with depth, and if anything
slightly negatively. **So no parameters are fitted from this segment.** The
machinery would have produced a tight posterior — the synthetic phases show it
converges — and a fitted cancellation rate for a market whose cancellations do not
scale with depth would be exactly the confident, unsupported number this project
exists to avoid. The bar for not doing it was set in advance, which is the only
time such a bar is worth anything.

### What is there instead

Arrivals and cancellations move in near-lockstep (+0.98 second by second). That is
quote churn: market makers pulling and re-posting together. The model treats the two
as independent Poisson streams, so this is a **missing mechanism, not a mis-tuned
rate** — and it is the most likely explanation for why cancellations track arrivals
rather than depth. It also explains the dispersion: a whole ladder added or pulled at
once is one decision producing thousands of correlated lot events.

### A prediction that was right for the wrong reason

PREREGISTRATION.md predicted overdispersion and attributed it to lot discretisation
— one k-lot order counted as k arrivals. That mechanism implies a larger lot cures
it. **Raising the lot tenfold made the measured dispersion worse** (arrivals 511 →
3785), because the wider band exposed the dominant cause: cross-level correlation,
which no lot size can absorb.

Two other pre-registered predictions, scored honestly:

- **Prediction 1's direction was wrong.** It said dispersion would be largest for
  `n_market`. Measured, `n_market` is the *smallest* of the three (92 against 3785
  and 4601) — trades are far less correlated across levels than quote updates.
- **Prediction 2 could not be scored as stated.** It expected the uniform-arrival
  assumption to show in the per-band depths, but the first band definition was
  wrong (see Spike 2.1's revision) and the replacement changes what "per level"
  means, so the comparison it described no longer exists.

### Branch selected

PLAN.md's Spike 2.2 branches: residuals acceptable → parametric form suffices, defer
ONNX; acceptable except in one identifiable place → Phase 5 scoped to that
component; **bad across the board → the model form is wrong, not the parameters;
return to the domain model before proceeding to Phase 3.**

The third. This is not one identifiable component failing — the coupling that
identifies the model is absent and the likelihood family is wrong by three orders of
magnitude. **Phase 5 (ONNX) does not proceed on this evidence**: PLAN.md is right
that adding a learned component to a model that is structurally wrong produces an
unconvincing artifact, and a learned inter-arrival distribution would not supply the
missing churn mechanism either.

### Open — for the maintainer

Returning to the domain model is a modelling decision, not an implementation one:

- **Add a churn mechanism** (coupled arrival/cancellation rather than independent
  streams). Follows the evidence most directly; a real modelling phase.
- **Change the likelihood family** (negative binomial for the dispersion). Treats
  the symptom — it does nothing about the absent depth coupling.
- **Accept crypto as out of scope for this model form** and keep this as the
  recorded negative result.

Worth weighing for the third: the model's lineage is Santa Fe / `lobsim`, which is an
equity model, so the negative result may be about the market chosen rather than about the
model. This log does not select between the three. What narrows it is `pkg/replication`:
the failure is not specific to one symbol or one window within crypto spot, which rules
out the narrowest version of "wrong market" without touching the asset-class version.

---

## Spike 3.1 — propagation half, discharged

The remaining half of Spike 3.1 (recorded above as "half-discharged") is now closed.
PLAN.md required the suspect marking to reach the calibration rather than a log
line, and the chain runs end to end:

	gap -> ErrSequenceGap -> the interval is marked -> column 10 of the row
	    -> LoadSegment drops those rows -> they are not analysed

`TestSuspectRowsAreExcluded` pins the far end, which is the half that mattered.

**Honest limitation: the recording saw zero gaps.** Over 480 seconds the connection
never dropped, so the path is proven by unit tests against deliberately induced
holes, not by a real disconnect. What has been demonstrated on live data is that the
detector does not produce FALSE positives; the true-positive path is tested only
synthetically.

## Spike 3.3 — replay harness, partially discharged

PLAN.md asks for "a recorded-feed replay harness that makes race testing
deterministic. Capture a live segment, replay it in CI."

The capture is committed at `testdata/btcusdt_depth.log` (480 rows, 52 KB) and the
diagnostics replay it with no network, so their numbers are reproducible in CI. The
`pkg/feed` logic that carries the risk — the sequence contract, the aggregation, the
bucket timing — is entirely network-free and runs under `-race` deterministically.

What is NOT discharged: the live streaming path in `cmd/record-feed` (a websocket
goroutine feeding a channel) has been run under real conditions but never under
`-race` against a replayed feed, because the replay harness stops at the Recorder
rather than driving the network shell. That is the remaining piece, and it is small.

---

## Data access policies — and what they cost us

**2026-07-29.** The data licence was read before pushing anything. It excludes
redistribution, so **no market data, raw or derived, is committed.**
[README.md](README.md) quotes it in full; the operative words are the licence "solely as
necessary to allow you to receive the Binance Services for **non-commercial personal or
internal business use**".

Aggregation does not open an escape route: Binance's definition of Intellectual Property
Rights names **database rights** explicitly. And Binance binds on access rather than
registration — this project holds no account, but recording public streams is still
"accessing the Binance Platform".

### The order of operations was wrong

The fixtures were committed first and the licences read afterwards. Nothing was
pushed, so nothing was published and the fix was cheap — but the check belonged
before the data entered the repository, not after. Recorded because the cheapness of
the recovery here was luck, not process.

### What it costs, stated rather than glossed

The Spike 2.2 diagnostics are now **the one set of results in this repo that CI
cannot re-check**. Concretely:

- `testdata/` is git-ignored; `pkg/crypto` skips on a fresh clone, naming the command
  that regenerates its input.
- It is not registered in `internal/claimset`, so it does not reach the generated
  `CLAIMS.md`. Registering it would make the page depend on a file a fresh clone lacks,
  and `TestClaimsUpToDate` would then fail for whoever had less data.
- Its numbers live here, as prose with provenance, which is the honest place for a
  result nothing can automatically re-verify.
- `CLAIMS.md` says on its own face that measurements needing non-redistributable
  data are excluded, so a reader counting claims cannot mistake the omission for
  completeness.

**This is a genuine weakening of the claim↔test↔result bond** that Phase 0 built and
that every other result in the repo leans on. It is accepted because the alternative
is redistributing data under a licence that forbids it. It is not a precedent for
loosening the bond anywhere else.

The one mitigation that genuinely works is re-recordability: a segment can be regenerated
from public endpoints in minutes, so `pkg/replication` states its findings as **bounds any
fresh recording must satisfy** rather than as point values. A stranger can falsify those
in eight minutes, which a committed fixture cannot be.

The property that makes the arrangement safe is mechanical and was verified rather
than assumed: `CLAIMS.md` regenerates **byte-identical with and without the fixtures
present** (same MD5 both ways), so CI and a machine holding the data cannot disagree
about the page.

---

## Spike 4.2 — counterfactual output suite: one of four is supportable

PLAN.md lists four stability outputs and gates on **which are actually answerable**,
instructing that unanswerable ones be *marked as such rather than approximated*,
because "an honestly absent output is worth more than a plausible one that the
calibration doesn't support". The audit is against the model's structure and needs
no calibration.

| output | verdict | why |
|---|---|---|
| Spread response to a shock in order arrival intensity | **not answerable** | The model has no prices. Its ladder is six slots addressed by 0/1 masks (`touch_bid`, `touch_ask`), so there is no mid and no spread. |
| Fraction of resting liquidity surviving a large marketable order | **not answerable** | Market orders consume at the touch only, clipped by `takes = min(resting, ...)`. A "large" order cannot sweep deeper levels, so the surviving fraction is fixed ladder geometry rather than a response. The sweep *is* the question. |
| Depth recovery time following a liquidity event | **answerable** | Depth is what the model is about. |
| Queue-position distribution under varying tick regimes | **not answerable** | Volume is continuous with no order identity and no FIFO queue, so there is no queue position; and with no prices there is no tick to vary. |

**Correction to an earlier estimate.** When first sketching this I said two of four
were answerable, counting the liquidity-survival output. That was wrong, and wrong
in the direction that flatters the model: I checked whether market orders and depth
existed, not whether a *large* order could do anything a small one could not. It
cannot — consumption is clipped at the touch — so the "fraction surviving" is a
constant of the ladder, not a counterfactual. Reading the binding was what caught
it.

### The one that works, and what it yields

Depth obeys `dq/dt = limit_rate − cancel_rate·q`, relaxing exponentially towards
`limit_rate/cancel_rate` with timescale `1/cancel_rate`.
[cfg/lob_depth_recovery.yaml](cfg/lob_depth_recovery.yaml) removes 90% of resting
depth at a chosen step and lets the book refill. Two claims come out, and the second
is the one worth having:

- Recovery is **faster when cancellation is faster** — 23.2, 11.4, 6.0 steps as
  `cancel_rate` goes 0.10 → 0.225. Not a paradox: the rate that empties a queue also
  sets how fast it relaxes back, so an aggressively-cancelling book recovers quickly
  *to a shallower level*.
- **Recovery time is set by the cancellation rate, not the arrival rate.** Tripling
  the arrival rate changes equilibrium depth by 2.71× while recovery time moves by
  1.8 steps. Level and speed are different levers.

### What these claims are worth, stated plainly

They are counterfactuals about **this model**, which Spike 2.2 established does not
reproduce real order flow in either market tested. Their `Data` field says so. The
second claim is weaker still than it looks: the invariance is exact *by
construction*, since the relaxation timescale contains no arrival term — so it
confirms the implementation matches its own mathematics rather than telling us
anything independent about markets.

That is the honest state of PLAN.md's "the outputs are the framing" argument at this
point: **the framing is thin, because three of the four outputs need structure the
minimal generator does not have, and the fourth is about a model known not to fit.**
Whichever way the domain-model question is resolved, the outputs that make the
stability case are downstream of it — a model with prices and order identity would
make three of these four answerable at once.

`TestUnanswerableOutputsAreNotFaked` guards the audit against drift: it checks the
mask-addressed ladder and the clipped consumption are still what they are, and fails
if a claim id ever mentions spread, queue position or tick regime. If the model
gains prices, that test fails and forces a re-audit rather than letting a newly
possible output go unnoticed.

---

## The domain model: prices and book-walking (order identity deferred)

**Decided 2026-07-29**, in response to Spike 2.2 sending the project back to the
domain model and Spike 4.2 finding three of four stability outputs unsupportable.
Both pointed at the same missing structure, which is what made this worth doing as
one step.

[cfg/lob_priced.yaml](cfg/lob_priced.yaml) adds a real price ladder (8 levels a side
at tick offsets from a reference mid), an **emergent** spread, marketable orders
that **walk the book**, and arrival intensity that decays with distance from the
mid. It is still pure config, and it does **not** replace
`cfg/lob_generator.yaml` — that model is the Phase 1 trust anchor whose recovery
claims everything downstream leans on.

### Feasibility was checked before designing, and split the request in two

The DSL's only non-elementwise construct is `each`, which binds a lane index but
gives a lane no access to earlier lanes of its own comprehension. There is no scan.

- **Book-walking survives that**, because a sweep can be reformulated as prefix-sum
  plus clamp. Verified before building: `q = [3 2 1 4 5 6]` with a sweep of 7 gives
  `taken = [3 2 1 1 0 0]`.
- **Order identity does not.** Assigning *k* simultaneous arrivals to the first *k*
  free slots is an allocation problem — each order must see which slots the previous
  ones took — and that cannot become a prefix sum, because what accumulates is
  *which slots are taken* rather than a running total.

So order identity is deferred, and the reason is recorded as
[STOCHADEX_GAPS.md](STOCHADEX_GAPS.md) entry 1 rather than as a scoping preference.
Closing it means either bespoke Go — breaking the pure-config property every model
here holds, which is an Invariant A decision — or a `scan` primitive in the engine.

### Spike 4.2 re-audited: three of four

| output | minimal generator | priced book |
|---|---|---|
| Spread response to an arrival shock | not answerable | **answerable** |
| Liquidity surviving a large marketable order | not answerable | **answerable** |
| Depth recovery after a liquidity event | answerable | answerable |
| Queue position across tick regimes | not answerable | **still not answerable** |

Three new claims, all emergent rather than parameterised — nothing in the config
sets a spread:

- Spread **tightens** with limit-order arrivals: 4.27 → 3.30 → 2.20 ticks.
- Spread **widens** with marketable flow: 2.42 → 3.30 → 5.12 ticks.
- Resting liquidity surviving a marketable order falls with its size: 72.4% →
  50.3% → 12.8%.

### What these claims are and are not

They are **consequences of the mechanism**, not independent evidence. Arrivals
refill the inner levels, so of course the spread narrows. Confirming it mostly says
the implementation does what the mechanism implies. What it establishes is that
**the outputs exist and respond**, which is exactly what Spike 4.2 gates on and what
the minimal generator could not offer at any parameter setting.

**This model has not been calibrated against anything.** Spike 2.2's failure was
measured against `cfg/lob_generator.yaml`. Whether a priced book with decaying
arrival intensity does better on the depth-coupling and churn diagnostics is
**untested**, and nothing here should be read as having answered it. That is the
obvious next measurement, and it is cheap: `pkg/diagnostics` already applies the
same three tests to any row shape.

Two behaviours worth knowing before running that:

- The survival response **saturates**. An order at or above total resting depth
  clears the side outright and leaves the book one-sided, so the claim uses sizes
  below that cliff to measure a graded response rather than a floor.
- At the extreme parameter settings a side empties on a substantial minority of
  steps — a third of them at the lowest arrival rate. The spread average excludes
  those steps rather than folding in the sentinel, because averaging a sentinel
  would silently turn "the book broke" into "the spread was wide".

---

## Did prices fix Phase 2? No — measured, not assumed

The priced model was built because Spike 2.2 sent the project back to the domain
model. The obvious question is whether prices, book-walking and decaying arrival
intensity move it towards the real diagnostics. Running the **same three
measurements** (`pkg/diagnostics`, shared verbatim) against both synthetic models:

| | corr(depth, cancel) | corr(arrivals, cancel) | dispersion |
|---|---|---|---|
| minimal synthetic | **+0.37** | +0.04 | 1.0 / 1.2 / 1.0 |
| priced synthetic | **+0.64** | −0.04 | 0.9 / 1.7 / 4.0 |
| crypto (real) | −0.12 | **+0.98** | 3785 / 4601 / 92 |


### The control the real-market result had been missing

Until now the diagnostics had only ever been run on real data, where they returned
near-zero depth coupling. That reading was ambiguous — a correlation near zero could
mean the coupling is absent from the market, or that the measurement cannot see one
that is there.

Both synthetic models have the coupling **by construction** (cancellations are drawn
as `poisson(cancel_rate × resting)`), and the same functions detect it at +0.37 and
+0.64. **So the diagnostic has power, and crypto's −0.12 is the absence of a coupling
rather than the absence of a detector.** That materially
strengthens the Spike 2.2 conclusion, and it should have been measured at the time
rather than after — a control is worth most before you rely on the result, not once
someone asks.

These three claims are also the only part of the Spike 2.2 evidence CI **can**
re-check, since no third-party data is involved.

### The answer: prices were necessary but are not the missing mechanism

The priced model's depth coupling is *stronger* (+0.64 against +0.37), taking it
**further** from the market rather than closer. Its churn correlation stays at −0.04
against crypto's +0.98. Dispersion stays near 1 by construction, against crypto's
thousands.

So the domain-model step did what it was built for — three of four stability outputs
are now answerable — and did **nothing** for the failure that stopped Phase 2. That
was the honest expectation rather than a surprise: prices were added for Spike 4.2's
outputs, and nothing in them couples arrivals to cancellations. Measuring it turns an
expectation into a result, and rules the hypothesis out rather than leaving it
assumed.

**Coupled arrival and cancellation — quote churn — is now the only candidate left
standing**, and it stands alone. The remaining alternative from the earlier list,
changing the likelihood family, addresses dispersion but nothing here suggests it
would touch the depth coupling, which is the failure that actually kills
identification.

### What this does not establish

Nothing here tests whether **adding** a churn mechanism would reproduce the real
diagnostics. It narrows the field to one candidate; it does not confirm it. The next
measurement is a model with coupled streams, run through the same three functions.

---

## The churn model — predicted first, then measured

Predictions were fixed in [PREREGISTRATION.md](PREREGISTRATION.md) and **committed
on their own** (`642ca42`) before `cfg/lob_churn.yaml` existed, so the ordering is
verifiable in the history rather than asserted. This was the hypothesis most at risk
of being confirmed by construction.

| | prediction | measured | |
|---|---|---|---|
| **A** | `corr(arrivals, cancels)` > +0.5 | +0.62 | pass |
| **B** | `corr(depth, cancels)` < +0.2 | **+0.60** | **fail** |
| **C** | dispersion > 1.5 | 27.4 / 12.3 / 4.1 | pass |

**A was recorded in advance as near-certain and uninformative.** Couple two streams
through one driver and they correlate. It is claimed only so it cannot later be
presented as a discovery.

### C is a genuine positive, and the first of its kind here

A Poisson mixed over a gamma intensity is over-dispersed, and the measured
variance/mean of 27.4 and 12.3 is the first departure from Poisson any model here
produces — both earlier models sit at ~1.0 by construction. This one does it as a
consequence of the driver rather than by being told to. **No real-market magnitude is
claimed: the finding is that the driver produces overdispersion, not that it matches
one.**

That is weak evidence for a mechanism (many processes are over-dispersed, and the
shape parameter was not fitted), but it is the first time any part of the real
residual signature has been reproduced at all.

### B failed, and the diagnosis is not what I expected

I expected a common activity factor to couple everything to everything, depth
included. Measured, it does not:

| | corr(depth, arrivals) | corr(depth, cancels) |
|---|---|---|
| priced | −0.015 | +0.638 |
| churn | **+0.019** | +0.596 |
| crypto (real) | −0.21 | −0.12 |

Arrivals are uncorrelated with depth, **matching real markets**. Cancellations stay
coupled. The asymmetry is the clue: arrivals have no depth-dependent term anywhere,
while the cancellation path still has two — residual attrition drawn against resting
volume, and the `min(...)` clip stopping volume going negative. Making churn dominate
the cancellation *flow* did not remove the depth dependence, because the
depth-dependent parts of the *path* were still there.

### A fault in my own pre-registration, recorded rather than fixed

The stated parameter criterion was "churn flow comparable to arrival flow". **That
does not pin the regime.** What decides whether the clip binds is churn relative to
**depth**, and at the rates chosen churn is a large fraction of the resting book each
step, so the clip binds often and mechanically ties cancellations to depth.

So B's failure is reported as **inconclusive about the mechanism**, not a refutation
of it. The pre-registered reading ("churn is not the mechanism") stands as written
and is pinned as a claim — but the honest gloss is that the test put the model
outside the regime the mechanism describes.

**Re-testing in a lighter regime needs a fresh pre-registration**, not a re-run of
this one. Adjusting rates now and reporting a pass is precisely what the document
exists to prevent, and doing it immediately after seeing which way the number went
would be the clearest possible case of it.

### Where the mechanism question now stands

Neither the priced model nor the churn model reproduces the real signature —
arrivals *and* cancellations both decoupled from depth while tightly coupled to each
other. The churn model gets two of the three parts (arrivals decoupled from depth,
overdispersion) and misses the third.

What the evidence now points at is churn that is **depth-neutral**: cancellations
replacing what was just posted, so the book barely moves while both flows swing
together. That is a *lagged* coupling rather than a common-factor one, and the DSL
has `lag()`. It is the obvious next test, and it needs its own predictions written
down first.

## Removing attrition: all four correlations match, and the book stops conserving

Predictions were committed in `d731d07` **before `cancel_rate` was touched**, and
deliberately about what the change would **cost** rather than what it would fix —
because deleting the depth-coupled term was always going to move the depth coupling.

| | prediction | measured | |
|---|---|---|---|
| D | `corr(depth, cancels)` < +0.2 | +0.009 | pass (near-forced, declared) |
| E | depth 2nd-half / 1st-half > 1.5 | **2.72** | pass |
| F | spread < 2.5 ticks, sd < 0.5 | **2.00 ± 0.00** | pass |

The arithmetic behind G predicted ≈ 2.7 against a measured 2.72.

| | with attrition | attrition removed |
|---|---|---|
| `corr(depth, cancels)` | +0.237 | **+0.009** |
| `corr(arrivals, cancels)` | +0.896 | **+0.950** |
| depth drift | 0.95 | **2.72** |
| spread | 2.05 ± 0.21 | **2.00 ± 0.00** |

### The finding

**All four correlation signatures moved the way a match would require, and the model
became less usable, not more.** Attrition is the only depth-stabilising force in it —
arrivals, churn and the marketable sweep are all depth-independent — so removing it
leaves depth a random walk with drift. The book grows without bound, and with the
inner levels permanently occupied the spread collapses to a constant, destroying the
spread-response output Spike 4.2 had unlocked.

So the depth/cancellation coupling is fixable **only** by deleting the mechanism that
conserves the book. That trades a correlation failure for a worse one, and the
pre-registered reading stands: **neither form works.**

### Why this is the strongest case in the project for the discipline

Without G and H, the honest-looking write-up would have been *"removing attrition
fixes the coupling and improves the co-movement to +0.95 — the churn mechanism
works"*, shipped alongside a model whose book triples over a run and whose spread has
zero variance. **Every correlation in that report would have been true.** The costs
were only visible because they were predicted in advance and measured on purpose.

### Where it points, with the next test's signature stated now

Depth stationarity needs a depth-dependent force. If that force is *cancellation* the
coupling comes back, so it has to act on **arrivals** — posting rate falling as the
queue deepens. That is economically sensible (less incentive to join a long queue)
and is the standard shape in the Santa Fe literature this model descends from.

Its signature: `corr(depth, arrivals)` should turn **negative**. Note that this is a
prediction about the model, not a test against data: damping arrivals by resting depth is
what produces the sign, so it is forced by construction.

---

## Depth-dependent arrivals: the mechanism that holds

Predictions committed in `3b1f756` before `cfg/lob_arrivals.yaml` existed. Cancellation
is pure churn with **no depth term at all**; the stabiliser moves to the arrival side,
posting into a level slowing as that level fills.

| | prediction | measured |
|---|---|---|
| G | depth drift < 1.3 | **1.008** |
| H | `corr(depth, cancels)` < +0.2 | **−0.002** |
| I | spread sd > 0.1 | **0.41** |

All three pass. Against the attrition model it replaces:

| | attrition removed | depth-damped arrivals |
|---|---|---|
| depth drift | 2.72 | **1.008** |
| `corr(depth, cancels)` | +0.009 | **−0.002** |
| `corr(arrivals, cancels)` | +0.950 | +0.897 |
| spread | 2.00 ± 0.00 | **2.17 ± 0.41** |

### H is the result; the rest are scaffolding

Only H was genuinely uncertain. G was near-forced — the one free parameter was chosen
precisely to make depth stationary — and I is a cost check in the shape of F.

H was not forced. Cancellations contain no depth term, but depth is now
anti-correlated with arrivals and arrivals share the activity driver with
cancellations, so that indirect path could have reintroduced a coupling of either
sign. It did not. **Depth can be stabilised without the coupling returning**, and the
mechanism that does it is the economically obvious one: less incentive to join a long
queue.

> **Withdrawn 2026-07-31: H was nearer to forced than the two paragraphs above claim.**
> The argument for its uncertainty ran through the shared activity driver — and that
> path cannot carry a contemporaneous correlation. The driver is drawn **iid per step**;
> the depth this correlates against is depth at the **start** of the step, so it depends
> on activity only up to `t−1`, while cancellation depends on activity at `t`. Those are
> independent by construction, so `corr(depth, cancels)` sits near zero **whatever
> mechanism is present**, and H could not have failed for the reason it was thought to
> be risky.
>
> What H still establishes is real and worth keeping: no depth term leaked into
> cancellation, which is a checkable property of the config. What it does not establish
> is that a coupling was available and avoided. "H is the result" overstates it; the
> block's honest summary is that all three predictions were closer to scaffolding than
> one of them was recorded as being.
>
> **This generalises past H**, which is why it is worth the space. Every model in this
> project has used an iid driver, so every ≈0 reading on the cancellation side has been
> partly structural rather than mechanistic. It does not touch readings that came out
> LARGE — the recycled model's +0.458 is not something a blind measurement produces — so
> the eliminations that turned on large positives stand unaffected. It is the near-zero
> passes that were weaker evidence than they looked, and that is what makes a persistent
> driver the next thing to try rather than one option among several.

Spike 2.2's original failure is therefore explained. The minimal and priced models
coupled cancellations to depth because that was their only way to conserve a book;
real books conserve on the *arrival* side instead, which is why their cancellation
flow shows no depth dependence.

### The mismatch that remains — restated 2026-07-31, with its numbers

This section previously asserted that `corr(depth, arrivals)` was the axis the model
missed and that the reason was established, **without stating either the numbers or the
reason**. An assertion with no measurement behind it is the exact failure mode
`CLAIMS.md` exists to prevent, and it should not have survived in this document. It is
restated below from measurements re-run today, and the model side is now pinned as a
claim — `depth_stabilisation_moves_the_brake_onto_the_arrival_side` — rather than living
only in prose.

**The model, and every earlier variant:**

| | `corr(depth, arrivals)` | `corr(depth, cancels)` |
|---|---|---|
| attrition model | — | +0.638 |
| priced | −0.015 | +0.638 |
| churn | +0.019 | +0.596 |
| **depth-damped arrivals** | **−0.116** | **−0.002** |

**Binance spot, all five concurrently-recorded segments plus the original capture:**

| segment | `corr(depth, arrivals)` | `corr(depth, cancels)` |
|---|---|---|
| BTCUSDT (original 8-min capture) | −0.212 | −0.123 |
| BTCUSDT | −0.267 | −0.220 |
| ETHUSDT | −0.339 | −0.246 |
| SOLUSDT | −0.121 | −0.074 |
| XRPUSDT | −0.131 | −0.015 |
| DOGEUSDT | −0.206 | −0.078 |

#### The mismatch is real but it is NOT large, and the earlier framing was wrong

The model gets the **sign** right and lands at the weak edge of the observed range:
−0.116 against a real span of −0.121 to −0.339. It is about half the original capture's
−0.212 and about a third of ETHUSDT's. So "the one axis the model misses" overstates it —
on this axis alone the model is closer to the market than on any other diagnostic in
Spike 2.2, where the failures were sign reversals and three orders of magnitude.

#### What the numbers actually show is a different mismatch, and it is structural

Read the two columns together rather than one at a time. **On every real segment BOTH
flows are mildly anti-correlated with depth**, and by comparable amounts — arrivals
somewhat the stronger of the two. The model puts −0.116 on one flow and −0.002 on the
other. Every earlier variant is worse in the same way, concentrating everything on one
flow and often with the wrong sign.

That is the trade-off named above, seen from the data side: in this model's vocabulary a
brake couples depth to exactly one flow, so no setting of it can produce two comparable
mild anti-correlations. Real books do not appear to concentrate their brake.

#### What this does not establish, and one confound that could explain all of it

The paired-negative reading is a **pattern in six segments, not a mechanism**. Nothing
here identifies what produces it, and no model in this project has been fitted to it.

The confound is specific: arrivals and cancellations are both **inferred from net depth
changes** rather than observed as messages, so both columns run through one inference
path and an artefact there would show up in both at once — including on segments that
share no other property. Replicating across five instruments does not touch it, because
they were all measured the same way.

**Corrected 2026-07-31, hours after first writing it.** The paragraph above originally
ran further, claiming that artefact "could produce mild negatives in both columns on
every segment at once", and it was written without re-reading `pkg/feed/bucket.go`. That
overstates it, and in a direction that made the target look weaker than the evidence
supports.

The strong form — the inference *manufactures* the signature by erasing churn — is
explicitly designed against. **Decision 4** accumulates deltas per depth update rather
than netting a bucket's open against its close, giving that exact reason: netting "erases
churn: volume added and removed inside the same bucket cancels out, understating both
arrivals and cancellations, and understating them MORE the busier the market is." The
feed is `@depth@100ms` against 1-second buckets, so roughly ten updates back each row and
the annihilation window is **100ms, not one second**. **Decision 3** splits cancellations
from fills against the trade stream — a second source rather than an assumption.

What genuinely remains: netting inside a single update at a single price, which
understates both counts together and does so more in busy periods; both counts being sums
over the same update stream, so update frequency reaches both; and no order identity at
all, which is a hard ceiling rather than a confound. The lot size is a shared divisor but
cannot affect a correlation, being scale-invariant — that is a dispersion confound and is
already declared as L's.

So the paired-negative signature is still a candidate target rather than a settled fact
about order flow, and message-level data is still the only way to settle it. But it is a
**better-supported** candidate than the original wording implied, which means the
mechanisms eliminated against it were eliminated on firmer ground, not softer.

### What this does not establish

**Nothing here is a calibration.** One parameter was fitted, to mean depth, with the
correlations invisible while choosing it. Reproducing a correlation structure is much
weaker than reproducing the dynamics that generate it; this model has never been fitted
to real data or tested out of sample; the dispersion agreement is order-of-magnitude
rather than close; and no spread distribution has ever been compared against a real
one.

What has been established is narrow and worth stating exactly: **there exists a
depth-stabilising mechanism, expressible as pure config, that conserves the book without
coupling cancellation to depth.** No claim is made that it resembles a real book — nothing
in this section is calibrated against market data, and `arrival_scale` is a chosen value
rather than a fitted one.

A near-miss worth recording: `arrival_scale` was very nearly set from a mean depth carried
over from a run at a different `churn_rate` than the one in use, which would have put the
model in a three-times-thinner regime. Checking the number rather than reusing a
remembered one caught it.

## Depth-neutral churn: the last candidate fails, and it fails by identity

Predictions T–W were committed in `480992d` before `cfg/lob_churn_recycled.yaml`
existed. Three of four failed.

| | prediction | measured | |
|---|---|---|---|
| T | `corr(depth, cancels)` in [−0.30, −0.02] | **+0.458** | **FAIL — opposite sign** |
| U | arrival brake < −0.05 and the stronger | −0.110 vs +0.458 | **FAIL** on ordering |
| V | `corr(arrivals, cancels)` > +0.7 | **+0.436** | **FAIL** |
| W | drift < 1.3, spread sd > 0.1 | 1.066, 0.579 | pass |

### The finding

Cancellation was made proportional to `arr(t−1)`, which carries no depth term anywhere.
It nevertheless produced the **strongest depth coupling any model here has had** — +0.458
against the minimal model's +0.37, the very failure this whole line of work started from.

The reason is an accounting identity rather than a rate. `arr(t−1)` is what is **resting**
at `t`: a book accumulates its own recent arrivals. Keying cancellation to recent arrivals
therefore hands cancellation and depth a shared term, and the positive correlation is
forced by construction rather than by any modelling choice.

**Depth-neutral in the rate is not depth-neutral in the correlation.**

That eliminates a family, not an instance. No value of `recycle` rescues it — the identity
does not depend on the coefficient or the lag — so sweeping the parameter would be work
with a known answer. Quote churn was the last candidate standing after prices and
attrition were ruled out; the *lagged* form of it is now ruled out too, and by a mechanism
general enough to rule out the un-lagged form of the same keying.

### What is left, stated narrowly

What survives untouched is cancellation rules keyed to something **other than arrivals**.
The identity bites only because arrivals are a component of depth. A rule keyed to elapsed
time-in-queue, to the touch's own movement, or to a latent driver that is not itself
accumulated into the book would not inherit it. None of those has been tried, none is
implied by the evidence, and each would need its own pre-registration.

### The half-prediction that was right, and the half that was not asked

PREREGISTRATION.md argued a pure lag would collapse the contemporaneous co-movement and
chose a half-weight mixture to avoid it. Right about the mechanism, wrong about the size:
+0.897 fell to +0.436 at half weight, far below the +0.7 floor. The cost of lagging scales
faster than its weight.

More usefully, that reasoning was incomplete rather than merely imprecise. It examined
what lagging **costs** and never asked what keying cancellation to arrivals **buys** —
which is exactly what T answered, badly. Predicting one side of a trade-off is not
predicting the trade, and this is the clearest instance of that in the project so far.

### A lag-length error, found 2026-07-31 and corrected before it changed a conclusion

The pre-registered mechanism said `recycle · arr_i(t−1)`. The first implementation wrote
`recycle * lag(posted_bid, 1)`, which is **arr(t−2)** — because a bare field name is
already row 0, the previous committed step, so `lag(x, 1)` reaches one row further back
than that. **The model first scored was not the model pre-registered.**

Verified rather than reasoned about, with a counter promoted into a state row:

| recorded `counter` | bare `counter` | `lag(counter, 1)` |
|---|---|---|
| 5 | 4 | 3 |

Corrected to the bare form and re-scored:

| | 2-step (first published) | 1-step (as pre-registered) | verdict |
|---|---|---|---|
| T `corr(depth, cancels)` | +0.458 | **+0.560** | fail, unchanged |
| U margin | −0.348 | **−0.443** | fail, unchanged |
| V `corr(arrivals, cancels)` | +0.436 | **+0.432** | fail, unchanged |
| W drift / spread sd | 1.066 / 0.579 | **1.020 / 0.615** | pass, unchanged |

**All four verdicts unchanged, and T's failure is stronger.** That is what the identity
argument predicts — a shorter lag shares *more* with what is resting now — so the
accident left behind an unplanned second data point supporting the explanation rather
than undermining it. Both numbers are kept here for that reason.

The published numbers were the two-step model's, so CLAIMS.md moved when this was fixed.
A guard now asserts the bare spelling and rejects `lag(posted_…)` outright, so the error
cannot recur silently.

**Recorded here rather than in STOCHADEX_GAPS.md**, per that file's own rule: this was a
misreading of documented behaviour — the engine states that a bare name gives row 0 —
not an engine gap.

### Provenance of the one adjusted parameter

`churn_rate` moved from the inherited 1.15 to **0.55**, once, on mean depth alone: at 1.15
depth fell to 73.5 against the previous mechanism's 227.8. The sweep computed no
correlation while choosing — 0.4 → 397.8, 0.5 → 273.3, 0.6 → 195.4, 0.7 → 146.1,
0.8 → 110.6, 0.9 → 95.0, then 0.55 → 238.6 — and `recycle` stayed at its pre-registered
0.5. Mean depth is therefore provenance in the W claim, not a result.

### What this does not establish

The target band was taken from Binance segments whose arrival and cancellation counts are
both inferred from net depth changes, and that confound was declared before running. It
does not rescue the mechanism — +0.458 is nowhere near the band on any reading — but it
does mean the search is aimed at a target that has not itself been verified. Settling that
needs message-level data, which this project does not have and cannot obtain from the
depth-snapshot feed it is allowed to use.

That confound is **narrower than it was first written**, and the correction is above under
"Corrected 2026-07-31": the churn-erasing version of it is designed against by
`pkg/feed/bucket.go` Decision 4, and the residual annihilation window is 100ms rather than
a full bucket. The target is therefore on firmer ground than the sentence above suggests.
Note also which finding does not depend on it at all: **depth-neutral in the rate is not
depth-neutral in the correlation** is an identity, forced by construction, and would hold
if every market number in this project were withdrawn tomorrow.


## A persistent driver: the closest miss, and the first correct ordering

Predictions X–AB were committed in `5c1081e` before `cfg/lob_persistent.yaml` existed.
X, Z and AB pass; **Y and AA fail, on magnitude rather than direction.**

**Validity precondition met:** the `min()` clip bound on **4.21%** of level-steps against
the pre-registered 5% ceiling, so Y and Z carry verdicts. Not comfortably — a thinner book
would put this test's validity in question, and that margin is part of the result. This is
the fault the churn block recorded and did not measure; here it was measured.

| | prediction | measured | |
|---|---|---|---|
| X | `corr(depth, activity)` < −0.05 | **−0.307** | pass |
| Y | cancels ∈ [−0.30,−0.01] **and** arrivals ∈ [−0.40,−0.05] | −0.286 ✓ / **−0.417 ✗** | **FAIL** by 0.017 |
| Z | arrivals the stronger brake | 0.417 vs 0.286 | pass |
| AA | `corr(arrivals, cancels)` > +0.85 | **+0.822** | **FAIL** |
| AB | drift < 1.3, spread sd > 0.1 | 1.164, 0.530 | pass |

### The finding, and it is a positive one for once

| | this model | previous best | Binance range |
|---|---|---|---|
| `corr(depth, cancels)` | **−0.286** | −0.002 | −0.015 … −0.246 |
| `corr(depth, arrivals)` | **−0.417** | −0.116 | −0.121 … −0.339 |
| `corr(arrivals, cancels)` | +0.822 | +0.897 | +0.940 … +0.980 |

**Both flows are negative against depth with arrivals the stronger — the real ordering,
produced for the first time.** Every earlier model put the whole brake on one flow with
the other at zero or the wrong sign; the recycled model inverted the ordering outright.
The structural obstacle is gone: X establishes that depth now responds to the driver at
all, which no iid-driver model could have shown whatever its mechanism.

### Why the failures are one parameter rather than one mechanism

The damping is at full strength — `s_eff = arrival_scale · act_ref/act`, so `q* ∝ 1/act`.
That overshoots both depth correlations. It also costs co-movement, and by an identifiable
route: arrivals now **saturate** in activity, being proportional to
`act/(1 + q·act/(s·act_ref))` whose denominator grows with `act`, while cancellation stays
proportional to it. Two flows that were both proportional to the driver now track it
differently, so they track each other less closely.

A weaker dependence — `(act_ref/act)^γ` with γ < 1 — would reduce both effects together.
**That needs its own pre-registration.** Sweeping γ having seen which way X, Y and AA
missed is the move PREREGISTRATION.md exists to prevent, and the saturation account is an
explanation consistent with the numbers rather than an independently tested claim.

### What this does not establish

Nothing here is a calibration. `persistence`, the damping form and the innovation moments
were all fixed in advance, and `churn_rate` moved once on mean depth alone (1.15 → 1.05,
because the damping thinned the book to 188.9 against the previous models' 227.8–235.9).
The target bands carry the standing confound — both real flows are inferred from net depth
changes — so landing inside them would establish that a pure-config mechanism reproduces
the measured signature, not that the signature is a property of order flow.


## The first calibration: one parameter fitted, two predictions held out — and they land

Predictions AC–AG were committed before `cfg/lob_damping.yaml` existed. **AE, AF and AG
pass; AC and AD fail.**

**This block breaks a property every earlier one had.** Every other claim here says some
version of *nothing is fitted to market data*. This one fits the damping exponent to a
Binance number. That is Phase 2 beginning, not a slip — but it changes what the claims can
say, and it is why the whole block was built around holding two targets out.

| | fitted? | model | Binance |
|---|---|---|---|
| `corr(depth, arrivals)` | **FITTED** | −0.234 | −0.213 (five-segment mean) |
| `corr(depth, cancels)` | held out | **−0.138** | −0.127 (five-segment mean) |
| `corr(arrivals, cancels)` | held out | **+0.876** | +0.940 … +0.980 |

γ was selected mechanically — grid, target and rule all fixed in advance — landing on 0.6
at a distance of 0.021 from the target, inside the 0.05 at which the fit would have been
declared failed. The cancellation side was free to land anywhere in [−1, +1] and came
within **0.011** of the market mean with the ordering intact.

### Why this is different from every previous result here

Four mechanisms were eliminated by *failing* to reproduce a signature. This is the first
one that reproduces something it was not shown. One parameter cannot chase three numbers,
so the second and third are tests rather than fits — which is the only reason a
calibration can be evidence at all.

### AC failed, and the fault is mine rather than the model's

`corr(depth, arrivals)` **crosses zero inside the grid**: +0.277 at γ=0, −0.394 at γ=1. Its
absolute value therefore dips and rises, and AC — written with the absolute value — is
false. The *signed* response is strictly monotone across all seven points, which is what AC
existed to establish and what makes fitting well-posed, but restating it that way is a
post-hoc reformulation and is recorded as measured structure, not as a prediction that
passed.

The crossing **confirms a prediction made earlier**: the persistent-driver block declared
before running that constant marketable consumption competes with the damping and pushes
this correlation positive. At γ=0 that effect is unopposed and it wins. The competing
effect was real; at γ=1 it was simply outweighed.

### AD failed narrowly, and the noise floor is now visible

Co-movement falls +0.885 → +0.823 as predicted but inverts once, by **0.003**, between
γ=0.5 and γ=0.6. That is within single-seed run-to-run variation — which is itself worth
recording, because it puts a rough floor of a few thousandths on how finely any correlation
in this project can be read.

### Where the control missed its own band, declared

`churn_rate` was set per grid point on mean depth into a stated 227.8–235.9. Two points
missed: the selected γ=0.6 at **223.0** and γ=0.8 at 221.7. Depth spans 221.7–234.5 across
the sweep — roughly but not exactly fixed, and 2.1% low at the point that matters.

### What it does not establish, in four parts

Not that the mechanism is right. Not free of the standing inference confound, so what is
reproduced is the *measured* signature. Five segments, one 8-minute window, one venue, one
seed, and **no out-of-sample test of any kind**. And the co-movement at +0.876 against a
market +0.94–+0.98 is narrowed, not matched — the one gap that has survived every model
built here.

**The next step was fixed before this result existed and still stands: a fresh recording
and a prediction against it, not a stronger claim about this one.**


## Out of sample, the calibration fails — and the market, not the model, is what moved

Predictions AH–AK were committed in `f166872` before the window was recorded. **AH, AI and
AK fail; AJ passes.** The model was frozen throughout and a test asserts it still is.

Capture: Saturday 2026-08-01, 08:51–08:59 UTC, five symbols concurrently, 480 clean rows
each, 0 suspect, 0 gaps. The pre-registered weak-test gate **did not fire** — the market
moved 0.145 against a 0.03 threshold — so the verdicts are real rather than vacuous.

| | old (Thu 07-30) | fresh (Sat 08-01) | market moved |
|---|---|---|---|
| mean `corr(depth, arrivals)` | −0.213 | **−0.068** | **+0.145** |
| mean `corr(depth, cancels)` | −0.127 | **+0.035** | **+0.162** |
| mean `corr(arrivals, cancels)` | +0.955 | +0.904 | −0.051 |

BTCUSDT's arrival correlation **changed sign**, −0.267 → +0.058. ETHUSDT moved +0.359.

### The finding

**The signature this project has spent four mechanisms chasing is not stable over two
days.** γ was fitted to a property of one Thursday morning, and the calibration does not
transfer: the frozen model sits 0.166 and 0.173 from the fresh means against a 0.12
tolerance that was itself set generously, from the market's own observed drift.

The in-sample result is not retracted as a measurement — one parameter fitted to one
number did put a held-out number within 0.011 of the market, and that happened. What is
retracted is the *reading* that this was evidence of prediction. `pkg/damping`'s AE claim
now carries **WITHDRAWN AS PREDICTIVE** and its package doc says so before anything else.

### What is missing is sharper than any of the four eliminations gave

The model cannot track this and **nothing in it could**: no parameter varies by window. A
model that tracked it would need a regime changing over hours, and none of the five
mechanisms tested here — prices, attrition removal, common-factor churn, recycled churn,
persistent driver — has one. The persistent driver has a latent process with a ~5-step
correlation time; this is a shift across hours, three orders of magnitude slower.

That is a concrete, positively-stated gap, and it only became visible because a second
window was recorded. It is worth more than the calibration it destroyed.

### What survives, and it is the result that has never weakened

Fresh `corr(depth, cancels)` per symbol: +0.057, +0.040, +0.050, −0.009, +0.037 — **every
one below +0.2.** The coupling the model's parameterisation requires is absent on every
symbol in a second independent window, so **Phase 2's central conclusion is now replicated
across two windows as well as five instruments.**

### One thing that would not replicate, recorded as context

The cross-segment block's +0.9 co-movement floor **would fail on this window**: XRPUSDT
reads +0.852 and DOGEUSDT +0.867. That is not a rescoring of a claim pre-registered
against the segments it was measured on — it is evidence that the co-movement floor is
also window-dependent, and that any future bound on it needs a temporal qualifier.

### Why AJ passing says little

The gap closed to 0.028 because the market's co-movement **came down** toward the model,
not because the model rose. A loose bound on the one quantity that moved least is the
weakest of the four results here.

### What this does not establish

Two windows on one venue give no distribution for the instability — many more recordings
would. Nothing here identifies its cause; Saturday flow being thinner and less
intermediated than Thursday's is the obvious candidate and is untested. And the standing
inference confound is untouched: both flows are still inferred from net depth changes, so
what moved may be the measurement's behaviour rather than the market's.


## The noise floor, measured at last — and it dissolves the regime story

Predictions AL–AO were committed before the windows existed. **AL, AM and AO pass; AN
fails.** Five windows inside one Sunday morning at ten-minute starts, plus the Saturday
window ~23 hours earlier; 480 rows each, 0 suspect, 0 gaps. (`nf3_DOGEUSDT` recorded 476
rows — a four-second short capture with no gaps or suspect rows, so the pre-registered
exclusion criterion does not apply and window 3 is kept.)

### The number this project never had

**Within one morning, nothing changed but the clock:**

| quantity | within-morning range |
|---|---|
| five-symbol mean `corr(depth, arrivals)` | **0.079** |
| five-symbol mean `corr(depth, cancels)` | **0.112** |
| five-symbol mean `corr(arrivals, cancels)` | **0.032** |

Every correlation in `CLAIMS.md` and `DECISIONS.md` is quoted to three decimals with
nothing beside it. This is what belongs beside them.

### The verdicts pass and the conclusion is still negative

AM passes **by 0.008**: its six-window range of 0.155 is 95% of the whole
Thursday→Saturday gap, reproduced among weekend windows where no day changed. AN fails 3
of 6 — and one of the three passing is the Saturday reference itself, so of five genuinely
new windows **two land nearer Thursday than nearer the other weekend window**.

The windows do not group by day. **The out-of-sample failure needs no regime explanation,
because the gap it turned on is barely larger than what these quantities do between
windows minutes apart. No weekday study is warranted** — it would be chasing a variable
that has not been shown to matter.

### Two claims qualified rather than left standing

- **The calibration headline.** `pkg/damping` reported a held-out number landing within
  **0.011** of the market. That quantity wanders **0.112** between windows ten minutes
  apart. The agreement was inside the noise by an order of magnitude — a second,
  independent reason the headline was wrong, alongside the out-of-sample failure.
- **Prediction M.** Scored on a margin of **0.032** against a co-movement noise floor of
  **0.032**. Not distinguishable from chance in either direction. Already recorded as a
  pass whose reasoning is unsupported; now measurably so.

### What survives its own noise floor

**AO: thirty measurements — five symbols × six windows — worst case +0.083 against +0.2.**
The coupling the model's parameterisation requires is absent everywhere it has ever been
looked for: seven windows counting Thursday, five instruments, two days. Its margin to the
bound is 0.117, *larger* than the wander of 0.112.

That distinction is the useful one to carry forward. **This project's one-sided bounds
survive the noise floor; its magnitude comparisons mostly do not.** Bounds were the right
form for a claim here and the reason is now quantitative rather than stylistic.

### What this does not establish

Six windows give a range, not a distribution. The within-morning figure understates
variability, since adjacent windows share market conditions. All six are weekend mornings
on one venue inside 24 hours, and eight minutes is a window length this project chose —
a longer one would average more and wander less, which is itself worth testing before any
future claim rests on a magnitude.


## The model's own seed noise, and the claims it takes down

Measured 2026-08-02 while establishing a yardstick for the partition refactor. Eight
seeds per config, nothing else changed:

| config | `corr(d,arr)` range | `corr(d,can)` range | `corr(arr,can)` range | drift range |
|---|---|---|---|---|
| `lob_churn` | 0.054 | 0.051 | 0.048 | 0.092 |
| `lob_arrivals` | 0.065 | 0.093 | 0.014 | 0.053 |
| `lob_churn_recycled` | 0.064 | 0.119 | 0.058 | 0.113 |
| `lob_persistent` | 0.116 | 0.126 | 0.028 | 0.175 |
| `lob_damping` | **0.132** | **0.137** | 0.038 | 0.138 |

**The model wanders as much between seeds as the market does between windows** (0.079 /
0.112 / 0.032, measured the same day). Every model correlation this project has published
is a single-seed number quoted to three decimals.

### The most fundamental casualty: the calibration's grid selection

The γ sweep chose 0.6 because its `corr(depth, arrivals)` sat **0.021** from the fit
target. That quantity's eight-seed range is **0.132**. The neighbouring grid points were
0.061 and 0.098 away — all inside one seed's noise.

**The selection was noise-dominated: a different seed would very likely have chosen a
different γ.** The out-of-sample failure and the market noise floor were reasons the
*result* did not generalise. This is a reason the *procedure* could not have worked, and
it is the third and most basic of the three.

### Verdicts that do not survive their own seed

| claim | bound | 8-seed range | status |
|---|---|---|---|
| `depth_stabilisation_moves_the_brake_onto_the_arrival_side` | `d/arr` < −0.05 | −0.115 … **−0.051** | margin **0.001** — one seed from breaking |
| **Y** (persistent) | `d/arr` < −0.40, pinning a FAILURE | −0.417 … **−0.301** | **most seeds do not overshoot; Y would have PASSED on them** |
| **AF** (damping) | `arr/can` > +0.85, a PASS | **+0.843** … +0.881 | **fails on several seeds** |
| **AA** (persistent) | `arr/can` < +0.85, pinning a failure | +0.807 … +0.835 | margin 0.015, but fails on all eight — direction robust |
| **AC/AD** (damping) | monotone across γ | per-step ≈0.11 vs noise 0.132 | **adjacent-pair ordering not resolvable** |

Every one has been qualified in place rather than withdrawn or rescored. Two deserve
naming: **AF is a PASS that the model does not reliably achieve**, and **Y is a FAILURE
the model does not reliably produce** — the seed decided both.

### What survives comfortably

The large-margin results are untouched, and it is worth being explicit about which:
H's −0.002 against the attrition model's +0.638; T's +0.458 against a band ending at
−0.01; B's +0.60 against +0.2; V's +0.43 against +0.7. **One-sided bounds with margins of
0.3 and up are not in question at a seed range of 0.13.**

That is the same lesson the market noise floor gave, from the other side: **bounds with
wide margins survive; comparisons decided by a tenth do not.** Two independent noise
measurements now say the same thing about how this project should state results.

### The bound that was set wrongly, and is left wrong

`brakeBound` at −0.05 was chosen descriptively after measuring one seed. With the spread
in view it should have been set further out. **It is left as recorded rather than
widened** — widening a bound to accommodate its own noise is exactly the edit this
project refuses, and the claim now carries the fragility in its limitations instead.

### What this does not establish

Eight seeds give a range, not a standard error. The ranges are for one window length
(2000 steps) and one settle-in (100 rows); a longer run would average more and wander
less, which is untested and is the obvious lever if these claims are to be made robust
rather than annotated.


## Claims move to ensembles — and two of my own measurements were wrong

The seed audit established that single-seed claim values are noise-dominated at the scale
this project's comparisons are decided. `pkg/cfgrun` now has `RunEnsemble`, and
`pkg/damping` is migrated: **32 members at 8000 steps**, every reported number an ensemble
mean.

### Two corrections to the sizing work itself

Both were mine, both from the same mistake — reading a **range** over few samples as if it
were a spread:

1. **"Noise stops falling above 8000 steps" was wrong.** Measured properly by standard
   deviation over 64 members, it falls as 1/√n with no saturation: 0.0526 → 0.0244 for a
   4× length increase (×0.46, against ×0.50 predicted). The apparent saturation was range
   noise.
2. **"Ensembling costs 32× the compute" was wrong by two orders of magnitude.** 64 members
   at 8000 steps take **2.6 seconds**. The engine runs members concurrently. The cost
   argument against ensembling did not survive being measured.

A range over 8 samples is so noisy that two independent estimates of the same quantity
came out **0.056 and 0.115**. Report standard deviations.

### Why the engine's ensembler rather than a loop

`simulator.RunSeededEnsemble` varies the **global** seed through the `ConfigGenerator`,
reseeding every partition coherently. Substituting a partition's `seed:` line — which is
what a loop would do — varies one partition and leaves others pinned, so members would
share randomness they should not. The engine also rebuilds a fresh generator per member,
which is load-bearing: `GenerateConfigs` hands back the same `Iteration` pointers it was
given, so reusing one generator across concurrent members would share mutable state.

### The sizing, from measurement

At 8000 steps a depth correlation's across-member SD is ~0.024, so an N-member mean has
standard error 0.024/√N: **32 members → ~0.004**, five times finer than the 0.021 the
damping calibration tried and failed to resolve.

### Two verdicts changed, and both directions are recorded

| | one seed, 2000 steps | 32 members, 8000 steps |
|---|---|---|
| **AD** | **FAIL** — one 0.003 inversion | **PASS** — strictly monotone, every step ≫ SE |
| **AF** | pass at +0.876, but failing on several seeds | **PASS** at +0.863, ~18 SE clear of its floor |
| AC | fails as written; ordering unresolvable | fails as written; ordering now resolved |

**The pre-registered bounds were not touched.** What changed is the precision of the
measurement, decided and justified before any claim was re-scored and for reasons
independent of any claim's outcome. AD's flip was anticipated in its own limitations,
which already called the inversion noise — so the ensemble confirms what was written
rather than rescuing a surprise. AF moved the other way: a pass that was luck is now a
pass that is real.

Recording both directions matters. **A re-measurement that only ever improves results is
not a re-measurement.**

### The grid selection is now resolvable

γ=0.6 sits 0.012 from the fit target against γ=0.5's 0.049 — separated by 0.037 at a
standard error of 0.005. On one seed those points were indistinguishable. This does **not**
rescue the calibration, which failed out of sample and still does; it means the grid choice
is a measurement rather than a coin toss.

### A stronger control, as a side effect

`TestGammaOneIsExactlyThePersistentModel` used to compare γ=1 against recorded constants,
which went stale the moment the measurement changed. It now runs both configs at the same
seed and length and requires **bit-identical output**. That is a far stronger check of the
`pow()` reparameterisation, and it cannot go stale.

### Not yet migrated

`pkg/churn`, `pkg/arrivals`, `pkg/recycled` and `pkg/persistent` still report single-seed
values, and three carry seed-fragility qualifications from the audit — including **Y**,
a recorded FAILURE that most seeds do not reproduce. The migration is mechanical from
here; until it is done those claims stand as annotated rather than resolved.


## Three more packages on ensembles — and Y becomes a pass

`pkg/persistent`, `pkg/arrivals` and `pkg/recycled` now report 32-member ensemble means at
8000 steps, joining `pkg/damping`. Bounds untouched throughout.

### Y flips from FAIL to PASS, and that needs to be auditable

| | one seed, 2000 steps | 32 members, 8000 steps |
|---|---|---|
| Y `corr(d,arr)` | **−0.417**, outside [−0.40, −0.05] → **FAIL** | **−0.387**, inside → **PASS** |
| Y `corr(d,can)` | −0.286, inside | −0.257, inside |
| AA `corr(arr,can)` | +0.822, margin 0.015 → fail, in doubt | +0.816, ~6 SE below floor → **fail, robust** |
| AB drift | 1.164 — highest of any model here | **0.996** — the single seed was an unlucky draw |

**The persistent-driver block's story changes.** It was recorded as "Y and AA fail, on
magnitude rather than direction". On ensemble means **only AA fails**: both depth
correlations land in their bands, in the right order. This model reproduces the paired
depth signature and misses only the co-movement.

**Why this is not a rescue.** The seed audit measured this quantity's spread and recorded,
*before any re-scoring*, that most seeds do not overshoot and Y would have passed on them.
The ensemble confirms a prediction already written down. The bands did not move, the
measurement changed for reasons independent of any claim, and AA moved the other way —
from a failure in doubt to a failure that is robust. AB moved too, revealing its
single-seed drift of 1.164 as an unlucky draw rather than a sign the moving equilibrium
strains conservation.

Y's margin is 0.013 inside the band against a standard error of 0.004 — three standard
errors. A pass, not a comfortable one, and its claim says so.

### The brake claim's fragility is resolved

The audit found `depth_stabilisation_moves_the_brake_onto_the_arrival_side` with its −0.05
bound sitting **0.001** from the single-seed spread. On ensemble means the arrival side is
−0.077 with a standard error of ~0.002 — about twelve standard errors clear. **The bound
was not widened**; the measurement got finer, which is the only legitimate way to rescue a
claim that close to its edge.

`pkg/arrivals`'s H also moved sign, −0.002 to +0.052, while staying far inside its +0.2
ceiling. A sign change that does not touch a verdict is worth noticing: it is a reminder
that the near-zero readings this project used to quote to three decimals were never
distinguishable from zero.

### `pkg/recycled`: nothing moved

T +0.560 → +0.514, V +0.432 → +0.418, **not one verdict changed**. This model's results
were never close enough to their bounds for the seed to decide them — T fails its band by
sign and V misses its floor by 0.28. Wide margins are unaffected by a spread of 0.02,
which is the same lesson the market noise floor gave.

### Still on one seed

`pkg/churn`, `pkg/lob` and `pkg/baseline` share `baseline.Measure`, so migrating them is a
change to a helper three packages depend on rather than the per-package edit the others
needed. Left for the follow-up, and flagged here so the corpus is not assumed uniform:
**four of seven model packages are ensembled, three are not.**


## The ensemble migration completes — all seven model packages

`baseline.Measure` now runs a 32-member ensemble at 8000 steps, which carries `pkg/churn`,
`pkg/lob` and `pkg/baseline` over together; `pkg/churn`'s local `noAttrition` was
ensembled with it. **Every model package in the project now reports ensemble means.**

### No verdict changed in this tranche

A +0.62 → +0.60, C 27.4/12.3 → 26.4/12.0, E's drift 2.72 → 2.90, the minimal/priced
coupling +0.37 → +0.39 and +0.64 → +0.65. Like `pkg/recycled`, these results were never
close enough to their bounds for the seed to decide them.

### The numbers moved toward theory, which is the best evidence the change was right

`pkg/baseline`'s models draw **independent Poisson streams**, so their dispersions are
exactly 1 and their arrival/cancellation correlation exactly 0 **by construction**. One
seed gave 0.96, 0.98 and ±0.04. The ensemble means give **1.00 and 0.00**.

Those single-seed deviations were never findings — they were noise around values the
construction fixes. It matters here more than anywhere: this package is the synthetic
**control** the real-market diagnostics are read against, and a control whose own numbers
wander is a poor yardstick.

### Where the corpus now stands

| | measurement |
|---|---|
| 7 model packages | 32-member ensemble means at 8000 steps |
| `pkg/recovery`, `pkg/windowing` | single seed — inference claims (ESS, posterior width), not correlations |
| `pkg/crypto`, `pkg/replication`, `pkg/oos`, `pkg/noisefloor` | real data; no seed to vary |

`pkg/recovery` and `pkg/windowing` are deliberately left: their claims are about a
sampler's behaviour rather than a correlation, they run the macros tier which the engine's
ensembler does not cover, and they are already the two slowest packages in the suite. If
their margins ever need defending, that is a separate piece of work with a different
justification.

### The whole arc, in one line

Two independent noise measurements — the market's 0.079–0.112 between windows, the model's
0.024–0.053 between seeds — said the same thing, and the corpus now reflects it: **quote
means with spreads, and trust bounds with wide margins rather than comparisons decided by a
tenth.**


## A gap I filed that was not a gap: params_from_upstream does read same-step

**Retracted 2026-08-02, hours after filing it.** I recorded STOCHADEX_GAPS entry 1b —
"the expressions tier has no same-step cross-partition read", severity high — and
concluded from it that the planned partition refactor was impossible. **Both were wrong.**

### What I actually tested, and what I claimed

I tested `upstreams: {alias: partition}` in an expressions block, found it gives row 0
(the previous committed step), and generalised from that one mechanism to the whole tier.

The engine has a second mechanism I did not test. `params_from_upstream` is a
**partition-level YAML key** on `PartitionConfig` — plain config, not a Go iteration —
which forwards an upstream partition's output as a named param. Probed the same way: where
the source emits 5, the sink reading it through `params_from_upstream` sees **5**.

| mechanism | what it gives |
|---|---|
| `upstreams: {alias: partition}` in the expressions block | row 0 — the **previous** step |
| `params_from_upstream: {name: {upstream: partition}}` on the partition | the **current** step |

Both are pure config. The engine's `CheckForDeadlock` exists precisely to police cycles in
the second one, which should have told me it was a live within-step mechanism available to
any partition — I read that line and took it to mean the opposite.

### What this reverses

- **STOCHADEX_GAPS entry 1b is deleted**, per that file's own rule: "not things that turned
  out to be my misunderstanding — those get recorded as corrections in DECISIONS.md
  instead." The gap list is back to one high-severity entry, the missing `scan`.
- **The partition refactor is back on.** It is expressible: components that need a
  contemporaneous read use `params_from_upstream`, and components where a one-step lag is
  correct — the book reading its own previous state — use the ordinary row-0 semantics.
- **"The monolithic partition is forced, not stylistic" is withdrawn.** It is not forced.
  It is how these models happen to have been written.

### The pattern in the error, which is worth more than the correction

This is the third time today I asserted something from partial checking: the inference
confound (overstated without reading `bucket.go`), the noise-floor saturation (read off a
range statistic too noisy to support it), and now this. All three followed the same shape —
one measurement, generalised past what it covered, stated with more confidence than the
evidence carried.

The two earlier ones were caught by measuring again. This one was caught by the maintainer,
because I filed it as a finished finding rather than as a question. **A negative result
about a tool's capability deserves the same adversarial check as a positive result about a
model** — and "I could not find a way" is not the same claim as "there is no way".


## Step 3 done: the model decomposes four ways, and the model is unchanged

`cfg/lob_split.yaml` is `cfg/lob_damping.yaml` as four partitions — **driver, flows, book,
observables**. The monolith is untouched and stays pinned; a test asserts that, so the
refactor cannot quietly become an edit to the config the scored AC–AG claims rest on.

### A second thing I got wrong, corrected by the maintainer

The first version stopped at three partitions, with this reasoning recorded in the config:
an observables partition would need both the previous and current step of `book`, and
"whether one partition may reach another by both mechanisms at once is untested", so it
was avoided.

**That was a guess presented as caution, and it was wrong.** It is allowed. `observables`
now reads `book` through `params_from_upstream` for the post-update ladder the spread comes
off, and through an `upstreams:` alias for the pre-update ladder the flows were drawn
against — simultaneously, in one partition.

This is the same failure as the retracted gap: not testing a mechanism and writing down the
absence of evidence as a property of the tool. The difference is only that this one was
hedged rather than asserted, which made it cheaper to correct but no better reasoned.

### The dependency structure

```
activity ──(current)──> flows ──(current)──> book ──(current)──┐
                          ^  │                 │               ▼
                          │  └───(current)─────┼──────> observables
                          └───(previous)───────┘               ▲
                                                (previous)─────┘
```

`flows` reads `book` at the **previous** step, which is not a concession — the monolith
already damps arrivals by the previous step's depth, since its `bid` is its own row 0. That
is what breaks the apparent flows↔book cycle. `CheckForDeadlock` passes.

### Only the driver split costs anything

**Two-way, three-way and four-way splits give identical numbers to four decimals on all
four scored quantities.** `book` and `observables` draw no randomness — both are
deterministic given the flows — so lifting them out cannot change a result. A test forbids
a draw appearing in either, because that property is what makes those boundaries free.

Every difference from the monolith comes from separating the **driver**, which moves the
activity draws onto their own RNG stream:

| | monolith | 4-way split | difference | diff SE |
|---|---|---|---|---|
| `corr(depth, arrivals)` | −0.2244 | −0.2284 | 0.0039 | ~0.010 |
| `corr(depth, cancels)` | −0.1200 | −0.1251 | 0.0051 | ~0.010 |
| `corr(arrivals, cancels)` | +0.8629 | +0.8661 | 0.0031 | ~0.002 |
| depth drift | 1.0018 | 0.9964 | 0.0054 | ~0.009 |

### What step 3 produced

**No new gap**, and it retired one — this is the working counter-example to the entry
claiming the expressions tier cannot read another partition at the current step. The list
going into step 4 is unchanged at one high-severity entry, the missing `scan`.

The earlier prediction explains the thin haul: refactoring a model that already works
surfaces only *ergonomic* gaps, while capability gaps come from expressing something new.
What it surfaced instead were two errors of mine about the engine's capabilities, both in
the same direction — assuming a mechanism absent because I had not exercised it.


## Gate 3.4 — Invariant A boundary (RESOLVED: inference stays downstream)

**Branch 1 selected by the maintainer on 2026-07-31.** PLAN.md reserves this gate for
the maintainer and instructs the agent to halt; the agent halted, assembled the
evidence below, and the maintainer chose. The evidence is left exactly as it was
gathered, before the choice, and the resolution is recorded at the end of this section
rather than woven through it — so a reader can check that the reasoning was not
written backwards from the answer.

What follows is that evidence, including three plan premises that turn out not to
hold.

### Three Phase 3 premises that do not hold

Checked against stochadex v0.13.1.

1. **There is no streaming source stanza.** The complete set of `data:` sources is
   `csv`, `json_log`, `postgres` (in-engine, `pkg/api/macros_data.go`) plus `arrow`
   and `s3` (registered by the CLI). All five read a **complete** dataset into a
   `StateTimeStorage` and return. PLAN.md's Phase 3 "exercises the streaming source
   stanza" — there is not one to exercise.
2. **There is no data-agreement or schema-negotiation layer.** Grepping the engine
   for it returns nothing; the only "agreement" hits are declarative-twin numerics
   in `models/`. Spike 3.2 as written has nothing to exercise.
3. **The Postgres schema is fixed by the engine**, not negotiated:
   `(partition_name, time, state float64[])`, the same three columns for source and
   sink (`pkg/analysis/postgres.go`). It already serves the dual role Spike 3.2 sets
   out to design. Nothing to design.

The websocket in `pkg/api/socket.go` is **output only** (address/handle/delay — it
serves state updates to a dashboard). There is no ingress anywhere.

### The boundary was already drawn upstream

PLAN.md frames Invariant A as open, with the risk that Phase 3 resolves it by fait
accompli. That framing is out of date. stochadex's `CLAUDE.md` restates it
deliberately for the config surface: inference-*as-forward-simulation* — a posterior
stepped as a partition — **is in scope for the engine**, and `posterior_estimation`
and the other inference macros belong there. What stays downstream is "the dataset,
the calibration loop, and the decision layer". `data:` is the object carrying the
split.

**Phase 1 confirms that split by construction rather than by argument.** A complete
calibration — generator, likelihood, posterior, SMC — with no Go in either the model
or the inference. What this repo actually supplied was exactly three things: the
dataset, the trust/claim layer, and a measurement harness. The observed split equals
the restated invariant, and nothing was arranged to make it come out that way.

**But that holds only because everything so far is batch.** Today `data:` is a
*noun* — a finished dataset handed over. Streaming would make it a *verb*. That is
what keeps the gate live.

### The gate, restated concretely

Since the ingress does not exist, 3.4 is not "should inference move into the
engine?" — that is answered. It is: **who owns the websocket client, the
sequence-gap detector, and the growing storage?**

And there is a structural fact that matters: **`RegisterDataSource`'s contract
already permits a middle path.** Its signature is
`build(fields) (*simulator.StateTimeStorage, error)`, so a downstream-contributed
source may block on a live feed and return once a window is full. That is streaming
ingress requiring **no engine change**, and no change to the macro tier's assumption
that storage is complete before anything consumes it. `arrow` and `s3` are
contributed exactly this way, from the CLI, to keep their dependency trees out of the
engine's `go.mod` — a Binance/Coinbase client is the same shape of dependency.

[cfg/lob_calibrate_from_log.yaml](cfg/lob_calibrate_from_log.yaml) demonstrates the
resulting architecture end to end: a recorded segment on disk, read through the
ordinary `json_log` source, calibrated by the same SMC as
[cfg/lob_recovery_smc.yaml](cfg/lob_recovery_smc.yaml). Swapping `json_log` for
`postgres` (which additionally takes `start_time`/`end_time`, so the windowing
becomes the query) changes nothing else.

**Branches 1 and 3 therefore converge mechanically** — collector → Postgres →
existing source → calibration. They use identical machinery and differ only in
whether a live collector also runs, which makes deferring nearly free.

**Branch 2 is the only one demanding engine change, and the plan misidentifies which
change.** It is not inference-in-the-engine (settled, in scope). It is
*growing-storage-in-the-engine*: the analysis tier's assumption that a
`StateTimeStorage` is complete before macros consume it. That is a deeper change than
the gate's framing suggests, and it is the real cost to weigh.

### Measured: throughput is not the constraint

The open question was whether repeated windowed re-runs can keep up with arriving
data. Measured in-process (which is how a continuous calibrator would run — not
forking a CLI per window), Apple M4, stochadex v0.13.1, 2026-07-28:

| window | wall clock | throughput |
|---|---|---|
| 100 rows | 0.112 s | 896 rows/s |
| 200 rows | 0.201 s | 997 rows/s |
| 400 rows | 0.363 s | 1102 rows/s |
| 800 rows | 0.696 s | 1149 rows/s |
| 1600 rows | 1.359 s | 1177 rows/s |
| 3200 rows | 2.748 s | 1165 rows/s |

Cost is linear in window length at ~0.85 ms/row, and throughput is flat — the slight
rise is fixed overhead amortising. Roughly **1100 rows per compute-second**,
including reading the segment back off disk through the `json_log` source.

Against a crypto depth feed this is not close. A 200-row window of a feed arriving at
~10 updates/s takes 20 s to collect and 0.2 s to calibrate — about **100x headroom**.
Compute is not what decides this gate.

These numbers are recorded here rather than as claims because a wall-clock figure is
machine-dependent and would change on every run, which is exactly what the claim
mechanism forbids (`TestClaimsUpToDate` would fail constantly). The claim mechanism
correctly refuses to carry them.

### What the throughput measurement turned up instead, which matters more

| window | peak ESS | truth, in posterior sd |
|---|---|---|
| 200 rows | 26.0 | 0.73 |
| 400 rows | 13.6 | 1.68 |
| 800 rows | 7.8 | 3.00 |
| 1600 rows | 4.9 | 7.34 |

**ESS roughly halves each time the window doubles, and the posterior's
overconfidence tracks it.** More data sharpens the likelihood, which widens the
log-likelihood gaps between particles, which degenerates the weights; SMC then fits
its proposal covariance to those degenerate weights, so the posterior narrows faster
than the mean converges. Point estimates stay good throughout (2–5%); it is only the
uncertainty that rots. This is the general form of the round-count finding — **ESS is
the governing quantity, and anything that sharpens the likelihood degrades it.**

Two consequences, both pinned as claims in [pkg/windowing](pkg/windowing/behaviour.go):

- **For this gate: short windows are not a compromise forced by streaming.** For this
  sampler they are *better calibrated* than one long batch run. The windowed
  architecture is favoured on statistical grounds, not merely permitted on
  architectural ones — which is the opposite of what one would assume, and is a point
  in favour of branches 1 and 3 that had nothing to do with the boundary argument.
- **For Phase 2: do not calibrate one long window and trust the interval.**
  Either keep windows short or raise the particle count. This also qualifies
  `smc_posterior_uncertainty_is_calibrated`, whose statement now says explicitly that
  it holds at the window length measured and not in general.

### Still unmeasured

- **Whether gap semantics are expressible in config at all.** Spike 3.1 requires a
  loud failure mode on sequence gaps; that almost certainly needs Go inside the
  source, which is itself an argument about where the source lives.
- **Invariant B for a streaming source.** It is ingress rather than hot loop, so B
  probably does not bind — but it has not been checked.
- **Real feed behaviour.** Everything above is synthetic flow. Arrival rates,
  burstiness and gap frequency on a real exchange feed are unknown here.

### RESOLVED 2026-07-31 — Branch 1, inference stays downstream

**Selected by the maintainer.** The agent presented the three branches priced against
the evidence above and recommended this one; the choice is the maintainer's and is
recorded as theirs.

**What it ratifies rather than decides.** The engine had already restated the boundary
for the config surface — inference-as-forward-simulation is in scope for the engine,
while the dataset, the calibration loop and the decision layer stay downstream. A
streaming ingress owns a live dataset and a collection loop, so it is downstream *under
the existing invariant*. This branch states that conclusion rather than inventing one.

**Why not branch 2.** Its real cost is not the one PLAN.md names. Inference-in-the-engine
is settled and in scope; what branch 2 actually buys is **growing-storage in the engine**,
which breaks the analysis tier's assumption that a `StateTimeStorage` is complete before
macros consume it. That is a deep change to a core assumption, and the windowing evidence
says it would purchase the wrong regime: ESS halves as the window doubles and posterior
overconfidence tracks it, so growing storage buys exactly the long-window behaviour that
degrades calibration. It would also pull exchange-specific sequence semantics into the
engine, per the gap-semantics item above.

**Why not branch 3.** Deferring is nearly free mechanically — branches 1 and 3 converge on
identical machinery. But it leaves the *principle* unstated, and
[STOCHADEX_GAPS.md](STOCHADEX_GAPS.md) entry 1 cannot be resolved without it: that entry
frames its own options as "bespoke Go — breaking the pure-config property every model here
holds, which is an Invariant A decision — or a `scan` primitive in the engine". A deferred
gate means that question reopens this one.

### What this decision now settles downstream

1. **Streaming ingress is a downstream-contributed data source**, built on
   `RegisterDataSource`'s existing contract — `build(fields) (*StateTimeStorage, error)`,
   blocking on the feed and returning once a window is full. **No engine change**, the
   same shape `arrow` and `s3` already use to keep their dependency trees out of the
   engine's `go.mod`.
2. **Phase 3 is unblocked** and its shape is fixed: collector → Postgres → existing
   source → windowed calibration, which
   [cfg/lob_calibrate_from_log.yaml](cfg/lob_calibrate_from_log.yaml) already
   demonstrates end to end against a recorded segment.
3. **Gap detection and the websocket client are downstream**, where domain-specific Go
   is allowed and does not compromise the pure-config property, which applies to
   *models* rather than to sources.
4. **STOCHADEX_GAPS entry 1 is engine work, unambiguously.** A `scan` primitive concerns
   the expressiveness of forward simulation, which this branch places squarely in the
   engine. That is what makes the upstream release work resolvable rather than another
   boundary argument.

Three of PLAN.md's Phase 3 spikes are affected by the premises recorded above rather
than by this decision: 3.2 has nothing to exercise (no data-agreement layer exists),
and the streaming-source half of 3.1 is now scoped to a downstream source. Those are
consequences of the measurements, not of the branch.

---

## Engine findings — reported and fixed in stochadex v0.13.1

Both were hit while building Phase 1, reported upstream, and fixed the same day in
[v0.13.1](https://github.com/umbralcalc/stochadex/releases/tag/v0.13.1) (PR #72).
This repo now pins `v0.13.1` and both workarounds have been removed.

They are kept in this log rather than deleted because the first one carries a lesson
that outlives the fix, and because PLAN.md's standing rule is that a downstream repo
working around a silent engine behaviour is the thing to avoid.

### 1. `window_data_history_depth` larger than the window depth silently INVERTED the likelihood

**Fixed: the engine now warns on the oversized side.** It previously validated only
the too-shallow side — which cannot silently void anything — so the dangerous
direction passed without comment.

`window_data_history_depth` sets the **outer replay partition's** state-history
depth. `general.FromHistoryIteration` walks that buffer from row
`StateHistoryDepth-2` down to `StateHistoryDepth-Depth-1`, so it must **equal**
`Window.Depth`: equality consumes the buffer exactly, and anything larger anchors the
window in rows that are still zero-filled (the buffer starts zeroed and gains one
real row per outer step).

**The upstream fix corrects how this log first described the effect, and the
correction matters.** Scoring against the buffer's zeros does not merely weaken the
likelihood — it *inverts* it. On the engine's own test scenario the correct depth
scores the true mean at −44.8 against −62.9 for a mean of zero, while the oversized
depth scores zero at −36.0 against −53.4 for the truth. A near-zero parameter
therefore outscores the truth, and a posterior driven by it does not wander on noise:
it **converges onto its prior and sits there**.

That is a better account of this spike's first result than the one recorded above it.
The posterior froze at `[0.668, 0.046, 0.344]` against a prior of `[0.4, 0.05, 0.3]` —
and the two weakly-informed coordinates had landed essentially *on* the prior
(0.046 vs 0.05, 0.344 vs 0.3). It was not failing to move; it was being actively held
there by a likelihood that preferred zero. Which is exactly why no amount of tuning
window depth, memory depth, past discount or proposal width recovered anything.

The warning is a warning rather than a panic: every in-repo config already used
equality, so making it fatal breaks nothing and stays open as a follow-up. Verified
firing on this repo's own config path, naming the fix:

```
macros: WARNING window data partition "lob_flow" has StateHistoryDepth 2000 >
Window.Depth 100; the window will replay zero-filled rows for the first 1998 steps
and lag the data by 1900 steps thereafter, so the likelihood will be near-constant
and carry no information — set the history depth equal to Window.Depth (100)
```

`TestWindowHistoryDepthMatchesWindowDepth` is **kept** despite the fix. A warning
goes to the log, where CI will not fail on it and a human may not read it; and the
equality is a property of these configs worth asserting locally rather than
inheriting from a dependency.

### 2. `comparison.model` with no `params:` panicked on a nil map

**Fixed: `ParameterisedModel.Init` now allocates `Params`** (as does
`ParameterisedModelWithGradient`). It previously allocated only the two
parameter-wiring maps, so `NewLikelihoodComparisonPartition` setting `cumulative` and
`burn_in_steps` into a nil map died with a bare `assignment to entry in nil map`
naming nothing that pointed back at the config.

It was reachable from any config whose likelihood needs no scalar params of its own —
the natural shape when the mean comes from upstream, which is exactly this repo's
Poisson comparison. The `params: {}` workaround has been removed from both
[cfg/lob_recovery.yaml](cfg/lob_recovery.yaml) and
[cfg/lob_likelihood_surface.yaml](cfg/lob_likelihood_surface.yaml).

### Effect on the Phase 1 result: none

The upgrade moved no numbers. `CLAIMS.md` regenerates byte-identical on `v0.13.1`,
which is what should happen — one fix is a log line and the other is an allocation on
a path this repo had already worked around. **The Spike 1.2 escalation stands
unchanged:** identification is sound, ESS = 1.00 of 16, and the methodology decision
is still open.

---

## Unplanned findings that change later phases

Neither was known to the plan. Both reduce scope.

### Spike 4.1 (Arrow) is largely resolved upstream

`pkg/arrowstore` **already exists** as an opt-in separate module
(`github.com/umbralcalc/stochadex/pkg/arrowstore`), kept out of the engine's
`go.mod` so Arrow's dependency tree stays away from the core. It is built and
tested with `-race` in the engine's own CI.

Its `doc.go` already reports the benchmark the spike asks for, and reports it
against itself:

- **Append hot path:** allocation *count* collapses to a constant in row count
  (~50 vs ~2000 allocs at 2000 rows — a real GC-pressure win), but wall-clock is
  only comparable-to-better at *wide* state and is **slower at mid widths**, with
  higher transient memory from builder capacity doubling.
- **Getting to Arrow:** wins decisively, ~2.2–2.7× faster with roughly half the
  memory versus appending to `StateTimeStorage` and converting afterwards.

Its own guidance: "reach for `ArrowStateTimeStorage` when the output is destined
for the columnar/analytical world; keep the pure-Go `StateTimeStorage` otherwise."

That is PLAN.md's second branch — *Arrow moves strictly to the egress boundary* —
already selected upstream, with numbers. Spike 4.1 should not re-litigate
Invariant B. What remains is domain-side and much smaller: use the Arrow output
function for Phase 4 egress, and measure on *this* model's state width to confirm
the width regime rather than assuming it.

### Phase 5's runtime already exists

`pkg/onnx` exists as an opt-in module: a frozen ONNX model behind
`simulator.Iteration`, registering `{type: onnx_inference}` via
`api.RegisterIteration`, behind an `onnx` build tag with CGO. The engine's CI
fetches ONNX Runtime 1.27.1 and runs its differential inference test, and also
compile-checks the CLI with the tag.

Spike 5.1's allocation question is **still open** — module existence is not an
allocation profile, and this has not been measured. But Phase 5, if Spike 2.2
triggers it, is a config surface plus a trained model, not an integration project.
Also worth holding onto: Phase 5 remains conditional. PLAN.md is right that adding
a learned component to a model that does not need one weakens the claim.

### Gate 3.4 has moved

PLAN.md frames Invariant A as an open boundary that Phase 3 walks into. The engine
has since **restated it explicitly** for the config surface (stochadex `CLAUDE.md`,
"Invariant A restated for this surface"): inference-*as-forward-simulation* — a
posterior stepped as a partition — is *in scope for the engine*;
`posterior_estimation` and the other inference macros belong there. What stays
downstream is the **dataset** (`data:`), the calibration loop, and the decision
layer.

That does not resolve Gate 3.4, and this log does not select a branch — PLAN.md
reserves it for the maintainer. But it narrows the question a long way. Under the
restated invariant, a streaming calibration is downstream because it owns a *live
dataset and a collection loop*, not because a posterior is being stepped. The
`mcts_self_play` split (search settings are data; the `Environment` is a
downstream-registered Go hook) is the worked precedent for how such a boundary gets
drawn rather than deferred.

