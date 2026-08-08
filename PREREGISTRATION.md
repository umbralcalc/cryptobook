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

> **Withdrawn 2026-07-31.** The paragraph above is wrong about *why* H was safe, and the
> bound and the measurement are untouched by the correction — only the claim that H was
> uncertain. The indirect path it relies on runs through the shared activity driver, and
> that driver is drawn **iid per step**. The depth H correlates against is depth at the
> **start** of the step, depending on activity only up to `t−1`, while cancellation
> depends on activity at `t`; they are independent by construction. So
> `corr(depth, cancels)` sits near zero whatever mechanism is present, and **H could not
> have failed for the reason that made it look risky.**
>
> H still establishes that no depth term leaked into cancellation. It does not establish
> that a coupling was available and avoided. Recorded as a withdrawal of the reasoning
> rather than an edit to it, for the same reason the labels were normalised in place and
> the scored numbers were not: what was written is what was thought at the time.
>
> The point generalises — every model here has used an iid driver, so every near-zero
> cancellation-side reading has been partly structural. Large readings are unaffected
> (the recycled model's +0.458 is not what a blind measurement produces), so it is the
> near-zero *passes* that were weaker evidence than they appeared. That is what makes a
> persistent driver the next mechanism rather than one candidate among several.

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
| **T** | `corr(depth, cancels)` in [−0.30, −0.02] | **+0.560** | **FAIL — opposite sign** |
| **U** | arrival brake below −0.05 **and** the stronger | −0.117 vs +0.560; margin **−0.443** | **FAIL** on the ordering |
| **V** | `corr(arrivals, cancels)` > +0.7 | **+0.432** | **FAIL** |
| **W** | drift < 1.3, spread sd > 0.1 | 1.020, 0.615 | pass |

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

#### Corrected 2026-07-31: the model first scored was not the model pre-registered

This block's mechanism says `recycle · arr_i(t−1)`. The implementation wrote
`lag(posted_bid, 1)`, which reaches **two** steps back — a bare field name is already row
0, so `lag(x, 1)` goes one row further. The scored model was a two-step recycler.

Corrected and re-scored, with **every verdict unchanged**: T +0.458 → **+0.560**, U's
margin −0.348 → **−0.443**, V +0.436 → **+0.432**, W 1.066/0.579 → **1.020/0.615**. The
table above carries the corrected figures; the originals are kept in DECISIONS.md.

T's failure got *stronger*, which is what the identity argument predicts — a shorter lag
shares more with what is currently resting — so the error left an unplanned second data
point behind it. The predictions were not re-run until they passed; they were re-run
because the implementation did not match what was pre-registered, and they failed again.

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

---

## Persistent driver with activity-dependent damping — predictions fixed before `cfg/lob_persistent.yaml` exists

**Fixed 2026-07-31, committed before the config is written.** This block also closes out
the re-test that [DECISIONS.md](DECISIONS.md) has been carrying since 2026-07-29 — "re-testing
in a lighter regime needs a fresh pre-registration" — by making that regime a stated
precondition rather than an assumption. See "Validity precondition" below.

### The question this has to answer

Four mechanisms have now been eliminated: prices, attrition removal, common-factor churn,
and recycled churn. The best surviving model (`cfg/lob_arrivals.yaml`) reads
`corr(depth, arrivals)` −0.116, `corr(depth, cancels)` −0.002, `corr(arrivals, cancels)`
+0.897, against a real signature where **both** flows sit mildly negative against depth,
arrivals the stronger, with co-movement +0.94 to +0.98.

So: **can one latent driver put both flows mildly negative against depth, in the right
order, without losing the co-movement?**

### Why every previous model read ≈0 on the cancellation side — a structural reason, not a missing term

This is the insight the block turns on, and it was not available before today.

Depth is a slow accumulator. The activity driver is drawn **iid per step**. A white-noise
driver therefore cannot move depth within the step it acts on, so the contemporaneous
correlation between depth and anything driven by activity is ≈0 **whatever the sign of the
underlying mechanism**. The previous models were not merely missing a depth-cancellation
term — the measurement could not have shown one.

Persistence is what makes the depth response observable at all: a sustained activity regime
gives depth many steps to move in one direction.

### The homogeneity trap, stated because it rules out the obvious design

The natural first design is "one driver scaling every flow". That **cannot work**, and the
reason is worth recording so it is not attempted later.

If arrivals, cancellations and marketable flow all scale with `act`, the system is
homogeneous in `act`: it cancels out of the equilibrium condition entirely. Setting
`limit_rate·act/(1 + q/s) = churn_rate·act` gives `q* = s·(limit_rate/churn_rate − 1)`,
with **no activity term**. Activity would rescale time — changing how fast depth relaxes —
without changing where it relaxes to. That is a variance effect, not a mean effect, and
`corr(depth, activity)` stays ≈0.

**Something has to break the homogeneity.** This block breaks it on the arrival damping.

### The mechanism, stated before measuring

Two changes to `cfg/lob_arrivals.yaml`, and nothing else:

1. **The driver becomes persistent.** `act(t) = φ·act(t−1) + (1−φ)·innovation`, with φ =
   **0.8**, chosen a priori to give a correlation time of ~5 steps — comparable to the
   6–23 step depth relaxation the stability claims measure, so the driver is slow enough
   for depth to follow and fast enough to vary within the window. The innovation's shape
   and rate are set so the **stationary marginal mean and variance match the incumbent's**
   (mean 4, variance 8), which requires scaling the innovation variance by
   (1+φ)/(1−φ) = 9. So the driver's marginal distribution is held FIXED and only its
   autocorrelation changes — any effect is attributable to persistence rather than to a
   busier or burstier market.

2. **The arrival damping scale becomes activity-dependent:** `s_eff = arrival_scale ·
   act_ref / act`, where `act_ref` is the stationary mean, so at average activity it
   reduces exactly to the incumbent. Economically: a maker is less willing to sit in a
   long queue when the market is moving. This makes `q* ∝ 1/act` and is what breaks the
   homogeneity above.

**Marketable flow is deliberately left unscaled**, which is the pre-existing
inconsistency — quoting scales with activity, trading does not. It is left alone because
changing it would be a third simultaneous change, and because it produces a **competing
effect** that makes X genuinely uncertain: see below.

### The competing effect, declared in advance

Marketable consumption is constant while quoting scales with activity. During a **low**
activity run the restoring force weakens while consumption continues, dragging depth down
— which pushes `corr(depth, activity)` **positive**, opposing change 2.

**Which effect dominates is not known, and I am not confident of the sign of X.** Writing
both down now is the point: whichever way it comes out, the reading was fixed in advance,
and neither direction can be presented afterwards as the one expected.

### Validity precondition — the fault from the churn block, made mechanical

Prediction B's failure was recorded as **inconclusive about the mechanism** because the
stated parameter criterion did not pin the regime: churn was a large fraction of the
resting book each step, so the `min(q, ...)` clip bound often and mechanically tied
cancellations to depth. That fault is not repeated by assertion here — it is measured.

**The clip-binding rate must be reported, and must be below 5% of level-steps.** If it
exceeds 5%, Y and Z are recorded as **inconclusive** — not as passes and not as failures —
and the model is separately recorded as unable to reach the regime the mechanism describes,
which is itself a finding about its usable range rather than a free pass.

### Parameters

Chosen a priori and stated now: **φ = 0.8**, the innovation moment-matching, and
`act_ref` = the stationary mean. `churn_rate` may be re-set **once** on mean depth alone,
as in the previous two blocks, with the sweep recorded and no correlation computed while
choosing. Nothing is adjusted against X, Y, Z, AA or AB.

### Predictions

Labelled X–AB, continuing from W. (N–S remain skipped, per the note at the top of this
file.)

**X — depth responds to the driver at all, and negatively.** `corr(depth, activity)` <
**−0.05**.

The intermediate that everything else runs through, and reported as an observation on
every claim below so a failure can be localised. Genuinely uncertain per the competing
effect above. If X fails positive, change 2 was outweighed by the constant-consumption
drag; if X lands ≈0, persistence did not carry the response and the homogeneity argument
was incomplete.

**Y — both flows land mildly negative against depth.** `corr(depth, cancels)` in
**[−0.30, −0.01]** AND `corr(depth, arrivals)` in **[−0.40, −0.05]**.

The joint landing is the test. Either alone is easy; the six real segments occupy both
bands simultaneously and no model here has.

**Z — the ordering matches.** `|corr(depth, arrivals)| > |corr(depth, cancels)|`.

Arrivals carry an extra negative path — they are damped by depth directly, while
cancellation reaches depth only through the driver — so this is *expected* rather than
uncertain, and is declared as the weaker of the predictions. It holds on all six real
segments, which is why it is scored at all.

**AA — the co-movement survives.** `corr(arrivals, cancels)` > **+0.85**.

The cost check that V failed. The floor is set **below** the incumbent's +0.897 on
purpose: this tests that the mechanism did not break the co-movement, not that it beat the
incumbent by a nose, and a nose-width improvement is not what is being examined. Both
flows stay contemporaneously on one driver here, so unlike V there is no lag to pay for.

**AB — the book survives.** Depth 2nd-half / 1st-half ratio **< 1.3**, and spread standard
deviation **> 0.1** ticks.

The same survival check every mechanism has had to pass. A driver-dependent equilibrium
means depth is now chasing a moving target, so conservation is less obvious than it looks.

### What each outcome means

| X | Y | Z | AA | AB | reading |
|---|---|---|---|---|---|
| pass | **pass** | pass | pass | pass | The first model in this project to reproduce the paired depth signature *and* the co-movement together. It would make the correlation structure jointly reproducible for the first time, and calibration against real data becomes worth attempting — subject to the standing confound below. |
| pass | **fail** | — | — | — | Depth responds to the driver, but not by the right amount in both columns. Informative and continuable: the response exists and its strength is a parameter question, which a fresh pre-registration could sweep. |
| **fail (positive)** | — | — | — | — | The constant-consumption drag dominates activity-dependent damping. That points at the *marketable* flow rather than the quoting flows as the thing to change next, which no evidence has previously pointed to. |
| **fail (≈0)** | — | — | — | — | Persistence did not make the depth response observable, so the structural argument above is wrong or incomplete. That would be the most surprising outcome and would need diagnosing before any further mechanism is tried. |
| pass | pass | pass | **fail** | — | A third mechanism trading the co-movement away. Recorded as a failure, not a partial success. |
| pass | pass | pass | pass | **fail** | Correlations match and the book does not survive — the axis that killed the attrition-free variant. |
| — | *inconclusive* | *inconclusive* | — | — | Clip bound on >5% of level-steps. The regime was not reached; Y and Z carry no verdict, and that is recorded as a limit on the mechanism's usable range. |

### The standing confound, unchanged

The target bands come from Binance segments whose arrival and cancellation counts are both
inferred from net depth changes rather than observed as messages. That confound is narrower
than it was first stated — see the correction dated 2026-07-31 above — but it is not
eliminated, and it means a full pass would establish that *a pure-config mechanism
reproduces the measured signature*, not that the signature is a property of order flow.
Message-level data remains the only way to settle that, and this project cannot obtain it
from the feed it is permitted to use.

### Scored, 2026-07-31

**Validity precondition: the clip bound on 4.21% of level-steps**, clearing the
pre-registered 5%, so Y and Z carry verdicts rather than being recorded inconclusive.
Not by much, though — a thinner book would put this test's validity in question, and
that margin is part of the result.

| | prediction | measured | |
|---|---|---|---|
| **X** | `corr(depth, activity)` < −0.05 | **−0.307** | pass |
| **Y** | cancels ∈ [−0.30,−0.01] **and** arrivals ∈ [−0.40,−0.05] | −0.286 ✓ / **−0.417 ✗** | **FAIL** by 0.017 |
| **Z** | arrivals the stronger brake | 0.417 vs 0.286 | pass |
| **AA** | `corr(arrivals, cancels)` > +0.85 | **+0.822** | **FAIL** |
| **AB** | drift < 1.3, spread sd > 0.1 | 1.164, 0.530 | pass |

`churn_rate` moved once, on mean depth alone: 1.15 → **1.05**, because the
activity-dependent damping thinned the book to 188.9 against the 227.8–235.9 the previous
two models produced. Sweep, no correlation computed while choosing: 0.85 → 340.3,
0.90 → 299.1, 0.95 → 280.5, 1.00 → 259.4, 1.05 → **235.4**. `persistence` stayed at its
pre-registered 0.8 and the innovation moments were not touched.

#### The pre-registered reading applies, and it is the continuable one

Y's failure lands on the row this document already contains: *"Depth responds to the
driver, but not by the right amount in both columns. Informative and continuable: the
response exists and its strength is a parameter question, which a fresh pre-registration
could sweep."* That is exactly the situation — unlike the previous block, where T came out
in a direction the table did not contain.

AA's failure has its own row (*"a third mechanism trading the co-movement away"*). The
table has no row for Y and AA failing **together**, which is recorded as a gap in it; both
individual readings apply and they do not conflict.

#### This is the closest any model in this project has come

| | this model | Binance range |
|---|---|---|
| `corr(depth, cancels)` | −0.286 | −0.015 … −0.246 |
| `corr(depth, arrivals)` | −0.417 | −0.121 … −0.339 |
| `corr(arrivals, cancels)` | +0.822 | +0.940 … +0.980 |

**Both flows are negative against depth, with arrivals the stronger — the first time any
model here has produced the real ordering.** Every previous one put the whole brake on a
single flow, or inverted the ordering. The failures are of magnitude, not direction.

#### Both failures plausibly share one cause, and it is a parameter

The damping's activity dependence is at full strength: `s_eff = arrival_scale · act_ref/act`,
so `q* ∝ 1/act`. That overshoots the depth correlations. It also costs co-movement, because
arrivals now **saturate** in activity — `arr ∝ act/(1 + q·act/(s·act_ref))`, whose
denominator grows with `act` — while cancellation stays proportional to it, so the two
flows track each other less closely than when both were proportional.

A weaker dependence, e.g. `s_eff = arrival_scale · (act_ref/act)^γ` with γ < 1, would
reduce both effects at once. **That is a continuation and it needs its own
pre-registration.** Sweeping γ now, having seen which way X, Y and AA missed, is precisely
the move this file exists to prevent. The saturation account is also an explanation
consistent with the numbers rather than an independently tested claim.

#### What this does not establish

Nothing here is a calibration; no parameter was fitted to any market number, and
`persistence`, the damping form and the innovation moments were all fixed in advance. The
target bands carry the standing confound — both real flows are inferred from net depth
changes — so a model landing inside them would establish that a pure-config mechanism
reproduces the measured signature, not that the signature is a property of order flow.

---

## Damping strength — the project's FIRST CALIBRATION, fixed before the sweep is run

**Fixed 2026-07-31, committed before `cfg/lob_damping.yaml` exists and before any value of
γ other than 1 has been run.**

### Read this first: this block breaks a property every previous block had

Every claim in this repo says, in one form or another, *nothing here is fitted to market
data*. **This block fits a parameter to a market number.** That is a deliberate change of
activity — Phase 2 is calibration and this is the first of it — but it means the resulting
claims cannot be worded like the previous ones, and it means the usual danger is now the
main danger: γ was pre-registered as a *sweep* precisely because I have already seen that
γ = 1 misses, and a sweep with a free hand is fitting dressed as discovery.

Three structural commitments make it a test rather than a fit:

1. **One parameter, one target, stated now.** γ is fitted to `corr(depth, arrivals)` and to
   nothing else.
2. **Two held-out targets become predictions.** `corr(depth, cancels)` and
   `corr(arrivals, cancels)` are *not* fitted to and are scored at whatever γ the rule
   below selects. One parameter cannot chase three numbers; the other two are free to miss.
3. **The grid and the selection rule are mechanical and fixed here**, so there is no room
   to refine toward an answer.

### My contamination, declared

I have run γ = 1 and seen its scores: `corr(depth, arrivals)` −0.417,
`corr(depth, cancels)` −0.286, `corr(arrivals, cancels)` +0.822, clip 4.21%, drift 1.164,
spread sd 0.530. That point is in the sweep and is not blind. Every other grid point is.

### The mechanism, stated before measuring

Exactly one thing changes from `cfg/lob_persistent.yaml`:

	s_eff = arrival_scale · (act_ref / act)^γ

γ = 1 is the persistent model already scored. **γ = 0 is not `cfg/lob_arrivals.yaml`** —
it is that file's activity-independent *damping* carrying a *persistent* driver, which no
model has run, so its value is unknown and that endpoint is blind like the rest. Nothing
else moves.

### The grid, the fit target and the selection rule — all mechanical

- **Grid:** γ ∈ **{0.0, 0.2, 0.4, 0.5, 0.6, 0.8, 1.0}**. Seven points, fixed. No
  interpolation, no refinement, no adding points afterwards.
- **Fit target:** `corr(depth, arrivals)` = **−0.2128**, the mean across the five
  concurrently-recorded Binance segments (−0.267, −0.339, −0.121, −0.131, −0.206). Computed
  from data already in hand and stated here so it cannot drift.
- **Rule:** take the grid γ whose `corr(depth, arrivals)` is **closest in absolute
  distance** to −0.2128. Ties go to the **larger** γ.
- **Per-point control:** at each grid point `churn_rate` is re-set on **mean depth alone**
  to land in **227.8–235.9**, the range the previous two models produced, so depth level is
  held roughly fixed across the sweep and γ is the only thing varying in effect. No
  correlation is computed while choosing, exactly as in the previous three blocks.

**If no grid point brings `corr(depth, arrivals)` within 0.05 of the target**, the fit
fails outright: AE, AF and AG are recorded as **not reached**, and the finding is that this
damping family cannot produce the observed arrival-side strength at any strength setting.

### Validity precondition, unchanged

The `min()` clip must bind on **< 5%** of level-steps at the selected γ, reported. Above
that, AE is recorded **inconclusive** rather than scored, for the reason the churn block
established.

### Predictions

**AC — the depth response is monotone in γ.** `|corr(depth, arrivals)|` increases across
the grid as γ increases.

**AD — the co-movement is monotone in γ.** `corr(arrivals, cancels)` decreases across the
grid as γ increases.

AC and AD are *expected* — that is the mechanism's whole logic — and are declared as the
weaker predictions. They are scored because **they are preconditions for the fit being
well-posed at all**: if the response is not monotone, "choose γ to hit a target" is
ill-defined and the selected point is arbitrary. They are also where a surprise would
appear if the damping is doing something other than what is assumed.

**AE — the held-out depth target lands.** At the selected γ, `corr(depth, cancels)` lies in
**[−0.30, −0.01]**, AND arrivals remain the stronger brake
(`|corr(depth, arrivals)| > |corr(depth, cancels)|`).

**This is the test.** γ is fitted to the arrival side; the cancellation side is free to
land anywhere. The five segments put it at −0.127 on average with the ordering holding on
all five, so the band and the ordering are both real constraints rather than formalities.
I do not know whether one parameter can satisfy both sides at once.

**AF — the held-out co-movement target clears.** At the selected γ,
`corr(arrivals, cancels)` > **+0.85**.

The second held-out number, and the one the persistent model missed at +0.822. Lower γ
should raise it, so this is where the sweep is *expected* to help — but the selected γ is
chosen by the arrival side, not by this, so it may land anywhere.

**AG — the book survives at the selected γ.** Drift < **1.3**, spread sd > **0.1**.

### What each outcome means

| AC/AD | AE | AF | AG | reading |
|---|---|---|---|---|
| pass | **pass** | pass | pass | One parameter, fitted to one market number, reproduces the other two. **That is a calibration that predicts**, and it is the first result in this project that would justify attempting Phase 2 properly. Subject to the standing confound, which does not go away. |
| pass | **pass** | **fail** | — | The depth structure is reconcilable but the co-movement is not — the saturation cost is not escapable by weakening the damping, and something other than the damping has to supply the co-movement. |
| pass | **fail** | — | — | One parameter cannot satisfy both sides of the depth signature. The arrival and cancellation correlations are not two views of one mechanism, and the model needs a second, separately-motivated term. |
| **fail** | — | — | — | The response is not monotone in γ, so the fit is ill-posed and the selected point means nothing. AE–AG would be recorded as not reached, and the damping does not behave as its algebra suggests — which would need diagnosing before anything else. |
| — | *inconclusive* | — | — | Clip bound above 5% at the selected γ. Same treatment as the previous block. |
| *fit fails* | — | — | — | No grid point within 0.05 of −0.2128. The family cannot produce the observed arrival-side strength at any setting, which is a cleaner elimination than any of the four so far. |

### What a pass would and would not establish

It **would** establish that one pure-config parameter, fitted to one market number,
predicts two others it was not fitted to. That is a real and unusual thing for this project
to be able to say, and it is the first step of Phase 2 rather than the end of it.

It would **not** establish that the mechanism is right. The three target numbers come from
Binance segments whose arrival and cancellation counts are both inferred from net depth
changes — narrower than first stated (see the 2026-07-31 correction above) but not
eliminated — so a model that reproduces them reproduces *the measured signature*, which may
not be a property of order flow. It would also be a fit on five segments in one 8-minute
window on one venue, with no out-of-sample window, no second venue and no asset class other
than crypto spot. **The natural next step after a pass is a fresh recording and a
prediction against it, not a stronger claim about this one.**

### Scored, 2026-07-31

**The fit succeeded and the rule selected γ = 0.6**, at a distance of **0.0207** from the
−0.2128 target — well inside the 0.05 at which this document declared the fit would have
failed outright. **Validity precondition met:** the clip bound on **3.99%** of level-steps
at the selected point.

The full sweep, with `churn_rate` set per point on mean depth alone:

| γ | churn | `corr(d,arr)` | `corr(d,can)` | `corr(arr,can)` | drift | sd | clip% | depth |
|---|---|---|---|---|---|---|---|---|
| 0.0 | 1.128 | **+0.277** | +0.420 | +0.885 | 1.006 | 0.724 | 4.12 | 231.2 |
| 0.2 | 1.096 | +0.064 | +0.200 | +0.883 | 1.035 | 0.523 | 4.06 | 234.5 |
| 0.4 | 1.091 | −0.067 | +0.036 | +0.878 | 1.055 | 0.491 | 3.95 | 229.8 |
| 0.5 | 1.075 | −0.152 | −0.037 | +0.873 | 0.951 | 0.578 | 4.28 | 229.5 |
| **0.6** | **1.075** | **−0.234** | **−0.138** | **+0.876** | **0.976** | **0.515** | **3.99** | **223.0** |
| 0.8 | 1.069 | −0.311 | −0.159 | +0.839 | 0.963 | 0.478 | 4.08 | 221.7 |
| 1.0 | 1.061 | −0.394 | −0.260 | +0.823 | 1.080 | 0.529 | 4.73 | 228.0 |

| | prediction | measured | |
|---|---|---|---|
| **AC** | `\|corr(depth, arrivals)\|` monotone increasing in γ | crosses zero; not monotone | **FAIL** |
| **AD** | `corr(arrivals, cancels)` monotone decreasing in γ | falls +0.885→+0.823, inverts once by 0.003 | **FAIL** |
| **AE** | held-out `corr(depth, cancels)` in band, arrivals stronger | **−0.138**, margin +0.096 | **pass** |
| **AF** | held-out `corr(arrivals, cancels)` > +0.85 | **+0.876** | **pass** |
| **AG** | drift < 1.3, spread sd > 0.1 | 0.976, 0.515 | **pass** |

#### The result: one parameter, fitted to one number, predicted two others

| | fitted? | model | Binance |
|---|---|---|---|
| `corr(depth, arrivals)` | **FITTED** | −0.234 | −0.213 (five-segment mean) |
| `corr(depth, cancels)` | held out | **−0.138** | −0.127 (five-segment mean) |
| `corr(arrivals, cancels)` | held out | **+0.876** | +0.940 … +0.980 |

The cancellation side was free to land anywhere in [−1, +1] and landed **0.011** from the
market mean, with the ordering intact. This is the first predictive statement this project
has produced.

#### AC failed, and the fault is the prediction's

`corr(depth, arrivals)` **crosses zero inside the grid** — +0.277 at γ=0, −0.394 at γ=1 —
so its absolute value dips and rises. AC was written with the absolute value and is false.
The **signed** response is strictly monotone across all seven points, which is what AC
existed to establish and what makes the fit well-posed; restating it that way is a
post-hoc reformulation and is claimed as measured structure rather than as a prediction
that passed.

The zero crossing **confirms** something pre-registered earlier: the persistent-driver
block declared, before running, that constant marketable consumption competes with the
damping and pushes this correlation positive. At γ=0 that effect is all there is, and it
wins. The competing effect was real and was simply outweighed at γ=1.

#### AD failed narrowly

Co-movement falls +0.885 → +0.823 as predicted but inverts once, by **0.003**, between
γ=0.5 and γ=0.6. That is within run-to-run noise on a single seed. Saying so does not turn
a failed prediction into a passed one, and no repeat-seed run was made — it would have to
be pre-registered, since it would be run knowing which pair inverted.

#### Where the control missed its own band

`churn_rate` was set per point on mean depth into a stated 227.8–235.9. Two points missed:
γ=0.6 at **223.0** (the selected one) and γ=0.8 at 221.7. Depth spans 221.7–234.5 across
the sweep — held roughly but not exactly fixed, and 2.1% below the floor at the point that
matters. Declared rather than smoothed.

#### What this establishes, and the four things it does not

It establishes that **one pure-config parameter, fitted to one market number by a rule
fixed in advance, predicts a second to within 0.011 and clears a pre-registered floor on a
third.**

It does not establish that the mechanism is right. It does not escape the standing
inference confound, so what is reproduced is the *measured* signature. It is five segments
in one 8-minute window on one venue, one seed, with **no out-of-sample test of any kind**.
And the co-movement, at +0.876 against a market +0.94–+0.98, is narrowed rather than
matched — the one gap that has survived every model in this project.

**The next step fixed before this result existed still stands: a fresh recording and a
prediction against it, not a stronger claim about this one.**

---

## Out-of-sample: does the calibrated model survive a fresh window? Fixed before recording

**Fixed 2026-08-01 08:45 UTC, before any new data exists.** This is the step the previous
block committed to *before its own result was known*: "a fresh recording and a prediction
against it, not a stronger claim about this one."

### The model is frozen

`cfg/lob_damping.yaml` as shipped: `damping_gamma` 0.6, `churn_rate` 1.075, every other
parameter inherited. **Nothing may be refitted for this test, ever.** If the fresh window
disagrees, the model was wrong about the fresh window — that is the whole point, and
re-tuning γ afterwards would destroy the only out-of-sample evidence this project has.

### Protocol

The same five symbols — `BTCUSDT`, `ETHUSDT`, `SOLUSDT`, `XRPUSDT`, `DOGEUSDT` — recorded
**concurrently**, 8 minutes, one-second buckets, `feed.DefaultLotSize`, identical in every
respect to the 2026-07-30 capture **except the wall-clock window**. Time is the only thing
that varies, so nothing else can explain a difference.

The original capture was Thursday 2026-07-30, ~07:00 UTC. This one is **Saturday
2026-08-01**, which is a genuinely different regime for crypto — weekend flow is thinner
and less intermediated — and that is a reason to expect the market itself to have moved.

**Data quality gates, fixed now:** any segment with a sequence gap or with suspect rows is
excluded and the exclusion reported. Fewer than five clean segments is reported rather than
worked around.

### The test is only meaningful if the market moved — pre-registered as a gate

A fresh window that reads the same as the old one tests nothing: the model would "predict"
numbers that never changed. So this is fixed **before** seeing them:

> If the fresh five-symbol mean `corr(depth, arrivals)` is within **0.03** of the old
> −0.2128, the test is recorded as **WEAK BY CONSTRUCTION** — the window did not differ
> enough to test anything — regardless of whether AH–AK pass.

The market's own drift, per symbol and in the mean, is **reported alongside the model's
error in every case**, so a reader can see whether the model tracked a moving target or
merely sat where the market stayed.

### Where the tolerances come from, and what they are not

The only temporal comparison this project has is BTCUSDT across two windows:
`corr(depth, arrivals)` −0.212 → −0.267 (0.055 apart) and `corr(depth, cancels)` −0.123 →
−0.220 (0.097 apart). **The market's own single-symbol drift between windows is therefore
0.05–0.10.**

Tolerances below are set at **0.12** — above that observed drift, so they are calibrated to
market instability rather than to model precision. A tighter bound would be predicting the
model tracks the market more closely than the market tracks itself. This is stated so the
bound is not later read as a precision claim: it is not.

### Predictions

**AH — the fitted quantity survives the window change.** The fresh five-symbol mean
`corr(depth, arrivals)` is within **0.12** of the model's **−0.234**.

γ was fitted to the *old* mean. This asks whether that target was a property of the market
or of one Thursday morning. **If AH fails, the calibration was fitted to a transient** and
the previous block's result is much weaker than it reads — which is the single most
important thing this recording can tell us.

**AI — the held-out quantity survives out of sample.** The fresh five-symbol mean
`corr(depth, cancels)` is within **0.12** of the model's **−0.138**.

**This is the test.** It was held out from the fit, and it is now held out in *time* as
well. A model that lands here has predicted a number it was never shown, in a window it
was never fitted to.

**AJ — the co-movement gap stays bounded.** The model's **+0.876** is within **0.15** of
the fresh five-symbol mean.

Stated as a bound rather than a match because the gap is already known: the market reads
+0.94–+0.98 and the model +0.876. This asks whether the gap stays roughly where it is or
widens, not whether it closes. Predicting it closes would be predicting a known failure
away.

**AK — the ordering replicates.** `|corr(depth, arrivals)| > |corr(depth, cancels)|` on
**every** fresh segment.

It has held on all six segments recorded so far. A single counterexample matters more than
the mean here, so this is scored per segment rather than on the average.

### What each outcome means

| gate | AH | AI | reading |
|---|---|---|---|
| moved | pass | **pass** | The model predicts, out of sample, a quantity it was never fitted to, in a window it was never fitted to. That is the strongest statement this project could make on the data it is permitted to use, and it is what would justify Phase 3 in earnest. |
| moved | pass | **fail** | The market signature is stable but the model's held-out agreement was in-sample luck. The calibration reproduces what it was fitted to and nothing more — which is the ordinary fate of one-parameter fits and would be an honest place to stop. |
| moved | **fail** | — | The fit target was a property of one window. γ was calibrated to a transient, and the previous block's result must be restated as such. The most informative failure available here. |
| **weak** | — | — | The window did not differ enough. Nothing is established either way, no claim is published from it, and the honest response is a third recording at a genuinely different time rather than a re-reading of this one. |

### What even a full pass would not establish

One venue, crypto spot, two 8-minute windows two days apart, five USDT majors, one seed on
the model side. It would not touch the standing inference confound — both real flows are
still inferred from net depth changes — so what would be reproduced out of sample is still
the *measured* signature. And the co-movement would still be narrowed rather than matched.

A pass would justify **more recordings across more windows**, not a claim about markets.

### Scored, 2026-08-01

Capture: Saturday 2026-08-01, 08:51–08:59 UTC, five symbols concurrently, **480 clean rows
each, 0 suspect, 0 gaps**. No segment excluded.

**The gate did NOT fire.** The market moved **+0.145** on the mean arrival correlation,
against the 0.03 threshold at which this test would have been recorded weak by
construction. So the window genuinely differed and the verdicts below are real.

| | old (Thu 07-30) | fresh (Sat 08-01) | market moved |
|---|---|---|---|
| mean `corr(depth, arrivals)` | −0.213 | **−0.068** | **+0.145** |
| mean `corr(depth, cancels)` | −0.127 | **+0.035** | **+0.162** |
| mean `corr(arrivals, cancels)` | +0.955 | +0.904 | −0.051 |

Per symbol, the arrival correlation moved by +0.325 (BTCUSDT), +0.359 (ETHUSDT), −0.037
(SOLUSDT), −0.058 (XRPUSDT), +0.138 (DOGEUSDT). **BTCUSDT changed sign**, −0.267 → +0.058.

| | prediction | measured | |
|---|---|---|---|
| **AH** | fitted correlation within 0.12 of the model's −0.234 | **0.166** away | **FAIL** |
| **AI** | held-out correlation within 0.12 of the model's −0.138 | **0.173** away | **FAIL** |
| **AJ** | co-movement within 0.15 of the model's +0.876 | 0.028 away | pass |
| **AK** | ordering on every segment | 4 of 5; fails on ETHUSDT | **FAIL** |

#### The pre-registered reading, and it is the one this document called most informative

AH's failure lands on the row fixed in advance: *"The fit target was a property of one
window. γ was calibrated to a transient, and the previous block's result must be restated
as such. The most informative failure available here."*

That is what happened, and the restatement has been made — `pkg/damping`'s AE claim now
carries **WITHDRAWN AS PREDICTIVE**, and the package doc says so before anything else. The
in-sample numbers stand as measured. The reading that they were evidence of *prediction*
does not.

#### What actually moved was the market, not the model

The model cannot track this and nothing in it could: no parameter varies by window. A model
that did track it would need a regime changing over hours, which **none of the five
mechanisms tested in this project has**. That is a sharper statement of what is missing
than any of the four eliminations produced, and it is only visible because a second window
was recorded.

#### AJ passed, and mostly for the wrong reason

The gap closed to 0.028 — but because the market's co-movement **fell** from a +0.940 floor
to +0.852 (XRPUSDT) and +0.867 (DOGEUSDT), not because the model rose. Note the
consequence: **the cross-segment replication's +0.9 co-movement floor would not hold on
this window.** That is recorded as context, not as a rescoring of a claim that was
pre-registered against the segments it was measured on.

#### AK failed, and carries less than the other two

Arrivals were the stronger brake on all six earlier segments and on four of five here,
failing on ETHUSDT (+0.020 vs +0.040). With both correlations within 0.05 of zero in this
window there is little left to order, so this may be noise around zero rather than a
reversal — which does not rescue a prediction written without a magnitude qualifier, but
does mean it carries less than AH and AI.

#### What survives, and it is the thing that always mattered most

Fresh `corr(depth, cancels)` per symbol: +0.057, +0.040, +0.050, −0.009, +0.037 — **every
one below +0.2.** The coupling the model's parameterisation requires, `cancel_rate × depth`,
is still absent on every symbol in a second independent window. **Phase 2's central
conclusion is now replicated across two windows as well as five instruments**, and it is
the one result in this project that has never weakened.

#### What this establishes

That the depth-correlation signature this project has been chasing is **not stable over
two days**, and that a one-parameter calibration to it does not transfer. Two windows on
one venue do not give a distribution for that instability — many more recordings would —
and nothing here identifies the cause, though Saturday flow being thinner and less
intermediated than Thursday's is the obvious candidate and is untested.

---

## How unstable are these correlations, really? Fixed before the repeat windows are recorded

**Fixed 2026-08-01, before any window after 08:59 UTC exists.**

### The question, and why it comes before weekday-vs-weekend

The out-of-sample test showed the mean `corr(depth, arrivals)` moving 0.145 and
`corr(depth, cancels)` 0.162 between Thursday 07:00 and Saturday 08:51. The tempting
reading is a weekday/weekend regime change. **That reading is unavailable until the
ordinary variability of these quantities is known**, and this project has never measured
it: every correlation in `CLAIMS.md` and `DECISIONS.md` is quoted to three decimals with
no noise floor beside it.

So the primary deliverable of this block is not a prediction at all. It is **a measured
noise floor for these correlations at the eight-minute-window scale**, against which every
past and future number in this project should be read.

### What this design can and cannot do — the asymmetry, stated first

Repeat windows within one morning measure **short-timescale** variability. That gives:

- **A refutation.** If windows minutes apart vary as much as Thursday and Saturday did,
  then the 0.145 shift needs no regime explanation, and there is nothing for a
  weekday/weekend study to find.
- **Not a confirmation.** Tight short-timescale windows do *not* establish that the
  48-hour gap is a regime change, because variance may simply grow with separation.

This block is therefore a falsification test. It is worth running first because it is the
cheap half, and because it can make the expensive half unnecessary.

### Protocol — CORRECTED 2026-08-02 07:46 UTC, before any new window exists

**The paragraph below originally said "six Saturday windows". That was wrong: I wrote it
believing it was still Saturday, and it is Sunday 2026-08-02.** The correction is made
here, before a single new row is recorded, and **no numeric bound moves** — AL's 0.145,
AM's 0.162 and AO's +0.2 are exactly as committed. Only the description of when the
windows will be taken changes, because it was a statement of fact and it was false.

**Five windows**, each 8 minutes, starting at 10-minute intervals, all five symbols
concurrently, identical protocol to both previous captures. They will be **Sunday
2026-08-02, from ~07:50 UTC**. With the Saturday 08:51 window already recorded that gives
**six weekend windows**: five within one Sunday morning, plus one from the previous
morning, ~23 hours earlier.

This is **better** for the question than the within-morning design it replaces, and the
gain should be stated rather than claimed later. The Thursday→Saturday gap is 48 hours;
five windows inside one morning measure minutes-scale variability and would have needed a
large extrapolation. This design measures both at once:

- the **within-morning range** over the five Sunday windows — the noise floor, reported
  as its own number;
- the **six-window range** including Saturday — a ~23-hour figure, much nearer the
  timescale of the gap being explained, and the quantity AL and AM are scored on exactly
  as committed.

The "windows minutes apart share market conditions, so this understates variability"
caveat therefore applies to the within-morning number and only weakly to the six-window
one.

Windows minutes apart share market conditions, so this is a **conservative** estimate of
variability — it will tend to *understate* it. Declared now so a tight result is not
over-read.

Any window with a sequence gap or suspect rows in any symbol is excluded whole, and the
exclusion reported. Fewer than five usable windows is reported rather than worked around.

### Predictions

**AL — the within-morning spread is smaller than the Thursday→Saturday gap.** The range
(max − min) of the five-symbol mean `corr(depth, arrivals)` across the six windows is
**< 0.145**.

**AM — the same for the cancellation side.** The range of the five-symbol mean
`corr(depth, cancels)` across the six windows is **< 0.162**.

AL and AM are the test. **If either fails, the out-of-sample failure is explained by
ordinary short-run variability and no regime story is needed** — and, much more
importantly, it would mean these correlations are not stable enough at this window size to
support *any* of the comparisons this project has built on them, including the
cross-segment replication. That is the outcome with the largest consequences and I do not
know which way it falls.

**AN — the weekend windows cluster with each other rather than with Thursday.** The mean
`corr(depth, cancels)` of **every** one of the six windows is closer to the Saturday 08:51
window's +0.035 than to Thursday's −0.127.

Scored per window, not on an average: one window landing nearer Thursday would show the
two days are not cleanly separated.

**AO — the central conclusion holds in every window on every symbol.** `corr(depth,
cancels)` < **+0.2** for all five symbols in all six windows — thirty measurements.

The coupling the model's parameterisation requires has been absent in every segment ever
recorded here. This is the largest test of it the project has run, and it is the one
result that has never weakened.

### What each outcome means, and what it decides next

| AL/AM | AN | reading and consequence |
|---|---|---|
| pass | pass | Short-run variability is materially smaller than the two-day gap, and the days separate. A weekday study becomes worth doing, **and its bounds must then be pre-registered afresh** — nothing here fixes them. |
| pass | **fail** | The windows are individually stable but do not group by day, so whatever moved is not a day-type effect. A weekday study would be chasing the wrong variable. |
| **fail** | — | **These correlations are not stable at the eight-minute scale.** No regime explanation is needed for the out-of-sample failure — and every cross-window and cross-segment comparison in this project, including the replication block's bounds, would need restating with a noise floor attached. No weekday study is warranted. This is the outcome that costs the most and is worth the most. |

### What is not being decided here

No model parameter is touched. Nothing is fitted. This block measures the *measurement*,
not the model — and its result applies to every correlation this project has published,
retrospectively.

### Scored, 2026-08-02

Five windows, Sunday 2026-08-02, starts 08:12–08:56 UTC, 480 rows each, 0 suspect, 0 gaps.
One file — `nf3_DOGEUSDT` — recorded 476 rows rather than 480, a four-second short capture
with no gaps and no suspect rows, so the pre-registered exclusion criterion does not apply
and window 3 is kept. Declared rather than passed over.

| window | mean `corr(d,arr)` | mean `corr(d,can)` | mean `corr(arr,can)` |
|---|---|---|---|
| Sat 08:51 | −0.068 | +0.035 | +0.904 |
| Sun w1 | −0.117 | −0.044 | +0.946 |
| Sun w2 | −0.123 | −0.058 | +0.978 |
| Sun w3 | −0.179 | −0.120 | +0.974 |
| Sun w4 | −0.099 | −0.008 | +0.949 |
| Sun w5 | −0.177 | −0.083 | +0.947 |

| | prediction | measured | |
|---|---|---|---|
| **AL** | six-window range of `corr(d,arr)` < 0.145 | **0.111** | pass |
| **AM** | six-window range of `corr(d,can)` < 0.162 | **0.155** | pass, by 0.008 |
| **AN** | every window nearer Saturday than Thursday | **3 of 6** | **FAIL** |
| **AO** | `corr(d,can)` < +0.2, all 5 symbols × 6 windows | worst **+0.083** | pass |

#### The deliverable: a measured noise floor

**Within one Sunday morning, with nothing changed but the clock**, the five-symbol means
span **0.079** (`corr(depth, arrivals)`), **0.112** (`corr(depth, cancels)`) and **0.032**
(`corr(arrivals, cancels)`). That is the number that should sit beside every correlation
this project has published, and until now none of them had one.

#### The verdicts pass; the reading does not follow the verdicts

AL and AM pass as written. **AM passes by 0.008** — its range, 0.155, is 95% of the entire
Thursday→Saturday gap it had to come in under, reproduced among *weekend* windows where no
day changed at all. Treating that as evidence of stability would be correct by the letter
of this document and wrong by its spirit.

And **AN fails 3 of 6** — and one of the three that passed is the Saturday reference
itself, so of five genuinely new windows, **two land nearer Thursday than nearer the other
weekend window recorded a day earlier.** The windows do not group by day.

The pre-registered row for pass/fail says: *"The windows are individually stable but do not
group by day, so whatever moved is not a day-type effect. A weekday study would be chasing
the wrong variable."* The second half stands. The first half — "individually stable" — is
not supported by margins this thin, and that phrase was written before the sizes were
known.

**So: no weekday study. The out-of-sample failure needs no regime explanation, because the
gap it turned on is barely larger than what these quantities do between windows minutes
apart.**

#### What this costs, retrospectively

Two claims elsewhere have been qualified rather than left standing:

- **The calibration's headline.** `pkg/damping` reported one parameter fitted to one
  market number putting a held-out number within **0.011** of the market. That quantity
  wanders **0.112** between windows ten minutes apart, so the agreement was inside the
  noise by an order of magnitude and was never the precision it reads as. This is a
  second, independent reason that headline was wrong, alongside the out-of-sample failure.
- **Prediction M.** Scored on a margin of **0.032**, against a co-movement noise floor
  measured here at **0.032**. The margin is the same size as the noise, so the verdict is
  not distinguishable from chance in either direction. It was already recorded as a pass
  whose reasoning is unsupported; it is now measurably so.

#### What survives, and is now the best-tested result here

**AO: thirty measurements, five symbols × six windows, worst case +0.083 against a +0.2
ceiling.** The coupling the model's parameterisation requires is absent everywhere it has
ever been looked for — seven windows counting Thursday, five instruments, two days. Its
margin to the bound (0.117) is larger than the wander (0.112), so unlike every magnitude
comparison in this project, **this one survives its own noise floor.**

#### What this does not establish

Six windows give a range, not a distribution — a standard error would need many more. The
within-morning figure understates variability, since windows ten minutes apart share
market conditions. All six are weekend mornings on one venue inside 24 hours. And it says
nothing about other window lengths: eight minutes is a choice this project made, and a
longer window would average more and wander less.

---

## Time-in-queue cancellation — predictions fixed before `cfg/lob_ages.yaml` exists

**Fixed 2026-08-02, before the config is written and before any age-structured model has
been run.**

### The question, and why it is the complement of the one that failed

Four mechanisms have been eliminated. The sharpest thing the failures produced was an
identity: **any cancellation rule keyed to RECENT arrivals inherits a positive depth
coupling**, because recent arrivals *are* current depth. That killed recycled churn at
+0.514 and it kills the whole family, whatever the lag or coefficient.

Time-in-queue with a **rising** hazard is the complement of that rule. It weights
cancellation toward the OLDEST resting volume, not the newest. A burst of arrivals then
raises depth while adding low-hazard volume, so cancellation does not rise with it; the
cohort cancels later, when depth has already moved on.

**The identity argument does not settle which way this comes out**, because the shared term
runs the other way — and that is what makes it worth running. Cancellation still scales
with resting volume summed over cohorts, so the coupling could easily stay strongly
positive and the age weighting simply not be enough.

### The mechanism, stated before measuring

Everything on the arrival side is **inherited unchanged** from `cfg/lob_damping.yaml` — the
AR(1) driver, the activity-dependent damping at γ = 0.6 — so exactly one thing changes and
the comparison is clean. Cancellation is replaced.

Each price level carries **8 age cohorts**. Per step:

	arrivals enter cohort 0
	cohort c cancels a fraction  h(c) = haz0 * (1 + 0.5 * c)     (rising hazard)
	marketable orders consume OLDEST-first (price-time priority)
	survivors age by one; the oldest cohort is absorbing

Eight cohorts to match the eight price levels — arbitrary, stated, not fitted. The hazard
SHAPE is fixed a priori at `0.5` per cohort, so hazard doubles by cohort 2; only its LEVEL
is adjustable, per the parameter rule below.

**Rising rather than falling, and the reason is economic**: an order that has sat unfilled
is increasingly likely to be stale, and the maker re-prices it. Falling hazard would encode
commitment instead — and would be the recycled-churn family again, keyed to recent
arrivals, which the identity already rules out.

### This does NOT need order identity, which was checked before writing this

Age cohorts give the aggregate behaviour without per-order tracking, including price-time
priority: consuming oldest-first is a prefix sum from the old end, the same reformulation
the price-level sweep uses. Verified by running a one-level cohort model before this block
was written.

**Order identity buys the queue-position stability output, not this mechanism** — that is
a separate block, and conflating the two would have made this one look like it needed an
engine release it does not.

> **Amended 2026-08-02, before the config was run and before any result existed.** This
> block originally said "`scan` is not used here". At full size the sweep's prefix sum is
> over 128 slots, and the `each`-of-`sum`-over-slices idiom is O(n²) — about 16k operations
> per step, which puts a 32-member ensemble into minutes and would make this package the
> critical path of the suite. `scan` is therefore used **as a prefix-sum primitive**, which
> is O(n).
>
> The substantive claim is unchanged and is the one that matters: **the mechanism does not
> NEED order identity**, verified by running a one-level cohort model without `scan` before
> this block was written. What `scan` buys here is speed, not capability. The amendment is
> recorded rather than made silently because "does not use the new primitive" was a cleaner
> statement than the truth, and the truth is that it uses it for something other than what
> it was released for.

### Validity precondition — the mechanism must actually be operating

An age-structured model whose volume all sits in one cohort is not testing anything: with
everything in cohort 0 it is memoryless churn, and with everything in the oldest it is
constant-hazard attrition wearing a disguise.

> **The fraction of resting volume in the oldest cohort must lie in [5%, 60%]**, and is
> reported whichever way it falls. Outside that band, AP and AQ are recorded
> **inconclusive** rather than scored, and the model is separately recorded as unable to
> reach the regime the mechanism describes.

This is the churn block's fault made mechanical, as the clip-binding precondition was.

### Parameters

`haz0` may be re-set **once**, on **mean depth alone**, into the 227.8–235.9 band the
previous models produced — the same single adjustment every block here has had, with the
sweep recorded and no correlation computed while choosing. The hazard shape, the cohort
count and everything on the arrival side are fixed and may not move.

### Predictions

Labelled AP–AS. Bands are **identical to prediction Y's** so the two mechanisms are
directly comparable.

**AP — the paired depth signature.** `corr(depth, cancels)` in **[−0.30, −0.01]** AND
`corr(depth, arrivals)` in **[−0.40, −0.05]**.

The joint landing, and the test. The arrival side is inherited so its half is near-forced;
the cancellation side is the open question and could plausibly come out anywhere from
strongly positive (age weighting too weak to matter) to inside the band.

**AQ — the ordering.** `|corr(depth, arrivals)| > |corr(depth, cancels)|`, as on all six
Binance segments.

**AR — the co-movement.** `corr(arrivals, cancels)` > **+0.85**.

**The cost check, and the one I expect to be hardest.** Cancellation now depends on the
AGE DISTRIBUTION rather than on contemporaneous activity, so it may decouple from arrivals
and lose the co-movement — which is the signature these models reproduce best and which
the damping model already fails at +0.816. If AR fails, this mechanism buys the depth
structure by selling the co-movement, which is the same bad trade recycled churn made.

**AS — the book survives.** Depth drift **< 1.3** and spread standard deviation **> 0.1**.

### What each outcome means

| AP | AR | reading |
|---|---|---|
| **pass** | **pass** | The first mechanism to reproduce the paired depth signature AND clear the co-movement floor. That would be the strongest model this project has produced, and the point at which a fresh out-of-sample recording is worth making against it. |
| **pass** | **fail** | The depth structure is reachable through ageing but costs the co-movement. Read together with the damping model's identical failure, that would say the two signatures are in tension in this vocabulary — a structural finding, and a more useful one than another elimination. |
| **fail (positive)** | — | Age weighting is not enough: cancellation summed over cohorts still tracks depth. The identity generalises further than recent arrivals, to *any* rule proportional to resting volume however weighted, which would be a sharper statement than the current one. |
| **fail (overshoot)** | — | The response exists and is too strong at this hazard shape. A parameter question, needing a fresh pre-registration rather than a sweep here. |
| *inconclusive* | — | Oldest-cohort share outside [5%, 60%]. The regime was not reached; that is a limit on the mechanism's usable range, not a free pass. |

### What even a full pass would not establish

Every number is a 32-member ensemble mean at 8000 steps and model-internal. The target
bands come from Binance segments whose flows are both **inferred from net depth changes**,
so the standing confound is untouched. And the noise-floor work applies: a pass decided by
less than ~0.01 is not decided at all.

Nothing here is a calibration — no parameter is fitted to a market number, and `haz0`
moves only on mean depth.

### Scored, 2026-08-02 — INCONCLUSIVE, and the regime is unreachable

**AP and AQ carry no verdict.** The validity precondition cannot be met at any value of the
one adjustable parameter, which this block pre-registered as an outcome: *"the model is
separately recorded as unable to reach the regime the mechanism describes."*

`haz0` was swept on mean depth alone, as permitted. The response has a near-discontinuity:

| `haz0` | mean depth | oldest-cohort share |
|---|---|---|
| 0.030 | 161.2 | 0.077 |
| 0.015 | 181.2 | 0.095 |
| 0.0120 | **113.6** | **0.027** |
| 0.0118 | **350.1** | **0.639** |
| 0.0110 | 362.3 | 0.655 |
| 0.0050 | 568.8 | 0.809 |

**The target depth band (227.8–235.9) and the validity window ([5%, 60%]) both fall inside
the gap between 0.0120 and 0.0118.** There is no setting that reaches either.

#### Why, mechanically

The oldest cohort is **absorbing** — it keeps its own survivors as well as inheriting
cohort 6's. Its outflow is `hazard × volume` against an inflow set by what survives seven
younger cohorts. Below a threshold hazard the absorbing cohort accumulates until the
arrival damping brakes it, and above it the cohort is swept out almost entirely. Either old
volume is negligible or it dominates the book; the intermediate state the mechanism
describes is where the model does not sit.

#### A bug found and fixed before this scoring, not after

The first config **discarded** cohort 7's survivors rather than absorbing them, which capped
residence at 8 steps and capped depth at ~181 regardless of hazard. That contradicted the
mechanism as pre-registered ("the oldest cohort is absorbing"), so it was fixed. The
numbers above are from the corrected config.

Worth recording: the **buggy** version had an oldest-cohort share of 0.095 — inside the
validity window — and a depth of 181, outside the band. So the discarding variant reaches
the regime the absorbing one cannot. That is a different mechanism from the pre-registered
one and is **not** scored here; noting it is not a licence to adopt it after the fact.

#### What is and is not established

Established: **with 8 cohorts and an absorbing tail, rising-hazard time-in-queue has no
setting at which old volume is a moderate fraction of the book.** That is a limit on the
mechanism's usable range, and it is why AP and AQ carry no verdict rather than a failure.

Not established: that time-in-queue cancellation cannot produce the paired depth signature.
The mechanism was never scored on it. A finite maximum age, or more cohorts, or a hazard
that flattens rather than rising without bound, are all untested variants — and each would
need its own pre-registration, because choosing one now would be choosing it having seen
that the first attempt could not be scored.

Also untested: whether the transition is genuine bistability with hysteresis or simply a
very steep monotone response. Only a one-directional sweep was run.

---

## Time-in-queue with a finite patience horizon — predictions fixed before `cfg/lob_ages_finite.yaml` exists

**Fixed 2026-08-02, before the config is written.** A second attempt at the fifth
mechanism, after AP–AS came out inconclusive because the model could not reach its own
regime.

### The diagnosis the variant is chosen from

The absorbing oldest cohort is an **unbounded reservoir with a fixed per-unit hazard**. Its
outflow is `hazard × volume` against an inflow set by what survives seven younger cohorts,
so below a threshold it accumulates until the arrival damping brakes it and above it is
swept out. That is what produced a near-discontinuity between `haz0` 0.0120 and 0.0118,
with the depth band and the validity window both inside the gap.

Three variants were named as untested. Judged against that diagnosis:

| variant | does it remove the reservoir? |
|---|---|
| **finite maximum age** — an order is withdrawn with certainty at age *A* | **yes** — residence is bounded, so there is no accumulating tail |
| more cohorts | no — it moves the reservoir further out and leaves it there |
| hazard that flattens rather than rising | no — a plateau is still a fixed hazard on an unbounded tail |

**Only the first addresses the diagnosed cause**, and the other two are predicted to
reproduce the same failure. That is the ground the choice is made on.

Economically it is the standard quote-refresh cycle: increasing impatience below a hard
deadline, and a maker who has sat unfilled for *A* steps re-prices without exception.

### My contamination, declared

The inconclusive block's **first config had this bug** — it discarded the oldest cohort's
survivors instead of absorbing them — so I have already seen the finite-age variant run. At
`haz0` between 0.03 and 0.015 it gave an oldest-cohort share of 0.077–0.095, **inside the
validity window**, at depths of 161–181, **below the target band**. I have not seen it
below `haz0` 0.015, and I have never seen any correlation from it.

So this is not a blind choice and must not be presented as one. What is blind: whether a
setting exists that reaches the depth band, and every correlation AT–AW is scored on.

**The reasoning above stands independently of that sighting** — a reservoir with fixed
hazard is what caused the failure, and only bounding residence removes it. But the sighting
is why this block, unlike the last, states its bands and preconditions knowing the
mechanism can at least reach the window.

### The mechanism, stated before measuring

Identical to the AP–AS block except the tail. Arrival side inherited unchanged from
`cfg/lob_damping.yaml`; 8 cohorts; `h(c) = haz0 * (1 + 0.5c)`; marketable orders consume
oldest-first.

	the oldest cohort's survivors are DISCARDED, not carried forward

so residence is at most 8 steps and no cohort accumulates.

### Preconditions, both mechanical

1. **The depth band must be reachable.** `haz0` is swept on mean depth alone and must land
   in **227.8–235.9**. If no setting reaches it, the block is recorded **inconclusive on
   reachability** — the same verdict as last time and for a related reason, which would say
   the bounded-residence variant trades one unreachable regime for another.
2. **The oldest-cohort share must lie in [5%, 60%]** at the selected `haz0`, reported
   either way. Outside it, AT and AU are **inconclusive**.

### Predictions

Bands identical to AP–AS and to prediction Y, so all three mechanisms are comparable.

**AT — the paired depth signature.** `corr(depth, cancels)` in **[−0.30, −0.01]** AND
`corr(depth, arrivals)` in **[−0.40, −0.05]**. The test.

**AU — the ordering.** `|corr(depth, arrivals)| > |corr(depth, cancels)|`.

**AV — the co-movement.** `corr(arrivals, cancels)` > **+0.85**. The cost check, and still
the one I expect to be hardest: cancellation depends on the age distribution rather than on
contemporaneous activity, so it may decouple from arrivals.

**AW — the book survives.** Drift **< 1.3**, spread sd **> 0.1**.

### What each outcome means

| AT | AV | reading |
|---|---|---|
| **pass** | **pass** | The first mechanism to reproduce the paired depth signature *and* clear the co-movement floor. Worth a fresh out-of-sample recording against it — which, on this project's record, is where results have gone to die. |
| **pass** | **fail** | Depth structure reachable through bounded ageing, at the cost of the co-movement — the same trade the damping model and recycled churn both made. Three mechanisms failing the same way stops being a coincidence and starts being a statement about the vocabulary. |
| **fail (positive)** | — | Cancellation summed over cohorts still tracks depth even with bounded residence. The identity would then generalise to *any* rule proportional to resting volume however weighted, which is a much stronger claim than the current one and would close the whole cancellation-side family. |
| **fail (overshoot)** | — | Response too strong at this hazard shape. A parameter question needing its own block. |
| *inconclusive* | — | Either precondition missed. Recorded as a limit on usable range, not a free pass — and a second inconclusive result would say the age-structured family is hard to place in a testable regime at all. |

### What even a full pass would not establish

Model-internal, 32-member ensemble means at 8000 steps. The target bands come from Binance
segments whose flows are both inferred from net depth changes. Nothing is fitted to a
market number — `haz0` moves only on mean depth. And the noise floor applies: a margin
under ~0.01 is not a result.

### Scored, 2026-08-03 — INCONCLUSIVE on reachability, and the reason is arithmetic

**AT–AW carry no verdict.** Precondition 1 fails: no value of `haz0` puts mean depth in the
227.8–235.9 band. Swept on depth alone, as permitted:

| `haz0` | mean depth |
|---|---|
| 0.0150 | 184.3 |
| 0.0100 | 186.9 |
| 0.0060 | 187.8 |
| 0.0030 | 190.6 |
| 0.0010 | 187.8 |
| 0.0003 | **193.7** |

Depth barely moves across a **50× range** of the parameter and never approaches the band.

#### Why: the residence cap, not the hazard

At `haz0` = 0.0003 the hazard is doing essentially nothing — 0.2 cancellations per step —
and the model reports an **implied residence of 7.59 steps against a hard cap of 8**, with
25.5 arrivals per step and a depth of 193.7 ≈ 25.5 × 7.59.

So `depth = arrivals × residence`, residence is bounded at 8 by construction, and arrivals
are damped to ~25 at that depth. The product cannot exceed ~200. **The band would need a
residence of about 9 steps, which eight cohorts cannot supply at any hazard.**

Precondition 2 would have passed — the oldest-cohort share is 0.115, inside [5%, 60%] — so
this variant does reach the regime the absorbing one could not. It just cannot reach the
depth the comparison requires.

#### The structural finding, which is worth more than either result alone

**Both age-structured variants miss the reference depth band, for opposite reasons.**

| variant | failure |
|---|---|
| absorbing tail | bistable: 113.6 at `haz0` 0.0120, 350.1 at 0.0118. The band lies **inside the gap**. |
| finite patience | capped: ~194 however low the hazard. The band lies **above the ceiling**. |

With 8 cohorts, age-structured cancellation cannot produce the reference depth at the
inherited arrival damping — the unbounded tail overshoots discontinuously and the bounded
one undershoots by arithmetic. That is a statement about the family, not about two
settings, and neither block alone would have supported it.

#### What is NOT established, and the variant this diagnoses

That time-in-queue cannot reproduce the paired depth signature. **It has still never been
scored on it** — two attempts, two failures to reach a testable regime.

The diagnosis names the next variant precisely: **more cohorts raise the residence cap**,
and depth ≈ arrivals × residence says roughly 12 cohorts would put the ceiling near 300 and
bring the band within reach. That is a concrete, arithmetically-motivated change — and it
needs its own pre-registration, because naming it here having seen why this one failed is
exactly the sequence that pre-registration exists to keep honest.

Also unexamined: whether the *comparison band itself* is the right target for a model whose
residence is structurally bounded. Holding mean depth fixed across mechanisms is what makes
correlations comparable, so it was not relaxed — but a third failure on reachability would
make it worth asking whether the constraint, rather than the mechanism, is what keeps
failing.

---

## Twelve cohorts — predictions fixed before `cfg/lob_ages12.yaml` exists

**Fixed 2026-08-03, before the config is written.** The third attempt at the fifth
mechanism. Two attempts have failed on *reachability* and the mechanism has **still never
been scored on the test it exists for**, so this block leads with a quantitative
prediction about reachability itself.

### The change, and why it is arithmetic rather than a guess

`cfg/lob_ages_finite.yaml` capped at a mean depth of ~194 however low the hazard, because
`depth = arrivals × residence` and residence is bounded at 8 by the cohort count. At
`haz0` = 0.0003 the hazard did essentially nothing — 0.2 cancellations per step — and the
model reported residence 7.59 against a cap of 8.

**Only the cohort count moves: 8 → 12.** Hazard shape, arrival side, driver, everything
else inherited unchanged.

### AX — the ceiling prediction, stated with its working

Undamped arrivals are `2 × E[activity] × Σ exp(−0.35 i)` over 8 levels and 2 sides ≈
**50.9/step**. Damping is `≈ 1/(1 + depth/304)` at mean activity, and residence runs at
about 0.95 of the cap. So the ceiling solves

	depth = 50.9 × 0.95n / (1 + depth/304)

giving **223 at n = 8** and **295 at n = 12**. The formula overestimates — it gave 223
where 194 was measured — so scaling by that 0.87 factor puts the 12-cohort ceiling at
**≈ 256**.

> **AX: the mean-depth ceiling exceeds 235.9**, so the target band becomes reachable and
> some `haz0` lands inside it. Point estimate **256**, and the prediction fails if the
> ceiling comes in below the band exactly as it did twice before.

This is falsifiable and it is the whole point of the block: if AX fails, the arithmetic
that diagnosed both previous failures is wrong, and three reachability failures would say
the problem is the comparison constraint rather than the mechanism.

### Preconditions, unchanged from AT–AW

1. `haz0` swept on **mean depth alone** must land in **227.8–235.9**. This is AX restated
   as a gate; failing it makes AY–BB inconclusive.
2. The oldest-cohort share must lie in **[5%, 60%]** at the selected `haz0`, reported
   either way. (At 8 cohorts it was 0.115 — inside — so this is not expected to bind, and
   saying so now means a surprise is visible as one.)

### Predictions

Bands identical to Y, AP–AS and AT–AW, so every mechanism remains comparable.

**AY — the paired depth signature.** `corr(depth, cancels)` in **[−0.30, −0.01]** AND
`corr(depth, arrivals)` in **[−0.40, −0.05]**. The test the mechanism has never reached.

**AZ — the ordering.** `|corr(depth, arrivals)| > |corr(depth, cancels)|`.

**BA — the co-movement.** `corr(arrivals, cancels)` > **+0.85**. Still expected hardest:
cancellation depends on the age distribution rather than contemporaneous activity.

**BB — the book survives.** Drift **< 1.3**, spread sd **> 0.1**.

### What each outcome means

| AX | AY | BA | reading |
|---|---|---|---|
| pass | **pass** | pass | Time-in-queue reproduces the paired depth signature and keeps the co-movement — the first mechanism to do both, after five candidates. Worth a fresh out-of-sample recording. |
| pass | **pass** | **fail** | The **third** mechanism to buy depth structure by selling co-movement, after the damping model and recycled churn. Three independent mechanisms failing the same way is a statement about the vocabulary, not about any one of them. |
| pass | **fail (positive)** | — | Cancellation summed over cohorts still tracks depth at 12 cohorts as at 8. The identity would generalise to *any* rule proportional to resting volume however weighted — closing the entire cancellation-side family, which is the most valuable failure available. |
| **fail** | — | — | The arithmetic that diagnosed both previous failures is wrong. Three reachability failures would shift the question to whether holding mean depth fixed across mechanisms is the right constraint at all — a protocol decision, not a modelling one, and one for the maintainer. |

### What even a full pass would not establish

Model-internal, 32-member ensemble means at 8000 steps. Target bands come from Binance
segments whose flows are both inferred from net depth changes. Nothing is fitted to a
market number — `haz0` moves only on mean depth. A margin under ~0.01 is not a result.

### Scored, 2026-08-03 — AX passes, the rest fails, and my precondition was badly designed

**AX PASSED.** The mean-depth ceiling is **291.4** against a predicted **256** — within 14%
— and comfortably clears 235.9. `haz0` = 0.016 gives an ensemble mean depth of **233.2**,
inside the band. **Precondition 1 is satisfied for the first time in three attempts**, and
the arithmetic that diagnosed both earlier failures was right.

| | measured | verdict |
|---|---|---|
| AX ceiling > 235.9 | **291.4** | **pass** |
| precondition 1, depth in band | 233.2 (SE 0.5) | pass |
| precondition 2, oldest share in [5%, 60%] | **0.0474** | **FAIL** |
| AY `corr(depth, cancels)` in [−0.30, −0.01] | **+0.940** (SE 0.0006) | inconclusive, and wrong-signed |
| AY `corr(depth, arrivals)` in [−0.40, −0.05] | **+0.312** (SE 0.0033) | inconclusive, and wrong-signed |
| AZ ordering margin > 0 | −0.628 | inconclusive |
| BA `corr(arrivals, cancels)` > +0.85 | **+0.180** (SE 0.0042) | **FAIL** |
| BB drift < 1.3, spread sd > 0.1 | 1.0027, **0.0958** | **FAIL** on the spread limb |

#### My precondition was not scale-free, and that is a design error

Precondition 2 measures the share of resting volume in **the oldest cohort**. That is not
invariant to how many cohorts there are: spreading the same age distribution over 12 bins
instead of 8 mechanically lowers the top bin's share. It fell from 0.115 to 0.0474 for
exactly that reason, not because the age structure stopped operating.

So the precondition did not test what it was written to test, and it fired on a change of
resolution rather than a change of behaviour. **A scale-free version — mean age relative to
the cap, or the share in the oldest third — would not have fired.** I am recording that
rather than substituting one now, because swapping a precondition after it fails is the
edit this file exists to forbid. AY and AZ stay inconclusive on the rule as written.

#### The numbers are formally inconclusive and substantively decisive

`corr(depth, cancels)` = **+0.940**. That is not a weak version of the target; it is nearly
a deterministic relationship, and it is the attrition signature in its purest form yet
seen. With a stable age distribution, `Σ hazard(c) × volume(c)` is an effective rate times
depth, so summed cancellation tracks depth almost exactly however the hazard is weighted
across ages.

**That is the "AY fails positive" row of the outcome table**, which this block called the
most valuable failure available: the identity generalises from *rules keyed to recent
arrivals* to **any rule proportional to resting volume however weighted by age**. Age
weighting does not break the coupling — it only reweights which volume contributes.

BA at +0.180 is the predicted cost realised at full strength: cancellation follows a slow
age distribution while arrivals follow a fast driver, so the two decouple almost entirely.

#### A measurement-definition problem, found while scoring and worth more than the scores

**Volume leaving by the age cap is not counted as a cancellation.** `n_cancel` sums only
hazard cancellations; the oldest cohort's discarded survivors simply vanish from the book.
In the Binance data, an order removed for any non-trade reason *is* a cancellation.

So the model's cancellation series and the market's are **not like-for-like**, and every
`corr(depth, cancels)` comparison in this block — and in the two blocks before it — is
between quantities defined differently. It does not rescue this result, since counting the
expiry term would add something also roughly proportional to depth. But it is a real defect
in the comparison, it was introduced by the age-cap mechanism, and it should be fixed before
any age-structured model is scored against market data again.

#### Where the fifth mechanism now stands

Three attempts. The first two never reached a testable regime; this one did, and the
mechanism **failed on the axis it was built for** — with the caveat that its two most
diagnostic predictions are formally inconclusive on a precondition that fired for the wrong
reason.

What is established: **age-structured cancellation does not break the depth coupling**, and
the reason generalises beyond this mechanism. What is not: anything about a corrected
cancellation definition, or about a scale-free precondition, both of which would need a
fresh block.

---

## PROTOCOL CHANGE, 2026-08-03: the target is the pooled grand mean, not a single window

Sanctioned by the maintainer. Every block up to now scored against a **single window's**
five-symbol mean. Pooling seven recorded windows shows why that was wrong:
`corr(depth, arrivals)` has a grand mean of **−0.1394** with a between-window SD of
**0.0516**, and the Thursday window γ was calibrated to sits at −0.2129 — **1.4 SD into the
tail**. Fitting to one draw of a quantity that wanders by 0.05, then testing on another,
fails for reasons that have nothing to do with any mechanism.

**From this block on, targets are the pooled grand mean and tolerances are multiples of the
between-window SD.** Recorded as a change rather than applied silently, because it follows
a run of failures against single-window targets and would otherwise look like moving the
goalposts. The earlier blocks are not rescored; their bands stand as written.

Reference values, seven windows × five symbols:

| quantity | grand mean | between-window SD |
|---|---|---|
| `corr(depth, arrivals)` | −0.1394 | 0.0516 |
| `corr(depth, cancels)` | −0.0577 | 0.0585 |
| `corr(arrivals, cancels)` | **+0.9499** | **0.0243** |

---

## The co-movement gap — predictions fixed before `cfg/lob_var.yaml` exists

**Fixed 2026-08-03, before the config is written.** The pooled measurement showed the model
is within ordinary market variation on both depth correlations (1.1–1.6 SD) and **3.6 SD
low on co-movement**, against the market's steadiest signature. So the co-movement is the
target, not a cost check.

### The gap has an arithmetic explanation, and it is not the mechanism

With `arr ~ Poisson(λₐ·A)` and `can ~ Poisson(λ_c·A)` sharing driver `A`, independent
Poisson noise caps the achievable correlation:

	corr = λₐλ_c·Var(A) / (N + λₐλ_c·Var(A))

At the shipped driver — mean 4, variance 8 — and ~26 counts/step, that ceiling is
**0.9286**. The market sits at **0.9499, above the model's ceiling.** No amount of
mechanism work reaches it while the driver's variance stays at 8: the model is not failing
to couple the flows, it is failing to give them enough *shared* variation relative to their
Poisson noise.

Setting the ceiling equal to the market grand mean gives the variance required:

	Var(A) = 0.9499·N / (λₐλ_c·(1 − 0.9499)) = **11.67**

### The change: one number, computed in advance, not swept

Driver variance **8 → 11.67**, mean held at 4. For the AR(1) at φ = 0.8 the stationary
variance is `Var(innovation)/9`, so the innovation becomes `gamma(0.152367, 0.038092)` —
mean 4.000, variance 105.01. Everything else is inherited from `cfg/lob_damping.yaml`
unchanged, γ included.

**This is a point prediction from theory, not a fit.** No sweep, no adjustable parameter,
no depth re-set: the value comes out of the algebra above and is committed here before the
config runs. If the arithmetic is right, BC lands without anyone aiming at it.

### Predictions

**BC — the arithmetic holds.** `corr(arrivals, cancels)` lands in **[0.930, 0.970]**, i.e.
the market grand mean ±0.02.

The direct test of the Poisson-ceiling account. It could fail two ways: short, if arrival
saturation in activity still drags it below the ceiling (that saturation is what put the
model at 0.863 rather than 0.929 today, and raising variance does not remove it); or long,
if higher driver variance couples the flows more than the algebra allows.

**BD — the held-out cancellation side survives.** `corr(depth, cancels)` within **1.5 SD**
of the pooled grand mean: **[−0.146, +0.030]**.

**BE — the held-out arrival side survives.** `corr(depth, arrivals)` within **1.5 SD** of
the pooled grand mean: **[−0.217, −0.062]**.

BE is a real constraint and the model **marginally fails it today** at −0.2244, seven
thousandths outside. Raising driver variance will move it, and I do not know which way.

**BF — the book survives.** Drift **< 1.3**, spread sd **> 0.1**.

### What each outcome means

| BC | BD/BE | reading |
|---|---|---|
| **pass** | **pass** | The co-movement gap was a *noise-ratio* problem, not a mechanism problem, and a number computed from theory closed it without touching the mechanism or being fitted to anything. That would be the first quantitative prediction this project has made and had land. |
| **pass** | **fail** | Co-movement is reachable but costs the depth structure — the same trade in the opposite direction from every earlier block, and evidence the three signatures cannot be held at once in this vocabulary. |
| **fail (short)** | — | Arrival saturation dominates the Poisson ceiling, so the binding constraint is the damping's activity dependence rather than the driver's variance. That points at γ, which is fitted, and would mean the co-movement and the depth fit are coupled through one parameter. |
| **fail (long)** | — | The Poisson-ceiling algebra is wrong about this model, most likely because the flows share more than the driver. Worth knowing precisely because the algebra is what makes this a prediction rather than a sweep. |

### What even a full pass would not establish

Model-internal, 32-member ensemble means at 8000 steps, against a grand mean from **three
distinct occasions** on one venue — the effective independent sample is nearer 3 than 7 and
the SD is the defensible statistic, which is why tolerances are stated in SD. The standing
inference confound is untouched. And nothing here is calibrated: the one number that moves
was computed from the algebra above, not fitted.

### Scored, 2026-08-03 — BC fails SHORT, and the failure closes the direction with arithmetic

| | measured | wanted | |
|---|---|---|---|
| driver variance realised | 11.81 | 11.67 | construction correct |
| **BC** `corr(arrivals, cancels)` | **+0.8869** (SE 0.0008) | [0.930, 0.970] | **FAIL, short** |
| **BD** `corr(depth, cancels)` | −0.1227 (SE 0.0049) | [−0.146, +0.030] | pass |
| **BE** `corr(depth, arrivals)` | −0.2157 (SE 0.0049) | [−0.217, −0.062] | pass by **0.0013** |
| **BF** | drift 1.0009, spread sd 0.6388 | < 1.3, > 0.1 | pass |

#### The algebra predicts the DERIVATIVE and misses the LEVEL, by a constant

| Var(A) | Poisson ceiling | achieved | gap |
|---|---|---|---|
| 8.00 | 0.9286 | 0.8629 | **0.0657** |
| 11.81 | 0.9505 | 0.8869 | **0.0636** |

The ceiling moved **+0.0219** and the model moved **+0.0240** — the response tracks the
prediction almost exactly. But both sit about **0.065 below** their ceiling, and that offset
is **invariant to the driver's variance**.

That offset is the arrival saturation identified in the persistent block: arrivals are
proportional to `act/(1 + q·act^γ/s)`, which saturates in activity, while cancellation stays
proportional to it. Raising the shared variance lifts both flows' ceiling and does nothing
to the saturation.

#### Which closes the direction, not just this attempt

If the penalty is a constant ≈0.065 and the Poisson ceiling cannot exceed 1, then reaching
the market's **+0.9499** would need a ceiling of **1.015**. **No driver variance reaches the
market's co-movement while the arrival saturation is present.** The direction is exhausted
by arithmetic rather than by trying values.

This is the pre-registered "fail short" reading, and it lands where that row said it would:
*the binding constraint is the damping's activity dependence rather than the driver's
variance. That points at γ, which is fitted.* The co-movement and the depth fit **are
coupled through one parameter** — γ sets both how well the depth correlation matches and how
much co-movement the saturation costs.

#### BE passed and should not be read as a result

−0.2157 against a floor of −0.217 is inside by **0.0013**, about a quarter of one standard
error and far below the 0.05 between-window SD the tolerance was built from. It is a pass on
the rule as written and nothing more; a different seed block could put it either side.

#### What this establishes

**The co-movement gap is not a mechanism problem and not a noise-ratio problem.** It is the
arrival damping's saturation, and it is quantitatively pinned: 0.065 of correlation, stable
across a 48% change in driver variance.

That is the first quantitative account this project has of *why* a signature is missed,
rather than a measurement that it is. Five mechanism blocks treated the co-movement as a
cost to be paid; it turns out to be a fixed levy charged by the arrival side, which none of
them touched.

#### What it does not establish

That reducing γ would close the gap without breaking the depth fit — γ was fitted to the
depth correlation and moving it changes both, which is exactly the coupling this result
identifies. Testing that trade-off is a fresh block, and it is a two-target problem rather
than the one-target problems every block so far has been.

Nothing here is fitted: the one number that moved was computed from algebra before the
config ran, and it produced the predicted *change* while revealing a constant the algebra
did not contain.

---

## Can any γ hold both targets? Predictions fixed before `cfg/lob_gamma.yaml` exists

**Fixed 2026-08-03.** The first **two-target** block: every previous one asked whether a
mechanism could hit one signature, and this asks whether one parameter can hit two that BC
showed are coupled through it.

### My contamination, declared, and it is heavy

BC established that co-movement ≈ Poisson ceiling − saturation, with the saturation stable
in driver variance. The γ sweep at Var(A) = 8 is already recorded. Together those let me
**extrapolate the whole answer**:

| γ | `d/arr` | `d/can` | `arr/can` at Var 8 | saturation gap |
|---|---|---|---|---|
| 0.0 | +0.216 | +0.357 | 0.8872 | 0.0414 |
| 0.4 | −0.092 | +0.014 | 0.8771 | 0.0515 |
| 0.6 | −0.224 | −0.120 | 0.8629 | 0.0657 |
| 1.0 | −0.387 | −0.259 | 0.8183 | 0.1103 |

At the raised variance the ceiling is 0.9505, so co-movement ≈ 0.9505 − gap: about **0.909
at γ = 0** falling to 0.885 at γ = 0.6. The 1.5 SD floor is **0.9134**. The depth bands are
satisfied only around γ ≈ 0.4–0.6, where co-movement is ~0.89–0.90.

**So I expect the sets to be disjoint and I can say roughly by how much.** This block is
therefore mostly a *confirmation of an extrapolation*, not a blind test, and is labelled so.

### What is genuinely uncertain, and where the prediction earns its keep

The **margin**. If co-movement peaks at 0.909 against a floor of 0.9134, the sets miss by
**0.004** — a hair, and a very different conclusion from missing by 0.05. Whether the
saturation gap at γ = 0 holds its Var-8 value of 0.041 at the raised variance is untested;
BC only checked γ = 0.6.

That is what BG is scored on, and it is why the prediction is stated as a number rather
than a direction.

### The sweep

γ ∈ **{0.0, 0.15, 0.30, 0.45, 0.60}**, fixed. Driver variance held at the raised **11.81**
and everything else inherited from `cfg/lob_var.yaml`. No parameter is fitted and none is
re-set on depth — this is a sweep to map a trade-off, not to select a model.

Targets are the pooled grand means at 1.5 between-window SD, per the protocol change:

	corr(depth, arrivals)   in [−0.217, −0.062]
	corr(depth, cancels)    in [−0.146, +0.030]
	corr(arrivals, cancels) ≥ 0.9134

### Predictions

**BG — the sets are disjoint, and narrowly.** No γ in the grid satisfies all three. The
**maximum co-movement across the grid lands in [0.900, 0.915]** — below the floor, but by
less than 0.015.

The structural claim and the quantitative one in a single prediction. It fails if some γ
satisfies all three, and it fails if the maximum lands outside that band even while staying
below the floor — the second being the more likely miss and the more informative.

**BH — the trade-off is monotone.** `corr(depth, arrivals)`, `corr(depth, cancels)` and
`corr(arrivals, cancels)` all **decrease** as γ increases across the grid.

Declared the weak one: it is what the Var-8 sweep already shows. It is scored because
monotonicity is what makes "no γ works" a statement about the whole interval rather than
about five points — without it, an untested γ between grid points could satisfy both.

**BI — the depth window is non-empty.** At least one γ in the grid satisfies **both** depth
bands, so the disjointness in BG is a real trade-off rather than an artefact of the depth
targets being unreachable too.

**BJ — the book survives at every γ.** Drift **< 1.3** and spread sd **> 0.1** at all five.

### What each outcome means

| BG | BI | reading |
|---|---|---|
| **pass** | pass | One parameter cannot hold both signatures, and the miss is small. **The model is one structural change away, not a rebuild** — and the size of the gap says how much a change would have to buy. |
| **fail (some γ works)** | pass | A γ exists satisfying all three, which the extrapolation said should not. The saturation account would be wrong somewhere, and finding where matters more than the γ. |
| **fail (max outside band)** | pass | Direction right, magnitude wrong — the saturation gap does not hold its Var-8 value across γ. That would qualify BC's "constant penalty" finding, which was only measured at one γ. |
| — | **fail** | No γ satisfies the depth bands at the raised variance, so BC's BD/BE passes were specific to γ = 0.6 and the raised variance narrowed the depth window. That would make this a one-target problem again. |

### What even a full pass would not establish

That no *other* change closes the gap — only that γ alone cannot. Model-internal, 32-member
ensemble means at 8000 steps, against a grand mean from three distinct occasions on one
venue. Tolerances are 1.5 SD of a between-window spread estimated from those three, so they
are softer than they look.

### Scored, 2026-08-03 — all four pass, and the quantitative prediction landed

| γ | `corr(d,arr)` | `corr(d,can)` | `corr(arr,can)` | depth bands | co-movement floor |
|---|---|---|---|---|---|
| 0.00 | +0.2737 | +0.4104 | **0.9026** | ✗ | ✗ |
| 0.15 | +0.1751 | +0.2965 | 0.8995 | ✗ | ✗ |
| 0.30 | +0.0303 | +0.1342 | 0.8991 | ✗ | ✗ |
| 0.45 | −0.0989 | −0.0048 | 0.8938 | **✓** | ✗ |
| 0.60 | −0.2157 | −0.1227 | 0.8869 | **✓** | ✗ |

| | measured | |
|---|---|---|
| **BG** disjoint, max co-movement in [0.900, 0.915] | no γ satisfies all three; **max 0.9026** | **pass, both limbs** |
| **BH** all three monotone decreasing in γ | strictly, on all three | **pass** |
| **BI** some γ satisfies both depth bands | γ = 0.45 and 0.60 | **pass** |
| **BJ** book survives at every γ | drift 0.996–1.005, sd 0.639–0.691 | **pass** |

**The first block in this project where every prediction passed**, and the headline one was
quantitative with two ways to miss.

#### The margin, stated where it matters rather than where it flatters

BG scored the maximum co-movement across the grid: **0.9026 against a floor of 0.9134**, a
shortfall of **0.011**. But that maximum sits at γ = 0, where the depth correlations are
strongly *positive* and hopeless.

**Inside the depth-feasible region the shortfall is larger: 0.020 at γ = 0.45 and 0.027 at
γ = 0.60.** That is the number any future work has to buy, and quoting the 0.011 would be
picking the flattering end of a trade-off this block exists to measure.

#### BC's constant-penalty finding extrapolates across γ, which it had not been tested on

BC measured the saturation gap at one γ. Against the Var = 11.81 ceiling of 0.9505:

| γ | gap at Var 8 | gap at Var 11.81 |
|---|---|---|
| 0.00 | 0.0414 | 0.0479 |
| 0.60 | 0.0657 | 0.0636 |

Roughly stable at both ends, so the account generalises. The extrapolation this block
declared predicted 0.909 at γ = 0 and 0.885 at γ = 0.60; measured 0.9026 and 0.8869 —
within 0.007 and 0.002. **The arithmetic that made this a prediction rather than a sweep
holds across the parameter it was never tested on.**

#### What this establishes

**One parameter cannot hold both signatures.** The feasible sets are disjoint, monotonically
so across the whole interval rather than at five points, and the depth window is non-empty —
so this is a genuine trade-off, not an artefact of an unreachable target.

And it is **quantified**: a structural change would have to buy about **0.02–0.03 of
co-movement without moving the depth correlations**. That is a specification for the next
change rather than a direction, which is the first time this project has had one.

#### What it does not establish

That no *other* change closes it — only that γ alone cannot. The co-movement floor is
1.5 SD of a between-window spread estimated from **three distinct occasions**, so it is
softer than a 1.5 SD band normally implies; a wider tolerance would put γ = 0.45 inside.
And the whole block is a confirmation of a declared extrapolation, so its value is in the
magnitudes, not the direction.

---

## Buying the specification — predictions fixed before `cfg/lob_burst.yaml` exists

**Fixed 2026-08-03.** BG produced a specification rather than a direction: a change must buy
**0.02–0.03 of co-movement without moving the depth correlations**. This block attempts it
with the one lever the arithmetic says is still open.

### The lever, and why it is the only one left

Co-movement = Poisson ceiling − saturation, and BG confirmed the saturation is roughly
constant in both driver variance and γ. So the ceiling is the only term available, and
`ceiling = λ²·Var(A) / (N + λ²·Var(A))` has exactly two inputs: the driver's variance and
the flow counts. Counts cannot move without moving depth, which is the thing being held.

**So: raise the driver's variance again, to a value computed to close the gap.** At γ = 0.45
the saturation gap is 0.0567, so clearing the 0.9134 floor needs a ceiling of 0.9701, which
needs **Var(A) = 19.95**. Mean held at 4; for the AR(1) at φ = 0.8 the innovation becomes
`gamma(0.089121, 0.022280)` — mean 4.000, variance 179.5. Driver CV rises to **1.12**, so
activity becomes markedly burstier, which is at least the right direction for a market.

### γ is selected on the DEPTH target, and co-movement is held out

γ = **0.45**, chosen by a rule stated on the depth side alone: of the two depth-feasible
values BG found, it is the nearer to the pooled grand mean (`corr(depth, arrivals)` −0.0989
vs −0.1394, distance 0.041; γ = 0.60 is 0.076 away). **Co-movement plays no part in the
selection** — which is what keeps BK a prediction rather than a fit.

### My contamination, declared

I have seen the full γ sweep at Var = 11.81, so I know γ = 0.45's co-movement shortfall is
0.020 and I computed the variance to close exactly it. What is **not** seen: whether the
saturation gap stays constant at a 69% larger variance — BC checked one 48% step and BG
checked two γ values, neither at Var ≈ 20 — and whether the depth correlations stay in band
when the driver becomes this bursty.

### Predictions

**BK — the co-movement clears its floor.** `corr(arrivals, cancels)` lands in
**[0.905, 0.925]**, a band straddling the 0.9134 floor.

Deliberately straddling: the point estimate is 0.9134 by construction, so a band that
excluded the floor would be unfalsifiable in one direction. It fails short if the
saturation grows with variance, and long if the ceiling algebra over-corrects.

**BL — the arrival side stays in band.** `corr(depth, arrivals)` in **[−0.217, −0.062]**.

**BM — the cancellation side stays in band.** `corr(depth, cancels)` in **[−0.146, +0.030]**.

BL and BM are the "without moving the depth correlations" half of the specification, and
they are **not** near-forced: a CV of 1.12 is a substantially different activity process,
and the depth correlations moved measurably for the smaller variance step in BC.

**BN — the book survives.** Drift **< 1.3**, spread sd **> 0.1**.

Worth its own line here rather than as a formality: a burstier driver means larger swings in
both flows, and one-sided steps are excluded from the spread average rather than counted, so
a book that empties often would flatter the spread statistic while breaking.

### What each outcome means

| BK | BL/BM | reading |
|---|---|---|
| **pass** | **pass** | **All three pooled targets met simultaneously for the first time.** The specification BG produced would be satisfied, and by a value computed in advance rather than fitted. The honest next step is a fresh recording, since every prior in-sample success in this project has failed out of sample. |
| **pass** | **fail** | The co-movement is buyable but the depth correlations pay for it, so the trade-off BG found in γ also exists in driver variance. That would make it a property of the model rather than of one parameter. |
| **fail (short)** | — | The saturation is not constant at large variance — it grows. That contradicts BC and BG at a point neither tested, and would mean the ceiling account has a limited range rather than being general. |
| **fail (long)** | — | The ceiling algebra over-corrects at high variance, most likely because the flows share more than the driver once it dominates. |

### What even a full pass would not establish

That the model is right — only that it meets three pooled targets, from three distinct
occasions on one venue, with tolerances of 1.5 SD on a spread estimated from those three.
The standing inference confound is untouched. Nothing is fitted: the variance was computed
from the algebra and γ was selected on the depth side alone.

### Scored, 2026-08-03 — both halves of the specification fail, and BC's constancy has a range

| | measured | wanted | |
|---|---|---|---|
| driver variance realised | 19.82 | 19.95 | construction correct |
| **BK** `corr(arrivals, cancels)` | **+0.8951** (SE 0.0011) | [0.905, 0.925] | **FAIL, short** |
| **BL** `corr(depth, arrivals)` | **−0.0284** (SE 0.0064) | [−0.217, −0.062] | **FAIL** |
| **BM** `corr(depth, cancels)` | **+0.0746** (SE 0.0070) | [−0.146, +0.030] | **FAIL** |
| **BN** drift / spread sd | 0.9995 / 0.9050 | < 1.3 / > 0.1 | pass |

#### The saturation is NOT constant at large variance, which qualifies BC and BG

| Var(A) at γ = 0.45 | Poisson ceiling | achieved | gap |
|---|---|---|---|
| 11.81 | 0.9505 | 0.8938 | **0.0567** |
| 19.82 | 0.9699 | 0.8951 | **0.0748** |

**The ceiling rose +0.0194 and the co-movement rose +0.0013.** The lever has essentially
stopped working: the saturation absorbed 93% of the gain.

BC found the penalty constant over 8 → 11.81 and BG confirmed it across γ, and this block
was built on that. It holds over that range and **fails beyond it** — the pre-registered
"fail short" row said exactly this would mean *the ceiling account has a limited range
rather than being general*, and that is now the reading. The account is not wrong; its
domain is narrower than two blocks assumed, and neither of those blocks tested this far out.

#### And the lever costs the depth side heavily

`corr(depth, arrivals)` moved **+0.0705** and `corr(depth, cancels)` **+0.0794** for the
same variance step — both out of band and toward positive. A burstier driver pushes depth
and both flows up together, so the shared-driver *positive* coupling starts to swamp the
damping's *negative* one.

**So driver variance trades the same two things against each other that γ does**, in the
same direction, and saturates before reaching the target. Two parameters, one trade-off.

#### What this establishes

**The specification BG produced cannot be bought with driver variance.** Both terms of the
ceiling account degrade at once: the gain saturates and the depth correlations pay for what
little arrives.

Taken with BG, the model does not reach all three pooled targets anywhere tested in the
(γ, Var(A)) plane, and the reason is structural rather than a matter of settings — both
axes move co-movement and the depth correlations the same way, so there is no direction in
that plane that improves one without costing the other.

#### What it does not establish

The joint plane is **not mapped**: five γ at one variance, three variances at one or two γ.
A region away from both axes is untested, though the monotonicity BH established in γ and
the direction seen here make a hidden feasible pocket unlikely rather than excluded.

Nor does it establish that no *other* change works — only that neither parameter this model
exposes does. The counts `N` remain the untouched third input to the ceiling, and they were
excluded here because raising them moves depth, which was the thing being held. Whether
raising counts *and* re-setting depth reaches the target is a different block, and it would
have to hold depth by a route other than the damping.

---

## Counts instead of variance — predictions fixed before `cfg/lob_counts.yaml` exists

**Fixed 2026-08-03.** BK raised the driver's variance to lift the Poisson ceiling and the
saturation ate 93% of the gain. This block reaches **the same ceiling by the other route**,
which turns the pair into a controlled comparison rather than a second attempt.

### The algebra that makes this a matched pair

With `E[A] = 4`, the ceiling is

	ceiling = N·Var(A) / (E[A]² + N·Var(A))

so **N and Var(A) enter only through their product.** BK sat at N·V = 515 (N = 26,
V = 19.82) with a ceiling of 0.9699. This block sits at N·V = 520 (N = 44, V = 11.81) with
a ceiling of 0.9701. **The two are at the same ceiling by construction.**

They differ in one thing only: raising Var(A) changes the driver's distribution and
therefore how the damping denominator `1 + q·act^γ/s` behaves, while **raising counts at
fixed variance does not touch `act` at all.** If the saturation is a property of the
driver's spread, it should grow in BK and stay put here.

That is the whole content of the block, and it is not something either arm could establish
alone.

### The change, and how depth is held

`limit_rate` **2.0 → 3.381** and `churn_rate` **1.075 → 1.817**, the same factor 1.691, so
their ratio — which sets the depth equilibrium — is unchanged. γ stays at 0.45 and Var(A)
stays at 11.81, the value BC established and BK overshot.

`churn_rate` may be re-set **once** on mean depth alone if the ratio does not hold depth in
227.8–235.9, since the marketable term does not scale with the rates. That is the standing
single adjustment and nothing else may move.

### Predictions

**BO — the saturation does not grow with counts.** The gap between the Poisson ceiling and
the achieved co-movement lands in **[0.045, 0.070]**, i.e. near its 0.0567 value at
N = 26 and **not** the 0.0748 that BK produced at the same ceiling.

**This is the block.** It fails if the gap lands near BK's, which would mean the saturation
tracks the product N·V rather than the driver's spread — and would make the ceiling account
wrong about its own mechanism, not merely limited in range.

**BP — the co-movement clears its floor.** `corr(arrivals, cancels)` in
**[0.905, 0.925]**, the same band BK was scored on, straddling the 0.9134 floor.

Follows from BO arithmetically, and is stated separately so the mechanism claim and the
outcome can fail independently.

**BQ — the depth correlations stay in band.** `corr(depth, arrivals)` in [−0.217, −0.062]
AND `corr(depth, cancels)` in [−0.146, +0.030].

BK failed this badly — both moved about +0.07 toward positive when the driver got burstier.
Counts at fixed variance should not do that, since the driver is untouched, but the book is
turning over 69% faster and that is not nothing.

**BR — the book survives.** Drift **< 1.3**, spread sd **> 0.1**.

### What each outcome means

| BO | BP/BQ | reading |
|---|---|---|
| **pass** | **pass** | **All three pooled targets met at once**, and the saturation is identified as a property of the driver's spread rather than of the ceiling. BK's failure and this success at matched N·V would together be a clean mechanism result, not a lucky setting. The honest next step is a fresh recording. |
| **pass** | **fail (BQ)** | The saturation account is right and the counts route still costs the depth correlations — through turnover rather than burstiness. A third distinct route to the same trade-off. |
| **fail** | — | The saturation tracks N·V, not the driver's spread. The ceiling account would be wrong about *why*, which matters more than this block's outcome: BC, BG and BK all rest on that account, and their readings would need restating. |

### What even a full pass would not establish

That the model is right — only that it meets three pooled targets from three distinct
occasions on one venue, at 1.5 SD tolerances on a spread estimated from those three. Nothing
is fitted: both rates move by a factor computed from the algebra, and `churn_rate`'s
permitted re-set is on depth alone.

### Scored, 2026-08-03 — all four pass, and the matched pair identifies the saturation

| | measured | wanted | |
|---|---|---|---|
| **BO** saturation gap | **0.0574** | [0.045, 0.070] | **pass** |
| **BP** `corr(arrivals, cancels)` | **+0.9154** (SE 0.0006) | [0.905, 0.925] | **pass** |
| **BQ** `corr(depth, arrivals)` | **−0.1832** | [−0.217, −0.062] | **pass** |
| **BQ** `corr(depth, cancels)` | **−0.0752** | [−0.146, +0.030] | **pass** |
| **BR** drift / spread sd / depth | 1.0001 / 0.4380 / 233.7 | — | **pass** |

`churn_rate` took its single permitted adjustment: the scaled ratio left depth at 253.6,
outside the band, because the marketable term does not scale with the rates. Swept on depth
alone — 1.817 → 253.6, 1.900 → 233.7, 1.960 → 221.6 — and 1.900 selected. No correlation was
computed while choosing.

#### The matched pair is the result, not the pass

Two configs at essentially the same Poisson ceiling, reached by different routes:

| | N·V | ceiling | achieved | **saturation gap** |
|---|---|---|---|---|
| `lob_burst` — via **variance** | 515 | 0.9699 | 0.8951 | **0.0748** |
| `lob_counts` — via **counts** | 573 | 0.9728 | 0.9154 | **0.0574** |
| (`lob_var`, N = 26) | 307 | 0.9505 | 0.8938 | 0.0567 |

**The saturation is a property of the driver's spread, not of the ceiling.** Raising counts
at fixed variance leaves it at 0.057, exactly where it sat at N = 26; raising variance to
the same ceiling drove it to 0.075. Neither arm could have shown this alone, and it is why
the block was built as a pair.

That also rehabilitates BC's account rather than merely bounding it: the constancy claim was
right about the *mechanism* and wrong only in being stated over the wrong variable. The
penalty is constant in **counts**, not in the ceiling.

#### All three pooled targets are met at once — for the first time — but read the margin

| | model | pooled grand mean | distance |
|---|---|---|---|
| `corr(depth, arrivals)` | −0.1832 | −0.1394 ± 0.0516 | **0.85 SD** |
| `corr(depth, cancels)` | −0.0752 | −0.0577 ± 0.0585 | **0.30 SD** |
| `corr(arrivals, cancels)` | +0.9154 | +0.9499 ± 0.0243 | **1.42 SD** |

All inside 1.5 SD. **But the co-movement clears its floor by 0.0020** against a market
between-window SD of 0.0243 — it sits at the very edge of the tolerance, and a slightly
tighter band would exclude it. The depth correlations are comfortable; the co-movement is
not, and calling this "three targets met" without that sentence would be the flattering
reading of a marginal one.

#### What this establishes

That a pure-config model can hold all three pooled signatures simultaneously, at parameters
computed from an explicit account rather than fitted — the rate scaling came from the
ceiling algebra and only `churn_rate` moved, on depth alone.

And, more durably than the pass: **the saturation penalty is caused by the driver's spread
interacting with the arrival damping, not by the ceiling itself.** That was established by
construction, in a designed comparison, and it explains BC, BG and BK's results as one
phenomenon rather than three.

#### What it does not establish, and the next step fixed in advance

Nothing about out-of-sample behaviour. **Every prior in-sample success in this project has
failed out of sample**, and this one is marginal on the very signature that has always been
weakest. The pre-registration committed to a fresh recording as the honest next step and
that stands — with the pooled protocol this time, so the comparison is against a grand mean
rather than a single window.

The tolerances are 1.5 SD on a between-window spread estimated from **three distinct
occasions** on one venue, so they are softer than they look. The standing inference confound
is untouched.

---

## Out of sample again, with the pooled protocol — fixed before the recording

**Fixed 2026-08-08 15:04 UTC, before a single new row exists.** `cfg/lob_counts.yaml` met
all three pooled targets in-sample. Every prior in-sample success in this project has failed
out of sample, and the pre-registration that produced this one committed to a fresh
recording as the next step. This is it.

### The model is frozen

`cfg/lob_counts.yaml` exactly as shipped — `limit_rate` 3.381, `churn_rate` 1.900,
`damping_gamma` 0.45, driver variance 11.81. **Nothing may be refitted for this test, ever.**
A test asserts the shipped values, and if they change after this window is recorded the
result must be withdrawn rather than re-scored.

### A correction to the tolerance basis, made before recording

Previous blocks used a **between-window** SD pooled over seven windows — but five of those
seven were minutes apart on one Sunday morning, so that spread is mostly *within*-occasion
and understates how much these quantities move between genuinely separate occasions.

Computed properly over the three distinct occasions:

| quantity | between-**window** SD (used before) | between-**occasion** SD (used here) |
|---|---|---|
| `corr(depth, arrivals)` | 0.0516 | **0.0727** |
| `corr(depth, cancels)` | 0.0585 | **0.0814** |
| `corr(arrivals, cancels)` | 0.0243 | **0.0303** |

Predicting a *new occasion* is a between-occasion question, so the wider figure is the right
one and tolerances below use it. This **loosens** the test relative to earlier blocks, which
is stated plainly because loosening a tolerance is the direction that flatters a model.

### The known weakness of this test, declared in advance

The between-occasion SD is estimated from **three occasions, all mornings** (Thu ~07:00,
Sat ~08:51, Sun ~08:12–09:04 UTC). This recording is **Saturday afternoon**, ~15:10 UTC —
a time of day with no prior data at all.

So the tolerance may understate the true variability across times of day, and **if the test
fails, "the tolerance was estimated from mornings only" is a live explanation.** Saying so
now means it cannot be produced afterwards as a rescue.

### Protocol

**Three windows**, each 8 minutes, at 10-minute starts, five symbols concurrently —
`BTCUSDT`, `ETHUSDT`, `SOLUSDT`, `XRPUSDT`, `DOGEUSDT` — identical to every prior capture.
Three rather than five because within-morning windows proved highly correlated, so they
estimate one occasion's mean and more would add little.

The occasion's value is the mean over the three windows of the five-symbol mean. Any window
with a sequence gap or suspect rows in any symbol is excluded whole and the exclusion
reported; fewer than three usable windows is reported rather than worked around.

### Predictions

Each asks whether the frozen model predicts the new occasion within 1.5 between-occasion SD.

**BS — the arrival side.** |model −0.1832 − new occasion mean| < **0.109**.

**BT — the cancellation side.** |model −0.0752 − new occasion mean| < **0.122**.

**BU — the co-movement.** |model +0.9154 − new occasion mean| < **0.046**.

**BU is the one at risk.** Its tolerance is the tightest in absolute terms, the model sits
1.42 SD from the pooled mean on it already, and it is the signature every model in this
project has been weakest on. The three prior occasions read +0.9529, +0.9035 and +0.9586; if
the new one lands near the top of that range the model misses.

**BV — the data is clean.** Three usable windows, 480 rows each, no gaps and no suspect rows.

### What each outcome means

| BS/BT | BU | reading |
|---|---|---|
| pass | **pass** | The model predicts a genuinely new occasion — different day, different time of day — within known variability. That would be **the first out-of-sample success in this project**, and the pooled protocol would be vindicated as the fix rather than a better story. |
| pass | **fail** | The depth structure generalises and the co-movement does not. Given the co-movement is the market's steadiest signature and the model's weakest, that would locate the remaining error precisely rather than diffusely. |
| **fail** | — | The model does not generalise across occasions even at loosened tolerances. With the pooled protocol already applied, that would leave the target's instability and the model's structure as the two candidates, and would need a third occasion type to separate them. |

### What even a full pass would not establish

One venue, crypto spot, four occasions, 8-minute windows, one model seed-ensemble. The
standing inference confound — both flows inferred from net depth changes — is untouched. And
a pass at 1.5 between-occasion SD is a weaker statement than a pass at the tighter
between-window figure earlier blocks used; the loosening is declared above and should be
carried into how any pass is described.
