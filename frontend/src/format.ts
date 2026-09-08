// Display-only formatting. The backend already did every computation in
// exact decimal (shopspring/decimal) - nothing here re-derives a number,
// it only renders an already-final value. Parsing via Number() for
// display can lose precision on astronomically large values, which is an
// acceptable, deliberate simplification for a demo wallet's dollar
// figures - it would NOT be acceptable if this app did further math on
// the parsed result, which it never does.

const usdFormatter = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

export function formatUsd(value: string | null): string {
  if (value === null) return 'N/A'
  return usdFormatter.format(Number(value))
}

export function formatQuantity(value: string): string {
  // Amounts arrive already decimal-shifted (the backend divides by each
  // token's own decimals at fetch time - see etherscan.go's parseAmount),
  // so this only trims display precision, it never re-derives the value.
  // Showing all 18 raw decimal places would be noise; 6 is a reasonable
  // middle ground for a value nothing further computes from.
  return Number(value).toLocaleString('en-US', { maximumFractionDigits: 6 })
}

export function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function shortHash(hash: string): string {
  if (hash.length <= 14) return hash
  return `${hash.slice(0, 8)}…${hash.slice(-6)}`
}

export function shortAddress(addr: string): string {
  if (addr.length <= 12) return addr
  return `${addr.slice(0, 6)}…${addr.slice(-4)}`
}

// Client-side check only: fast UX feedback for an obviously malformed
// address. This is NOT the security boundary - the backend's
// ethaddr.Validate (EIP-55 checksum) is the actual authority, and runs
// again server-side regardless of what this returns.
const ADDRESS_FORMAT = /^0x[0-9a-fA-F]{40}$/

export function looksLikeAddress(value: string): boolean {
  return ADDRESS_FORMAT.test(value.trim())
}
