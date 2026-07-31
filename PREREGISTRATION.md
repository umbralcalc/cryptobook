# Pre-registered thresholds

PLAN.md, Spike 1.2: *"define the tolerance **before** running; do not tune it to the
result."* This file is that commitment, kept separately from the results so the
order is auditable rather than asserted.

A threshold recorded here does not move to accommodate an outcome. If a result
misses it, the result missed it — the record says so and the claim states the
failure. Widening a bound after seeing the number would make every other claim in
this repo worth less.

**Labels normalised 2026-07-31, and recorded here rather than done quietly.** Three
blocks declared their predictions under different letters than they were scored
under: removing attrition declared F/G/H and scored D/E/F, depth-dependent arrivals
declared I/J/L and scored G/H/I, and cross-segment replication declared P/Q/R/S and
scored J/K/L/M. Everything else in the project — every scored table, every outcome
table, every claim ID in `CLAIMS.md`, and the commit messages — already used one
sequential A–M scheme, so the declaration headings were the outliers and they were
brought into line with it. **No threshold, no measured value and no pass/fail verdict
changed**, and the letters now run A–M with no gaps and no reuse. Letters N–S are
deliberately left unused: they appear with older meanings in commits made before this
edit, and leaving them empty means no letter ever carries two meanings across the
history of this repo.

---

## Spike 1.2 — synthetic parameter recovery

**Fixed:** 2026-07-28, before any tuning run.

Recovery target (`cfg/lob_recovery.yaml`): `limit_rate` 1.2, `cancel_rate` 0.15,
`market_rate` 0.8. Off-truth prior: `[0.4, 0.05, 0.3]`.

**Tolerance — PASS is:** final posterior mean within **25% relative error** of each
true value.

| Parameter | Truth | Pass band |
|---|---|---|
| `limit_rate` | 1.2 | 0.90 – 1.50 |
| `cancel_rate` | 0.15 | 0.1125 – 0.1875 |
| `market_rate` | 0.8 | 0.60 – 1.00 |

**Independently required:** the posterior mean must end closer to the truth than the
prior started, in every coordinate.

### First result, recorded before any tuning

Window depth 100, `memory_depth` 100, `past_discount` 0.999, and
`window_data_history_depth` set to the run length (2000):

> posterior mean `[0.6682, 0.04577, 0.3437]` — **FAIL on all three**, and frozen
> (identical for hundreds of consecutive steps).

That first configuration turned out to be broken rather than badly tuned: setting
`window_data_history_depth` to the run length made the embedded window score the
replay buffer's zero-filled rows, which *inverts* the likelihood rather than weakening
it — so the posterior was being actively held at its prior. (Reported upstream and
fixed in stochadex v0.13.1; see [DECISIONS.md](DECISIONS.md).) Setting it equal to the
window depth is what produced the results now in [CLAIMS.md](CLAIMS.md) — with the
tolerance above unchanged, and `market_rate` still failing it at every setting.

The tolerance was **not** revisited after that fix. See
[DECISIONS.md](DECISIONS.md) for the full spike result.

---

## Spike 2.2 — predictions for the crypto state spine

**Fixed: 2026-07-28, before any real market data has been collected or fitted.**

The state-spine mapping ([pkg/feed/bucket.go](pkg/feed/bucket.go)) makes assumptions
that are known to be wrong in specific ways. Writing down *how* they should fail,
before seeing a residual, is what separates a diagnosis from a rationalisation —
after the fact, any residual can be explained by whichever assumption is nearest to
hand.

### Prediction 1 — overdispersion in all three counts

Lot discretisation treats one k-lot order as k independent arrivals. Real order flow
therefore has **variance above the Poisson mean** in `n_limit`, `n_cancel` and
`n_market`.

**Expected direction:** variance/mean ratio > 1 for all three. **Largest** for
`n_market`, because trade sizes are the most heavily clustered (one marketable order
sweeping several lots at once is a single decision).

If the ratio is ≈ 1, the lot size is small enough that the idealisation holds and
this prediction was wrong. If it is < 1 (underdispersion), something is wrong with
the mapping rather than the model.

### Prediction 2 — the uniform-arrival assumption fails, but may not show here

The model assumes arrivals are uniform across the six ladder levels; real books
concentrate them at the touch. The observable is the *summed* count, so this
misspecification may not be visible in `n_limit`'s marginal at all. It should show
in the per-level resting depths (row indices 0–5): the model predicts roughly equal
depth per level, real books do not.

**Expected direction:** touch levels deeper than outer levels is the *opposite* of
the synthetic model's structural claim
(`the_touch_holds_less_resting_volume_than_the_level_behind_it`, which holds only
because market orders consume at the touch and arrivals are uniform). Whichever way
it falls on real data, the synthetic claim does not transfer, and that is stated
here rather than discovered.

### Prediction 3 — the cancellation/depth coupling survives

`E[n_cancel] = cancel_rate × depth` is the coupling Phase 1's identification result
rests on, and it is the most plausible of the model's assumptions: cancellations
genuinely do scale with resting volume.

