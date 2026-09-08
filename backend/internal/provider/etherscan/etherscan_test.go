package etherscan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/time/rate"
)

func usd(v string) decimal.Decimal { return decimal.RequireFromString(v) }

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.baseURL = srv.URL
	return c
}

func writeEnvelope(w http.ResponseWriter, result any) {
	json.NewEncoder(w).Encode(map[string]any{"status": "1", "message": "OK", "result": result})
}

func TestFetchTransactions_MapsNativeAndTokenRecords(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "txlist":
			writeEnvelope(w, []map[string]string{{
				"blockNumber": "100", "timeStamp": "1700000000", "hash": "0xnative",
				"from": "0xAlice", "to": "0xBob", "value": "1000000000000000000", // 1 ETH in wei
				"gasUsed": "21000", "gasPrice": "2000000000", "isError": "0",
			}})
		case "tokentx":
			writeEnvelope(w, []map[string]string{{
				"blockNumber": "101", "timeStamp": "1700000100", "hash": "0xtoken",
				"from": "0xAlice", "to": "0xBob", "value": "5000000", // 5 USDC (6 decimals)
				"contractAddress": "0xUSDC", "tokenSymbol": "USDC", "tokenDecimal": "6",
				"gasUsed": "50000", "gasPrice": "2000000000",
			}})
		}
	})

	txs, err := c.FetchTransactions(context.Background(), "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", 1)
	if err != nil {
		t.Fatalf("FetchTransactions failed: %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("got %d transactions, want 2", len(txs))
	}

	native, token := txs[0], txs[1]
	if native.TxHash != "0xnative" || !native.Success || native.ContractAddress != nil {
		t.Errorf("native tx mapped wrong: %+v", native)
	}
	if !native.Amount.Equal(usd("1")) {
		t.Errorf("native amount = %s, want 1 (wei->ETH shift)", native.Amount)
	}
	if native.ExplorerURL != "https://etherscan.io/tx/0xnative" {
		t.Errorf("ExplorerURL = %q", native.ExplorerURL)
	}

	if token.TxHash != "0xtoken" || token.ContractAddress == nil || *token.ContractAddress != "0xUSDC" {
		t.Errorf("token tx mapped wrong: %+v", token)
	}
	if !token.Amount.Equal(usd("5")) {
		t.Errorf("token amount = %s, want 5 (6-decimal shift)", token.Amount)
	}
	if !token.Success {
		t.Error("token transfer Success = false, want true (Transfer events only exist for successful calls)")
	}
}

func TestFetchTransactions_RejectsInvalidAddressBeforeAnyRequest(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	_, err := c.FetchTransactions(context.Background(), "not-an-address", 1)
	if err == nil {
		t.Fatal("err = nil, want an address-validation error")
	}
	if called {
		t.Error("network was called despite an invalid address - validation must happen first")
	}
}

func TestFetchTransactions_UnsupportedChainNeverCallsNetwork(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	_, err := c.FetchTransactions(context.Background(), "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", 999999)
	if err == nil {
		t.Fatal("err = nil, want an unsupported-chain error")
	}
	if called {
		t.Error("network was called for an unsupported chain")
	}
}

