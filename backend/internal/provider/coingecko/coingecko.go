// Package coingecko implements provider.PriceProvider against the
// CoinGecko API.
package coingecko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/provider"
)

const defaultBaseURL = "https://api.coingecko.com/api/v3"

// chainInfo maps a chain ID to the CoinGecko identifiers needed to price
// it. v1 scope per project.MD: Ethereum mainnet + one EVM L2 (Polygon).
// Unlisted chains are a hard error, never a guess.
type chainInfo struct {
	platformID   string // for /coins/{platform}/contract/{address}/history
	nativeCoinID string // for /coins/{id}/history (the chain's gas token)
}

var chains = map[int]chainInfo{
	1: {platformID: "ethereum", nativeCoinID: "ethereum"},
	// Polygon's native gas token migrated from MATIC to POL in Sep 2024;
	// "polygon-ecosystem-token" is POL's current CoinGecko coin id, not
	// the legacy "matic-network".
	137: {platformID: "polygon-pos", nativeCoinID: "polygon-ecosystem-token"},
}

// Client implements provider.PriceProvider against CoinGecko.
type Client struct {
	// apiKey is optional - an empty key falls back to CoinGecko's public,
	// unauthenticated tier (lower rate limits, still functional). It's
	// sent as a header, never a query parameter, so it never ends up in
	// a logged request URL.
	apiKey     string
	httpClient *http.Client
	baseURL    string // overridable in tests only - production must stay https
}

// NewClient builds a Client. apiKey should come from an environment
// variable at the call site (see cmd/server) - this package never reads
// the environment itself, so it stays trivially testable.
func NewClient(apiKey string) *Client {
	// Go's default transport caps idle connections at 2 per host.
	// aggregate.PriceEnricher runs up to 8 workers concurrently calling
	// this client - without raising this, most of those concurrent
	// requests can't reuse a pooled connection and pay a fresh TCP+TLS
	// handshake instead. Clone() keeps every other default (proxy
	// support, TLS config) rather than reconstructing them by hand.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 10

	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second, Transport: transport},
		baseURL:    defaultBaseURL,
	}
}

func (c *Client) Name() string { return "coingecko" }

// GetPriceAt implements provider.PriceProvider. CoinGecko's free-tier
// history endpoint only offers daily resolution, not exact-block-time
// pricing - that limitation is reported honestly via the returned
// domain.GranularityDaily rather than implying more precision than exists.
func (c *Client) GetPriceAt(ctx context.Context, asset domain.AssetRef, at time.Time) (decimal.Decimal, domain.PriceGranularity, error) {
	path, err := historyPath(asset)
	if err != nil {
		return decimal.Decimal{}, "", err
	}

	url := fmt.Sprintf("%s%s?date=%s", c.baseURL, path, at.UTC().Format("02-01-2006"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return decimal.Decimal{}, "", fmt.Errorf("coingecko: building request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return decimal.Decimal{}, "", fmt.Errorf("coingecko: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return decimal.Decimal{}, "", fmt.Errorf("coingecko: %w", provider.ErrRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		return decimal.Decimal{}, "", fmt.Errorf("coingecko: unexpected status %d", resp.StatusCode)
	}

	var body struct {
		MarketData struct {
			CurrentPrice map[string]decimal.Decimal `json:"current_price"`
		} `json:"market_data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return decimal.Decimal{}, "", fmt.Errorf("coingecko: decoding response: %w", err)
	}

	price, ok := body.MarketData.CurrentPrice["usd"]
	if !ok {
		return decimal.Decimal{}, "", fmt.Errorf("coingecko: no usd price in response for %s", path)
	}
	return price, domain.GranularityDaily, nil
}

func historyPath(asset domain.AssetRef) (string, error) {
	info, ok := chains[asset.ChainID]
	if !ok {
		return "", fmt.Errorf("coingecko: unsupported chain ID %d", asset.ChainID)
	}
	if asset.ContractAddress == nil {
		return fmt.Sprintf("/coins/%s/history", info.nativeCoinID), nil
	}
	return fmt.Sprintf("/coins/%s/contract/%s/history", info.platformID, *asset.ContractAddress), nil
}