**Expected direction:** a positive, roughly linear relationship between
`depth_start` and `n_cancel` in the recorded data. If this one fails, the
parameterisation does not transfer to real data at all and Phase 2 stops rather than
being tuned.

### What would count as failure

PLAN.md's Spike 2.2 branches turn on where residuals are bad. Recorded here so the
branch is chosen against a stated bar:

- Predictions 1 and 2 confirmed, 3 holding → the parametric form is usable with
  known limitations; ONNX stays deferred unless a specific component fails.
- Prediction 3 failing → the model form is wrong for real data, not the parameters.
  Return to the domain model rather than proceeding to Phase 3.

---

## The churn model — predictions fixed before it exists

**Fixed 2026-07-29, before `cfg/lob_churn.yaml` is written.** Committed on its own
so the ordering is verifiable in the history rather than asserted.

### Why this needs pre-registering more than anything so far

Quote churn is the last hypothesis standing for Spike 2.2's failure, and it is the
one I am most at risk of confirming by construction. If I build a model whose
arrivals and cancellations share a driver, it *will* show a high arrival/cancellation
correlation — exactly as the priced model's depth coupling came out strong because
the coupling was drawn in. That number carries almost no information.

The informative question is whether churn **also destroys the depth coupling**, the
way real markets show. That is not implied by coupling the two streams, and it is
what these predictions are about.

### The mechanism, stated before measuring

A latent per-step activity factor drives both streams: `activity_t` drawn from a
gamma with mean 1, arrivals scaled by it, and a cancellation term proportional to
`activity` rather than to resting depth — a market maker refreshing their own quotes
posts and pulls a roughly constant amount, not a fraction of the whole book. A
smaller depth-proportional cancellation term stays, because ordinary resting orders
do still get cancelled.

### Predictions

**A — arrivals and cancellations become correlated.** `corr(arrivals, cancels)`
rises from ~0.0 to **> +0.5**.

*Near-certain by construction. Recorded so it cannot later be presented as a
finding.* Binance BTCUSDT reads +0.98.

**B — the depth coupling collapses.** `corr(depth, cancels)` falls from +0.37
(minimal) and +0.64 (priced) to **< +0.2**.

**This is the real prediction.** It is not implied by A: coupling the two streams
says nothing directly about depth. It should follow only if the activity-driven
cancellation term dominates the depth-proportional one, making cancellation flow
mostly exogenous. If B fails while A succeeds, churn reproduces the co-movement but
not the missing coupling, and **churn is not the mechanism** — or not in this form.

**C — dispersion rises above Poisson.** Variance/mean for arrivals and
cancellations rises from ~1.0 to **> 1.5**.

A Poisson mixed over a random intensity is over-dispersed, so this should follow
from the gamma driver. It matters because the current models reproduce *none* of the
real overdispersion, and this is the first mechanism
that could produce it without being told to.

### What each outcome means

| A | B | C | reading |
|---|---|---|---|
| ✓ | ✓ | ✓ | Churn reproduces all three signatures at once. The strongest result available, and the point at which calibrating the churn model becomes worth attempting. |
| ✓ | ✗ | — | Co-movement without losing the depth coupling. **Churn is not the mechanism**, and the search reopens with no candidate left. |
| ✓ | ✓ | ✗ | Churn explains the correlations but not the overdispersion, which then has a separate source — the likelihood family becomes worth revisiting after all. |
| ✗ | — | — | The implementation does not do what it was built to do; a bug, not a finding. |

**No parameter tuning to reach these.** If the first honest parameterisation misses
B, that is the answer. Rates will be chosen to put the model in a plausible
microstructure regime — churn flow comparable to arrival flow — and not adjusted to
move a correlation.

### Scored, 2026-07-29

| | prediction | measured | |
|---|---|---|---|
| **A** | `corr(arrivals, cancels)` > +0.5 | **+0.62** | PASS |
| **B** | `corr(depth, cancels)` < +0.2 | **+0.60** | FAIL |
| **C** | dispersion > 1.5 | **27.4 / 12.3 / 4.1** | PASS |

**B failed, and it was the one that mattered.** The pre-registered reading for
"A pass, B fail" was that churn reproduces the co-movement but not the missing
coupling, so churn is not the mechanism. That reading stands as written.

**C is the genuine positive.** 27.4 and 12.3 are the first departure from Poisson any
model here produces — the first to reproduce any of the
overdispersion, and as a consequence of the driver rather than by being told to.

**A fault in this pre-registration, recorded rather than quietly fixed.** The stated
parameter criterion — "churn flow comparable to arrival flow" — does not pin the
regime. What decides whether the `min(...)` clip binds is churn relative to
**depth**, not to arrivals, and at the rates chosen churn is a large fraction of the
resting book each step. So B's failure is **inconclusive about the mechanism**
rather than a refutation of it: the cancellation path still contained
depth-dependent terms (residual attrition, and the clip), which the measurement
makes visible — arrivals came out uncorrelated with depth (+0.02, matching real
markets) while cancellations did not.

Re-testing in a regime where churn is a modest fraction of depth needs a **fresh
pre-registration**. Adjusting the rates now and reporting a pass would be exactly
what this document exists to prevent.

