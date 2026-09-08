// Package httpapi is the thin HTTP layer: routing, request/response JSON,
// CORS, and our own rate limit. No business logic lives here - every
// handler just translates HTTP into a call on analysis.Service.
package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/analysis"
)

// requestTimeout bounds how long a single /api/analyze call may run,
// regardless of client behavior - a truly pathological wallet (or a
// client that never disconnects) shouldn't be able to hang a server
// goroutine forever. Kept slightly under the frontend's own timeout (see
// api.ts's REQUEST_TIMEOUT_MS) so the backend gets a chance to return a
// real error before the frontend just gives up waiting.
//
// This is also a real, reachable ceiling, not just defense-in-depth: an
// extremely high-volume wallet (see etherscan.Client.FetchTransactions'
// doc comment) can need more total Etherscan pages than our shared rate
// limiter can serve within this window. That's an accepted limitation
// for a free-tier demo, not a bug - see project.MD's own scoping stance
// on not over-engineering for the rare extreme case.
const requestTimeout = 170 * time.Second

// Analyzer is what a Server needs from the pipeline. *analysis.Service
// satisfies it; tests can supply a trivial fake instead of standing up a
// real Etherscan/CoinGecko-backed pipeline.
type Analyzer interface {
	Analyze(ctx context.Context, walletAddress string, chainID int) (analysis.Result, error)
}

// Server builds the routed, middleware-wrapped HTTP handler.
type Server struct {
	analyzer      Analyzer
	allowedOrigin string

	// limiter guards /api/analyze specifically: it's the one endpoint that
	// fans out to both third-party APIs, so it's what actually protects
	// our shared Etherscan/CoinGecko quota from being burned by our own
	// public endpoint being hit too fast - not primarily an anti-abuse
	// measure (that would want per-client tracking, a v2 concern).
	limiter *rate.Limiter
	mux     *http.ServeMux
}

// NewServer builds a Server. allowedOrigin is the single frontend origin
// CORS will permit - there is deliberately no wildcard fallback.
func NewServer(analyzer Analyzer, allowedOrigin string) *Server {
	s := &Server{
		analyzer:      analyzer,
		allowedOrigin: allowedOrigin,
		limiter:       rate.NewLimiter(rate.Limit(0.2), 2), // ~12/min
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/analyze", s.handleAnalyze)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux = mux

	return s
}

// Handler returns the fully wrapped handler (CORS -> HTTPS check -> mux).
func (s *Server) Handler() http.Handler {
	return s.withCORS(enforceHTTPS(s.mux))
}

type analyzeRequest struct {
	WalletAddress string `json:"walletAddress"`
	ChainID       int    `json:"chainId"`
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	// Allow(), not Wait(): a blocked HTTP handler holds the connection
	// open indefinitely, which is worse than just telling the client to
	// retry.
	if !s.limiter.Allow() {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded - try again shortly")
		return
	}

	var req analyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	result, err := s.analyzer.Analyze(ctx, req.WalletAddress, req.ChainID)
	if err != nil {
		// Log the real error server-side; the client gets a generic
		// message - never our internal error text (which could
		// otherwise leak provider-specific detail) back over the wire.
		log.Printf("analyze failed (chain=%d): %v", req.ChainID, err)
		writeError(w, http.StatusBadGateway, "failed to analyze wallet")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// withCORS permits only the configured frontend origin - never "*".
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// enforceHTTPS rejects a request forwarded as plain HTTP. TLS termination
// itself happens at the hosting platform's edge (e.g. Render); this only
// catches a proxy that explicitly reports it forwarded an insecure
// request, via the header such a proxy sets.
func enforceHTTPS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-Proto") == "http" {
			writeError(w, http.StatusBadRequest, "https required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
