<script setup lang="ts">
import { ref } from 'vue'
import { looksLikeAddress } from '../format'
import { SUPPORTED_CHAINS } from '../types'

const props = defineProps<{ loading: boolean }>()
const emit = defineEmits<{ submit: [walletAddress: string, chainId: number] }>()

const walletAddress = ref('')
const chainId = ref<number>(SUPPORTED_CHAINS[0].id)
const touched = ref(false)

function isValid(): boolean {
  return looksLikeAddress(walletAddress.value)
}

function onSubmit() {
  touched.value = true
  if (!isValid() || props.loading) return
  emit('submit', walletAddress.value.trim(), chainId.value)
}
</script>

<template>
  <!-- Read-only public address only - never a private key or signature. -->
  <form class="wallet-form" @submit.prevent="onSubmit">
    <div class="field">
      <label for="wallet-address">Wallet address</label>
      <input
        id="wallet-address"
        v-model="walletAddress"
        type="text"
        placeholder="0x..."
        autocomplete="off"
        spellcheck="false"
        :aria-invalid="touched && !isValid()"
        @blur="touched = true"
      />
      <p v-if="touched && !isValid()" class="field-error" role="alert">
        Enter a valid EVM address (0x followed by 40 hex characters).
      </p>
    </div>

    <div class="field">
      <label for="chain">Chain</label>
      <select id="chain" v-model.number="chainId">
        <option v-for="chain in SUPPORTED_CHAINS" :key="chain.id" :value="chain.id">
          {{ chain.label }}
        </option>
      </select>
    </div>

    <button type="submit" :disabled="loading">
      {{ loading ? 'Analyzing…' : 'Analyze wallet' }}
    </button>
  </form>
</template>

<style scoped>
.wallet-form {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
  flex-wrap: wrap;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
label {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--text-muted);
}
input,
select {
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  font-size: 0.95rem;
  min-width: 22rem;
}
input[aria-invalid='true'] {
  border-color: var(--negative);
}
.field-error {
  color: var(--negative);
  font-size: 0.8rem;
  margin: 0;
}
button {
  padding: 0.6rem 1.25rem;
  border: none;
  border-radius: 6px;
  background: var(--accent);
  color: white;
  font-weight: 600;
  cursor: pointer;
  align-self: flex-end;
}
button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