## Removing attrition — what it should COST, fixed before the change is made

**Fixed 2026-07-29, committed before `cancel_rate` is touched.**

### The design problem this pre-registration exists to solve

The residual depth/cancellation coupling has a named suspect: the depth-proportional
attrition term supplying ~39% of cancellation flow. Deleting it would almost
certainly move the coupling, because the coupling is being deleted. **A prediction
that the deleted thing stops having an effect is not a test.**

So the predictions below are about what removing attrition **costs** — consequences
the change was not made to address, and which decide whether the resulting model is
usable at all.

### The mechanical claim behind them

**Attrition is the model's only depth-stabilising force.** Arrivals and churn are
both driven by activity and independent of depth; the marketable sweep removes
`min(size, available)`, which is also depth-independent once the book is deeper than
the order. `poisson(cancel_rate × q)` is the sole negative feedback. Remove it and
depth becomes a random walk with drift:

	(limit_rate − churn_rate) × Σdecay × 2 sides − market_rate × market_size
	  = (2.0 − 1.15) × 3.18 × 2 − 1.2 × 4
	  = 5.41 − 4.80 = +0.61 per step

Over the ~1900 scored steps that is roughly +1160 on a starting depth of 52.

### Predictions

**D — the coupling falls, as it must.** `corr(depth, cancels)` < **+0.2**.

*Near-forced and declared as such.* Recorded so it cannot be presented as the
result. If D fails, my diagnosis of the residual was simply wrong.

**E — depth stops being stationary.** Mean depth over the second half of the scored
window, divided by the mean over the first half, exceeds **1.5**.

The arithmetic above predicts a ratio near 2.7. A conserved book gives ~1, so a model
that drifts is failing to conserve the book at all. E could fail if the
clip or the sweep stabilise the book in a way the arithmetic misses — that is the
part actually being tested.

**F — the spread collapses to its floor.** Mean spread < **2.5 ticks** with a
standard deviation < **0.5**.

A book growing without bound keeps its inner levels permanently occupied, so the
touch never moves. Least certain of the three, because it depends on how growth
distributes across levels rather than on the aggregate.

### What each outcome means

| D | E | reading |
|---|---|---|
| pass | **pass** | The coupling is fixable only by removing the model's sole depth-stabiliser, so it trades a correlation failure for a stationarity failure. **Neither form works.** What is needed is a stabiliser that is not depth-proportional cancellation — depth-dependent *arrivals* is the natural candidate, and economically sensible: less incentive to post into a deep queue. |
| pass | fail | The coupling is fixed AND the book stays stationary, so something else stabilises it. The strongest outcome and worth pursuing. |
| fail | — | Removing attrition did not fix the coupling. My diagnosis of the residual was wrong and the search reopens. |

**No parameter adjustment.** Only `cancel_rate` changes, and only to zero. Every
other value stays as it was when the ratio was fitted.

### Scored, 2026-07-29

| | prediction | measured | |
|---|---|---|---|
| **D** | `corr(depth, cancels)` < +0.2 | **+0.009** | pass (near-forced, declared) |
| **E** | depth 2nd-half / 1st-half > 1.5 | **2.72** | pass |
| **F** | spread < 2.5 ticks, sd < 0.5 | **2.00 ± 0.00** | pass |

The arithmetic was near-exact: predicted ≈ 2.7 drift, measured 2.72.

| | with attrition | attrition removed | real |
|---|---|---|---|
| `corr(depth, cancels)` | +0.237 | **+0.009** | ~0 |
| `corr(arrivals, cancels)` | +0.896 | **+0.950** | — |
| depth drift | 0.95 | **2.72** | — |
| spread | 2.05 ± 0.21 | **2.00 ± 0.00** | variable |

**All four correlation signatures now match reality almost exactly — and the book no
longer conserves.** The reading from the pre-registered table (F pass, G pass) holds:
the coupling is fixable only by deleting the model's sole depth-stabiliser, so it
trades a correlation failure for a stationarity failure. **Neither form works.**

**This is the clearest case in the project for pre-registering costs rather than
only successes.** Without G and H, the honest-looking report would have been
"removing attrition fixes the coupling and improves the co-movement — the churn
mechanism works", accompanied by a model whose book grows without bound and whose
spread is a constant. Every correlation in that report would have been true.

### Where it points

Depth stationarity needs a depth-dependent force. If that force is *cancellation*
you get the coupling back; so it has to act on **arrivals** — posting rate falling
as the queue deepens, which is economically sensible (less incentive to join a long
queue) and is the standard shape in the Santa Fe literature this model descends
from.

That has a testable signature worth noting before anyone measures it:
`corr(depth, arrivals)` should go **negative**. Note that this is forced by
construction rather than informative: damping arrivals by resting depth is what
produces the sign, so it tests the implementation, not the hypothesis.

---

## Depth-dependent arrivals — predictions fixed before the mechanism exists

**Fixed 2026-07-29, committed before `cfg/lob_arrivals.yaml` is written.**

### The question this has to answer

