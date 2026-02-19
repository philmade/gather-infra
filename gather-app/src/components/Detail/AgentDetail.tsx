import { useEffect, useState } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { pb } from '../../lib/pocketbase'
import { updateClawSettings } from '../../lib/api'

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
