// Package domain holds the normalized, provider-agnostic types shared across
// the fetch, classification, and cost-basis layers. Nothing in this package
// should import a specific provider SDK (Etherscan, CoinGecko, etc.) - it
// describes *our* model of a transaction, not any single API's response shape.
package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// TxType classifies what kind of on-chain event a Transaction represents.
// Staking rewards and airdrops are deliberately their own top-level values
// (rather than a shared "income" type + sub-kind field) - decided so that
// classification code and callers can pattern-match directly on TxType
// without an extra field to thread through everything.
type TxType string

const (
	TxTransfer      TxType = "transfer"
	TxSwap          TxType = "swap"
	TxStakingReward TxType = "staking_reward"
	TxAirdrop       TxType = "airdrop"
	TxGasFee        TxType = "gas_fee"
	// TxUnknown is a first-class outcome, not an error case: a transaction
	// involving a contract/token the classifier doesn't recognize is still
	// recorded with best-effort fields and flagged for review in the UI -
	// never silently dropped.
	TxUnknown TxType = "unknown"
)

// Direction indicates which way an asset moved relative to the wallet being
// analyzed. Some transaction types (e.g. a swap) may need to be represented
// as two legs, each with its own Direction, rather than forcing one value.
type Direction string

const (
	DirectionIn  Direction = "in"
	DirectionOut Direction = "out"
	DirectionNA  Direction = "n/a"
)

// PriceGranularity documents how precise a price lookup actually was, so the
// UI can show honest precision instead of implying exact-block accuracy when
// the price source only offered daily resolution.
type PriceGranularity string

const (
	GranularityExactBlock PriceGranularity = "exact_block"
	GranularityDaily      PriceGranularity = "daily"
)

// Transaction is the normalized representation of a single on-chain event,
// after fetching but before cost-basis calculation. One Transaction should
// need no further lookups against the raw provider response to be displayed,
// verified, or fed into the cost-basis engine.
//
// TODO(you): review this shape before we wire up any fetch code. In
// particular: does a "swap" fit cleanly as one Transaction, or does it need
// to be modeled as two linked Transactions (one out-leg, one in-leg)? Worth
// deciding once we see a real Etherscan swap payload.
type Transaction struct {
	// --- identity ---
	TxHash         string
	ChainID        int // e.g. 1 = Ethereum mainnet, 137 = Polygon
	BlockNumber    uint64
	BlockTimestamp time.Time // from the chain itself - never "time we fetched it"
	Success        bool      // false = the tx reverted on-chain; gas was still spent

	// --- classification ---
	Type      TxType
	Direction Direction

	// --- asset + amount: no floats, ever, for token amounts or money ---
	AssetSymbol     string
	ContractAddress *string // nil => native token (ETH, MATIC/POL, ...)
	AssetDecimals   int     // e.g. 18 for ETH, 6 for USDC - looked up, never assumed
	Amount          decimal.Decimal

	// --- gas: recorded even on failed transactions ---
	GasFeeAmount decimal.Decimal
	GasFeeAsset  string // the chain's native token

	// --- provenance: what let a user verify this number themselves ---
	PriceUSD           *decimal.Decimal // nil if unpriced (e.g. lookup failed, or gas-only failed tx)
	PriceSource        string
	PriceTimestampUsed time.Time
	PriceGranularity   PriceGranularity
	ExplorerURL        string // link to the Etherscan/Polygonscan entry for this tx
}
