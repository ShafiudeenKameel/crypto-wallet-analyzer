package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/analysis"
)

type fakeAnalyzer struct {
	result analysis.Result
	err    error
}

func (f fakeAnalyzer) Analyze(ctx context.Context, walletAddress string, chainID int) (analysis.Result, error) {
	return f.result, f.err
}

func TestHandleAnalyze_HappyPath(t *testing.T) {
	srv := NewServer(fakeAnalyzer{result: analysis.Result{TransactionCount: 3}}, "https://example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBufferString(`{"walletAddress":"0xabc","chainId":1}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("CORS origin header = %q, want %q", got, "https://example.com")
	}
}

func TestHandleAnalyze_InvalidBodyIsBadRequest(t *testing.T) {
	srv := NewServer(fakeAnalyzer{}, "https://example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBufferString(`not json`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAnalyze_AnalyzerErrorNeverLeaksToClient(t *testing.T) {
	srv := NewServer(fakeAnalyzer{err: errSecretLeak{}}, "https://example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBufferString(`{"walletAddress":"0xabc","chainId":1}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("super-secret-api-key")) {
		t.Errorf("response body leaked the internal error: %s", rec.Body.String())
	}
}

type errSecretLeak struct{}

func (errSecretLeak) Error() string {
	return "request to https://api.example.com?apikey=super-secret-api-key failed"
}

func TestRateLimit_BurstThenReject(t *testing.T) {
	srv := NewServer(fakeAnalyzer{result: analysis.Result{}}, "https://example.com")

	body := `{"walletAddress":"0xabc","chainId":1}`
	var lastCode int
	for i := 0; i < 3; i++ { // burst is 2 - the 3rd request in immediate succession must be rejected
		req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("3rd immediate request status = %d, want 429", lastCode)
	}
}

func TestEnforceHTTPS_RejectsForwardedHTTP(t *testing.T) {
	srv := NewServer(fakeAnalyzer{}, "https://example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Forwarded-Proto", "http")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when X-Forwarded-Proto=http", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	srv := NewServer(fakeAnalyzer{}, "https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
