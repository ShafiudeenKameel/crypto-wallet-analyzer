// Package aggregate is the only package allowed to know about concurrency.
// It orchestrates provider calls across a bounded worker pool and collects
// results back through a channel - never a shared mutable slice/map guarded
// by a mutex.
package aggregate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/time/rate"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/provider"
)

// Result pairs an enriched Transaction with an error, so one failed price
// lookup never sinks the whole batch - callers see it in the result set
// rather than losing it silently.
type Result struct {
	Transaction domain.Transaction
	Err         error
}

// PriceEnricher fills in PriceUSD (and related provenance fields) on a
// batch of transactions, fanning the work out across a bounded pool of
// goroutines.
type PriceEnricher struct {
	Prices provider.PriceProvider

	Workers    int           // max concurrent in-flight requests
	Limiter    *rate.Limiter // requests/sec cap, shared across all workers
	MaxRetries int           // retries on provider.ErrRateLimited before giving up
}

// NewPriceEnricher builds a PriceEnricher sized for CoinGecko's free tier.
//
// Workers=8: latency-bound (network round trips), not CPU-bound, so a
// modest pool keeps requests overlapping without over-committing.
// Limiter=0.4 req/s (~24/min): the worker count alone only bounds how many
// requests are in flight at once, not how many happen per minute - this is
// what actually keeps us under CoinGecko's ~30/min free-tier cap, with some
// margin since the exact limit isn't guaranteed and can change.
func NewPriceEnricher(p provider.PriceProvider) *PriceEnricher {
	return &PriceEnricher{
		Prices:     p,
		Workers:    8,
		Limiter:    rate.NewLimiter(rate.Limit(0.4), 3),
		MaxRetries: 4,
	}
}

// priceKey groups lookups that are provably identical: CoinGecko's free
// tier only offers daily resolution anyway, so every transaction for the
// same asset on the same UTC day needs exactly the same answer - fetching
// it more than once would just repeat the same network round trip.
type priceKey struct {
	chainID  int
	contract string // "" = native
	day      string // YYYY-MM-DD (UTC)
}

func keyFor(asset domain.AssetRef, at time.Time) priceKey {
	contract := ""
	if asset.ContractAddress != nil {
		contract = strings.ToLower(*asset.ContractAddress)
	}
	return priceKey{chainID: asset.ChainID, contract: contract, day: at.UTC().Format("2006-01-02")}
}

type priceOutcome struct {
	price       decimal.Decimal
	granularity domain.PriceGranularity
	err         error
}

// lookup is one unit of work for the worker pool: a single (asset, day)
// pair, however many transactions end up needing that same answer.
type lookup struct {
	key   priceKey
	asset domain.AssetRef
	at    time.Time
}

// Enrich returns one Result per input transaction, in the same order as
// txs. Every transaction's main-asset and gas-asset price needs are first
// deduplicated by (asset, day) - a wallet with many same-day transactions
// makes far fewer network calls than it has transactions.
func (e *PriceEnricher) Enrich(ctx context.Context, txs []domain.Transaction) []Result {
	outcomes := e.fetchAll(ctx, dedupeLookups(txs))

	out := make([]Result, len(txs))
	for i, tx := range txs {
		out[i] = e.apply(tx, outcomes)
	}
	return out
}

// dedupeLookups collects every distinct (asset, day) a batch of
// transactions needs priced - each transaction needs its own asset, and,
// when different, the gas asset too.
func dedupeLookups(txs []domain.Transaction) []lookup {
	seen := make(map[priceKey]lookup)
	for _, tx := range txs {
		main := domain.AssetRef{ChainID: tx.ChainID, ContractAddress: tx.ContractAddress, Symbol: tx.AssetSymbol}
		mainKey := keyFor(main, tx.BlockTimestamp)
		if _, ok := seen[mainKey]; !ok {
			seen[mainKey] = lookup{key: mainKey, asset: main, at: tx.BlockTimestamp}
		}

		if !(tx.ContractAddress == nil && tx.AssetSymbol == tx.GasFeeAsset) {
			gas := domain.AssetRef{ChainID: tx.ChainID, Symbol: tx.GasFeeAsset}
			gasKey := keyFor(gas, tx.BlockTimestamp)
			if _, ok := seen[gasKey]; !ok {
				seen[gasKey] = lookup{key: gasKey, asset: gas, at: tx.BlockTimestamp}
			}
		}
	}

	out := make([]lookup, 0, len(seen))
	for _, l := range seen {
		out = append(out, l)
	}
	return out
}