Removing attrition showed the coupling is fixable only by deleting the model's sole
depth-stabiliser. So the open question is narrow and specific: **can depth be
stabilised WITHOUT reintroducing the depth/cancellation coupling?**

The candidate is a stabiliser acting on arrivals — posting rate falling as the queue
deepens, which is economically sensible (less incentive to join a long queue) and is
the standard shape in the Santa Fe literature this model descends from.

### The mechanism, stated before measuring

Cancellation stays pure churn (no attrition term, so no depth-proportional
cancellation). Arrival intensity gains a depth-dependent damping:

	arr_i ~ Poisson( limit_rate * decay_i * activity / (1 + q_i / arrival_scale) )

`arrival_scale` sets the queue depth at which posting halves. It will be chosen so
the resulting mean depth lands in the same range the attrition model produced, so
the comparison is not confounded by an overall scale change — **not** chosen by
looking at any correlation.

### Predictions

**G — stationarity is restored.** Depth 2nd-half / 1st-half ratio < **1.3**, against
2.72 with no stabiliser, against ~1 for a conserved book.

Expected but not certain: it depends on the damping being strong enough to bite
before the book runs away.

**H — the coupling stays fixed.** `corr(depth, cancels)` stays below **+0.2**.

**This is the real test.** Cancellations are pure churn and contain no depth term at
all, so naively this is forced — but it is not, because depth is now *anti*-correlated
with arrivals, and arrivals and cancellations share the activity driver. That
indirect path could reintroduce a depth/cancellation correlation of either sign.
**I do not know which way this comes out**, which is what makes it worth running.

**I — the spread stays alive.** Spread standard deviation > **0.1** ticks.

The cost check, in the shape of F. A stabilised book could still pin its inner
levels and kill the spread-response output; if it does, this variant fails on the
same axis the last one did.

### What each outcome means

| G | H | I | reading |
|---|---|---|---|
| pass | **pass** | pass | Depth is stabilised, the coupling stays broken, and the spread survives. **The first mechanism to reproduce the real correlation structure without breaking the book** — and the point at which calibrating against real data becomes worth attempting. |
| pass | **fail** | — | Stabilising depth reintroduces the coupling by an indirect route, even with no depth term in cancellation. That would say the coupling is a consequence of *any* depth stabiliser, not of attrition specifically — a much deeper result, and it would mean the real markets' near-zero coupling is evidence against depth stabilisation altogether. |
| fail | — | — | The damping is too weak to stabilise. A parameterisation problem, not a finding — and `arrival_scale` would be refitted to the depth range once, not tuned against J. |
| pass | pass | **fail** | Coupling fixed and book conserved, but the spread dies. Same failure axis as removing attrition, reached differently. |

**One parameter is chosen, and on one criterion:** `arrival_scale`, to put mean depth
in the range the attrition model produced. Nothing is adjusted against J, K or L.

### Scored, 2026-07-29

| | prediction | measured | |
|---|---|---|---|
| **G** | depth drift < 1.3 | **1.008** | pass |
| **H** | `corr(depth, cancels)` < +0.2 | **−0.002** | pass |
| **I** | spread sd > 0.1 | **0.41** | pass |

**Only H was genuinely uncertain, and H is the result.** G was near-forced (the one free
parameter was chosen precisely to make depth stationary) and I is a cost check. H was not
forced: cancellations contain no depth term at all, but
depth is now anti-correlated with arrivals and arrivals share the activity driver with
cancellations, so the indirect path could have reintroduced a coupling of either sign. It
did not.

**So depth can be stabilised without the coupling returning**, and the mechanism that
does it is the economically obvious one: less incentive to join a long queue. That is a
statement about this model alone: nothing in this section is compared against market
data.

### What this does not establish

Nothing here is a calibration, and there is **no real-market evidence in this section at
all**. What is established is that a pure-config mechanism can
conserve the book while keeping cancellation free of any depth term. Whether that
resembles a real book is unmeasured by this project. No spread distribution has ever been
compared against a real one.

## Do the crypto signatures replicate across segments? Fixed before recording

**Fixed 2026-07-30, committed before any new segment is recorded.**

### Why this is needed

Every crypto number in this project rests on **one 8-minute BTCUSDT capture**. Phase 2's
central conclusion — that the model form does not transfer — rests on that single
segment. A result that survives only on the sample that produced it is not a result.

Crypto makes the fix cheap: segments are re-recordable from public endpoints with no gate,
so the honest form for a claim here is a **bound that holds on any independently recorded
segment** rather than a point value.

### My contamination, declared

One thing I already know, which would shape predictions if left unstated:

**I have seen the single BTCUSDT segment.** Its readings are `corr(depth, cancels)`
   ≈ −0.12, `corr(arrivals, cancels)` ≈ +0.98, and dispersion in the high hundreds to
   thousands. So J, K and L below are not blind — they are the question of whether one
   sample generalises, and they would be near-worthless as evidence of anything else.
### Protocol

