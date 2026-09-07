// Package aggregate is the only package allowed to know about concurrency.
// It orchestrates provider calls across a bounded worker pool and collects
// results back through a channel - never a shared mutable slice/map guarded
// by a mutex.
package aggregate

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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

// Enrich returns one Result per input transaction, in no particular order -
// sort by BlockTimestamp at the call site if display order matters.
func (e *PriceEnricher) Enrich(ctx context.Context, txs []domain.Transaction) []Result {
	jobs := make(chan int)
	results := make(chan Result, len(txs))

	var wg sync.WaitGroup
	for w := 0; w < e.Workers; w++ {
		wg.Add(1)
		go e.worker(ctx, txs, jobs, results, &wg)
	}

	go func() {
		defer close(jobs)
		for i := range txs {
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

	out := make([]Result, 0, len(txs))
	for r := range results {
		out = append(out, r)
	}
	return out
}

func (e *PriceEnricher) worker(ctx context.Context, txs []domain.Transaction, jobs <-chan int, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := range jobs {
		tx, err := e.priceOne(ctx, txs[i])
		results <- Result{Transaction: tx, Err: err}
	}
}

// priceOne looks up and applies the price for a single transaction,
// retrying on rate-limit errors with exponential backoff.
func (e *PriceEnricher) priceOne(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	asset := domain.AssetRef{ChainID: tx.ChainID, ContractAddress: tx.ContractAddress, Symbol: tx.AssetSymbol}

	var lastErr error
	for attempt := 0; attempt <= e.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * 200 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return tx, ctx.Err()
			}
		}

		if err := e.Limiter.Wait(ctx); err != nil {
			return tx, err
		}

		price, granularity, err := e.Prices.GetPriceAt(ctx, asset, tx.BlockTimestamp)
		if err == nil {
			tx.PriceUSD = &price
			tx.PriceSource = e.Prices.Name()
			tx.PriceTimestampUsed = tx.BlockTimestamp
			tx.PriceGranularity = granularity
			return tx, nil
		}

		lastErr = err
		if !errors.Is(err, provider.ErrRateLimited) {
			return tx, err // not a rate-limit error - retrying won't help
		}
	}
	return tx, fmt.Errorf("price lookup failed after %d retries: %w", e.MaxRetries, lastErr)
}
