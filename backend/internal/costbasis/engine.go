// Package costbasis computes FIFO cost basis, realized/unrealized gain,
// income, and gas totals from a wallet's classified, priced transactions.
// Like classify, it is pure - no I/O - so it takes the current market price
// as an input rather than fetching it itself.
package costbasis

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/ShafiudeenKameel/crypto-wallet-analyzer/backend/internal/domain"
)

// assetKey is a comparable map key for grouping by asset. domain.AssetRef
// itself isn't comparable (it holds a *string), so this is the map-friendly
// equivalent, built the same way from a Transaction or a CurrentPrice.
type assetKey struct {
	chainID  int
	contract string // "" = native token
}

func keyFor(chainID int, contractAddress *string) assetKey {
	contract := ""
	if contractAddress != nil {
		contract = strings.ToLower(*contractAddress)
	}
	return assetKey{chainID: chainID, contract: contract}
}

// state is the mutable working data for one asset while Compute walks the
// transaction history in order.
type state struct {
	asset domain.AssetRef
	lots  []Lot // FIFO queue: oldest first

	disposals    []Disposal
	incomeEvents []IncomeEvent

	totalSpentUSD    decimal.Decimal
	totalReceivedUSD decimal.Decimal
	realizedGainUSD  decimal.Decimal
	totalIncomeUSD   decimal.Decimal
}

// Compute runs FIFO cost-basis matching over txs (any order - sorted by
// BlockTimestamp internally) and returns one AssetSummary per asset
// touched, plus wallet-level Totals.
func Compute(txs []domain.Transaction, currentPrices []CurrentPrice) Result {
	sorted := append([]domain.Transaction(nil), txs...) // don't mutate caller's slice
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].BlockTimestamp.Before(sorted[j].BlockTimestamp)
	})

	prices := make(map[assetKey]decimal.Decimal, len(currentPrices))
	for _, cp := range currentPrices {
		prices[keyFor(cp.Asset.ChainID, cp.Asset.ContractAddress)] = cp.PriceUSD
	}

	states := make(map[assetKey]*state)
	var gasFeesUSD decimal.Decimal
	// Non-nil even when empty: encoding/json marshals a nil slice as null,
	// and a well-behaved API shouldn't make every caller defensively
	// null-check an array that's simply empty.
	skipped := make([]domain.Transaction, 0)

	for _, tx := range sorted {
		if gas, ok := gasPaidUSD(tx); ok {
			gasFeesUSD = gasFeesUSD.Add(gas)
		}

		switch {
		case tx.Type == domain.TxGasFee, !tx.Success:
			continue // nothing moved - any gas was already counted above
		case tx.Type == domain.TxUnknown:
			skipped = append(skipped, tx)
			continue
		case tx.Direction == domain.DirectionNA:
			skipped = append(skipped, tx)
			continue
		case tx.PriceUSD == nil:
			skipped = append(skipped, tx) // can't value this event at all
			continue
		}

		key := keyFor(tx.ChainID, tx.ContractAddress)
		st, exists := states[key]
		if !exists {
			st = &state{
				asset:        domain.AssetRef{ChainID: tx.ChainID, ContractAddress: tx.ContractAddress, Symbol: tx.AssetSymbol},
				lots:         make([]Lot, 0),
				disposals:    make([]Disposal, 0),
				incomeEvents: make([]IncomeEvent, 0),
			}
			states[key] = st
		}

		valueUSD := tx.Amount.Mul(*tx.PriceUSD)
		if tx.Direction == domain.DirectionOut {
			st.recordDisposal(tx, valueUSD)
		} else {
			st.recordAcquisition(tx, valueUSD)
		}
	}

	result := Result{Skipped: skipped, PerAsset: make([]AssetSummary, 0, len(states))}
	result.Totals.GasFeesUSD = gasFeesUSD

	for _, st := range states {
		currentPrice, havePrice := prices[keyFor(st.asset.ChainID, st.asset.ContractAddress)]
		summary := st.summary(currentPrice, havePrice)
		result.PerAsset = append(result.PerAsset, summary)

		result.Totals.TotalSpentUSD = result.Totals.TotalSpentUSD.Add(summary.TotalSpentUSD)
		result.Totals.TotalReceivedUSD = result.Totals.TotalReceivedUSD.Add(summary.TotalReceivedUSD)
		result.Totals.RealizedGainUSD = result.Totals.RealizedGainUSD.Add(summary.RealizedGainUSD)
		result.Totals.TotalIncomeUSD = result.Totals.TotalIncomeUSD.Add(summary.TotalIncomeUSD)
		if summary.UnrealizedGainUSD != nil {
			result.Totals.UnrealizedGainUSD = result.Totals.UnrealizedGainUSD.Add(*summary.UnrealizedGainUSD)
		}
	}

	return result
}