// fetchAll runs one worker-pool job per unique lookup - the same bounded
// fan-out/fan-in as a per-transaction pool would use, just over
// deduplicated price lookups instead of one job per transaction. The
// single goroutine draining results into outcomes is the only writer to
// that map, so no mutex is needed here either.
func (e *PriceEnricher) fetchAll(ctx context.Context, lookups []lookup) map[priceKey]priceOutcome {
	type keyedOutcome struct {
		key priceKey
		out priceOutcome
	}

	jobs := make(chan int)
	results := make(chan keyedOutcome, len(lookups))

	var wg sync.WaitGroup
	for w := 0; w < e.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				l := lookups[i]
				price, granularity, err := e.FetchPrice(ctx, l.asset, l.at)
				results <- keyedOutcome{l.key, priceOutcome{price: price, granularity: granularity, err: err}}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for i := range lookups {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	outcomes := make(map[priceKey]priceOutcome, len(lookups))
	for r := range results {
		outcomes[r.key] = r.out
	}
	return outcomes
}

// apply builds the enriched Transaction for tx purely from already-fetched
// outcomes - no I/O happens here. A missing outcome (e.g. ctx was
// cancelled mid-fetch, so fewer lookups completed than were needed) is
// reported as an error rather than silently defaulting to a zero price.
func (e *PriceEnricher) apply(tx domain.Transaction, outcomes map[priceKey]priceOutcome) Result {
	main := domain.AssetRef{ChainID: tx.ChainID, ContractAddress: tx.ContractAddress, Symbol: tx.AssetSymbol}
	mainOut, ok := outcomes[keyFor(main, tx.BlockTimestamp)]
	if !ok {
		return Result{Transaction: tx, Err: fmt.Errorf("no price outcome for %s on %s", tx.AssetSymbol, tx.BlockTimestamp)}
	}
	if mainOut.err != nil {
		return Result{Transaction: tx, Err: mainOut.err}
	}
	tx.PriceUSD = &mainOut.price
	tx.PriceSource = e.Prices.Name()
	tx.PriceTimestampUsed = tx.BlockTimestamp
	tx.PriceGranularity = mainOut.granularity

	// The tx's own asset already IS the native gas token - reuse the
	// price just applied instead of a second lookup.
	gasPrice := mainOut.price
	if !(tx.ContractAddress == nil && tx.AssetSymbol == tx.GasFeeAsset) {
		gas := domain.AssetRef{ChainID: tx.ChainID, Symbol: tx.GasFeeAsset}
		gasOut, ok := outcomes[keyFor(gas, tx.BlockTimestamp)]
		if !ok {
			return Result{Transaction: tx, Err: fmt.Errorf("no price outcome for gas asset %s on %s", tx.GasFeeAsset, tx.BlockTimestamp)}
		}
		if gasOut.err != nil {
			return Result{Transaction: tx, Err: gasOut.err}
		}
		gasPrice = gasOut.price
	}
	gasUSD := tx.GasFeeAmount.Mul(gasPrice)
	tx.GasFeeUSD = &gasUSD

	return Result{Transaction: tx}
}

// FetchPrice looks up one asset's price, respecting the enricher's shared
// rate limiter and retrying on rate-limit errors via provider.RetryOnRateLimit.
// Exported so callers outside a batch Enrich (e.g. a one-off current-price
// lookup for unrealized gain) reuse the same limiter and backoff policy
// instead of a third copy of this logic.
func (e *PriceEnricher) FetchPrice(ctx context.Context, asset domain.AssetRef, at time.Time) (decimal.Decimal, domain.PriceGranularity, error) {
	var price decimal.Decimal
	var granularity domain.PriceGranularity

	err := provider.RetryOnRateLimit(ctx, e.MaxRetries, func() error {
		if err := e.Limiter.Wait(ctx); err != nil {
			return err // ctx cancellation, not a rate-limit error - don't retry
		}
		p, g, err := e.Prices.GetPriceAt(ctx, asset, at)
		if err != nil {
			return err
		}
		price, granularity = p, g
		return nil
	})
	if err != nil {
		return decimal.Decimal{}, "", fmt.Errorf("price lookup failed: %w", err)
	}
	return price, granularity, nil
}
