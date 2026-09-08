package domain

// AssetRef identifies a specific on-chain asset. A bare symbol like "USDC"
// is ambiguous across chains and contracts, so price lookups need more.
type AssetRef struct {
	ChainID         int     `json:"chainId"`
	ContractAddress *string `json:"contractAddress"` // nil = native token
	Symbol          string  `json:"symbol"`
}
