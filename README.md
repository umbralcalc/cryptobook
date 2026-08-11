# cryptobook

A limit-order-book microsimulation, written entirely as configuration, calibrated against
Binance BTC/USD spot data and analysed for market stability. Every output is a counterfactual
about market state — spread response, depth recovery, queue dynamics — never a directional claim
about price.

The model dynamics live in YAML (`cfg/*.yaml`), run by the
[stochadex](https://github.com/umbralcalc/stochadex) engine; there is no bespoke Go in any model.
The discipline is that predictions are fixed *before* the config that tests them exists, every
published number comes from a CI-enforced test and is generated into `CLAIMS.md`, and every
decision is recorded with the evidence that forced it.

## What was learned

**[WRITEUP.md](WRITEUP.md) is the full account.** In short: the parametric model form does not
transfer from synthetic data to real crypto spot flow — established against a pre-registered bar,
with no parameters fitted after the failure. Rebuilding the domain model around a shared latent
activity driver (quote churn) produced [`cfg/lob_counts.yaml`](cfg/lob_counts.yaml), which
reproduces all three of the market's pooled correlation signatures at once. Out of sample, that
model's **co-movement signature replicates** across occasions while its **arrival–depth coupling
does not**. The result is honest and located: a real mechanism captured, a specific remaining
instability, and a clear statement of what one venue and eight-minute windows can and cannot
support.

## Where things are

| File | What it holds |
|---|---|
| [WRITEUP.md](WRITEUP.md) | The synthesis — what was established, what is not, and the one open question. |
| [CLAIMS.md](CLAIMS.md) | Every claim re-derivable from this repo alone, bound to the test that enforces it. **Generated — do not edit.** |
| [DECISIONS.md](DECISIONS.md) | Decisions and results, with the evidence that forced them; market-comparison numbers that need non-redistributable data live here. |
| [PREREGISTRATION.md](PREREGISTRATION.md) | Thresholds fixed *before* the runs that test them. |
| [cfg/](cfg/) | The models and inference. Pure config — no Go. |

The best model is [`cfg/lob_counts.yaml`](cfg/lob_counts.yaml);
[`cfg/lob_counts_split.yaml`](cfg/lob_counts_split.yaml) is the same model decomposed into four
modular partitions (`activity`, `flows`, `book`, `observables`), verified by `pkg/split` to
reproduce the monolith.

## Data

One dataset: Binance public spot depth-diff and trade streams, recorded live from unauthenticated
endpoints. The licence permits use but not redistribution, so **no market data — raw or derived —
is committed**; `dat/` and `testdata/` are git-ignored. Record your own from public endpoints in
minutes:

```bash
go run ./cmd/record-feed -symbol BTCUSDT -duration 8m -out testdata/btcusdt_depth.log
```

Because the data cannot be shipped, the market-comparison tests skip on a fresh clone and their
numbers live in `DECISIONS.md` rather than the generated `CLAIMS.md`. Claims are stated as
**bounds any independently recorded segment must satisfy**, so anyone can regenerate the input and
falsify them in eight minutes.

## Working here

```bash
go build ./...
go test ./...                    # merge gate; -race runs nightly
go run ./cmd/gen-claims          # regenerate CLAIMS.md after changing a claim
```

Adding a claim: expose `ObservedBehaviour() []claims.Claim` from a non-test file in the phase
package, consume it from a test that runs one subtest per claim ID, register the provider in
[internal/claimset](internal/claimset/claimset.go), then regenerate. A claim will not validate
without a stated dataset, a stated limitation, and a binding test — see
[pkg/claims](pkg/claims/claims.go) for why each is required.