Five symbols recorded **concurrently**, so they share one wall-clock window and differ
only by instrument: `BTCUSDT`, `ETHUSDT`, `SOLUSDT`, `XRPUSDT`, `DOGEUSDT`. Eight
minutes, one-second buckets, `feed.DefaultLotSize` for all five. Liquidity is ranked by
Binance's own 24h quote volume, fetched and recorded at capture time rather than assumed.

The pre-existing BTCUSDT segment is kept as a **sixth, earlier window**, which is the
only temporal comparison available.

Nothing is tuned. No model parameter is touched by this section — it tests whether the
*measurements* replicate, not whether the model fits.

### Predictions

**J — the depth/cancellation decoupling replicates.** `corr(depth, cancellations)` is
below **+0.2** in every one of the five segments. The model requires this strongly
positive, and this is the reading that stopped Phase 2. **If J fails on any segment,
Phase 2's central conclusion was a property of one capture** and has to be reopened.

**K — the churn co-movement replicates.** `corr(arrivals, cancellations)` is above
**+0.9** in every one of the five segments.

**L — overdispersion replicates.** Variance/mean exceeds **10** for both arrival and
cancellation counts in every segment, against Poisson's exactly 1.

**M — co-movement weakens with liquidity.** `corr(arrivals, cancellations)` on the
lowest-quote-volume symbol is **lower** than on BTCUSDT. Quote churn is a market-maker
behaviour — pulling and re-posting together — so thinner participation should show less
of it. This is the only prediction here that is not a replication check.

### What I expect, before running it

**J holds.** The mechanism is not liquidity-specific: the model's requirement is that
cancellations scale with resting depth, and nothing about a thin book makes that more
true. This is also the prediction G most want to be wrong, since it is load-bearing.

**K is the one at risk.** +0.9 is a demanding floor, and it was measured on the most
heavily market-made instrument in existence. On `DOGEUSDT` or `XRPUSDT` I would not be
surprised by +0.7.

**L is near-forced and declared as such.** Overdispersion here is substantially produced
by inferring lot-sized events from net depth changes, which every segment shares. It is
claimed so it cannot later be presented as a discovery.

**M is a coin flip**, and it is the only genuinely uncertain one.

### The confound that limits R and any magnitude comparison

One `DefaultLotSize` is used across five instruments whose unit prices differ by orders
of magnitude, so one "lot" is a wildly different economic quantity per symbol. Dispersion
*magnitudes* are therefore not comparable across segments, and no cross-symbol dispersion
comparison is claimed. L is a one-sided bound, which survives this; a claim that
dispersion is or is not stable across instruments would not, and none is made.

### What each outcome means

| | reading |
|---|---|
| **J fails anywhere** | Phase 2's conclusion is reopened. The depth/cancellation decoupling would be a BTCUSDT property rather than a crypto one, and the "model form does not transfer" claim would need withdrawing pending a proper survey. |
| **J holds, K fails** | The decoupling is robust but the churn signature is liquidity-dependent. That weakens quote churn as *the* missing mechanism and makes it one that only some markets exhibit — which matters, because churn is the only candidate still standing. |
| **J and K both hold** | Both Spike 2.2 findings are properties of crypto spot markets rather than of one capture. That is the strongest thing this narrowed project can currently claim, and it is a replication rather than a new result. |

**No adjustment after the fact.** The five symbols, the duration, the lot size and the
four bounds are fixed here. Whatever the segments say is what gets scored.

### Scored, 2026-07-30

**All four pass**, including the one I recorded as most at risk. Five symbols, 480
one-second rows each, recorded concurrently; no gaps and no suspect rows in any segment.

| symbol | 24h quote vol | `corr(depth,can)` | `corr(arr,can)` | dispersion arr/can |
|---|---|---|---|---|
| BTCUSDT | 1.07e9 | −0.220 | +0.973 | 2.9e3 / 3.1e3 |
| ETHUSDT | 5.53e8 | −0.246 | +0.966 | 4.0e4 / 4.9e4 |
| SOLUSDT | 1.06e8 | −0.074 | +0.940 | 4.2e5 / 4.9e5 |
| XRPUSDT | 6.96e7 | −0.015 | +0.943 | 2.1e7 / 2.7e7 |
| DOGEUSDT | 2.86e7 | −0.078 | +0.942 | 2.9e8 / 3.6e8 |

| | bound | worst case | |
|---|---|---|---|
| **J** | `corr(depth, cancels)` < +0.2 everywhere | −0.015 (XRPUSDT) | **pass** |
| **K** | `corr(arrivals, cancels)` > +0.9 everywhere | +0.940 (SOLUSDT) | **pass** |
| **L** | dispersion > 10 everywhere | 2.9e3 (BTCUSDT) | **pass** |
| **M** | DOGEUSDT co-movement < BTCUSDT | −0.032 | **pass** |

#### What J establishes, and it is the point of the exercise

**Phase 2's central conclusion now rests on five instruments instead of one.** The
depth/cancellation coupling the model requires is absent — negative, in fact — on every
symbol tested, spanning a 37× range in quote volume. Until today that conclusion rested
entirely on the capture that produced it.

It remains a replication rather than a discovery, and it was not blind: I had seen the
earlier BTCUSDT segment. It also says nothing about any other asset class; this model's
lineage is an equity one.

