# Crypto Wallet Transaction Analyzer

A read-only crypto wallet analyzer: paste a public wallet address, and it fetches
the on-chain transaction history, classifies each event (transfer, swap, staking
reward, airdrop, gas fee), and computes FIFO cost basis, realized/unrealized
gain, income totals, and gas paid - with every aggregate number drillable back
to the underlying transactions and a link to a public block explorer.

Full requirements and design rationale: [project.MD](./project.MD).

## Status

Early scaffolding. See `project.MD` for the full spec; design decisions made
so far are recorded as comments in the code they affect (e.g.
`backend/internal/domain/transaction.go`).

## Layout

```
backend/    Go API (target: Render)
  cmd/server/       HTTP entrypoint
  internal/domain/  normalized types shared across all layers (Transaction, TxType, ...)
  internal/provider/    interfaces for blockchain + price data sources
  internal/classify/    transaction classification logic
  internal/costbasis/   FIFO cost-basis engine
  internal/aggregate/   worker pool + result aggregation
  internal/httpapi/     HTTP handlers (thin - no business logic)
frontend/   Vue app (target: Vercel)
```

## Non-negotiables (see project.MD for full detail)

- Never touches a private key or signature - public address input only.
- No floats for token amounts or money - `shopspring/decimal` throughout.
- Every aggregate figure must be traceable back to its source transactions
  and priced with documented provenance (source, timestamp, granularity).
