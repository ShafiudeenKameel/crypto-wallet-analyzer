// Package analysis wires the pipeline together: fetch -> classify ->
// price -> cost-basis. No package below it knows about any other, so this
// is the one place the full data flow exists.
package analysis

import (
	"context"
	"fmt"
	"time"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/aggregate"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/classify"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/costbasis"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/provider"
)

// FailedLookup pairs a transaction with the price-lookup error that
// prevented it from being valued - surfaced, never silently dropped.
type FailedLookup struct {
	Transaction domain.Transaction
	Err         error
}

// Result is the full output of one wallet analysis run.
type Result struct {
	WalletAddress string
	ChainID       int

	// TransactionCount is the raw count fetched, before any filtering -
	// this is the "Found and processed N transactions" figure a user can
	// cross-check against the block explorer themselves.
	TransactionCount int

	CostBasis          costbasis.Result
	FailedPriceLookups []FailedLookup
}

// Service runs the full pipeline for one wallet.
type Service struct {
	Chain    provider.BlockchainProvider
	Enricher *aggregate.PriceEnricher
}

// NewService builds a Service. prices backs both historical pricing
// (during Enrich) and current pricing (for unrealized gain) - one
// provider, one rate budget, shared via the same PriceEnricher.
func NewService(chain provider.BlockchainProvider, prices provider.PriceProvider) *Service {
	return &Service{
		Chain:    chain,
		Enricher: aggregate.NewPriceEnricher(prices),
	}
}

// Analyze fetches, classifies, prices, and FIFO-matches every transaction
// for walletAddress on chainID.
func (s *Service) Analyze(ctx context.Context, walletAddress string, chainID int) (Result, error) {
	raw, err := s.Chain.FetchTransactions(ctx, walletAddress, chainID)
	if err != nil {
		return Result{}, fmt.Errorf("analysis: fetching transactions: %w", err)
	}

	// Classification is pure, cheap CPU work (no I/O) - sequential here is
	// a deliberate simplification; the actual bottleneck is the priced
	// network calls below, which do run through a bounded worker pool.
	classified := make([]domain.Transaction, len(raw))
	for i, tx := range raw {
		classified[i] = classify.Classify(tx, walletAddress)
	}

	enriched := s.Enricher.Enrich(ctx, classified)

	priced := make([]domain.Transaction, 0, len(enriched))
	var failed []FailedLookup
	for _, r := range enriched {
		if r.Err != nil {
			failed = append(failed, FailedLookup{Transaction: r.Transaction, Err: r.Err})
			continue
		}
		priced = append(priced, r.Transaction)
	}

	cb := costbasis.Compute(priced, s.currentPrices(ctx, priced))

	return Result{
		WalletAddress:      walletAddress,
		ChainID:            chainID,
		TransactionCount:   len(raw),
		CostBasis:          cb,
		FailedPriceLookups: failed,
	}, nil
}

type assetMapKey struct {
	chainID  int
	contract string
}

func keyFor(chainID int, contractAddress *string) assetMapKey {
	contract := ""
	if contractAddress != nil {
		contract = *contractAddress
	}
	return assetMapKey{chainID: chainID, contract: contract}
}

// currentPrices looks up today's price for every distinct asset touched,
// for the unrealized-gain calculation. Best-effort: a failed lookup just
// means that asset's UnrealizedGainUSD stays nil (costbasis already
// treats a missing price that way) rather than failing the whole analysis
// over one asset's price lookup.
func (s *Service) currentPrices(ctx context.Context, txs []domain.Transaction) []costbasis.CurrentPrice {
	seen := make(map[assetMapKey]domain.AssetRef)
	for _, tx := range txs {
		key := keyFor(tx.ChainID, tx.ContractAddress)
		if _, ok := seen[key]; !ok {
			seen[key] = domain.AssetRef{ChainID: tx.ChainID, ContractAddress: tx.ContractAddress, Symbol: tx.AssetSymbol}
		}
	}

	now := time.Now()
	prices := make([]costbasis.CurrentPrice, 0, len(seen))
	for _, asset := range seen {
		price, _, err := s.Enricher.FetchPrice(ctx, asset, now)
		if err != nil {
			continue
		}
		prices = append(prices, costbasis.CurrentPrice{Asset: asset, PriceUSD: price})
	}
	return prices
}