#### K held, and my stated reason for doubting it was wrong

I predicted K was the one at risk, on the argument that +0.9 had only ever been measured
on the most heavily market-made instrument in existence and that a thinner pair might read
+0.7. The floor holds with margin everywhere — the minimum is +0.940. Quote churn's
signature is not a large-cap phenomenon within the range tested.

#### M passes its test but not its reasoning, and that distinction matters

The stated comparison holds: DOGEUSDT's co-movement is below BTCUSDT's, by 0.032. **But
the ordering is not monotonic** — the lowest of the five is SOLUSDT, third by volume — and
0.032 is small against a 37× liquidity range. So the specific prediction passes while the
story it was written to test, that co-movement tracks market-making intensity, is not
supported. Reported as a pass with that caveat rather than as evidence for the mechanism,
because a two-point comparison is what I pre-registered and a two-point comparison is all
it can carry.

#### The declared confound showed up exactly as expected

Dispersion spans 2.9e3 to 3.6e8 across symbols. That is almost entirely the fixed lot size
meeting instruments whose unit prices differ by orders of magnitude — DOGEUSDT's unit price
is tiny, so a fixed lot means enormous unit counts. **This is why L was pre-registered as a
one-sided bound and why no cross-symbol dispersion comparison is claimed.** Had the
confound not been declared in advance, this table would have looked like a finding about
market structure. It is a finding about the measurement.

#### What none of this establishes

All five segments share **one 8-minute window on one venue**, so a market-wide regime
peculiar to that window would not be caught, and temporal replication is barely tested —
the only other window is the older BTCUSDT capture. All five are USDT majors; no genuinely
illiquid pair was recorded. And arrivals and cancellations are both inferred from net depth
changes rather than observed as messages, so a shared inference artefact could inflate K on
every segment at once. Replicating across instruments does not touch that.

> **Qualified 2026-07-31.** That last sentence is true but was left broader than the code
> warrants, and the correction belongs next to it. `pkg/feed/bucket.go` Decision 4
> accumulates deltas **per depth update** rather than netting a bucket's open against its
> close, for exactly this reason — and the feed is `@depth@100ms` against 1-second
> buckets, so roughly ten updates back each row and the window in which a post and a pull
> annihilate unseen is **100ms, not one second**. Decision 3 separates cancellations from
> fills using the **trade stream**, a second source, rather than assuming a split. What
> remains is genuinely residual: netting inside a single update at a single price, which
> understates both counts together and does so more in busy periods, and the fact that
> both counts are sums over the same update stream. The lot size is a shared divisor but
> cannot touch a correlation, which is scale-invariant — it is a dispersion confound, and
> it is already declared as L's.

---

## Depth-neutral churn — predictions fixed before `cfg/lob_churn_recycled.yaml` exists

**Fixed 2026-07-31, committed before the config is written.**

### The question this has to answer

Restating `corr(depth, arrivals)` with its numbers turned up a sharper mismatch than the
one that had been recorded. Reading the two correlation columns together rather than one
at a time: **on every Binance segment BOTH flows are mildly anti-correlated with depth**,
arrivals somewhat the stronger. Every model this project has built concentrates the whole
depth-stabilising brake on ONE flow — the depth-damped arrivals model reads −0.116 on
arrivals against −0.002 on cancellations, and the earlier variants put +0.6 on
cancellations with the wrong sign.

So: **can a model produce two comparable mild anti-correlations with depth, without
losing the contemporaneous co-movement that is the strongest real signature?**

### Feasibility, checked before designing — and it constrains the mechanism

`lag(name, n)` reads a partition's **committed state** n rows back. It cannot read an
intermediate binding from the same step: a bare field name or an upstreams alias only
ever gives row 0. Two consequences, both verified by running a throwaway config rather
than reasoned about:

- Per-level arrivals must be **promoted into the state row** (`fields:` plus `outputs:`),
  because only the totals are carried today. The state row widens from 21 to 37.
- `state_history_depth` must rise from 1 to 2.

Verified: the promoted config loads and runs, 401 rows at width 37. **No correlation was
computed from that run**, and the feasibility file was deleted rather than kept.

### Why NOT the pure lag DECISIONS.md pointed at

The recorded direction was "cancellations replacing what was just posted", a lagged
coupling. Taken literally — `can(t) = f · arr(t−1)` and nothing else — that is predicted
to fail **before it is built**, and the reasoning is stated here so the mixture below is
not mistaken for a retreat after seeing a result:

contemporaneous `corr(arrivals, cancels)` would become `corr(arr(t), arr(t−1))`, which is
approximately **zero**, because the shared activity driver is an iid gamma draw per step
with no persistence. That would trade the model's strongest match (+0.897, against +0.98
real) for the weaker one being chased. Swapping one reproduced signature for another is
not progress.

### The mechanism, stated before measuring

The arrival side is **unchanged**, so the brake and its damping stay exactly as scored.
Cancellation gains a recycled term alongside the existing same-step churn:

	can_i(t) = min( q_i , recycle · arr_i(t−1) + Poisson(churn_rate · decay_i · activity · dt) )

