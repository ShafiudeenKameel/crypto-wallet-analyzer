<script setup lang="ts">
import { reactive } from 'vue'
import type { AssetSummary } from '../types'
import { formatUsd, formatQuantity, formatDate, shortHash } from '../format'

defineProps<{ assets: AssetSummary[] }>()

// A plain reactive Set works fine here - this component only ever runs in
// one browser tab for one viewer, no shared/concurrent mutation to guard.
const expanded = reactive(new Set<number>())
function toggle(i: number) {
  if (expanded.has(i)) expanded.delete(i)
  else expanded.add(i)
}

function gainClass(value: string): string {
  const n = Number(value)
  if (n > 0) return 'positive'
  if (n < 0) return 'negative'
  return ''
}
</script>

<template>
  <section aria-label="Per-asset breakdown">
    <h2>Assets</h2>
    <p v-if="assets.length === 0" class="empty">
      No confidently-attributable asset activity found for this wallet.
    </p>

    <!-- Every total above is just a sum of the per-asset figures shown
         here, and every figure here expands to the transactions that
         produced it - the drill-down project.MD asks for. -->
    <div v-for="(asset, i) in assets" :key="i" class="asset-row">
      <button class="asset-header" type="button" :aria-expanded="expanded.has(i)" @click="toggle(i)">
        <span class="symbol">{{ asset.asset.symbol || '(unknown asset)' }}</span>
        <span class="chain">chain {{ asset.asset.chainId }}</span>
        <span class="figure">Spent {{ formatUsd(asset.totalSpentUSD) }}</span>
        <span class="figure">Received {{ formatUsd(asset.totalReceivedUSD) }}</span>
        <span class="figure" :class="gainClass(asset.realizedGainUSD)">
          Realized {{ formatUsd(asset.realizedGainUSD) }}
        </span>
        <span class="figure" :class="asset.unrealizedGainUSD ? gainClass(asset.unrealizedGainUSD) : ''">
          Unrealized {{ formatUsd(asset.unrealizedGainUSD) }}
        </span>
        <span class="chevron">{{ expanded.has(i) ? '▾' : '▸' }}</span>
      </button>

      <div v-if="expanded.has(i)" class="asset-detail">
        <div v-if="asset.incomeEvents.length" class="detail-block">
          <h3>Income events</h3>
          <table>
            <thead>
              <tr><th>Tx</th><th>Type</th><th>Quantity</th><th>Value</th></tr>
            </thead>
            <tbody>
              <tr v-for="e in asset.incomeEvents" :key="e.sourceTxHash">
                <td><a :href="e.sourceExplorerUrl" target="_blank" rel="noopener noreferrer">{{ shortHash(e.sourceTxHash) }}</a></td>
                <td>{{ e.type }}</td>
                <td>{{ formatQuantity(e.quantity) }}</td>
                <td>{{ formatUsd(e.valueUSD) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="asset.disposals.length" class="detail-block">
          <h3>Disposals (realized gain source)</h3>
          <table>
            <thead>
              <tr><th>Tx</th><th>Quantity</th><th>Proceeds</th><th>Cost Basis</th><th>Gain</th></tr>
            </thead>
            <tbody>
              <tr v-for="d in asset.disposals" :key="d.sourceTxHash">
                <td><a :href="d.sourceExplorerUrl" target="_blank" rel="noopener noreferrer">{{ shortHash(d.sourceTxHash) }}</a></td>
                <td>{{ formatQuantity(d.quantity) }}</td>
                <td>{{ formatUsd(d.proceedsUSD) }}</td>
                <td>{{ formatUsd(d.costBasisUSD) }}</td>
                <td :class="gainClass(d.realizedGainUSD)">{{ formatUsd(d.realizedGainUSD) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="asset.remainingLots.length" class="detail-block">
          <h3>Current holdings (unconsumed lots)</h3>
          <table>
            <thead>
              <tr><th>Tx</th><th>Quantity</th><th>Cost Basis</th><th>Acquired</th></tr>
            </thead>
            <tbody>
              <tr v-for="l in asset.remainingLots" :key="l.sourceTxHash">
                <td><a :href="l.sourceExplorerUrl" target="_blank" rel="noopener noreferrer">{{ shortHash(l.sourceTxHash) }}</a></td>
                <td>{{ formatQuantity(l.quantity) }}</td>
                <td>{{ formatUsd(l.costBasisUSD) }}</td>
                <td>{{ formatDate(l.acquiredAt) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.empty {
  color: var(--text-muted);
}
.asset-row {
  border: 1px solid var(--border);
  border-radius: 8px;
  margin-bottom: 0.6rem;
  overflow: hidden;
}
.asset-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  width: 100%;
  padding: 0.75rem 1rem;
  background: var(--surface);
  border: none;
  cursor: pointer;
  font-size: 0.9rem;
  text-align: left;
}
.symbol {
  font-weight: 700;
  min-width: 5rem;
}
.chain {
  color: var(--text-muted);
  font-size: 0.78rem;
}
.figure {
  margin-left: auto;
}
.figure.positive {
  color: var(--positive);
}
.figure.negative {
  color: var(--negative);
}
.chevron {
  margin-left: 0.75rem;
}
.asset-detail {
  padding: 0.75rem 1rem 1rem;
  border-top: 1px solid var(--border);
}
.detail-block {
  margin-bottom: 1rem;
}
.detail-block h3 {
  font-size: 0.85rem;
  color: var(--text-muted);
  margin: 0 0 0.4rem;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}
th,
td {
  text-align: left;
  padding: 0.35rem 0.5rem;
  border-bottom: 1px solid var(--border);
}
td.positive {
  color: var(--positive);
}
td.negative {
  color: var(--negative);
}
</style>
