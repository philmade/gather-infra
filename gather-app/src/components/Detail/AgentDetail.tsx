import { useEffect, useState, useCallback } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { pb } from '../../lib/pocketbase'
import { updateClawSettings, createClawCheckout } from '../../lib/api'

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

  useEffect(() => {
    if (!state.selectedAgent) return
    async function fetchClaw() {
      try {
        const resp = await fetch(pb.baseURL + `/api/claws/${state.selectedAgent}`, {
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
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