// TestFetchTransactions_AdvancesPastTenThousandWindow proves the wallet
// isn't silently truncated once Etherscan's page*offset<=10000 window is
// exhausted: 10 full pages (10000 records) must trigger a startblock
// advance and a fresh page-1 request, picking up the remaining records.
func TestFetchTransactions_AdvancesPastTenThousandWindow(t *testing.T) {
	var txlistRequests atomic.Int32 // pages within a wave are fetched concurrently - this handler is called from multiple goroutines
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("action") == "tokentx" {
			writeEnvelope(w, []map[string]string{})
			return
		}
		txlistRequests.Add(1)

		startBlock, _ := strconv.Atoi(q.Get("startblock"))
		page, _ := strconv.Atoi(q.Get("page"))
		count := pageSize
		if startBlock != 0 {
			count = 500 // round 2's first page - partial, ends pagination
		}

		records := make([]map[string]string, count)
		for i := 0; i < count; i++ {
			blockNum := startBlock + (page-1)*pageSize + i
			records[i] = map[string]string{
				"blockNumber": strconv.Itoa(blockNum), "timeStamp": "1700000000", "hash": "0x" + strconv.Itoa(blockNum),
				"from": "0xAlice", "to": "0xBob", "value": "0", "gasUsed": "21000", "gasPrice": "1", "isError": "0",
			}
		}
		writeEnvelope(w, records)
	})

	txs, err := c.FetchTransactions(context.Background(), "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", 1)
	if err != nil {
		t.Fatalf("FetchTransactions failed: %v", err)
	}

	wantTotal := 10*pageSize + 500
	if len(txs) != wantTotal {
		t.Errorf("got %d transactions, want %d", len(txs), wantTotal)
	}
	// 10 to fill the window (2 waves of 4 + 1 wave of 2, all full) + 4 more
	// for round 2's first wave, which fetches pages 1-4 concurrently before
	// noticing page 1 was already partial - the bounded waste (up to
	// pageWaveSize-1 extra calls) that concurrent pagination trades for
	// speed on genuinely large wallets. Only page 1's 500 records get used;
	// pages 2-4's results are fetched but never appended - see fetchWindow.
	if wantRequests := int32(14); txlistRequests.Load() != wantRequests {
		t.Errorf("made %d txlist requests, want %d", txlistRequests.Load(), wantRequests)
	}
}

// TestFetchTransactions_FetchesTxlistAndTokentxConcurrently proves the two
// endpoints run in parallel rather than one after the other: each response
// is delayed 50ms, so a sequential implementation would take >=100ms while
// a concurrent one takes ~50ms.
func TestFetchTransactions_FetchesTxlistAndTokentxConcurrently(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		writeEnvelope(w, []map[string]string{})
	})

	start := time.Now()
	if _, err := c.FetchTransactions(context.Background(), "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", 1); err != nil {
		t.Fatalf("FetchTransactions failed: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed >= 90*time.Millisecond {
		t.Errorf("took %v, want close to 50ms (concurrent) not ~100ms (sequential)", elapsed)
	}
}

// TestFetchWindow_PagesFetchConcurrently proves pages within one window
// actually fetch concurrently. Calls fetchWindow directly (same package)
// rather than the full FetchTransactions - isolates the measurement from
// rate limiting (disabled here; tested separately) and from tokentx's
// concurrent-but-otherwise-irrelevant noise, so the timing signal is
// clean instead of accumulating scheduling noise across many rounds.
func TestFetchWindow_PagesFetchConcurrently(t *testing.T) {
	const delay = 60 * time.Millisecond
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		records := make([]map[string]string, pageSize)
		for i := range records {
			records[i] = map[string]string{
				"blockNumber": strconv.Itoa(i), "timeStamp": "1700000000", "hash": "0xh",
				"from": "0xAlice", "to": "0xBob", "value": "0", "gasUsed": "21000", "gasPrice": "1", "isError": "0",
			}
		}
		writeEnvelope(w, records)
	})
	c.limiter = rate.NewLimiter(rate.Inf, 1000) // isolate concurrency from rate limiting

	start := time.Now()
	_, _, err := fetchWindow[rawNormalTx](context.Background(), c, 1, "txlist", "0xAlice", 0)
	if err != nil {
		t.Fatalf("fetchWindow failed: %v", err)
	}
	elapsed := time.Since(start)

	// One window = 10 pages, pageWaveSize=4 -> 3 waves (4+4+2). If pages
	// within a wave truly run concurrently, elapsed is roughly 3*delay
	// (180ms); sequential one-at-a-time fetching would need 10*delay
	// (600ms). Asserting well under that, with generous margin for
	// scheduling noise, while still clearly distinguishing the two.
	if maxExpected := 7 * delay; elapsed >= maxExpected {
		t.Errorf("took %v, want under %v (~3 wave-durations, not 10 sequential page fetches)", elapsed, maxExpected)
	}
}

func TestFetchTransactions_NoTransactionsFoundIsNotAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "0", "message": "No transactions found", "result": []any{}})
	})

	txs, err := c.FetchTransactions(context.Background(), "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", 1)
	if err != nil {
		t.Fatalf("FetchTransactions failed: %v", err)
	}
	if len(txs) != 0 {
		t.Errorf("got %d transactions, want 0", len(txs))
	}
}
