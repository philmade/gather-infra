import { useEffect, useState, useRef } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'
import { pb } from '../../lib/pocketbase'

const avatarColors = [
  '#e91e63', '#9c27b0', '#673ab7', '#3f51b5', '#2196f3',
  '#009688', '#4caf50', '#ff9800', '#ff5722', '#795548',
]

function avatarColor(id: string) {
  let hash = 0
  for (let i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) | 0
  return avatarColors[Math.abs(hash) % avatarColors.length]
}

interface ClawDeployment {
  id: string
  name: string
  status: string
  claw_type: string
}

const statusLabel: Record<string, string> = {
  queued: 'Queued',
  provisioning: 'Provisioning...',
  running: 'Running',
  expired: 'Trial Expired',
  stopped: 'Stopped',
  failed: 'Failed',
}

const statusClass: Record<string, string> = {
  queued: 'status-idle',
  provisioning: 'status-idle',
  running: 'status-running',
  expired: 'status-stopped',
  stopped: 'status-stopped',
  failed: 'status-stopped',
}

export default function Participants() {
  const { dispatch } = useWorkspace()
  const { state, getMembers, myUserId } = useChat()
  const [deployedClaws, setDeployedClaws] = useState<ClawDeployment[]>([])
  const [showInvite, setShowInvite] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteStatus, setInviteStatus] = useState<{ type: 'success' | 'error'; message: string } | null>(null)
  const [inviting, setInviting] = useState(false)
  const emailRef = useRef<HTMLInputElement>(null)

  async function handleInvite(e: React.FormEvent) {
    e.preventDefault()
    if (!inviteEmail.trim() || !state.activeWorkspace) return

    setInviting(true)
    setInviteStatus(null)
    try {
      const activeWs = state.workspaces.find(w => w.topic === state.activeWorkspace)
      const resp = await fetch(pb.baseURL + '/api/workspace/invite', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${pb.authStore.token}`,
        },
        body: JSON.stringify({
          email: inviteEmail.trim(),
          workspace_topic: state.activeWorkspace,
          workspace_name: activeWs?.name || '',
        }),
      })
      const data = await resp.json()
      if (resp.ok) {
        setInviteStatus({ type: 'success', message: data.message || `Invite sent to ${inviteEmail}` })
        setInviteEmail('')
      } else {
        setInviteStatus({ type: 'error', message: data.message || 'Invite failed' })
      }
    } catch {
      setInviteStatus({ type: 'error', message: 'Network error' })
    } finally {
      setInviting(false)
    }
  }

  // Fetch user's claw deployments (poll every 15s to catch status changes)
  useEffect(() => {
    async function fetchClaws() {
      try {
        const resp = await fetch(pb.baseURL + '/api/claws', {
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
          cache: 'no-store',
        })
        if (resp.ok) {
          const data = await resp.json()
          setDeployedClaws(data.claws || [])
        }
      } catch {
        // Silently fail — claws list is not critical
      }
    }
    fetchClaws()
    const interval = setInterval(fetchClaws, 15000)
    return () => clearInterval(interval)
  }, [])

  const members = getMembers()
  const humans = members.filter(m => !m.isBot)
  const bots = members.filter(m => m.isBot)
  const totalClaws = bots.length + deployedClaws.length

  return (
    <div>
      <div className="participants-section">
        <div className="participants-section-label">Humans &mdash; {humans.length}</div>

        {/* Invite People */}
        <div className="invite-section">
          {!showInvite ? (
            <button
              className="invite-people-btn"
              onClick={() => { setShowInvite(true); setInviteStatus(null); setTimeout(() => emailRef.current?.focus(), 0) }}
            >
              + Invite People
            </button>
          ) : (
            <form className="invite-form" onSubmit={handleInvite}>
              <input
                ref={emailRef}
                className="invite-input"
                type="email"
                placeholder="Email address"
                value={inviteEmail}
                onChange={e => setInviteEmail(e.target.value)}
                disabled={inviting}
              />
              <div className="invite-actions">
                <button className="invite-btn" type="submit" disabled={inviting || !inviteEmail.trim()}>
                  {inviting ? 'Inviting...' : 'Send Invite'}
                </button>
                <button
                  className="invite-cancel"
                  type="button"
                  onClick={() => { setShowInvite(false); setInviteEmail(''); setInviteStatus(null) }}
                >
                  Cancel
                </button>
              </div>
              {inviteStatus?.type === 'success' && (
                <div className="invite-status invite-success">{inviteStatus.message}</div>
              )}
              {inviteStatus?.type === 'error' && (
                <div className="invite-status invite-error">{inviteStatus.message}</div>
              )}
            </form>
          )}
        </div>

        {humans.map(m => (
          <div key={m.id} className="participant-item">
            <div
              className="participant-avatar"
              style={{ background: avatarColor(m.id) }}
            >
              {m.name.charAt(0).toUpperCase()}
              <span className={`presence-dot presence-${m.online ? 'online' : 'offline'}`} />
            </div>
            <span className="participant-name">{m.name}</span>
            {m.id === myUserId && <span className="participant-you">(you)</span>}
          </div>
        ))}
        {humans.length === 0 && (
          <div style={{ padding: '4px 12px', fontSize: '0.8rem', color: 'var(--text-muted)' }}>
            No members yet
          </div>
        )}
      </div>

      <div className="participants-section">
        <div className="participants-section-label">Claws &mdash; {totalClaws}</div>
        {bots.map(m => (
          <div key={m.id} className="agent-item">
            <div className="agent-avatar">{'\uD83E\uDD16'}</div>
            <div className="agent-info">
              <div className="agent-name">{m.name}</div>
              <div className={`agent-status status-${m.online ? 'running' : 'idle'}`}>
                <span className="status-dot" /> {m.online ? 'Running' : 'Idle'}
              </div>
            </div>
          </div>
        ))}
        {deployedClaws.map(claw => (
          <div
            key={claw.id}
            className="agent-item"
            onClick={() => dispatch({ type: 'SHOW_AGENT_DETAIL', agentId: claw.id })}
          >
            <div className="agent-avatar">{'\uD83E\uDD16'}</div>
            <div className="agent-info">
              <div className="agent-name">{claw.name}</div>
              <div className={`agent-status ${statusClass[claw.status] || 'status-idle'}`}>
                <span className="status-dot" /> {statusLabel[claw.status] || claw.status}
              </div>
            </div>
          </div>
        ))}
        {totalClaws === 0 && (
          <>
            {/* Example claw — shows what a deployed agent looks like */}
            <div className="agent-item" style={{ opacity: 0.45, pointerEvents: 'none' }}>
              <div className="agent-avatar">{'\uD83E\uDD16'}</div>
              <div className="agent-info">
                <div className="agent-name">
                  ResearchClaw
                  <span style={{
                    fontSize: '0.6rem',
                    background: 'var(--bg-tertiary)',
                    color: 'var(--text-muted)',
                    padding: '1px 5px',
                    borderRadius: '3px',
                    marginLeft: '6px',
                    fontWeight: 500,
                  }}>EXAMPLE</span>
                </div>
                <div className="agent-status status-stopped">
                  <span className="status-dot" /> Not deployed
                </div>
              </div>
            </div>
          </>
        )}
        <button
          className="deploy-agent-btn"
          onClick={() => dispatch({ type: 'OPEN_DEPLOY' })}
        >
          + Deploy Claw
        </button>
      </div>
    </div>
  )
}
