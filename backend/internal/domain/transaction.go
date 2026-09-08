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
//
// JSON tags are camelCase throughout, matching every other API type - this
// is what the frontend actually consumes, so its shape is as deliberate as
// the Go side.
type Transaction struct {
	TxHash         string    `json:"txHash"`
	ChainID        int       `json:"chainId"` // 1 = Ethereum mainnet, 137 = Polygon
	BlockNumber    uint64    `json:"blockNumber"`
	BlockTimestamp time.Time `json:"blockTimestamp"`
	Success        bool      `json:"success"` // false = reverted on-chain; gas was still spent

	Type      TxType    `json:"type"`
	Direction Direction `json:"direction"`

	FromAddress string `json:"fromAddress"`
	ToAddress   string `json:"toAddress"`

	AssetSymbol     string          `json:"assetSymbol"`
	ContractAddress *string         `json:"contractAddress"` // nil => native token (ETH, MATIC/POL, ...)
	AssetDecimals   int             `json:"assetDecimals"`   // e.g. 18 for ETH, 6 for USDC
	Amount          decimal.Decimal `json:"amount"`

	GasFeeAmount decimal.Decimal `json:"gasFeeAmount"`
	GasFeeAsset  string          `json:"gasFeeAsset"`

	// PriceUSD is a per-unit spot price (e.g. $/ETH); GasFeeUSD is the
	// already-multiplied total USD value of GasFeeAmount, since it's
	// often priced against a different asset (the chain's native token)
	// than AssetSymbol.
	PriceUSD           *decimal.Decimal `json:"priceUSD"` // nil if unpriced
	PriceSource        string           `json:"priceSource"`
	PriceTimestampUsed time.Time        `json:"priceTimestampUsed"`
	PriceGranularity   PriceGranularity `json:"priceGranularity"`
	GasFeeUSD          *decimal.Decimal `json:"gasFeeUSD"` // nil if unpriced
	ExplorerURL        string           `json:"explorerUrl"`
}
