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

// Claws
export interface ClawDeployment {
  id: string
  name: string
  status: string
  instructions?: string
  github_repo?: string
  claw_type: string
  user_id: string
  created: string
}

export function deployClaw(body: { name: string; instructions?: string; github_repo?: string }) {
  return apiPost<ClawDeployment>('/api/claws', body)
}

export function getClawStatus(id: string) {
  return apiFetch<ClawDeployment>(`/api/claws/${encodeURIComponent(id)}`)
}

export function listClaws() {
  return apiFetch<{ claws: ClawDeployment[]; total: number }>('/api/claws')
}

// Claw messaging
export interface ClawMessage {
  id: string
  author_id: string
  author_name: string
  body: string
  created: string
}

export function getClawMessages(clawId: string, since?: string) {
  const params = new URLSearchParams()
  if (since) params.set('since', since)
  const qs = params.toString()
  return apiFetch<{ messages: ClawMessage[] }>(`/api/claws/${encodeURIComponent(clawId)}/messages${qs ? '?' + qs : ''}`)
}

export function sendClawMessage(clawId: string, body: string) {
  return apiPost<{ message: ClawMessage; user_message_id: string }>(`/api/claws/${encodeURIComponent(clawId)}/messages`, { body })
}

export { apiFetch, apiPost }
