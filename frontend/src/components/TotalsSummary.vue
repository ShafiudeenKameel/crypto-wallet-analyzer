<script setup lang="ts">
import type { Totals } from '../types'
import { formatUsd } from '../format'

defineProps<{ totals: Totals; transactionCount: number }>()

function gainClass(value: string | null): string {
  if (value === null) return ''
  const n = Number(value)
  if (n > 0) return 'positive'
  if (n < 0) return 'negative'
  return ''
}
</script>

<template>
  <section class="totals" aria-label="Summary totals">
    <!-- The sanity-check count from project.MD: lets a user cross-check
         this figure against what they see themselves on the explorer. -->
    <p class="tx-count">Found and processed {{ transactionCount }} transactions.</p>

    <div class="cards">
      <div class="card">
        <span class="label">Total Spent</span>
        <span class="value">{{ formatUsd(totals.totalSpentUSD) }}</span>
      </div>
      <div class="card">
        <span class="label">Total Received</span>
        <span class="value">{{ formatUsd(totals.totalReceivedUSD) }}</span>
      </div>
      <div class="card" :class="gainClass(totals.realizedGainUSD)">
        <span class="label">Realized Gain / Loss</span>
        <span class="value">{{ formatUsd(totals.realizedGainUSD) }}</span>
      </div>
      <div class="card" :class="gainClass(totals.unrealizedGainUSD)">
        <span class="label">Unrealized Gain / Loss</span>
        <span class="value">{{ formatUsd(totals.unrealizedGainUSD) }}</span>
      </div>
      <div class="card">
        <span class="label">Income (staking / airdrops)</span>
        <span class="value">{{ formatUsd(totals.totalIncomeUSD) }}</span>
      </div>
      <div class="card">
        <span class="label">Gas Fees Paid</span>
        <span class="value">{{ formatUsd(totals.gasFeesUSD) }}</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.tx-count {
  color: var(--text-muted);
  font-size: 0.9rem;
  margin: 0 0 0.75rem;
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
  gap: 0.75rem;
}
.card {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.9rem 1rem;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
}
.label {
  font-size: 0.78rem;
  color: var(--text-muted);
}
.value {
  font-size: 1.25rem;
  font-weight: 700;
}
.card.positive .value {
  color: var(--positive);
}
.card.negative .value {
  color: var(--negative);
}
</style>
