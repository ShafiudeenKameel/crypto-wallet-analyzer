// Command server wires the pipeline together and starts listening. It
// contains no business logic - only dependency construction and config
// from the environment.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/analysis"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/httpapi"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/provider/coingecko"
	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/provider/etherscan"
)

func main() {
	// Provider API keys degrade gracefully to each provider's public tier
	// when unset (see NewClient in each package) - but CORS gets no such
	// default: a demo running open to any origin is a real exposure, not
	// a convenience, so we fail fast instead.
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		log.Fatal("ALLOWED_ORIGIN must be set (no wildcard CORS default)")
	}

	chain := etherscan.NewClient(os.Getenv("ETHERSCAN_API_KEY"))
	prices := coingecko.NewClient(os.Getenv("COINGECKO_API_KEY"))
	service := analysis.NewService(chain, prices)
	server := httpapi.NewServer(service, allowedOrigin)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on :%s (allowed origin: %s)", port, allowedOrigin)
	if err := http.ListenAndServe(":"+port, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
