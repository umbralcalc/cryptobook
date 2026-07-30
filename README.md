# cryptobook

Limit order book microsimulation calibrated against **crypto** market microstructure
data, analysed for market stability.

Every output here is a **counterfactual about market state** — spread response,
depth recovery, queue dynamics. Nothing in this repo is a directional claim about
price, and no dashboard shows a price path.

## Scope: crypto only, and deliberately so

**This project makes claims about crypto spot markets and nothing else.** The data is
Binance public spot depth-diff and trade streams, recorded from unauthenticated endpoints
under a licence that permits use but not redistribution.

**What that costs, stated up front.** The model's lineage is Santa Fe / `lobsim`, which is
an equity model. Testing it only on crypto means a negative result is ambiguous between
"the model is wrong" and "the market is different." `pkg/replication` narrows that — the
failure is not specific to one symbol or one window *within* crypto spot — but it cannot
close the asset-class version of the question, and this project does not claim to.

**What crypto gives back.** Anyone can record their own segment from public endpoints in
minutes, with no account, no gate and no approval. So claims here are stated as **bounds
that must hold on any independently recorded segment** rather than as point values pinned
to one capture — falsifiable by a stranger with eight minutes, which is a stronger form of
reproducibility than a committed fixture provides.

## Where things are

| File | What it holds |
|---|---|
| [PLAN.md](PLAN.md) | The phased plan: spikes, decision gates, and standing constraints. |
| [DECISIONS.md](DECISIONS.md) | Resolved gates with the evidence that forced them, plus what is still open. |
| [PREREGISTRATION.md](PREREGISTRATION.md) | Thresholds fixed *before* the runs that test them. |
| [STOCHADEX_GAPS.md](STOCHADEX_GAPS.md) | Verified capability gaps in the engine, recorded as they are hit. |
| [CLAIMS.md](CLAIMS.md) | Every claim re-derivable from this repo alone, bound to the test that enforces it. **Generated — do not edit.** |
| [cfg/](cfg/) | The models and inference. Pure config — no Go anywhere in either. |

## Status

**Phase 0 complete** (trust foundation): CI runs the suite under `-race` on every
push, and the claim mechanism is live. (Phase 0 also stood up a Postgres service
container, as PLAN.md requires; it was removed once Gate 3.4 established that the
Phase 3 premise needing it does not hold — see [DECISIONS.md](DECISIONS.md).)

**Phase 1 complete, Gate 1.2 resolved. Phase 2 reached real data and stopped
there, on purpose.** Thirty-two claims are bound and CI-enforced, plus four cross-segment
bounds that need recorded data and so sit outside the generated page.
The short version:

- The minimal LOB generator ([cfg/lob_generator.yaml](cfg/lob_generator.yaml)) is
  pure config, and behaves: depth responds correctly to both the arrival and the
  cancellation rate, and the touch thins as market orders consume it.
- **All three parameters are identified** — the likelihood surface peaks at the true
  value of each, including the cancellation rate, which is identifiable only through
  its coupling to queue depth.
- **The first sampler was degenerate.** ESS = 1.00 of 16 draws, so the weakly
  identified rate was not estimated at all and no parameter had usable uncertainty.
  That is PLAN.md's documented switch signal.
- **SMC fixes it.** Same model, same observables, only the inference layer changed:
  all three rates recover within tolerance at every setting (worst 7.6%, against
  113%), and the truth lies within 1.8 posterior standard deviations everywhere — so
  Phase 2 can deliver parameters *with* uncertainty.

Honest caveats, all pinned as claims: ESS peaks at only ~8% of the particles, and
the posterior's *interval* degrades on two axes — **raising the round count** and
**lengthening the window** both make it tighter and wronger. Full reasoning and the
like-for-like comparison are in [DECISIONS.md](DECISIONS.md).

### Phase 2: the model form does not transfer to crypto, on five instruments

First contact with real market data was decisive and negative — and crypto is now the
whole domain, so this is the project's central result rather than one market of two.

It rested on a single 8-minute BTCUSDT capture until bounds were **pre-registered and then
checked against five symbols recorded concurrently**, spanning a 37× range in 24h quote
volume. All four bounds hold:

