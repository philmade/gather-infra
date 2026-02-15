import { pb } from './pocketbase'

function getBaseUrl(): string {
  const hostname = window.location.hostname
  if (hostname === 'localhost' || hostname === '127.0.0.1') {
    return 'http://localhost:8090'
  }
  return window.location.origin
}

const baseUrl = getBaseUrl()

function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (pb.authStore.token) {
    headers['Authorization'] = `Bearer ${pb.authStore.token}`
  }
  return headers
}

async function apiFetch<T = unknown>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    headers: authHeaders(),
    ...options,
  })

  if (!response.ok) {
    const text = await response.text()
    let message: string
    try {
      const parsed = JSON.parse(text)
      message = parsed.title || parsed.message || parsed.detail || `HTTP ${response.status}`
    } catch {
      message = `HTTP ${response.status}: ${text.substring(0, 200)}`
    }
    throw new Error(message)
  }

  return response.json()
}

async function apiPost<T = unknown>(path: string, body: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

// Skills
export function getSkills(params: Record<string, string> = {}) {
  const qs = new URLSearchParams(params).toString()
  return apiFetch(`/api/skills${qs ? '?' + qs : ''}`)
}

export function getSkill(id: string) {
  return apiFetch(`/api/skills/${encodeURIComponent(id)}`)
}

// Rankings
export function getRankings(params: Record<string, string> = {}) {
  const qs = new URLSearchParams(params).toString()
  return apiFetch(`/api/rankings${qs ? '?' + qs : ''}`)
}

// Proofs
export function getProofs(params: Record<string, string> = {}) {
  const qs = new URLSearchParams(params).toString()
  return apiFetch(`/api/proofs${qs ? '?' + qs : ''}`)
}

export function getProof(id: string) {
  return apiFetch(`/api/proofs/${encodeURIComponent(id)}`)
}

// Reviews
export function getReviews(params: Record<string, string> = {}) {
  const qs = new URLSearchParams(params).toString()
  return apiFetch(`/api/reviews${qs ? '?' + qs : ''}`)
}

// Menu / Shop
export function getMenu() {
  return apiFetch('/api/menu')
}

export { apiFetch, apiPost }
