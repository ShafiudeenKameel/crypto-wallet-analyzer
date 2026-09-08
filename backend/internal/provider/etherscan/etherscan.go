// Package etherscan implements provider.BlockchainProvider against
// Etherscan's unified V2 API (one base URL + a chainid param covers every
// supported chain).
package etherscan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/ethaddr"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/provider"
)

const (
	defaultBaseURL = "https://api.etherscan.io/v2/api"
	pageSize       = 1000
	maxWindow      = 10000 // Etherscan: page * offset must stay <= 10000
	maxRetries     = 4
)

// chainInfo maps a chain ID to what's needed to fetch and link its data.
// v1 scope per project.MD: Ethereum mainnet + one EVM L2 (Polygon).
// Unlisted chains are a hard error, never a guess. The block-explorer
// website is still per-chain even though the API is unified (Polygon
// transactions are viewed on polygonscan.com, not etherscan.io).
type chainInfo struct {
	explorerBase string
	nativeSymbol string
}

var chains = map[int]chainInfo{
	1:   {explorerBase: "https://etherscan.io", nativeSymbol: "ETH"},
	137: {explorerBase: "https://polygonscan.com", nativeSymbol: "POL"},
}

// Client implements provider.BlockchainProvider against Etherscan.
type Client struct {
	// apiKey: Etherscan's API only supports query-param auth (no header
	// option, unlike CoinGecko) - we never include the full request URL
	// in any error or log we produce, which is what actually keeps the
	// key out of our own logs.
	apiKey     string
	httpClient *http.Client
	baseURL    string // overridable in tests only - production must stay https
}

// NewClient builds a Client. apiKey should come from an environment
// variable at the call site (see cmd/server) - this package never reads
// the environment itself.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		baseURL:    defaultBaseURL,
	}
}