| | bound, fixed before recording | worst of five | |
|---|---|---|---|
| `corr(depth, cancellations)` | < +0.2 on every symbol | −0.015 (XRPUSDT) | ✓ |
| `corr(arrivals, cancellations)` | > +0.9 on every symbol | +0.940 (SOLUSDT) | ✓ |
| `Var/Mean` of counts | > 10 on every symbol | 2.9e3 (BTCUSDT) | ✓ |
| co-movement vs liquidity | lower on the thinnest symbol | −0.032 | ✓ |

**The model requires `corr(depth, cancellations)` strongly positive. It is negative on
every instrument tested.** That is the reading that stopped Phase 2, and it is now a
property of crypto spot markets rather than of one capture.

The fourth row passes its test but not its reasoning: the ordering is **not** monotonic in
liquidity — the lowest co-movement of the five is SOLUSDT, third by volume — so
co-movement tracking market-making intensity is unsupported. Reported as a pass with that
caveat, because a two-point comparison is what was pre-registered.

A confound was declared before recording and duly appeared: dispersion spans 2.9e3 to
3.6e8 across symbols, almost entirely because one fixed lot size meets instruments whose
unit prices differ by orders of magnitude. That is a fact about the measurement, not the
market, and no cross-symbol dispersion comparison is made. Declaring it in advance is what
keeps the table honest.

The first line is the coupling the whole synthetic identification result rests on,
and [PREREGISTRATION.md](PREREGISTRATION.md) fixed the consequence before any data
existed: if it fails, Phase 2 stops rather than being tuned. It failed, so **no
parameters are fitted** — the machinery would have produced a confident answer,
which is exactly why the bar was set in advance.

**This is one market, and it is not the one the model was designed for.** The model's
lineage is an equity one, so "wrong market" is a live alternative reading to "wrong
model" — narrowed by the replication above, which rules out the within-crypto version,
but not closed.

What is there instead is quote churn: arrivals and cancellations moving in lockstep,
a mechanism the model does not have. Returning to the domain model is a maintainer
decision — see [DECISIONS.md](DECISIONS.md).

### Phase 4: one of four stability outputs is supportable

Spike 4.2 asks which counterfactual outputs the model can actually answer, and says
to mark the rest absent rather than approximate them. Three cannot be answered:
spread response and queue position need prices and order identity the model does not
have, and "liquidity surviving a large marketable order" needs orders that can sweep
the book, which these cannot. The fourth — **depth recovery after a liquidity
event** — works, and yields the one genuinely useful result: recovery *time* is set
by the cancellation rate, while the *level* recovered to is set by the ratio of both
rates. Different levers.

Stated plainly: the framing PLAN.md wanted these outputs to carry is thin, because
three need structure the minimal generator lacks and the fourth describes a model
Spike 2.2 showed does not fit real data.

### The domain model now has prices

Spike 2.2 sent the project back to the domain model, and Spike 4.2 found three of
four stability outputs unsupportable — both pointing at the same missing structure.
[cfg/lob_priced.yaml](cfg/lob_priced.yaml) adds a price ladder, an **emergent**
spread and marketable orders that walk the book, still as pure config. That takes
Spike 4.2 from one supportable output to three.

**Prices did not fix Phase 2, and that is now measured.** Running the same three
diagnostics against both synthetic models gives a control the real-market result had
been missing: the coupling is present by construction in both, and the diagnostic
finds it (+0.37, +0.64). So the crypto near-zero reading is a real absence, not a blind
measurement. But the priced model's churn correlation stays at −0.04 against crypto's
+0.98 — the domain-model step did what it was built for and
nothing for the failure that stopped Phase 2. **Quote churn is now the only
candidate left standing.**

**The churn hypothesis was pre-registered, then tested.** A shared activity driver
couples arrivals to cancellations (+0.62) but does **not** break the depth/cancellation
coupling (+0.60), which is the prediction that mattered. It failed, as pre-registered.

**Then removing the last depth-coupled term made the model's correlations line up — and
broke the model.** Predictions were committed beforehand about what the change would
*cost*, not what it would fix. `corr(depth, cancels)` fell to +0.009 and co-movement
rose to +0.950 — while the book stopped conserving (depth nearly triples over a run,
where a conserved book gives ~1) and the spread collapsed to a constant, destroying the
spread-response output.

