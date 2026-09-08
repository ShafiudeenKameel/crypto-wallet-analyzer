package aggregate

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/time/rate"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

// countingPrices always succeeds and counts how many times it was called.
type countingPrices struct {
	calls int32
	price decimal.Decimal
}

func (c *countingPrices) Name() string { return "fake" }
func (c *countingPrices) GetPriceAt(ctx context.Context, asset domain.AssetRef, at time.Time) (decimal.Decimal, domain.PriceGranularity, error) {
	atomic.AddInt32(&c.calls, 1)
	return c.price, domain.GranularityDaily, nil
}

// selectiveFailPrices fails only for one specific symbol, to test that
// one bad lookup doesn't affect unrelated transactions.
type selectiveFailPrices struct {
	failSymbol string
	price      decimal.Decimal
}

func (s *selectiveFailPrices) Name() string { return "fake" }
func (s *selectiveFailPrices) GetPriceAt(ctx context.Context, asset domain.AssetRef, at time.Time) (decimal.Decimal, domain.PriceGranularity, error) {
	if asset.Symbol == s.failSymbol {
		return decimal.Decimal{}, "", fmt.Errorf("no price for %s", asset.Symbol)
	}
	return s.price, domain.GranularityDaily, nil
}

// fastEnricher removes rate limiting so tests run instantly - the
// production limiter is tested at the provider/aggregate boundary
// elsewhere; here we're testing dedup and result-mapping logic only.
func fastEnricher(p interface {
	Name() string
	GetPriceAt(context.Context, domain.AssetRef, time.Time) (decimal.Decimal, domain.PriceGranularity, error)
}) *PriceEnricher {
	e := NewPriceEnricher(p)
	e.Limiter = rate.NewLimiter(rate.Inf, 1000)
	return e
}

func TestEnrich_DedupesSameAssetSameDay(t *testing.T) {
	day := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
	txs := []domain.Transaction{
		{ChainID: 1, AssetSymbol: "ETH", GasFeeAsset: "ETH", BlockTimestamp: day, Amount: decimal.NewFromInt(1), GasFeeAmount: decimal.NewFromInt(1)},
		{ChainID: 1, AssetSymbol: "ETH", GasFeeAsset: "ETH", BlockTimestamp: day.Add(2 * time.Hour), Amount: decimal.NewFromInt(2), GasFeeAmount: decimal.NewFromInt(1)},
	}
	fake := &countingPrices{price: decimal.NewFromInt(100)}
	e := fastEnricher(fake)

	results := e.Enrich(context.Background(), txs)

	if fake.calls != 1 {
		t.Errorf("GetPriceAt called %d times, want 1 (same asset, same UTC day)", fake.calls)
	}
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("result %d: unexpected error %v", i, r.Err)
		}
		if r.Transaction.PriceUSD == nil || !r.Transaction.PriceUSD.Equal(decimal.NewFromInt(100)) {
			t.Errorf("result %d: PriceUSD = %v, want 100", i, r.Transaction.PriceUSD)
		}
	}
}

func TestEnrich_DifferentDaysMakeSeparateCalls(t *testing.T) {
	txs := []domain.Transaction{
		{ChainID: 1, AssetSymbol: "ETH", GasFeeAsset: "ETH", BlockTimestamp: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), Amount: decimal.NewFromInt(1)},
		{ChainID: 1, AssetSymbol: "ETH", GasFeeAsset: "ETH", BlockTimestamp: time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC), Amount: decimal.NewFromInt(1)},
	}
	fake := &countingPrices{price: decimal.NewFromInt(50)}
	e := fastEnricher(fake)

	e.Enrich(context.Background(), txs)

	if fake.calls != 2 {
		t.Errorf("GetPriceAt called %d times, want 2 (different days)", fake.calls)
	}
}

func TestEnrich_SameAssetAsGasTokenSkipsSecondLookup(t *testing.T) {
	tx := domain.Transaction{
		ChainID: 1, AssetSymbol: "ETH", GasFeeAsset: "ETH", ContractAddress: nil,
		BlockTimestamp: time.Now(), Amount: decimal.NewFromInt(1), GasFeeAmount: decimal.NewFromInt(1),
	}
	fake := &countingPrices{price: decimal.NewFromInt(10)}
	e := fastEnricher(fake)

	results := e.Enrich(context.Background(), []domain.Transaction{tx})

	if fake.calls != 1 {
		t.Errorf("GetPriceAt called %d times, want 1 (main asset == gas asset)", fake.calls)
	}
	got := results[0].Transaction.GasFeeUSD
	if got == nil || !got.Equal(decimal.NewFromInt(10)) {
		t.Errorf("GasFeeUSD = %v, want 10 (1 gas * $10)", got)
	}
}

func TestEnrich_OneFailedLookupDoesntAffectOthers(t *testing.T) {
	day1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	txs := []domain.Transaction{
		{ChainID: 1, AssetSymbol: "BAD", GasFeeAsset: "BAD", BlockTimestamp: day1, Amount: decimal.NewFromInt(1)},
		{ChainID: 1, AssetSymbol: "GOOD", GasFeeAsset: "GOOD", BlockTimestamp: day2, Amount: decimal.NewFromInt(1)},
	}
	fake := &selectiveFailPrices{failSymbol: "BAD", price: decimal.NewFromInt(5)}
	e := fastEnricher(fake)
	e.MaxRetries = 0

	results := e.Enrich(context.Background(), txs)

	if results[0].Err == nil {
		t.Error("expected an error for the BAD-priced transaction")
	}
	if results[1].Err != nil {
		t.Errorf("GOOD transaction affected by BAD's failure: %v", results[1].Err)
	}
}