// FetchTransactions implements provider.BlockchainProvider: native
// transfers (txlist) and ERC-20 transfers (tokentx), fully paginated.
func (c *Client) FetchTransactions(ctx context.Context, walletAddress string, chainID int) ([]domain.Transaction, error) {
	info, ok := chains[chainID]
	if !ok {
		return nil, fmt.Errorf("etherscan: unsupported chain ID %d", chainID)
	}

	// Sanitize before anything touches the network - a malformed or
	// mistyped address must never reach a downstream API call.
	checksummed, err := ethaddr.Validate(walletAddress)
	if err != nil {
		return nil, fmt.Errorf("etherscan: %w", err)
	}

	// txlist and tokentx are independent API calls - fetching them
	// concurrently instead of one after the other roughly halves this
	// phase's wall-clock time. Each goroutine only ever writes its own
	// two result variables, so there's no shared mutable state to guard.
	var normal []rawNormalTx
	var tokenTransfers []rawTokenTx
	var normalErr, tokenErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		normal, normalErr = fetchPaginated[rawNormalTx](ctx, c, chainID, "txlist", checksummed)
	}()
	go func() {
		defer wg.Done()
		tokenTransfers, tokenErr = fetchPaginated[rawTokenTx](ctx, c, chainID, "tokentx", checksummed)
	}()
	wg.Wait()

	if normalErr != nil {
		return nil, fmt.Errorf("etherscan: fetching normal transactions: %w", normalErr)
	}
	if tokenErr != nil {
		return nil, fmt.Errorf("etherscan: fetching token transfers: %w", tokenErr)
	}

	txs := make([]domain.Transaction, 0, len(normal)+len(tokenTransfers))
	for _, raw := range normal {
		tx, err := raw.toTransaction(chainID, info)
		if err != nil {
			return nil, fmt.Errorf("etherscan: parsing tx %s: %w", raw.Hash, err)
		}
		txs = append(txs, tx)
	}
	for _, raw := range tokenTransfers {
		tx, err := raw.toTransaction(chainID, info)
		if err != nil {
			return nil, fmt.Errorf("etherscan: parsing token transfer %s: %w", raw.Hash, err)
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

// blockNumbered lets fetchPaginated advance past Etherscan's page*offset
// <= 10000 window without caring which raw record type it's paging
// through.
type blockNumbered interface {
	blockNum() (int, error)
}

// fetchPaginated pages through action's results in full. Etherscan caps
// page*offset at 10000 ("result window too large" past that) - once hit,
// we advance startBlock past the last-seen block and resume from page 1,
// so a wallet with a long history is never silently truncated.
func fetchPaginated[T blockNumbered](ctx context.Context, c *Client, chainID int, action, address string) ([]T, error) {
	var all []T
	startBlock := 0

	for {
		windowFull := false
		for page := 1; page*pageSize <= maxWindow; page++ {
			var batch []T
			err := provider.RetryOnRateLimit(ctx, maxRetries, func() error {
				b, err := fetchPage[T](ctx, c, chainID, action, address, startBlock, page)
				if err != nil {
					return err
				}
				batch = b
				return nil
			})
			if err != nil {
				return nil, err
			}

			all = append(all, batch...)
			if len(batch) < pageSize {
				return all, nil // fewer than a full page - reached the end
			}
			if page*pageSize == maxWindow {
				windowFull = true
			}
		}

		if !windowFull || len(all) == 0 {
			return all, nil
		}
		lastBlock, err := all[len(all)-1].blockNum()
		if err != nil {
			return nil, fmt.Errorf("parsing blockNumber: %w", err)
		}
		startBlock = lastBlock + 1
	}
}

func fetchPage[T any](ctx context.Context, c *Client, chainID int, action, address string, startBlock, page int) ([]T, error) {
	q := url.Values{}
	q.Set("chainid", strconv.Itoa(chainID))
	q.Set("module", "account")
	q.Set("action", action)
	q.Set("address", address)
	q.Set("startblock", strconv.Itoa(startBlock))
	q.Set("endblock", "99999999")
	q.Set("page", strconv.Itoa(page))
	q.Set("offset", strconv.Itoa(pageSize))
	q.Set("sort", "asc")
	if c.apiKey != "" {
		q.Set("apikey", c.apiKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var env struct {
		Status  string          `json:"status"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if env.Status != "1" {
		if strings.Contains(strings.ToLower(env.Message), "rate limit") ||
			strings.Contains(strings.ToLower(string(env.Result)), "rate limit") {
			return nil, provider.ErrRateLimited
		}
		if env.Message == "No transactions found" {
			return nil, nil // an empty page, not an error
		}
		return nil, fmt.Errorf("api error: %s", env.Message)
	}

	var records []T
	if err := json.Unmarshal(env.Result, &records); err != nil {
		return nil, fmt.Errorf("decoding result: %w", err)
	}
	return records, nil
}

// parseCommon parses the fields shared by both raw record types. Gas is
// always paid in the chain's native token (18 decimals), regardless of
// what asset the transaction itself moved.
func parseCommon(timeStampStr, gasUsedStr, gasPriceStr string) (time.Time, decimal.Decimal, error) {
	tsInt, err := strconv.ParseInt(timeStampStr, 10, 64)
	if err != nil {
		return time.Time{}, decimal.Decimal{}, fmt.Errorf("parsing timeStamp: %w", err)
	}
	gasUsed, err := decimal.NewFromString(gasUsedStr)
	if err != nil {
		return time.Time{}, decimal.Decimal{}, fmt.Errorf("parsing gasUsed: %w", err)
	}
	gasPrice, err := decimal.NewFromString(gasPriceStr)
	if err != nil {
		return time.Time{}, decimal.Decimal{}, fmt.Errorf("parsing gasPrice: %w", err)
	}
	gasFeeAmount := gasUsed.Mul(gasPrice).Shift(-18)
	return time.Unix(tsInt, 0).UTC(), gasFeeAmount, nil
}

// parseAmount decimal-shifts a raw on-chain integer amount by decimals -
// e.g. wei -> ETH (18) or a raw ERC-20 amount -> its display unit, using
// each token's own decimals rather than assuming 18.
func parseAmount(raw string, decimals int) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Decimal{}, err
	}
	return amount.Shift(int32(-decimals)), nil
}

// rawNormalTx mirrors Etherscan's txlist record shape (native transfers +
// contract calls). Every numeric field arrives as a JSON string.
type rawNormalTx struct {
	BlockNumber     string `json:"blockNumber"`
	TimeStamp       string `json:"timeStamp"`
	Hash            string `json:"hash"`
	From            string `json:"from"`
	To              string `json:"to"`
	Value           string `json:"value"`
	GasUsed         string `json:"gasUsed"`
	GasPrice        string `json:"gasPrice"`
	IsError         string `json:"isError"` // "0" success, "1" reverted
	ContractAddress string `json:"contractAddress"`
}

func (r rawNormalTx) blockNum() (int, error) { return strconv.Atoi(r.BlockNumber) }

func (r rawNormalTx) toTransaction(chainID int, info chainInfo) (domain.Transaction, error) {
	ts, gasFee, err := parseCommon(r.TimeStamp, r.GasUsed, r.GasPrice)
	if err != nil {
		return domain.Transaction{}, err
	}
	amount, err := parseAmount(r.Value, 18)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("parsing value: %w", err)
	}
	blockNum, err := r.blockNum()
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("parsing blockNumber: %w", err)
	}

	return domain.Transaction{
		TxHash:          r.Hash,
		ChainID:         chainID,
		BlockNumber:     uint64(blockNum),
		BlockTimestamp:  ts,
		Success:         r.IsError == "0",
		FromAddress:     r.From,
		ToAddress:       r.To,
		AssetSymbol:     info.nativeSymbol,
		ContractAddress: nil,
		AssetDecimals:   18,
		Amount:          amount,
		GasFeeAmount:    gasFee,
		GasFeeAsset:     info.nativeSymbol,
		ExplorerURL:     info.explorerBase + "/tx/" + r.Hash,
	}, nil
}

// rawTokenTx mirrors Etherscan's tokentx record shape (ERC-20 transfers,
// parsed from Transfer event logs).
type rawTokenTx struct {
	BlockNumber     string `json:"blockNumber"`
	TimeStamp       string `json:"timeStamp"`
	Hash            string `json:"hash"`
	From            string `json:"from"`
	To              string `json:"to"`
	Value           string `json:"value"`
	ContractAddress string `json:"contractAddress"`
	TokenSymbol     string `json:"tokenSymbol"`
	TokenDecimal    string `json:"tokenDecimal"`
	GasUsed         string `json:"gasUsed"`
	GasPrice        string `json:"gasPrice"`
}

func (r rawTokenTx) blockNum() (int, error) { return strconv.Atoi(r.BlockNumber) }

func (r rawTokenTx) toTransaction(chainID int, info chainInfo) (domain.Transaction, error) {
	ts, gasFee, err := parseCommon(r.TimeStamp, r.GasUsed, r.GasPrice)
	if err != nil {
		return domain.Transaction{}, err
	}
	decimals, err := strconv.Atoi(r.TokenDecimal)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("parsing tokenDecimal: %w", err)
	}
	amount, err := parseAmount(r.Value, decimals)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("parsing value: %w", err)
	}
	blockNum, err := r.blockNum()
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("parsing blockNumber: %w", err)
	}
	contract := r.ContractAddress

	return domain.Transaction{
		TxHash:         r.Hash,
		ChainID:        chainID,
		BlockNumber:    uint64(blockNum),
		BlockTimestamp: ts,
		// A Transfer event log only exists for a call that succeeded -
		// a reverted transaction never emits one, so every tokentx
		// record is inherently a successful transfer.
		Success:         true,
		FromAddress:     r.From,
		ToAddress:       r.To,
		AssetSymbol:     r.TokenSymbol,
		ContractAddress: &contract,
		AssetDecimals:   decimals,
		Amount:          amount,
		GasFeeAmount:    gasFee,
		GasFeeAsset:     info.nativeSymbol,
		ExplorerURL:     info.explorerBase + "/tx/" + r.Hash,
	}, nil
}
