# cryptobook — what was learned

A config-only limit-order-book microsimulation, validated against Binance BTC/USD spot data
under a discipline designed to make the reporting honest: every prediction is pre-registered
in [PREREGISTRATION.md](PREREGISTRATION.md) *before* the config that tests it exists, every
published number is produced by a CI-enforced test and generated into [CLAIMS.md](CLAIMS.md)
rather than written by hand, and every decision — including the ones that went against the
project's own hopes — is recorded in [DECISIONS.md](DECISIONS.md). The model dynamics live
entirely in YAML (`cfg/*.yaml`), run by the [stochadex](https://github.com/umbralcalc/stochadex)
engine; there is no bespoke Go in the model.

**The one-paragraph version.** The parametric model form this project started with does **not**
transfer from synthetic data to real crypto spot flow — established, not argued, against a
pre-registered bar. Rebuilding the domain model around a shared latent activity driver (quote
churn) produced `cfg/lob_counts.yaml`, which reproduces all three of the market's pooled
correlation signatures at once. Out of sample, that model's **co-movement signature replicates**
across occasions — the market's steadiest signature and every earlier model's weakest — while its
**arrival–depth coupling does not**, failing narrowly on the second held-out occasion. The result
is honest and located: a real mechanism captured, a specific remaining instability, and a clear
statement of what one venue and eight-minute windows can and cannot support.

---

## The arc

### Phase 1 — the parameters are recoverable (trust foundation)
Sequential Monte Carlo recovers the model's rates from its own generated order flow, including
the cancellation rate, which is identifiable *only* through its coupling to queue depth. An
earlier importance sampler collapsed to an effective sample size of ~1 and could not estimate the
weakly-identified rate; SMC fixed it. This is the trust bond every downstream claim leans on: the
inference machinery works when the model is exactly specified. (`pkg/recovery`, `pkg/windowing`.)

### Phase 2 — the model form does not transfer, and the rebuild that followed
This is the pivot of the project. Against a held-out Binance segment, the original model's central
assumption — that cancellation intensity scales with resting depth — **failed**: cancellations are
essentially uncorrelated with depth, and arrivals and cancellations move in near-lockstep
(+0.98 second-by-second), a *missing mechanism* (quote churn: makers pulling and re-posting
together), not a mistuned rate. The pre-registration had fixed the consequence in advance — "if
this fails, Phase 2 stops rather than being tuned" — so **no parameters were fitted** to a market
whose structure the model did not contain. (`pkg/damping` and Spike 2.2 in DECISIONS.md.)

The rebuild added a shared latent activity driver to both flows. A long mechanism hunt then
converged on a quantitative account of the one signature every version struggled with, the
arrival–cancellation co-movement: it is bounded by a **Poisson ceiling**,
`N·Var(A) / (E[A]² + N·Var(A))`, and falls short of it by a **saturation penalty** that a matched
pair of configs proved is a property of the *driver's spread*, not of the ceiling.
(`pkg/ceiling`.) The resulting model, `cfg/lob_counts.yaml`, meets all three pooled targets —
`corr(depth, arrivals)`, `corr(depth, cancellations)`, `corr(arrivals, cancellations)` —
simultaneously, at parameters computed from the ceiling algebra rather than fitted.

### The out-of-sample test — the honest crux
The model was frozen (`limit_rate` 3.381, `churn_rate` 1.900, `damping_gamma` 0.45, driver
variance ~11.8) and its three reference correlations and tolerances fixed *before* any fresh data
was recorded. Two genuinely new occasions have now been scored, each three windows × five symbols,
against those pre-registered bounds — the segments are Binance data that cannot be redistributed,
so the scoring lives in `pkg/oospool` and DECISIONS.md rather than CLAIMS.md.

| signature | occasion 2 (Sat afternoon) | occasion 3 (Sun morning) | verdict |
|---|---|---|---|
| `corr(depth, arrivals)` | pass (0.78 SD) | **fail (1.58 SD)** | **not stable across occasions** |
| `corr(depth, cancellations)` | pass (0.56 SD) | pass (0.99 SD) | holds, loosely |
| `corr(arrivals, cancellations)` | pass (1.22 SD, loosened) | **pass (0.24 SD)** | **holds, and cleanly in-distribution** |

The co-movement — the entire point of the counts route, and the signature pre-registered as *most
at risk* — passed most comfortably of any signature on any occasion, on the occasion squarely
inside the tolerance's basis. The arrival–depth coupling did not replicate: it lost its sign
entirely in one window and pulled the occasion mean off the model by more than the bound. This is
neither the clean out-of-sample success a full pass would have been nor a collapse — one signature
failing narrowly while the load-bearing one holds. Nothing was refitted or the bound widened to
rescue it.

### Phase 3 — offline calibration, explored to its boundary
Under Gate 3.4 (inference stays downstream; the engine owns forward simulation), calibration is
offline on recorded streams. Three synthetic blocks map exactly what the offline machinery can do
with the shipped inference tier: on a **clean** IID-gamma driver a negative-binomial SMC recovers
the driver's dispersion well (CA–CC); making the driver **persistent** biases the recovered
dispersion downward on finite windows and is blind to the persistence itself (CH); the arrival
**damping** suppresses the arrival stream's dispersion below the cancellation's (CI). The two
confounds stack, so the full model presents an offline calibration with three different
dispersions where it assumes one. The blocker is named precisely: not a wiring gap (the offline
path works) but a **misspecified likelihood** — recovering the truth needs a state-space filter
carrying the driver as a latent state and modelling the damping, which is a modelling phase, not
attempted here. (`pkg/offline`.)

### Phase 4 — stability outputs, and Arrow
All four of the counterfactual stability outputs are answered: depth recovery after a liquidity
event (`pkg/stability`), spread response and liquidity survival on the priced ladder
(`pkg/priced`), and queue-position distribution across tick regimes on a per-order FIFO book
(`pkg/queue`) — the last of which was twice recorded as "not answerable" before a correction showed
the blocking capability was a formulation error, not an engine gap. Arrow egress (Spike 4.1) is
implemented upstream in the opt-in `arrowstore` module; adoption was declined because this project
analyses runs in-process and exports to no columnar tool.

### Phase 5 — not entered
ONNX was gated on Spike 2.2 identifying a single failing parametric component to replace with a
learned one. Spike 2.2 instead found the model form wrong across the board, which the rebuild
addressed structurally; a learned component bolted to a structurally-wrong model would produce an
unconvincing artefact. Phase 5 does not proceed on this evidence.

---

## What this can and cannot support

**Established.**
- The parametric model form does not transfer to crypto spot, measured against a pre-registered
  bar, with no parameters fitted after the failure.
- Quote churn — a shared latent driver of arrivals and cancellations — is the missing mechanism,
  and a config-only model built around it reproduces all three pooled correlation signatures.
- The co-movement signature replicates out of sample across two genuinely separate occasions.
- The co-movement's ceiling-minus-saturation account, and that the saturation tracks the driver's
  spread (a designed matched-pair comparison, model-internal, CI-checked).

**Not established, and stated as such.**
- That the model is *right*. It meets three targets and replicates one of them out of sample; the
  arrival–depth coupling does **not** replicate.
- Anything beyond **one venue, crypto spot, four occasions, eight-minute windows, one
  seed-ensemble**. Three of four occasions are mornings; the tolerances are estimated from a spread
  that thin.
- Freedom from the **standing inference confound**: both order flows are *inferred from net depth
  changes* in a diff feed, not observed as messages (`pkg/feed/bucket.go`). A diff feed cannot see
  why volume left a level, so arrivals and cancellations are reconstructions, not observations. Every
  correlation above inherits this.
- Real-data calibration of the churn model, which the inference tier cannot currently support (see
  Phase 3).

---

## The open question

The one live scientific question is whether the arrival–depth coupling's out-of-sample instability
is a property of **the market** (the weak coupling genuinely varies occasion to occasion) or of
**the instrument** (an eight-minute window is too short to estimate a small correlation stably). Two
occasions cannot separate them. It needs a third occasion or materially longer windows — elapsed
time and fresh recordings, not more computation. Everything else is either closed, ruled out, or
named as a boundary reached.

---

*66 CI-enforced claims across 25 packages. Market-comparison numbers, which need non-redistributable
Binance segments, live in [DECISIONS.md](DECISIONS.md); everything re-derivable from this repository
alone is in [CLAIMS.md](CLAIMS.md). Predictions were fixed before their configs existed in
[PREREGISTRATION.md](PREREGISTRATION.md).*
