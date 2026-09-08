# Crypto Wallet Transaction Analyzer

A read-only crypto wallet analyzer: paste a public wallet address, and it fetches
the on-chain transaction history, classifies each event (transfer, swap, staking
reward, airdrop, gas fee), and computes FIFO cost basis, realized/unrealized
gain, income totals, and gas paid - with every aggregate number drillable back
to the underlying transactions and a link to a public block explorer.

Full requirements and design rationale: [project.MD](./project.MD).

## Status

Backend and frontend are both built and tested (Go: `go test -race ./...`;
frontend: `vue-tsc -b && vite build`, verified against a real browser).
Not yet deployed - see [DEPLOY.md](./DEPLOY.md) for Render/Vercel setup.
Design decisions are recorded as comments in the code they affect.

## Layout

```
backend/    Go API (target: Render)
  cmd/server/            HTTP entrypoint - env config + wiring only
  internal/domain/       normalized types shared across all layers (Transaction, TxType, ...)
  internal/provider/     BlockchainProvider/PriceProvider interfaces + shared retry helper
    etherscan/              concrete BlockchainProvider (Etherscan V2 API)
    coingecko/              concrete PriceProvider (CoinGecko)
  internal/ethaddr/      EIP-55 address validation
  internal/classify/     transaction classification logic
  internal/costbasis/    FIFO cost-basis engine
  internal/aggregate/    bounded worker pool + deduped price lookups
  internal/analysis/     orchestrates fetch -> classify -> price -> cost-basis
  internal/httpapi/      HTTP handlers, CORS, rate limiting (thin - no business logic)
frontend/   Vue 3 + TypeScript app (target: Vercel)
  src/types.ts, api.ts, format.ts
  src/components/  WalletForm, TotalsSummary, AssetBreakdown, IssuesList
```

## Non-negotiables (see project.MD for full detail)

- Never touches a private key or signature - public address input only.
- No floats for token amounts or money - `shopspring/decimal` throughout,
  string-encoded end to end including the frontend's TypeScript types.
- Every aggregate figure must be traceable back to its source transactions
  and priced with documented provenance (source, timestamp, granularity).
- v1 chain scope is intentionally Ethereum + Polygon - see project.MD for why
  non-EVM chains aren't "one more chain" to add.
