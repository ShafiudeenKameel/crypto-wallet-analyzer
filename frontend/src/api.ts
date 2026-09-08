import type { AnalysisResult } from './types'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL

// A hung request should never leave the UI stuck forever - the same
// don't-wait-indefinitely instinct the backend applies with context
// deadlines, just enforced client-side here.
const REQUEST_TIMEOUT_MS = 30_000

export class ApiError extends Error {
  readonly status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface ErrorBody {
  error?: string
}

export async function analyzeWallet(walletAddress: string, chainId: number): Promise<AnalysisResult> {
  if (!API_BASE_URL) {
    throw new Error('VITE_API_BASE_URL is not configured - see .env.example')
  }

  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)

  try {
    const res = await fetch(`${API_BASE_URL}/api/analyze`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ walletAddress, chainId }),
      signal: controller.signal,
    })

    if (!res.ok) {
      const body: ErrorBody | null = await res.json().catch(() => null)
      throw new ApiError(body?.error ?? `Request failed with status ${res.status}.`, res.status)
    }

    return (await res.json()) as AnalysisResult
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new ApiError('The server took too long to respond. Please try again.', 0)
    }
    throw err
  } finally {
    clearTimeout(timeout)
  }
}
