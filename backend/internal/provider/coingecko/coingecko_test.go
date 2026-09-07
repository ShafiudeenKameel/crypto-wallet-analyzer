package coingecko

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/provider"
)

func usd(v string) decimal.Decimal { return decimal.RequireFromString(v) }

func mustParseDate(t *testing.T) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, "2024-06-15T12:00:00Z")
	if err != nil {
		t.Fatalf("parsing test date: %v", err)
	}
	return tm
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.baseURL = srv.URL
	return c
}

func TestGetPriceAt_NativeToken(t *testing.T) {
	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"market_data":{"current_price":{"usd":2500.5}}}`))
	})

	price, granularity, err := c.GetPriceAt(context.Background(), domain.AssetRef{ChainID: 1, Symbol: "ETH"}, mustParseDate(t))
	if err != nil {
		t.Fatalf("GetPriceAt failed: %v", err)
	}
	if wantPath := "/coins/ethereum/history"; gotPath != wantPath {
		t.Errorf("request path = %q, want %q", gotPath, wantPath)
	}
	if !price.Equal(usd("2500.5")) {
		t.Errorf("price = %s, want 2500.5", price)
	}
	if granularity != domain.GranularityDaily {
		t.Errorf("granularity = %q, want %q", granularity, domain.GranularityDaily)
	}
}

func TestGetPriceAt_ContractToken(t *testing.T) {
	contract := "0xUSDC"
	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"market_data":{"current_price":{"usd":1.0}}}`))
	})

	_, _, err := c.GetPriceAt(context.Background(), domain.AssetRef{ChainID: 137, ContractAddress: &contract, Symbol: "USDC"}, mustParseDate(t))
	if err != nil {
		t.Fatalf("GetPriceAt failed: %v", err)
	}
	if wantPath := "/coins/polygon-pos/contract/0xUSDC/history"; gotPath != wantPath {
		t.Errorf("request path = %q, want %q", gotPath, wantPath)
	}
}

func TestGetPriceAt_RateLimited(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, _, err := c.GetPriceAt(context.Background(), domain.AssetRef{ChainID: 1, Symbol: "ETH"}, mustParseDate(t))
	if !errors.Is(err, provider.ErrRateLimited) {
		t.Errorf("err = %v, want it to wrap provider.ErrRateLimited", err)
	}
}

func TestGetPriceAt_UnsupportedChainNeverCallsNetwork(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	_, _, err := c.GetPriceAt(context.Background(), domain.AssetRef{ChainID: 999999, Symbol: "???"}, mustParseDate(t))
	if err == nil {
		t.Fatal("err = nil, want an unsupported-chain error")
	}
	if called {
		t.Error("network was called for an unsupported chain - should fail before ever requesting")
	}
}

func TestGetPriceAt_SendsAPIKeyAsHeaderNotQueryParam(t *testing.T) {
	var gotHeader, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-cg-demo-api-key")
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"market_data":{"current_price":{"usd":1}}}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient("secret-key")
	c.baseURL = srv.URL
	if _, _, err := c.GetPriceAt(context.Background(), domain.AssetRef{ChainID: 1, Symbol: "ETH"}, mustParseDate(t)); err != nil {
		t.Fatalf("GetPriceAt failed: %v", err)
	}

	if gotHeader != "secret-key" {
		t.Errorf("api key header = %q, want %q", gotHeader, "secret-key")
	}
	if strings.Contains(gotQuery, "secret-key") {
		t.Errorf("query string %q must never contain the api key", gotQuery)
	}
}
