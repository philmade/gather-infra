import { useEffect, useState } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { pb } from '../../lib/pocketbase'

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

  useEffect(() => {
    if (!state.selectedAgent) return
    async function fetchClaw() {
      try {
        const resp = await fetch(pb.baseURL + `/api/claws/${state.selectedAgent}`, {
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
        })
        if (resp.ok) {
          setClaw(await resp.json())
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
