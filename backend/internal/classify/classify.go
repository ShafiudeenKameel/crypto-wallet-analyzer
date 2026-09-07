// Package classify assigns a Type and Direction to a fetched transaction.
// It is pure logic - no I/O, deterministic given the same inputs - which is
// what makes it directly unit-testable and safe to run concurrently later.
package classify

import (
	"strings"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

// Classify returns tx with Direction and Type filled in, relative to
// walletAddress (the wallet this analysis is for).
//
// v1 recognizes plain transfers and zero-value (gas-only) transactions.
// Distinguishing staking rewards / airdrops from an ordinary incoming
// transfer needs positive evidence (a known-contract allowlist, a method
// signature) we don't have real sample data for yet - those branches
// aren't reached, rather than guessed at.
func Classify(tx domain.Transaction, walletAddress string) domain.Transaction {
	tx.Direction = direction(tx, walletAddress)
	tx.Type = txType(tx)
	return tx
}

func direction(tx domain.Transaction, walletAddress string) domain.Direction {
	switch {
	case strings.EqualFold(tx.ToAddress, walletAddress):
		return domain.DirectionIn
	case strings.EqualFold(tx.FromAddress, walletAddress):
		return domain.DirectionOut
	default:
		return domain.DirectionNA
	}
}

func txType(tx domain.Transaction) domain.TxType {
	switch {
	case tx.Amount.IsZero():
		return domain.TxGasFee // nothing moved - only the gas fee is real
	case tx.AssetSymbol == "":
		return domain.TxUnknown // something moved, but we don't recognize what
	default:
		return domain.TxTransfer
	}
}
