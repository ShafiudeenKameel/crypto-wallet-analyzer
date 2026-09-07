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
	Asset    domain.AssetRef
	PriceUSD decimal.Decimal
}

// Lot is a still-unconsumed acquisition - i.e. current holdings.
type Lot struct {
	Quantity     decimal.Decimal
	CostBasisUSD decimal.Decimal
	AcquiredAt   time.Time
	SourceTxHash string
}

// Disposal is one FIFO-matched transfer-out of an asset.
type Disposal struct {
	SourceTxHash    string
	Quantity        decimal.Decimal
	ProceedsUSD     decimal.Decimal
	CostBasisUSD    decimal.Decimal
	RealizedGainUSD decimal.Decimal
}

// IncomeEvent is a staking reward or airdrop, valued at fair market value
// when received - that value is both taxable income and the new lot's
// cost basis.
type IncomeEvent struct {
	SourceTxHash string
	Type         domain.TxType
	Quantity     decimal.Decimal
	ValueUSD     decimal.Decimal
}

// AssetSummary is the FIFO result for one asset. Assets are never merged
// across chains/contracts - see project.MD.
type AssetSummary struct {
	Asset domain.AssetRef

	RemainingLots []Lot // current holdings
	Disposals     []Disposal
	IncomeEvents  []IncomeEvent

	TotalSpentUSD    decimal.Decimal // acquisitions, at fair value when received
	TotalReceivedUSD decimal.Decimal // disposal proceeds
	RealizedGainUSD  decimal.Decimal
	TotalIncomeUSD   decimal.Decimal

	// UnrealizedGainUSD is nil when the caller didn't supply a current
	// price for this asset - never silently reported as zero.
	UnrealizedGainUSD *decimal.Decimal
}

// Totals aggregates every AssetSummary into wallet-level dashboard figures.
type Totals struct {
	TotalSpentUSD     decimal.Decimal
	TotalReceivedUSD  decimal.Decimal
	RealizedGainUSD   decimal.Decimal
	TotalIncomeUSD    decimal.Decimal
	UnrealizedGainUSD decimal.Decimal // sums only assets where it was computable
	GasFeesUSD        decimal.Decimal
}

// Result is the full output of Compute.
type Result struct {
	PerAsset []AssetSummary
	Totals   Totals

	// Skipped holds transactions Compute couldn't confidently value or
	// attribute (no price, unrecognized asset, ambiguous direction) -
	// never dropped, always visible for review.
	Skipped []domain.Transaction
}