- The **recycled** term is depth-neutral by construction: it depends on what was posted
  at that level last step, not on what is resting there now. This is the part that is new.
- The **same-step churn** term is retained unchanged, so contemporaneous co-movement keeps
  a source.

The route to T is indirect and is why T is uncertain rather than forced: the recycled term
carries no depth term at all, but last step's arrivals were themselves damped by last
step's depth, and depth is autocorrelated — so an anti-correlation can arrive at the
cancellation flow through `q(t−1)` without any depth term ever being written into it.

### One parameter is chosen, on one criterion, stated now

`recycle = 0.5` — the **midpoint** between "no recycling" (the current model) and "all
churn is recycled posting". Chosen as a midpoint before any run, **not fitted to any
correlation**.

If mean depth leaves the range the previous models produced, `churn_rate` may be re-set
**once**, on the depth criterion alone, exactly as `arrival_scale` was for the previous
mechanism. Nothing is adjusted against T, U, V or W.

### Predictions

Labelled T–W. The scored blocks above run A–M with no gaps, so N onwards is free —
but N–S are deliberately skipped rather than used. Those letters appear with older
meanings in commits made before the label normalisation recorded at the top of this
file, and skipping them means no letter in this repo ever carries two meanings
across its history. A gap costs nothing; an ambiguity would cost the audit trail.

**T — cancellations pick up a mild negative depth correlation.** `corr(depth, cancels)`
lands in **[−0.30, −0.02]**, against −0.002 today and a real span of −0.015 to −0.246.

**This is the test.** A two-sided bound on purpose: the point is to land in the observed
band, not merely to move off zero, and overshooting past −0.30 would be a different
failure from not moving at all. I do not know which way this comes out.

**U — the arrival side stays the stronger brake.** `corr(depth, arrivals)` stays below
**−0.05**, AND `|corr(depth, arrivals)| > |corr(depth, cancels)|`.

The first half is near-forced and declared as such — the arrival damping is untouched. The
**ordering** is the uncertain half, and it is the part that matches the real segments,
where arrivals are the stronger of the two on all six.

**V — the co-movement survives.** `corr(arrivals, cancels)` stays above **+0.7**, against
+0.897 today.

The cost check, and **the prediction most at risk**. Half the cancellation flow now comes
from a source that is contemporaneously uncorrelated with arrivals, so a drop is expected;
+0.7 is where the drop stops being acceptable. If V fails, the mechanism trades the
model's best-matched signature for the one being chased, which is the outcome the "why not
a pure lag" section above exists to avoid.

**W — the book survives.** Depth 2nd-half / 1st-half ratio **< 1.3**, and spread standard
deviation **> 0.1** ticks.

The same cost check every mechanism here has had to pass. Recycling removes cancellation
volume that no longer scales with what is resting, so the conservation argument is weaker
than it looks.

**Not predicted:** dispersion. The shared activity driver already produces it, it is
unchanged by this mechanism, and claiming it here would be claiming a result the previous
step established.

### What each outcome means

| T | U | V | W | reading |
|---|---|---|---|---|
| **pass** | pass | pass | pass | The first model in this project to reproduce the *paired* depth signature without losing the co-movement. It would make the correlation structure jointly reproducible for the first time, and calibration against real data becomes worth attempting — subject to the confound below, which is what stops this being conclusive. |
| **fail (no movement)** | pass | pass | pass | Depth-neutral recycling is not enough to move the cancellation flow off zero. The indirect route through `q(t−1)` is too weak, and the paired signature needs something other than a lag — the mechanism hunt continues with one more candidate eliminated. |
| **fail (overshoot)** | — | — | — | Past −0.30 the model has over-corrected and is no longer in the observed band. That is informative rather than fatal: it says the route through `q(t−1)` works and is too strong at `recycle = 0.5`, and the honest next step is a fresh pre-registration at a stated lower value, **not** a re-run of this one at a tuned one. |
| pass | **fail** | — | — | The brake has moved onto cancellation, inverting the real ordering. That would mean recycling does not add a second mild brake so much as relocate the existing one. |
| pass | pass | **fail** | — | One signature traded for another. Recorded as a failure of the mechanism, not a partial success — the co-movement is the strongest thing this model reproduces and it is not for sale. |
| pass | pass | pass | **fail** | The correlation structure matches and the book does not survive. Same axis that killed the attrition-free variant, reached differently. |

### The confound, declared before running rather than after

**A full pass would not establish the mechanism is right.** The target signature may
itself be a measurement artefact, and this is already recorded in this document: arrivals
and cancellations are both **inferred from net depth changes** rather than observed as
messages. Both real columns therefore run through one inference path, and an artefact of
that path could produce mild negatives in both on every segment at once — including
segments that share nothing else.

So the strongest available reading of a pass is: *a pure-config mechanism exists that
reproduces the measured signature*. Whether that signature is a property of order flow or
of how this project infers order flow **cannot be settled with the data this project
has**, and would need message-level data rather than depth snapshots. That is stated now
so a pass cannot later be presented as more than it is.

