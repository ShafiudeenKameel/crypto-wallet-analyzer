package analysis

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

const wallet = "0xWallet"

type fakeChain struct {
	txs []domain.Transaction
}

func (f fakeChain) FetchTransactions(ctx context.Context, walletAddress string, chainID int) ([]domain.Transaction, error) {
	return f.txs, nil
}

type fakePrices struct{ price decimal.Decimal }

func (f fakePrices) Name() string { return "fake" }
func (f fakePrices) GetPriceAt(ctx context.Context, asset domain.AssetRef, at time.Time) (decimal.Decimal, domain.PriceGranularity, error) {
	return f.price, domain.GranularityDaily, nil
}

func TestAnalyze_FullPipeline(t *testing.T) {
	chain := fakeChain{txs: []domain.Transaction{
		{
			TxHash: "0xin", ChainID: 1, BlockTimestamp: time.Now().Add(-time.Hour),
			Success: true, FromAddress: "0xSomeoneElse", ToAddress: wallet,
			AssetSymbol: "ETH", Amount: decimal.NewFromInt(2), GasFeeAmount: decimal.Zero,
		},
	}}
	prices := fakePrices{price: decimal.NewFromInt(100)}

	svc := NewService(chain, prices)
	result, err := svc.Analyze(context.Background(), wallet, 1)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}
	if len(result.FailedPriceLookups) != 0 {
		t.Errorf("FailedPriceLookups = %+v, want none", result.FailedPriceLookups)
	}
	if len(result.CostBasis.PerAsset) != 1 {
		t.Fatalf("PerAsset = %d entries, want 1", len(result.CostBasis.PerAsset))
	}

	asset := result.CostBasis.PerAsset[0]
	wantSpent := decimal.NewFromInt(200) // 2 ETH * $100
	if !asset.TotalSpentUSD.Equal(wantSpent) {
		t.Errorf("TotalSpentUSD = %s, want %s", asset.TotalSpentUSD, wantSpent)
	}
	if asset.UnrealizedGainUSD == nil {
		t.Error("UnrealizedGainUSD = nil, want computed (fakePrices always succeeds)")
	}
}
