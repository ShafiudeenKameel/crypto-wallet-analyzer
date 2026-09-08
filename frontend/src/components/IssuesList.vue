<script setup lang="ts">
import { computed } from 'vue'
import type { Transaction, FailedLookup } from '../types'
import { formatDate, shortHash } from '../format'

const props = defineProps<{ skipped: Transaction[]; failed: FailedLookup[] }>()

function skipReason(tx: Transaction): string {
  if (tx.type === 'unknown') return 'unrecognized asset/contract'
  if (tx.direction === 'n/a') return 'could not determine direction relative to this wallet'
  if (tx.priceUSD === null) return 'no price available for this date'
  return 'unclassified'
}

const hasIssues = computed(() => props.skipped.length > 0 || props.failed.length > 0)
</script>

<template>
  <section v-if="hasIssues" aria-label="Items needing review" class="issues">
    <h2>Needs review</h2>
    <!-- Honesty over false precision: these are shown, not silently
         excluded from the totals above. -->
    <p class="hint">
      These transactions couldn't be confidently valued or classified, so they're excluded
      from the totals above rather than guessed at.
    </p>

    <div v-if="failed.length" class="detail-block">
      <h3>Price lookup failed ({{ failed.length }})</h3>
      <ul>
        <li v-for="f in failed" :key="f.transaction.txHash">
          <a :href="f.transaction.explorerUrl" target="_blank" rel="noopener noreferrer">{{ shortHash(f.transaction.txHash) }}</a>
          — {{ formatDate(f.transaction.blockTimestamp) }} — {{ f.error }}
        </li>
      </ul>
    </div>

    <div v-if="skipped.length" class="detail-block">
      <h3>Skipped ({{ skipped.length }})</h3>
      <ul>
        <li v-for="tx in skipped" :key="tx.txHash">
          <a :href="tx.explorerUrl" target="_blank" rel="noopener noreferrer">{{ shortHash(tx.txHash) }}</a>
          — {{ formatDate(tx.blockTimestamp) }} — {{ skipReason(tx) }}
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.issues {
  margin-top: 1.5rem;
  padding: 1rem;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
}
.hint {
  color: var(--text-muted);
  font-size: 0.85rem;
}
.detail-block ul {
  list-style: none;
  padding: 0;
  margin: 0;
  font-size: 0.85rem;
}
.detail-block li {
  padding: 0.3rem 0;
  border-bottom: 1px solid var(--border);
}
</style>
