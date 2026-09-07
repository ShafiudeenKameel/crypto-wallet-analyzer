package costbasis

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

func usd(v string) decimal.Decimal { return decimal.RequireFromString(v) }
func usdPtr(v string) *decimal.Decimal {
	d := usd(v)
	return &d
}

func baseTx(hoursAgo int) domain.Transaction {
	return domain.Transaction{
		TxHash:         "tx",
		ChainID:        1,
		Success:        true,
		AssetSymbol:    "ETH",
		BlockTimestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(hoursAgo) * time.Hour),
	}
}

func TestCompute_FIFOMatchesOldestLotFirst(t *testing.T) {
	// Buy 1 @ $10, buy 1 @ $20, then sell 1.5. FIFO must consume all of the
	// $10 lot plus half of the $20 lot: cost basis = 10 + 10 = $20, not the
	// $15 an average-cost method would give.
	buy1 := baseTx(0)
	buy1.Direction = domain.DirectionIn
	buy1.Type = domain.TxTransfer
	buy1.Amount = usd("1")
	buy1.PriceUSD = usdPtr("10")

	buy2 := baseTx(1)
	buy2.Direction = domain.DirectionIn
	buy2.Type = domain.TxTransfer
	buy2.Amount = usd("1")
	buy2.PriceUSD = usdPtr("20")

	sell := baseTx(2)
	sell.Direction = domain.DirectionOut
	sell.Type = domain.TxTransfer
	sell.Amount = usd("1.5")
	sell.PriceUSD = usdPtr("30")

	result := Compute([]domain.Transaction{buy1, buy2, sell}, nil)

	if len(result.PerAsset) != 1 {
		t.Fatalf("PerAsset = %d entries, want 1", len(result.PerAsset))
	}
	got := result.PerAsset[0]

	wantCostBasis := usd("20")
	if len(got.Disposals) != 1 || !got.Disposals[0].CostBasisUSD.Equal(wantCostBasis) {
		t.Fatalf("disposal cost basis = %+v, want %s", got.Disposals, wantCostBasis)
	}

	wantGain := usd("25") // proceeds 45 (1.5 * 30) - cost basis 20
	if !got.RealizedGainUSD.Equal(wantGain) {
		t.Errorf("RealizedGainUSD = %s, want %s", got.RealizedGainUSD, wantGain)
	}

	wantRemaining := usd("0.5")
	if len(got.RemainingLots) != 1 || !got.RemainingLots[0].Quantity.Equal(wantRemaining) {
		t.Fatalf("RemainingLots = %+v, want one lot of %s", got.RemainingLots, wantRemaining)
	}
}

func TestCompute_StakingRewardIsIncomeAndALot(t *testing.T) {
	reward := baseTx(0)
	reward.Direction = domain.DirectionIn
	reward.Type = domain.TxStakingReward
	reward.Amount = usd("2")
	reward.PriceUSD = usdPtr("5")

	result := Compute([]domain.Transaction{reward}, nil)

	got := result.PerAsset[0]
	wantValue := usd("10")
	if len(got.IncomeEvents) != 1 || !got.IncomeEvents[0].ValueUSD.Equal(wantValue) {
		t.Fatalf("IncomeEvents = %+v, want one event of %s", got.IncomeEvents, wantValue)
	}
	if !got.TotalIncomeUSD.Equal(wantValue) {
		t.Errorf("TotalIncomeUSD = %s, want %s", got.TotalIncomeUSD, wantValue)
	}
	if len(got.RemainingLots) != 1 {
		t.Errorf("RemainingLots = %d, want 1 (the reward is also a lot)", len(got.RemainingLots))
	}
}

func TestCompute_GasOnlyCountedForOutgoingTx(t *testing.T) {
	incoming := baseTx(0)
	incoming.Direction = domain.DirectionIn
	incoming.Type = domain.TxTransfer
	incoming.Amount = usd("1")
	incoming.PriceUSD = usdPtr("100")
	incoming.GasFeeUSD = usdPtr("999") // someone else paid this - must not be counted

	outgoing := baseTx(1)
	outgoing.Direction = domain.DirectionOut
	outgoing.Type = domain.TxGasFee // zero-amount contract call the wallet initiated
	outgoing.Amount = decimal.Zero
	outgoing.GasFeeUSD = usdPtr("3")

	result := Compute([]domain.Transaction{incoming, outgoing}, nil)

	want := usd("3")
	if !result.Totals.GasFeesUSD.Equal(want) {
		t.Errorf("GasFeesUSD = %s, want %s", result.Totals.GasFeesUSD, want)
	}
}

func TestCompute_UnknownAndAmbiguousTxsAreSkippedNotDropped(t *testing.T) {
	unknown := baseTx(0)
	unknown.Direction = domain.DirectionIn
	unknown.Type = domain.TxUnknown
	unknown.Amount = usd("1")

	ambiguous := baseTx(1)
	ambiguous.Direction = domain.DirectionNA
	ambiguous.Type = domain.TxTransfer
	ambiguous.Amount = usd("1")
	ambiguous.PriceUSD = usdPtr("1")

	result := Compute([]domain.Transaction{unknown, ambiguous}, nil)

	if len(result.Skipped) != 2 {
		t.Fatalf("Skipped = %d, want 2", len(result.Skipped))
	}
	if len(result.PerAsset) != 0 {
		t.Errorf("PerAsset = %d, want 0 (nothing confidently attributable)", len(result.PerAsset))
	}
}

func TestCompute_UnrealizedGainOnlyWhenCurrentPriceSupplied(t *testing.T) {
	buy := baseTx(0)
	buy.Direction = domain.DirectionIn
	buy.Type = domain.TxTransfer
	buy.Amount = usd("2")
	buy.PriceUSD = usdPtr("10")

	withoutPrice := Compute([]domain.Transaction{buy}, nil)
	if withoutPrice.PerAsset[0].UnrealizedGainUSD != nil {
		t.Errorf("UnrealizedGainUSD = %v, want nil when no current price supplied", withoutPrice.PerAsset[0].UnrealizedGainUSD)
	}

	withPrice := Compute([]domain.Transaction{buy}, []CurrentPrice{
		{Asset: domain.AssetRef{ChainID: 1, Symbol: "ETH"}, PriceUSD: usd("15")},
	})
	got := withPrice.PerAsset[0].UnrealizedGainUSD
	want := usd("10") // holdings worth 2*15=30, cost basis 2*10=20, gain=10
	if got == nil || !got.Equal(want) {
		t.Errorf("UnrealizedGainUSD = %v, want %s", got, want)
	}
}
