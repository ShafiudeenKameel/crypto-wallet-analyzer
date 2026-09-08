package costbasis

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

// CurrentPrice is today's price for an asset - needed only for the
// unrealized-gain calculation. FIFO matching itself only needs each
// transaction's own historical price, already on domain.Transaction.
type CurrentPrice struct {
	Asset    domain.AssetRef `json:"asset"`
	PriceUSD decimal.Decimal `json:"priceUSD"`
}

// Lot is a still-unconsumed acquisition - i.e. current holdings.
type Lot struct {
	Quantity          decimal.Decimal `json:"quantity"`
	CostBasisUSD      decimal.Decimal `json:"costBasisUSD"`
	AcquiredAt        time.Time       `json:"acquiredAt"`
	SourceTxHash      string          `json:"sourceTxHash"`
	SourceExplorerURL string          `json:"sourceExplorerUrl"`
}

// Disposal is one FIFO-matched transfer-out of an asset.
type Disposal struct {
	SourceTxHash      string          `json:"sourceTxHash"`
	SourceExplorerURL string          `json:"sourceExplorerUrl"`
	Quantity          decimal.Decimal `json:"quantity"`
	ProceedsUSD       decimal.Decimal `json:"proceedsUSD"`
	CostBasisUSD      decimal.Decimal `json:"costBasisUSD"`
	RealizedGainUSD   decimal.Decimal `json:"realizedGainUSD"`
}

// IncomeEvent is a staking reward or airdrop, valued at fair market value
// when received - that value is both taxable income and the new lot's
// cost basis.
type IncomeEvent struct {
	SourceTxHash      string          `json:"sourceTxHash"`
	SourceExplorerURL string          `json:"sourceExplorerUrl"`
	Type              domain.TxType   `json:"type"`
	Quantity          decimal.Decimal `json:"quantity"`
	ValueUSD          decimal.Decimal `json:"valueUSD"`
}

// AssetSummary is the FIFO result for one asset. Assets are never merged
// across chains/contracts - see project.MD.
type AssetSummary struct {
	Asset domain.AssetRef `json:"asset"`

	RemainingLots []Lot         `json:"remainingLots"` // current holdings
	Disposals     []Disposal    `json:"disposals"`
	IncomeEvents  []IncomeEvent `json:"incomeEvents"`

	TotalSpentUSD    decimal.Decimal `json:"totalSpentUSD"`    // acquisitions, at fair value when received
	TotalReceivedUSD decimal.Decimal `json:"totalReceivedUSD"` // disposal proceeds
	RealizedGainUSD  decimal.Decimal `json:"realizedGainUSD"`
	TotalIncomeUSD   decimal.Decimal `json:"totalIncomeUSD"`

	// UnrealizedGainUSD is nil when the caller didn't supply a current
	// price for this asset - never silently reported as zero.
	UnrealizedGainUSD *decimal.Decimal `json:"unrealizedGainUSD"`
}

// Totals aggregates every AssetSummary into wallet-level dashboard figures.
type Totals struct {
	TotalSpentUSD     decimal.Decimal `json:"totalSpentUSD"`
	TotalReceivedUSD  decimal.Decimal `json:"totalReceivedUSD"`
	RealizedGainUSD   decimal.Decimal `json:"realizedGainUSD"`
	TotalIncomeUSD    decimal.Decimal `json:"totalIncomeUSD"`
	UnrealizedGainUSD decimal.Decimal `json:"unrealizedGainUSD"` // sums only assets where it was computable
	GasFeesUSD        decimal.Decimal `json:"gasFeesUSD"`
}

// Result is the full output of Compute.
type Result struct {
	PerAsset []AssetSummary `json:"perAsset"`
	Totals   Totals         `json:"totals"`

	// Skipped holds transactions Compute couldn't confidently value or
	// attribute (no price, unrecognized asset, ambiguous direction) -
	// never dropped, always visible for review.
	Skipped []domain.Transaction `json:"skipped"`
}
