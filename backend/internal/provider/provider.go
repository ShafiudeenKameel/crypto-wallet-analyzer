// Package provider defines the contracts for external data sources. Business
// logic depends on these interfaces, never on a concrete SDK/HTTP client.
package provider

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

// BlockchainProvider fetches a wallet's full transaction history on one
// chain. Implementations own their own pagination - callers get everything.
type BlockchainProvider interface {
	FetchTransactions(ctx context.Context, walletAddress string, chainID int) ([]domain.Transaction, error)
}

// PriceProvider looks up the USD price of an asset at a point in time.
type PriceProvider interface {
	// Name identifies this provider for domain.Transaction.PriceSource,
	// so callers never hard-code a specific provider's name.
	Name() string
	GetPriceAt(ctx context.Context, asset domain.AssetRef, at time.Time) (price decimal.Decimal, granularity domain.PriceGranularity, err error)
}
