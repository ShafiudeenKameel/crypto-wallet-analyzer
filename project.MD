# Claude Code Prompt: Crypto Wallet Transaction Analyzer

Paste everything below into Claude Code to kick off the project.

---

I'm building a portfolio project to prepare for a backend SDE interview at Bitwave, a crypto accounting/compliance company. I want this to be a genuinely solid, production-minded piece I can show in an interview — not a toy script.

## What the product does

A website where a user pastes a **public crypto wallet address** (read-only — no private key, no wallet connection, no signing, ever) and the system:
1. Fetches the wallet's full transaction history from the blockchain.
2. Classifies each transaction (transfer, swap, staking reward, airdrop, gas fee, etc.).
3. Calculates cost basis (FIFO) per asset and computes:
   - Total spent
   - Total received
   - Realized gain/loss
   - Unrealized gain/loss (current holdings vs. cost basis)
   - Income events ("revenue" — staking rewards, airdrops)
   - Total gas fees paid
4. Displays all of this on a clean results dashboard.

## Scope note: this is a single-user demo, zero infrastructure cost

- No persistent database for v1 — recompute everything fresh on each request. This simplifies deployment (no free-tier Postgres to manage) and is a reasonable tradeoff for a demo one person will use occasionally. Revisit persistence as a v2 feature if this evolves beyond a demo.
- Keep results in memory for the duration of a single request only.
- This does NOT relax the parallelization or security requirements below — if anything, without a database, doing the fetch efficiently and correctly on every single request matters even more, since there's no cache to fall back on.

## Architecture (agreed direction)

- **Frontend:** Vue.js — a simple page with a wallet address input and a results dashboard. **Hosted on Vercel** (free tier).
- **Backend:** Go — REST (or GraphQL) API that orchestrates everything. **Hosted separately from Vercel** — on Render's free tier (simplest to get running; free tier sleeps after inactivity, acceptable for a demo) — because Vercel's serverless functions have execution time limits (10s on free tier) that are a poor fit for the fetch+process work below.
- **Data fetch layer:**
  - Blockchain data provider: Etherscan API (free tier) for raw transaction history.
  - Historical price API: CoinGecko (free tier) for fair value at each transaction's exact block timestamp.
