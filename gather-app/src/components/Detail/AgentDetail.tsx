import { useEffect, useState, useCallback, useRef } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { pb } from '../../lib/pocketbase'
import { updateClawSettings, createClawCheckout, getClawEnv, saveClawEnv, restartClaw, getClawLogs } from '../../lib/api'

interface ClawDetail {
  id: string
  name: string
  status: string
  claw_type: string
  subdomain?: string
  url?: string
  port?: number
  instructions?: string
  github_repo?: string
  error_message?: string
  is_public: boolean
  heartbeat_interval: number
  heartbeat_instruction?: string
  paid?: boolean
  trial_ends_at?: string
  created: string
}

const statusLabel: Record<string, string> = {
  queued: 'Queued',
  provisioning: 'Provisioning...',
  running: 'Running',
  stopped: 'Stopped',
  failed: 'Failed',
}

export default function AgentDetail() {
  const { state, dispatch } = useWorkspace()
  const [claw, setClaw] = useState<ClawDetail | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [saving, setSaving] = useState(false)
  const [upgrading, setUpgrading] = useState(false)
  const [trialRemaining, setTrialRemaining] = useState('')
  const [isPublic, setIsPublic] = useState(true)
  const [heartbeatInterval, setHeartbeatInterval] = useState(0)
  const [heartbeatInstruction, setHeartbeatInstruction] = useState('')

  // Configuration state
  const [envLoading, setEnvLoading] = useState(false)
  const [envSaving, setEnvSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [apiKey, setApiKey] = useState('')
  const [apiBase, setApiBase] = useState('')
  const [model, setModel] = useState('')
  const [telegramBot, setTelegramBot] = useState('')
  const [telegramChatId, setTelegramChatId] = useState('')

  // Logs state
  const [logsOpen, setLogsOpen] = useState(false)
  const [logs, setLogs] = useState('')
  const [logsLoading, setLogsLoading] = useState(false)
  const logsEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!state.selectedAgent) return
    async function fetchClaw() {
      try {
        const resp = await fetch(pb.baseURL + `/api/claws/${state.selectedAgent}`, {
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
          cache: 'no-store',
        })
        if (resp.ok) {
          const data = await resp.json()
          setClaw(data)
          setIsPublic(data.is_public ?? true)
          setHeartbeatInterval(data.heartbeat_interval ?? 0)
          setHeartbeatInstruction(data.heartbeat_instruction ?? '')
        }
      } catch {
        // ignore
      }
    }
    fetchClaw()
  }, [state.selectedAgent])

  // Trial countdown
  useEffect(() => {
    if (!claw || claw.paid || !claw.trial_ends_at) {
      setTrialRemaining('')
      return
    }
    function tick() {
      const end = new Date(claw!.trial_ends_at!).getTime()
      const diff = Math.max(0, end - Date.now())
      const mins = Math.floor(diff / 60000)
      const secs = Math.floor((diff % 60000) / 1000)
      setTrialRemaining(`${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`)
    }
    tick()
    const id = setInterval(tick, 1000)
    return () => clearInterval(id)
  }, [claw?.paid, claw?.trial_ends_at])

  const handleUpgrade = useCallback(async () => {
    if (!claw) return
    setUpgrading(true)
    try {
      const { url } = await createClawCheckout(claw.id)
      window.location.href = url
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to create checkout')
    } finally {
      setUpgrading(false)
    }
  }, [claw?.id])

  async function handleDelete() {
    if (!claw) return
    setDeleting(true)
    try {
      const resp = await fetch(pb.baseURL + `/api/claws/${claw.id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${pb.authStore.token}` },
      })
      if (resp.ok) {
        dispatch({ type: 'SHOW_PARTICIPANTS' })
      }
    } catch {
      // ignore
    } finally {
      setDeleting(false)
    }
  }

  async function handleSaveSettings() {
    if (!claw) return
    setSaving(true)
    try {
      const updated = await updateClawSettings(claw.id, {
        is_public: isPublic,
        heartbeat_interval: heartbeatInterval,
        heartbeat_instruction: heartbeatInstruction,
      })
      setClaw(updated as unknown as ClawDetail)
    } catch {
      // ignore
    } finally {
      setSaving(false)
    }
  }

  // Load env vars when claw is loaded and running
  useEffect(() => {
    if (!claw || claw.status !== 'running') return
    setEnvLoading(true)
    getClawEnv(claw.id)
      .then(({ vars }) => {
        setApiKey(vars.ANTHROPIC_API_KEY || '')
        setApiBase(vars.ANTHROPIC_API_BASE || '')
        setModel(vars.ANTHROPIC_MODEL || '')
        setTelegramBot(vars.TELEGRAM_BOT || '')
        setTelegramChatId(vars.TELEGRAM_CHAT_ID || '')
      })
      .catch(() => {
        // No .env yet — fine
      })
      .finally(() => setEnvLoading(false))
  }, [claw?.id, claw?.status])

  const handleSaveEnv = useCallback(async (restart: boolean) => {
    if (!claw) return
    setEnvSaving(true)
    try {
      const vars: Record<string, string> = {}
      if (apiKey) vars.ANTHROPIC_API_KEY = apiKey
      if (apiBase) vars.ANTHROPIC_API_BASE = apiBase
      if (model) vars.ANTHROPIC_MODEL = model
      if (telegramBot) vars.TELEGRAM_BOT = telegramBot
      if (telegramChatId) vars.TELEGRAM_CHAT_ID = telegramChatId
      await saveClawEnv(claw.id, vars, restart)
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to save configuration')
    } finally {
      setEnvSaving(false)
    }
  }, [claw?.id, apiKey, apiBase, model, telegramBot, telegramChatId])

  const handleRestart = useCallback(async () => {
    if (!claw) return
    setRestarting(true)
    try {
      await restartClaw(claw.id)
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to restart')
    } finally {
      setRestarting(false)
    }
  }, [claw?.id])

  const fetchLogs = useCallback(async () => {
    if (!claw) return
    setLogsLoading(true)
    try {
      const { logs: text } = await getClawLogs(claw.id, 200)
      setLogs(text)
      setTimeout(() => logsEndRef.current?.scrollIntoView(), 50)
    } catch (e) {
      setLogs(e instanceof Error ? e.message : 'Failed to load logs')
    } finally {
      setLogsLoading(false)
    }
  }, [claw?.id])

  useEffect(() => {
    if (logsOpen && claw?.status === 'running') fetchLogs()
  }, [logsOpen])

  const settingsChanged = claw && (
    isPublic !== (claw.is_public ?? true) ||
    heartbeatInterval !== (claw.heartbeat_interval ?? 0) ||
    heartbeatInstruction !== (claw.heartbeat_instruction ?? '')
  )

  if (!claw) {
    return (
      <div style={{ padding: 'var(--space-lg)', textAlign: 'center', color: 'var(--text-muted)', fontSize: '0.85rem' }}>
        Loading...
      </div>
    )
  }

  const statusText = statusLabel[claw.status] || claw.status
  const statusCls = claw.status === 'running' ? 'status-running'
    : claw.status === 'failed' || claw.status === 'stopped' ? 'status-stopped'
    : 'status-idle'

  return (
    <div className="agent-detail active">
      <div className="agent-detail-header">
        <div className="agent-detail-avatar">{'\uD83E\uDD16'}</div>
        <div className="agent-detail-name">{claw.name}</div>
        <div className="agent-detail-type">{claw.claw_type || 'picoclaw'}</div>
        <div className={`agent-status ${statusCls}`} style={{ justifyContent: 'center' }}>
          <span className="status-dot" /> {statusText}
        </div>
      </div>

      {!claw.paid && claw.trial_ends_at && claw.status === 'running' && (
        <div style={{ margin: '0 var(--space-md) var(--space-sm)', padding: 'var(--space-sm)', background: 'rgba(255, 200, 0, 0.12)', border: '1px solid rgba(255, 200, 0, 0.3)', borderRadius: 'var(--radius-xs)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: '0.8rem' }}>
          <span style={{ color: 'var(--text-primary)' }}>Trial {trialRemaining}</span>
          <button
            className="btn btn-primary btn-sm"
            onClick={handleUpgrade}
            disabled={upgrading}
            style={{ fontSize: '0.75rem', padding: '2px 10px' }}
          >
            {upgrading ? '...' : 'Upgrade'}
          </button>
        </div>
      )}

      {claw.paid && (
        <div style={{ margin: '0 var(--space-md) var(--space-sm)', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.75rem', color: 'var(--green)' }}>
          Active subscription
        </div>
      )}

      {claw.status === 'stopped' && claw.error_message?.includes('Trial expired') && (
        <div style={{ margin: '0 var(--space-md) var(--space-sm)', padding: 'var(--space-sm)', background: 'rgba(255, 0, 0, 0.08)', border: '1px solid rgba(255, 0, 0, 0.2)', borderRadius: 'var(--radius-xs)', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
          Trial expired. Redeploy a new claw after upgrading.
        </div>
      )}

      <div className="agent-info-grid">
        <div className="agent-info-item">
          <div className="info-label">Status</div>
          <div className="info-value">{statusText}</div>
        </div>
        <div className="agent-info-item">
          <div className="info-label">Subdomain</div>
          <div className="info-value">{claw.subdomain || '—'}</div>
        </div>
        {claw.port ? (
          <div className="agent-info-item">
            <div className="info-label">Port</div>
            <div className="info-value">{claw.port}</div>
          </div>
        ) : null}
        {claw.github_repo ? (
          <div className="agent-info-item">
            <div className="info-label">Repo</div>
            <div className="info-value">{claw.github_repo}</div>
          </div>
        ) : null}
      </div>

      {claw.instructions && (
        <div style={{ marginBottom: 'var(--space-md)' }}>
          <div style={{ fontSize: '0.7rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--text-muted)', marginBottom: 'var(--space-xs)' }}>
            Instructions
          </div>
          <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', background: 'var(--bg-tertiary)', padding: 'var(--space-sm)', borderRadius: 'var(--radius-xs)', whiteSpace: 'pre-wrap' }}>
            {claw.instructions}
          </div>
        </div>
      )}

      {claw.url && claw.status === 'running' && (
        <div className="webtop-preview">
          <div className="webtop-preview-label">Workspace</div>
          <div
            className="webtop-thumbnail"
            onClick={() => window.open(`${claw.url}?token=${pb.authStore.token}`, '_blank')}
            style={{ cursor: 'pointer' }}
          >
            <div className="preview-label">{'\uD83D\uDDA5'} Open Workspace</div>
            <div className="expand-label">{claw.url}</div>
          </div>
        </div>
      )}

      {claw.error_message && (
        <div style={{ marginBottom: 'var(--space-md)', padding: 'var(--space-sm)', background: 'rgba(255,0,0,0.1)', borderRadius: 'var(--radius-xs)', fontSize: '0.8rem', color: 'var(--red)' }}>
          {claw.error_message}
        </div>
      )}

      <div style={{ marginBottom: 'var(--space-md)' }}>
        <div style={{ fontSize: '0.7rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--text-muted)', marginBottom: 'var(--space-sm)' }}>
          Settings
        </div>

        <label style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-xs)', fontSize: '0.8rem', color: 'var(--text-secondary)', marginBottom: 'var(--space-sm)', cursor: 'pointer' }}>
          <input
            type="checkbox"
            checked={isPublic}
            onChange={(e) => setIsPublic(e.target.checked)}
          />
          Public page
          <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>(anyone with the link can view)</span>
        </label>

        <div style={{ marginBottom: 'var(--space-sm)' }}>
          <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '2px' }}>Heartbeat</div>
          <select
            value={heartbeatInterval}
            onChange={(e) => setHeartbeatInterval(Number(e.target.value))}
            style={{ width: '100%', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.8rem', background: 'var(--bg-tertiary)', color: 'var(--text-primary)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-xs)' }}
          >
            <option value={0}>Off</option>
            <option value={15}>Every 15 min</option>
            <option value={30}>Every 30 min</option>
            <option value={60}>Every hour</option>
            <option value={360}>Every 6 hours</option>
            <option value={1440}>Every 24 hours</option>
          </select>
        </div>

        {heartbeatInterval > 0 && (
          <div style={{ marginBottom: 'var(--space-sm)' }}>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '2px' }}>Heartbeat instruction</div>
            <textarea
              value={heartbeatInstruction}
              onChange={(e) => setHeartbeatInstruction(e.target.value)}
              placeholder="Check your notifications and update your public page with anything interesting."
              maxLength={2000}
              rows={3}
              style={{ width: '100%', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.8rem', background: 'var(--bg-tertiary)', color: 'var(--text-primary)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-xs)', resize: 'vertical', fontFamily: 'inherit' }}
            />
          </div>
        )}

        {settingsChanged && (
          <button
            className="btn btn-primary btn-sm"
            onClick={handleSaveSettings}
            disabled={saving}
            style={{ width: '100%' }}
          >
            {saving ? 'Saving...' : 'Save Settings'}
          </button>
        )}
      </div>

      {claw.status === 'running' && (
        <div style={{ marginBottom: 'var(--space-md)' }}>
          <div style={{ fontSize: '0.7rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--text-muted)', marginBottom: 'var(--space-sm)' }}>
            Configuration
          </div>

          {envLoading ? (
            <div style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>Loading...</div>
          ) : (
            <>
              <div style={{ fontSize: '0.7rem', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: 'var(--space-xs)' }}>LLM Provider</div>

              <div style={{ marginBottom: 'var(--space-xs)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '2px' }}>API Key</div>
                <input
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder="sk-..."
                  style={{ width: '100%', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.8rem', background: 'var(--bg-tertiary)', color: 'var(--text-primary)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-xs)', fontFamily: 'monospace' }}
                />
              </div>

              <div style={{ marginBottom: 'var(--space-xs)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '2px' }}>API Base URL</div>
                <input
                  type="text"
                  value={apiBase}
                  onChange={(e) => setApiBase(e.target.value)}
                  placeholder="https://api.z.ai/api/anthropic"
                  style={{ width: '100%', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.8rem', background: 'var(--bg-tertiary)', color: 'var(--text-primary)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-xs)' }}
                />
              </div>

              <div style={{ marginBottom: 'var(--space-sm)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '2px' }}>Model</div>
                <input
                  type="text"
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder="glm-5"
                  style={{ width: '100%', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.8rem', background: 'var(--bg-tertiary)', color: 'var(--text-primary)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-xs)' }}
                />
              </div>

              <div style={{ fontSize: '0.7rem', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: 'var(--space-xs)' }}>Messaging</div>

              <div style={{ marginBottom: 'var(--space-xs)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '2px' }}>Telegram Bot Token</div>
                <input
                  type="password"
                  value={telegramBot}
                  onChange={(e) => setTelegramBot(e.target.value)}
                  placeholder="123456:ABC-..."
                  style={{ width: '100%', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.8rem', background: 'var(--bg-tertiary)', color: 'var(--text-primary)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-xs)', fontFamily: 'monospace' }}
                />
              </div>

              <div style={{ marginBottom: 'var(--space-sm)' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '2px' }}>Telegram Chat ID</div>
                <input
                  type="text"
                  value={telegramChatId}
                  onChange={(e) => setTelegramChatId(e.target.value)}
                  placeholder="-1001234567890"
                  style={{ width: '100%', padding: 'var(--space-xs) var(--space-sm)', fontSize: '0.8rem', background: 'var(--bg-tertiary)', color: 'var(--text-primary)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-xs)' }}
                />
              </div>

              <div style={{ display: 'flex', gap: 'var(--space-xs)' }}>
                <button
                  className="btn btn-primary btn-sm"
                  onClick={() => handleSaveEnv(true)}
                  disabled={envSaving}
                  style={{ flex: 1 }}
                >
                  {envSaving ? 'Saving...' : 'Save & Restart'}
                </button>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={handleRestart}
                  disabled={restarting}
                >
                  {restarting ? '...' : 'Restart'}
                </button>
              </div>
            </>
          )}
        </div>
      )}

      {claw.status === 'running' && (
        <div style={{ marginBottom: 'var(--space-md)' }}>
          <div
            style={{ fontSize: '0.7rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--text-muted)', marginBottom: 'var(--space-sm)', cursor: 'pointer', userSelect: 'none' }}
            onClick={() => setLogsOpen(!logsOpen)}
          >
            {logsOpen ? '\u25BC' : '\u25B6'} Logs
          </div>

          {logsOpen && (
            <div>
              <div style={{ background: '#0d1117', color: '#c9d1d9', padding: 'var(--space-sm)', borderRadius: 'var(--radius-xs)', fontSize: '0.7rem', fontFamily: 'monospace', maxHeight: '300px', overflowY: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-all', lineHeight: '1.4' }}>
                {logsLoading ? 'Loading...' : (logs || 'No logs available')}
                <div ref={logsEndRef} />
              </div>
              <button
                className="btn btn-secondary btn-sm"
                onClick={fetchLogs}
                disabled={logsLoading}
                style={{ marginTop: 'var(--space-xs)', fontSize: '0.7rem' }}
              >
                {logsLoading ? '...' : 'Refresh'}
              </button>
            </div>
          )}
        </div>
      )}

      <div className="agent-actions">
        {claw.status === 'running' && claw.url && (
          <button
            className="btn btn-primary btn-sm"
            onClick={() => window.open(`${claw.url}?token=${pb.authStore.token}`, '_blank')}
          >
            {'\uD83D\uDDA5'} Open
          </button>
        )}
        <button
          className="btn btn-secondary btn-sm"
          style={{ color: 'var(--red)' }}
          onClick={handleDelete}
          disabled={deleting}
        >
          {deleting ? 'Deleting...' : '\uD83D\uDDD1 Delete'}
        </button>
      </div>
    </div>
  )
}