// gasPaidUSD returns the gas cost attributable to the wallet for tx - only
// a transaction's sender pays gas in EVM, so an incoming transfer's gas was
// paid by someone else and isn't counted here.
func gasPaidUSD(tx domain.Transaction) (decimal.Decimal, bool) {
	if tx.Direction != domain.DirectionOut || tx.GasFeeUSD == nil {
		return decimal.Decimal{}, false
	}
	return *tx.GasFeeUSD, true
}

func (s *state) recordAcquisition(tx domain.Transaction, valueUSD decimal.Decimal) {
	s.lots = append(s.lots, Lot{
		Quantity:     tx.Amount,
		CostBasisUSD: valueUSD,
		AcquiredAt:   tx.BlockTimestamp,
		SourceTxHash: tx.TxHash,
	})
	s.totalSpentUSD = s.totalSpentUSD.Add(valueUSD)

	if tx.Type == domain.TxStakingReward || tx.Type == domain.TxAirdrop {
		s.totalIncomeUSD = s.totalIncomeUSD.Add(valueUSD)
		s.incomeEvents = append(s.incomeEvents, IncomeEvent{
			SourceTxHash: tx.TxHash,
			Type:         tx.Type,
			Quantity:     tx.Amount,
			ValueUSD:     valueUSD,
		})
	}
}

func (s *state) recordDisposal(tx domain.Transaction, proceedsUSD decimal.Decimal) {
	costBasisUSD := s.consumeLots(tx.Amount)
	gain := proceedsUSD.Sub(costBasisUSD)

	s.totalReceivedUSD = s.totalReceivedUSD.Add(proceedsUSD)
	s.realizedGainUSD = s.realizedGainUSD.Add(gain)
	s.disposals = append(s.disposals, Disposal{
		SourceTxHash:    tx.TxHash,
		Quantity:        tx.Amount,
		ProceedsUSD:     proceedsUSD,
		CostBasisUSD:    costBasisUSD,
		RealizedGainUSD: gain,
	})
}

// consumeLots removes qty from the FIFO queue (oldest lots first) and
// returns the total cost basis consumed. If qty exceeds everything on
// record - disposing of holdings acquired before our tracked history began
// - the shortfall is treated as zero-cost-basis: conservative, since it
// overstates realized gain rather than silently understating it.
func (s *state) consumeLots(qty decimal.Decimal) decimal.Decimal {
	var costBasisUSD decimal.Decimal
	remaining := qty

	for len(s.lots) > 0 && remaining.IsPositive() {
		lot := &s.lots[0]
		if lot.Quantity.LessThanOrEqual(remaining) {
			costBasisUSD = costBasisUSD.Add(lot.CostBasisUSD)
			remaining = remaining.Sub(lot.Quantity)
			s.lots = s.lots[1:]
			continue
		}

		// Partial consumption: split this lot proportionally.
		fraction := remaining.Div(lot.Quantity)
		consumedCost := lot.CostBasisUSD.Mul(fraction)
		costBasisUSD = costBasisUSD.Add(consumedCost)
		lot.CostBasisUSD = lot.CostBasisUSD.Sub(consumedCost)
		lot.Quantity = lot.Quantity.Sub(remaining)
		remaining = decimal.Zero
	}

	return costBasisUSD
}

func (s *state) summary(currentPrice decimal.Decimal, havePrice bool) AssetSummary {
	summary := AssetSummary{
		Asset:            s.asset,
		RemainingLots:    s.lots,
		Disposals:        s.disposals,
		IncomeEvents:     s.incomeEvents,
		TotalSpentUSD:    s.totalSpentUSD,
		TotalReceivedUSD: s.totalReceivedUSD,
		RealizedGainUSD:  s.realizedGainUSD,
		TotalIncomeUSD:   s.totalIncomeUSD,
	}
	if !havePrice {
		return summary
	}

	var holdingsValueUSD, holdingsCostUSD decimal.Decimal
	for _, lot := range s.lots {
		holdingsValueUSD = holdingsValueUSD.Add(lot.Quantity.Mul(currentPrice))
		holdingsCostUSD = holdingsCostUSD.Add(lot.CostBasisUSD)
	}
	gain := holdingsValueUSD.Sub(holdingsCostUSD)
	summary.UnrealizedGainUSD = &gain
	return summary
}