Attrition turned out to be the model's only depth-stabilising force, so the coupling
is fixable only by deleting the mechanism that conserves the book. **Neither form
works**, and the next candidate is a stabiliser acting on arrivals rather than
cancellations.

This is the clearest case in the project for pre-registering *costs*: without those
predictions the write-up would have read "the churn mechanism works", with every
correlation in it true. Full scoring, including a fault in one of my own outcome
tables, in [PREREGISTRATION.md](PREREGISTRATION.md).

**Order identity is deferred, for a reason worth naming:** a FIFO queue needs to
assign simultaneous arrivals to free slots, which is an allocation problem requiring
a scan the expressions DSL cannot express. Recorded in
[STOCHADEX_GAPS.md](STOCHADEX_GAPS.md) as a verified engine gap rather than a
preference. The priced book is **not yet calibrated** against either market — that
is the next measurement, not a result.

### And then depth-dependent arrivals held

Predictions were committed before the model existed. Moving the stabiliser to the
**arrival** side — posting into a level slowing as that level fills, with cancellation
left as pure churn — keeps the coupling broken while the book stays conserved:

| | attrition model | arrivals model |
|---|---|---|
| depth drift | 2.72 | **1.008** |
| `corr(depth, cancels)` | +0.638 | **−0.002** |
| `corr(arrivals, cancels)` | +0.950 | +0.897 |
| spread | collapsed to a constant | **2.17 ± 0.41** |

Predictions G, H and I were scored on this and all three pass; **H is the only one that
was genuinely uncertain**, since depth is now anti-correlated with arrivals and arrivals
share the activity driver with cancellations, so the indirect path could have brought
the coupling back. It did not.

**This is a statement about the model, not about markets.** Nothing in this section is
compared against market data, and the parameters are chosen values rather than fitted
ones. A model-internal trade-off does come out of it, and it needs no data to state:
cancellation here removes a fraction of **resting** volume, so any depth-stabilising work
it takes on shows up as a depth/cancellation correlation, while depth-damped arrivals put
that same work into a depth/arrival correlation. The book needs a brake, and in this
vocabulary each available brake couples depth to one of the two flows.

The candidate mechanism remains posting that responds to **queue position or
time-to-fill** rather than resting volume — which needs order identity, and the
stochadex DSL cannot express it
([STOCHADEX_GAPS.md](STOCHADEX_GAPS.md) entry 1).

**Gate 3.4 (Invariant A) is open, with evidence gathered and no branch selected** —
the plan reserves it for the maintainer. Three of Phase 3's premises turned out not
to hold (there is no streaming source stanza, no data-agreement layer, and the
Postgres schema is fixed rather than negotiated), and windowed re-run throughput was
measured at ~1100 rows/compute-second — about 100x headroom against a crypto depth
feed, so compute is not what decides it. See [DECISIONS.md](DECISIONS.md).

Two silent engine failures were found on the way, reported upstream, and **fixed in
stochadex v0.13.1** — an oversized `window_data_history_depth` that inverted the
likelihood rather than merely weakening it, and a nil-map panic on a comparison model
with no params. This repo pins `v0.13.1`; both workarounds are gone and no claim's
number moved.

### The models are config, not code

Every model in [cfg/](cfg/) is data: partitions, params, `expressions:` and the
`macros:` analysis tier, resolved and run in-process with no Go and no toolchain.
`pkg/lob` and `pkg/recovery` contain no model — they run a config, read its output,
and assert on it.

## Data sources and access policies

One dataset is used: Binance public spot market data, recorded from unauthenticated
endpoints. What follows is quoted from the provider's own documents, retrieved 2026-07-29.
Where a policy could not be retrieved, that is stated rather than filled in from memory.

### Binance spot market data — crypto

`BTCUSDT`, public spot depth-diff and trade WebSocket streams plus the REST depth
snapshot, recorded live. Public endpoints, no API key, no authentication.

Binance's **Terms of Use** (effective 21 July 2026, the ADGM Binance entities)
bind a user on access, not only on registration:

> "By accessing the Binance Platform and/or using the Binance Services, you: (i)
> agree that you have read, understood and accepted the Agreement; (ii) you
> acknowledge and agree that you will be bound by and will comply with the
> Agreement, as updated and amended from time to time [...]"

That matters here because this project holds no Binance account: recording public
market data is nonetheless "accessing the Binance Platform", so the Agreement is
engaged regardless.