### Scored, 2026-07-31

| | prediction | measured | |
|---|---|---|---|
| **T** | `corr(depth, cancels)` in [−0.30, −0.02] | **+0.458** | **FAIL — opposite sign** |
| **U** | arrival brake below −0.05 **and** the stronger | −0.110 vs +0.458; margin **−0.348** | **FAIL** on the ordering |
| **V** | `corr(arrivals, cancels)` > +0.7 | **+0.436** | **FAIL** |
| **W** | drift < 1.3, spread sd > 0.1 | 1.066, 0.579 | pass |

One value moved after the predictions were committed and only one: `churn_rate`, from
the inherited 1.15 to **0.55**, on mean depth alone. At 1.15 mean depth was 73.5 against
the previous mechanism's 227.8, outside the range, so the single adjustment this document
permits was used. The sweep computed **no correlation** while choosing — 0.4 → 397.8,
0.5 → 273.3, 0.6 → 195.4, 0.7 → 146.1, 0.8 → 110.6, 0.9 → 95.0, then 0.55 → 238.6.
`recycle` stayed at its pre-registered 0.5.

#### T failed in a direction this document did not contain, and that is recorded as a fault

The outcome table above has rows for T failing by **not moving** and for T failing by
**overshooting**. It has no row for T coming out **positive**, which is what happened:
+0.458, a stronger depth coupling than the +0.37 of the minimal model this entire line of
work began by rejecting.

So the reading below was written **after** seeing the number, and is therefore weaker
evidence than the pre-registered rows would have been. Saying so is the point of keeping
this file: a post-hoc explanation presented in a pre-registered document's voice is worth
less than one that admits which it is.

#### The reason is an accounting identity, not a rate

Cancellation was made proportional to `arr(t−1)`. But `arr(t−1)` is precisely what is
**resting** at `t` — a book accumulates its own recent arrivals — so cancellation and
depth were handed a shared term and a positive correlation followed by construction.

**Depth-neutral in the RATE is not depth-neutral in the CORRELATION.**

This kills the family rather than the instance. **Any** cancellation rule keyed to recent
arrivals inherits the same coupling, whatever the lag or the coefficient, so there is no
value of `recycle` that rescues it and sweeping for one would be wasted work. What it does
not touch is rules keyed to something *other than* arrivals, which is where the mechanism
hunt goes next and what a future pre-registration has to be about.

#### V was predicted in direction and badly underestimated in size

This document argued a *pure* lag would drive contemporaneous co-movement to about zero,
and chose a half-weight mixture to avoid that. **Half was already too much**: +0.897 fell
to +0.436, well under the +0.7 floor. The reasoning was right about the mechanism and
wrong about the magnitude — the cost of lagging scales faster than its weight.

That reasoning was also incomplete in a way worth naming. It examined what lagging
**costs** and never asked what keying cancellation to arrivals **buys**, which is the
question T answered badly. Predicting one side of a trade-off is not predicting the trade.

#### What this does not establish

The target band came from Binance segments whose arrival and cancellation counts are both
**inferred from net depth changes**, so — as declared before running — it may be an
artefact of that one inference path. A failure against a possibly-artefactual target is
still a real failure of *this* mechanism, because T's +0.458 is nowhere near the band on
any reading. But it means the hunt is chasing a target that has not itself been verified,
and message-level data remains the only way to settle that.

#### That confound was overstated, and the correction is recorded here rather than edited in

The paragraph above and the pre-registered "confound, declared before running" both put
more weight on the inference path than `pkg/feed/bucket.go` warrants. **The
pre-registered text is left exactly as it was committed** — editing a pre-registration
after scoring is what this file exists to prevent, even when the edit would be a
correction — so the accurate version goes here, dated.

The strong form of the worry is that the inference *manufactures* the signature by
erasing churn. That is specifically designed against. Decision 4 accumulates deltas **per
depth update** rather than netting a bucket's open against its close, and says why:
netting "erases churn: volume added and removed inside the same bucket cancels out,
understating both arrivals and cancellations, and understating them MORE the busier the
market is." The feed is `@depth@100ms` against 1-second buckets, so about ten updates
back each row and the annihilation window is **100ms rather than one second**. Decision 3
splits cancellations from fills against the **trade stream** — a separate source, not an
assumption.

What survives is narrower and worth stating exactly:

- Netting inside a **single update at a single price**, which understates both counts
  together and more so when the market is busy.
- Both counts being sums over the **same update stream**, so update frequency reaches
  both.
- No order identity at all, which is not a confound but a hard ceiling: queue position is
  unanswerable from this feed at any bucket size.

The lot size is a shared divisor and cannot affect a correlation, which is scale-invariant
— it is a **dispersion** confound and is already declared as L's.

**What this changes about the conclusions:** the target is on firmer ground than the
paragraph above implies, so the eliminations measured against it are worth more rather
than less. It does not rescue this mechanism, and it does not touch the finding that
survives regardless — depth-neutral in the rate is not depth-neutral in the correlation is
an identity, proved by construction, and holds whatever the market turns out to do.
