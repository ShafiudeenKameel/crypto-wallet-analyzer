// Package domain holds the normalized types shared across all layers.
package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// TxType classifies what kind of on-chain event a Transaction represents.
type TxType string

const (
	TxTransfer      TxType = "transfer"
	TxSwap          TxType = "swap"
	TxStakingReward TxType = "staking_reward"
	TxAirdrop       TxType = "airdrop"
	TxGasFee        TxType = "gas_fee"
	TxUnknown       TxType = "unknown" // unrecognized token/contract - still recorded, never dropped
)

// Direction is which way an asset moved relative to the wallet being analyzed.
type Direction string

const (
	DirectionIn  Direction = "in"
	DirectionOut Direction = "out"
	DirectionNA  Direction = "n/a"
)

// PriceGranularity is how precise a price lookup was.
type PriceGranularity string

const (
	GranularityExactBlock PriceGranularity = "exact_block"
	GranularityDaily      PriceGranularity = "daily"
)

// Transaction is a single normalized on-chain event, ready for
// classification/cost-basis with no further provider lookups needed.
type Transaction struct {
	TxHash         string
	ChainID        int // 1 = Ethereum mainnet, 137 = Polygon
	BlockNumber    uint64
	BlockTimestamp time.Time
	Success        bool // false = reverted on-chain; gas was still spent

	Type      TxType
	Direction Direction

	AssetSymbol     string
	ContractAddress *string // nil => native token (ETH, MATIC/POL, ...)
	AssetDecimals   int     // e.g. 18 for ETH, 6 for USDC
	Amount          decimal.Decimal

	GasFeeAmount decimal.Decimal
	GasFeeAsset  string

	PriceUSD           *decimal.Decimal // nil if unpriced
	PriceSource        string
	PriceTimestampUsed time.Time
	PriceGranularity   PriceGranularity
	ExplorerURL        string
}