- **Processing engine:** classification logic + FIFO cost-basis engine + aggregation — all in-memory, per request.
- **Scope for v1: multi-chain-capable architecture, validated on two chains.** The provider interfaces (blockchain data fetch, price lookup) should take a `chain` parameter and contain nothing hardcoded to "Ethereum" specifically — but only **Ethereum mainnet + one EVM-compatible L2 (Polygon or Base)** need to actually be wired up and tested for v1. This proves the abstraction holds across chains without the full cost of supporting every chain.
  - Use a unified API where possible (e.g., Etherscan's V2 API covers 50+ EVM chains with one API key) so adding a chain later is mostly configuration, not new integration code.
  - **Explicitly out of scope, and why:** non-EVM chains (Bitcoin, Solana, Cardano) use fundamentally different address formats and data models (e.g., Bitcoin's UTXO model vs. Ethereum's account model) — supporting them isn't "one more chain," it's a separate integration per chain family. Also out of scope for v1: combining/deduplicating the same token across chains in aggregate totals (e.g., "USDC on Ethereum" and "USDC on Polygon" are different contract addresses and should be tracked distinctly, not silently merged) — if we later show a combined total, this needs explicit, deliberate handling, not an accidental merge.
  - Each chain has its own native gas token (ETH on Ethereum, MATIC/POL on Polygon, etc.) — the gas-fee calculation and price lookups must be aware of which native token applies per chain.

## Parallelization design (important, not an afterthought)

This is a core engineering requirement, not a nice-to-have — and since there's no database in v1, getting this right on every single request matters even more:

- **Price lookups:** once transactions are fetched, price lookups for each transaction should be fanned out across a **bounded worker pool** (a fixed number of goroutines pulling jobs from a channel — e.g., 5-10 concurrent workers), not done sequentially and not fired with unbounded goroutines.
- **Paginated transaction fetching:** fetch multiple pages concurrently where the provider allows it, within the same bounded-concurrency approach.
- **Classification:** independent per transaction, so it's a natural fit for concurrent processing once data is fetched.
- **Critical constraint:** third-party APIs (Etherscan, CoinGecko) have rate limits. Concurrency must be bounded and include retry/backoff logic on rate-limit errors — "parallelize everything with no limit" will get us rate-limited or banned. Design the worker pool size to be configurable, and document the reasoning behind the chosen default.
- **Concurrency-safety is itself a security/accuracy requirement, not just a performance one:** when multiple goroutines write results back (e.g., into a shared slice or running total for cost-basis aggregation), this MUST be done safely — either by having each goroutine send its result through a channel to a single goroutine that owns the aggregation (preferred, idiomatic Go), or with a `sync.Mutex` protecting shared state if a channel-based design doesn't fit. A data race here wouldn't just crash — it could silently produce a WRONG financial number, which is arguably worse than a crash for this specific application. Please flag and explain the chosen synchronization approach explicitly rather than leaving it implicit.

## Validation & trust — the user must be able to verify the numbers themselves

This matters as much as the calculations being correct — a user (or interviewer) should never have to just "trust" a number on the dashboard:

- **Drill-down on every aggregate figure:** clicking "Total realized gain: $4,200" should show the exact list of underlying transactions that produced it.
- **Link every transaction to Etherscan:** each transaction in our data should link directly to its entry on a public blockchain explorer, so the user can independently verify the raw data against a trusted third-party source.
- **Show data provenance per number:** which price source and what exact timestamp was used for each fair-value lookup, visible on demand (e.g., a tooltip or expandable row) — not hidden.
- **Surface known limitations honestly in the UI:** e.g., if a price lookup only achieved daily granularity instead of exact block-time granularity, say so next to that specific number rather than presenting false precision.
- **Sanity-check counts:** display "Found and processed N transactions" so the user can cross-check that count against what they see themselves on Etherscan.

## Non-negotiable requirements

### Security
- **Never require or store a private key, seed phrase, or wallet signature.** Read-only public address input only — this is a hard design constraint, not a preference.
- Validate and sanitize the wallet address (proper Ethereum checksum validation) before it touches any downstream API call.
- All third-party API keys (Etherscan, CoinGecko) live only in backend environment variables / a secrets manager — never exposed to the frontend or in any API response.
- Rate-limit our own public-facing endpoint to prevent abuse of our third-party API quota.
- Enforce HTTPS and a strict CORS policy (only our frontend origin allowed).

### Accuracy
- **No floating-point arithmetic for token amounts or monetary values.** Use a decimal library (e.g., `shopspring/decimal` in Go) or big-integer arithmetic throughout — floats will silently corrupt cost-basis math over many transactions.
- Correctly handle per-token decimal places (ETH = 18 decimals, USDC = 6 decimals, etc. — this varies by ERC-20 contract and must be fetched/looked up, not assumed).
- Fully paginate transaction history from the data provider — don't silently truncate wallets with long histories.
- Use each transaction's actual **block timestamp** for fair-value price lookups, not the time our server happens to fetch it.
- Explicitly decide and document how failed transactions are handled (gas is still spent on a failed tx, so it's a real expense — don't silently drop it).
- Document any known simplifications (e.g., if a price API only offers daily granularity rather than exact-timestamp pricing) rather than silently absorbing the imprecision.

### Clean code
- Layered architecture: clear separation between HTTP handlers, business logic/services, and data access (repositories) — no business logic inside HTTP handlers.
- Define interfaces for external dependencies (blockchain data provider, price provider) so they're mockable in tests. Don't hard-couple business logic to a specific HTTP client or SDK.
- Each major concern (fetching raw data, classifying transactions, computing cost basis, aggregating results) should be its own clearly separated, independently testable module.
- Meaningful naming, small functions, proper error handling (wrapped errors with context, no swallowed errors).
- Include unit tests for the classification logic and the cost-basis engine specifically — these are the parts where correctness matters most and are the easiest to unit test in isolation from the network.

## How I want to work with you on this

- I want to be actively involved in writing the code, not just receive a finished project. For core logic (classification rules, cost-basis calculation, the main data flow), please scaffold structure and interfaces, explain the approach, and let me implement key pieces — then review what I write.
- For more boilerplate-heavy parts (basic CRUD handlers, frontend scaffolding, standard project setup), you can generate more directly — use your judgment on where the learning value is versus where it's just setup.
- When there's a genuine design decision to make (e.g., "should staking rewards be their own event type or a subtype of income?", "how do we handle a token we don't recognize?"), surface it to me and let's discuss before you decide.
- Flag anywhere the implementation might compromise on the security or accuracy requirements above — don't silently take a shortcut on those.

## Where to start

Let's begin with:
1. Project structure and repo scaffolding (backend + frontend as separate directories, or a monorepo — your recommendation). Backend should be structured for deployment on Render (free tier); frontend structured for Vercel.
2. The data model: what does a normalized "transaction" look like internally, before we write any fetching code? This needs to include not just the financial fields (asset, amount, classification, cost basis) but also **provenance fields** for the validation features above — e.g., which price source was used, the exact timestamp queried, a link/reference back to the source blockchain explorer entry.
3. The worker pool design for the data fetch layer — decide the concurrency pattern (bounded worker pool via goroutines + channels, with a clear, explicit aggregation strategy — channel-based collection preferred over shared mutable state) before writing the actual fetch code, since retrofitting concurrency onto sequential code later is much harder than designing for it from the start.
4. Then work outward from there — data fetch layer, classification, cost-basis engine, API, frontend.

Let's discuss the data model first before writing any code.