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
    cache: 'no-store',
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
  is_public: boolean
  heartbeat_interval: number
  heartbeat_instruction?: string
  paid?: boolean
  trial_ends_at?: string
  stripe_session_id?: string
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

export function updateClawSettings(id: string, settings: {
  is_public?: boolean
  heartbeat_interval?: number
  heartbeat_instruction?: string
}) {
  return apiFetch<ClawDeployment>(`/api/claws/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(settings),
  })
}

// Claw messaging
export interface ADKEvent {
  type: 'text' | 'tool_call' | 'tool_result'
  author?: string
  text?: string
  tool_name?: string
  tool_id?: string
  tool_args?: unknown
  result?: unknown
}

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
  return apiPost<{ message: ClawMessage; user_message_id: string; events?: ADKEvent[] }>(`/api/claws/${encodeURIComponent(clawId)}/messages`, { body })
}

// SSE streaming event from the bridge
export interface StreamEvent {
  type: 'text' | 'tool_call' | 'tool_result' | 'end' | 'done' | 'error'
  author?: string
  text?: string
  tool_name?: string
  tool_id?: string
  tool_args?: unknown
  result?: unknown
  message_id?: string
  user_message_id?: string
}

// Stream claw message via SSE — yields events as they arrive from the agent.
export async function* streamClawMessage(clawId: string, body: string): AsyncGenerator<StreamEvent> {
  const resp = await fetch(`${baseUrl}/api/claws/${encodeURIComponent(clawId)}/messages/stream`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify({ body }),
  })

  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`Stream failed: HTTP ${resp.status}: ${text.substring(0, 200)}`)
  }

  const reader = resp.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })

    // Parse SSE events from buffer
    const lines = buffer.split('\n')
    buffer = lines.pop() || '' // keep incomplete last line

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      const data = line.slice(6)
      try {
        const evt: StreamEvent = JSON.parse(data)
        yield evt
      } catch { /* skip unparseable */ }
    }
  }

  // Process any remaining data in buffer
  if (buffer.startsWith('data: ')) {
    try {
      yield JSON.parse(buffer.slice(6))
    } catch { /* skip */ }
  }
}

// Claw environment / config
export function getClawEnv(id: string) {
  return apiFetch<{ vars: Record<string, string> }>(`/api/claws/${encodeURIComponent(id)}/env`)
}

export function saveClawEnv(id: string, vars: Record<string, string>, restart = false) {
  return apiFetch<{ ok: boolean }>(`/api/claws/${encodeURIComponent(id)}/env`, {
    method: 'PUT',
    body: JSON.stringify({ vars, restart }),
  })
}

export function restartClaw(id: string) {
  return apiFetch<{ ok: boolean }>(`/api/claws/${encodeURIComponent(id)}/restart`, { method: 'POST' })
}

export function getClawLogs(id: string, tail = 200) {
  return apiFetch<{ logs: string }>(`/api/claws/${encodeURIComponent(id)}/logs?tail=${tail}`)
}

// Stripe checkout
export function createClawCheckout(id: string): Promise<{ url: string }> {
  return apiFetch(`/api/claws/${encodeURIComponent(id)}/checkout`, { method: 'POST' })
}

export { apiFetch, apiPost }
