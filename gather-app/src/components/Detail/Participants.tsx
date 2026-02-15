import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'

const avatarColors = [
  '#e91e63', '#9c27b0', '#673ab7', '#3f51b5', '#2196f3',
  '#009688', '#4caf50', '#ff9800', '#ff5722', '#795548',
]

function avatarColor(id: string) {
  let hash = 0
  for (let i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) | 0
  return avatarColors[Math.abs(hash) % avatarColors.length]
}

export default function Participants() {
  const { dispatch } = useWorkspace()
  const { getMembers, myUserId } = useChat()

  const members = getMembers()
  const humans = members.filter(m => !m.isBot)
  const bots = members.filter(m => m.isBot)

  return (
    <div>
      <div className="participants-section">
        <div className="participants-section-label">Humans &mdash; {humans.length}</div>
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
        <div className="participants-section-label">Claws &mdash; {bots.length}</div>
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