The intellectual-property chain runs as follows. "Intellectual Property Rights" is
defined to include **database rights** explicitly:

> "'Intellectual Property Rights' means: (a) copyright, patents, **database
> rights** and rights in trade marks, designs, know-how and confidential
> information (whether registered or unregistered) [...]"

"Binance IP" then covers what is delivered through the service:

> "'Binance IP' means the Created IP and all other Intellectual Property Rights
> owned by or licensed, on a sub-licensable basis, to us [...] **and which are
> provided by us to you in the course of providing you with the Binance
> Services**."

Clause 26 keeps ownership with Binance, and Clause 27 states the licence granted:

> "26. BACKGROUND IP — The Binance IP shall remain vested in Binance."

> "27. LICENCE OF BINANCE IP — We grant to you a **non-exclusive licence for the
> duration of the Agreement**, or until we suspend or terminate your access to the
> Binance Services, whichever is sooner, to use the Binance IP, excluding the Trade
> Marks, **solely as necessary to allow you to receive the Binance Services for
> non-commercial personal or internal business use**, in accordance with the
> Agreement."

**What this establishes.** Market data delivered over the depth and trade streams is
provided in the course of providing the Binance Services and falls within Binance
IP, database rights included. The licence to use it is limited to *non-commercial
personal or internal business use* and lasts only for the duration of the
Agreement. **It does not extend to redistribution**, and "internal business use" is
the opposite of publishing a derived dataset.

The official API documentation repository (`binance/binance-spot-api-docs`) carries
no `LICENSE` file and no separate data terms. Its only relevant statement is about
API support rather than data rights:

### Redistribution: the licence does not permit it, so no data is committed

Binance grants use "to receive the Binance Services", limited to "non-commercial
personal or **internal business use**", and its definition of Intellectual Property
Rights names **database rights** explicitly. Being *derived and aggregated* does not
settle it either way.

**So no market data, raw or derived, is committed to this repository.** `testdata/` is
git-ignored. The collector, the state spine and the diagnostics are all here; the data
itself has to be recorded from source.

What that costs is stated plainly rather than glossed: the Spike 2.2 crypto diagnostics
are the one set of results in this repo that **CI cannot re-check**. Their tests skip on
a fresh clone and enforce the claims only for someone holding a recorded segment, and
their numbers live in [DECISIONS.md](DECISIONS.md) as prose rather than in the generated
[CLAIMS.md](CLAIMS.md). That is a real weakening of the claim↔test↔result bond
everything else relies on, accepted because the alternative is redistributing data under
a licence that forbids it.

Publishing a *summary statistic* is a different act from publishing a dataset, so the
crypto findings themselves are reported normally. It is the row-level fixtures that are
not shipped.

### Reproducing the market-data results

```bash
# Crypto — records ~8 minutes of live Binance BTCUSDT depth + trades.
go run ./cmd/record-feed -symbol BTCUSDT -duration 8m -out testdata/btcusdt_depth.log

go test ./pkg/crypto/ -v   # skips without the data, enforces with it

# The cross-segment replication check — five symbols over one window, ~8 minutes.
for s in BTCUSDT ETHUSDT SOLUSDT XRPUSDT DOGEUSDT; do
  go run ./cmd/record-feed -symbol $s -duration 8m -out testdata/seg_$s.log &
done; wait

go test ./pkg/replication/ -v   # enforces the four pre-registered bounds
```

The replication bounds are the one place where non-redistributable data does **not** cost
reproducibility: they are stated as bounds any fresh recording must satisfy, so anyone can
regenerate the input from public endpoints and falsify them in eight minutes.

Obtaining the data is your own act under the licence above; this repository does not
redistribute it on your behalf.


## Working here

```bash
go build ./...
go test ./... -race -count=1     # what CI runs
go run ./cmd/gen-claims          # regenerate CLAIMS.md after changing a claim
```

Adding a claim: expose `ObservedBehaviour() []claims.Claim` from a non-test file in
the phase package, consume it from a test that runs one subtest per claim ID,
register the provider in [internal/claimset](internal/claimset/claimset.go), then
regenerate. A claim will not validate without a stated dataset, a stated
limitation, and a binding test — see [pkg/claims](pkg/claims/claims.go) for why
each is required.
