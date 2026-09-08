// Mirrors the backend's JSON wire format exactly (see backend/internal/
// domain, costbasis, analysis). Every monetary/token-amount field is a
// string, not a number - the backend uses shopspring/decimal specifically
// to avoid float corruption, and that guarantee is worthless if we throw
// it away the moment JSON crosses the wire. These stay strings all the
// way to the display layer; see formatUsd/formatQuantity in format.ts for
// the one place they're ever parsed, and only for display.

export type TxType =
  | 'transfer'
  | 'swap'
  | 'staking_reward'
  | 'airdrop'
  | 'gas_fee'
  | 'unknown'

export type Direction = 'in' | 'out' | 'n/a'

export type PriceGranularity = 'exact_block' | 'daily' | ''

export interface AssetRef {
  chainId: number
  contractAddress: string | null
  symbol: string
}

export interface Transaction {
  txHash: string
  chainId: number
  blockNumber: number
  blockTimestamp: string // RFC3339
  success: boolean

  type: TxType
  direction: Direction

  fromAddress: string
  toAddress: string

  assetSymbol: string
  contractAddress: string | null
  assetDecimals: number
  amount: string

  gasFeeAmount: string
  gasFeeAsset: string

  priceUSD: string | null
  priceSource: string
  priceTimestampUsed: string
  priceGranularity: PriceGranularity
  gasFeeUSD: string | null
  explorerUrl: string
}

export interface Lot {
  quantity: string
  costBasisUSD: string
  acquiredAt: string
  sourceTxHash: string
}

export interface Disposal {
  sourceTxHash: string
  quantity: string
  proceedsUSD: string
  costBasisUSD: string
  realizedGainUSD: string
}

export interface IncomeEvent {
  sourceTxHash: string
  type: TxType
  quantity: string
  valueUSD: string
}

export interface AssetSummary {
  asset: AssetRef
  remainingLots: Lot[]
  disposals: Disposal[]
  incomeEvents: IncomeEvent[]
  totalSpentUSD: string
  totalReceivedUSD: string
  realizedGainUSD: string
  totalIncomeUSD: string
  unrealizedGainUSD: string | null // null = no current price available, never "0"
}

export interface Totals {
  totalSpentUSD: string
  totalReceivedUSD: string
  realizedGainUSD: string
  totalIncomeUSD: string
  unrealizedGainUSD: string
  gasFeesUSD: string
}

export interface CostBasisResult {
  perAsset: AssetSummary[]
  totals: Totals
  skipped: Transaction[]
}

export interface FailedLookup {
  transaction: Transaction
  error: string
}

export interface AnalysisResult {
  walletAddress: string
  chainId: number
  transactionCount: number
  costBasis: CostBasisResult
  failedPriceLookups: FailedLookup[]
}

// Chains the backend actually supports (see provider/etherscan.go,
// provider/coingecko.go) - kept here as the single source of truth for
// the chain picker, rather than duplicating chain IDs across components.
export const SUPPORTED_CHAINS = [
  { id: 1, label: 'Ethereum' },
  { id: 137, label: 'Polygon' },
] as const
