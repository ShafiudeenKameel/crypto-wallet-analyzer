<script setup lang="ts">
import { ref } from 'vue'
import WalletForm from './components/WalletForm.vue'
import TotalsSummary from './components/TotalsSummary.vue'
import AssetBreakdown from './components/AssetBreakdown.vue'
import IssuesList from './components/IssuesList.vue'
import { analyzeWallet, ApiError } from './api'
import type { AnalysisResult } from './types'

type State =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'success'; result: AnalysisResult }

const state = ref<State>({ status: 'idle' })

async function onSubmit(walletAddress: string, chainId: number) {
  state.value = { status: 'loading' }
  try {
    const result = await analyzeWallet(walletAddress, chainId)
    state.value = { status: 'success', result }
  } catch (err) {
    const message = err instanceof ApiError || err instanceof Error ? err.message : 'Something went wrong.'
    state.value = { status: 'error', message }
  }
}
</script>

<template>
  <main class="app">
    <header>
      <h1>Crypto Wallet Transaction Analyzer</h1>
      <p class="subtitle">
        Paste a public wallet address - read-only, never a private key or signature.
      </p>
    </header>

    <WalletForm :loading="state.status === 'loading'" @submit="onSubmit" />

    <p v-if="state.status === 'loading'" class="status" role="status" aria-live="polite">
      Fetching and pricing transactions - this can take a moment on a wallet with a long history…
    </p>

    <p v-else-if="state.status === 'error'" class="status error" role="alert">
      {{ state.message }}
    </p>

    <template v-else-if="state.status === 'success'">
      <TotalsSummary
        :totals="state.result.costBasis.totals"
        :transaction-count="state.result.transactionCount"
      />
      <AssetBreakdown :assets="state.result.costBasis.perAsset" />
      <IssuesList
        :skipped="state.result.costBasis.skipped"
        :failed="state.result.failedPriceLookups"
      />
    </template>
  </main>
</template>

<style scoped>
.app {
  max-width: 60rem;
  margin: 0 auto;
  padding: 2rem 1.5rem 4rem;
}
header {
  margin-bottom: 1.5rem;
}
.subtitle {
  color: var(--text-muted);
  margin: 0;
}
.status {
  margin-top: 1rem;
  color: var(--text-muted);
}
.status.error {
  color: var(--negative);
}
</style>
