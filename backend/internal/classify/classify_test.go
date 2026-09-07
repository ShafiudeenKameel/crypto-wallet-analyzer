package classify

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

const wallet = "0xWallet"

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		tx       domain.Transaction
		wantDir  domain.Direction
		wantType domain.TxType
	}{
		{
			name: "incoming known asset is a transfer",
			tx: domain.Transaction{
				ToAddress:   wallet,
				FromAddress: "0xSomeoneElse",
				AssetSymbol: "USDC",
				Amount:      decimal.NewFromInt(100),
			},
			wantDir:  domain.DirectionIn,
			wantType: domain.TxTransfer,
		},
		{
			name: "outgoing known asset is a transfer",
			tx: domain.Transaction{
				FromAddress: wallet,
				ToAddress:   "0xSomeoneElse",
				AssetSymbol: "ETH",
				Amount:      decimal.NewFromInt(1),
			},
			wantDir:  domain.DirectionOut,
			wantType: domain.TxTransfer,
		},
		{
			name: "zero amount is gas fee regardless of asset",
			tx: domain.Transaction{
				FromAddress: wallet,
				ToAddress:   "0xSomeContract",
				AssetSymbol: "",
				Amount:      decimal.Zero,
			},
			wantDir:  domain.DirectionOut,
			wantType: domain.TxGasFee,
		},
		{
			name: "non-zero amount with no recognized asset is unknown",
			tx: domain.Transaction{
				ToAddress:   wallet,
				FromAddress: "0xSomeContract",
				AssetSymbol: "",
				Amount:      decimal.NewFromInt(50),
			},
			wantDir:  domain.DirectionIn,
			wantType: domain.TxUnknown,
		},
		{
			name: "neither address matches wallet",
			tx: domain.Transaction{
				FromAddress: "0xA",
				ToAddress:   "0xB",
				AssetSymbol: "USDC",
				Amount:      decimal.NewFromInt(10),
			},
			wantDir:  domain.DirectionNA,
			wantType: domain.TxTransfer,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.tx, wallet)
			if got.Direction != c.wantDir {
				t.Errorf("Direction = %q, want %q", got.Direction, c.wantDir)
			}
			if got.Type != c.wantType {
				t.Errorf("Type = %q, want %q", got.Type, c.wantType)
			}
		})
	}
}
