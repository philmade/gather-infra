import { useWorkspace } from '../../context/WorkspaceContext'
import { agents } from '../../data/agents'

export default function AgentDetail() {
  const { state, dispatch } = useWorkspace()
  const agent = agents.find(a => a.id === state.selectedAgent)
  if (!agent) return null

  const statusLabel = agent.status.charAt(0).toUpperCase() + agent.status.slice(1)

  return (
    <div className="agent-detail active">
      <div className="agent-detail-header">
        <div className="agent-detail-avatar">{'\uD83E\uDD16'}</div>
        <div className="agent-detail-name">{agent.name}</div>
        <div className="agent-detail-type">Claw</div>
        <div className={`agent-status status-${agent.status}`} style={{ justifyContent: 'center' }}>
          <span className="status-dot" /> {statusLabel}
        </div>
      </div>

      <div className="agent-info-grid">
        <div className="agent-info-item">
          <div className="info-label">Uptime</div>
          <div className="info-value">{agent.uptime}</div>
        </div>
        <div className="agent-info-item">
          <div className="info-label">Last Active</div>
          <div className="info-value">{agent.lastActive}</div>
        </div>
        <div className="agent-info-item">
          <div className="info-label">Connected Repo</div>
          <div className="info-value">{agent.connectedRepo}</div>
        </div>
        <div className="agent-info-item">
          <div className="info-label">Tasks Done</div>
          <div className="info-value">{agent.tasksDone}</div>
        </div>
      </div>

      <div className="webtop-preview">
        <div className="webtop-preview-label">Live Preview</div>
        <div
          className="webtop-thumbnail"
          onClick={() => dispatch({ type: 'OPEN_WEBTOP', agentName: agent.name })}
        >
          <div className="preview-label">{'\uD83D\uDDA5'} Agent Browser</div>
          <div className="expand-label">Click to expand</div>
        </div>
      </div>

      <div className="agent-actions">
        <button className="btn btn-secondary btn-sm">
          {agent.status === 'running' ? '\u25A0 Stop' : '\u25B6 Start'}
        </button>
        <button className="btn btn-secondary btn-sm">{'\u21BB'} Restart</button>
        <button className="btn btn-secondary btn-sm">{'\uD83D\uDCC4'} Logs</button>
        <button className="btn btn-secondary btn-sm">{'\u2699'} Settings</button>
      </div>
    </div>
  )
}
