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
