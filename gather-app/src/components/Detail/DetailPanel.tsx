import { useWorkspace } from '../../context/WorkspaceContext'
import Participants from './Participants'
import AgentDetail from './AgentDetail'

export default function DetailPanel() {
  const { state, dispatch } = useWorkspace()
  const isAgentView = state.detailView === 'agent-detail'

  const title = isAgentView ? 'Agent' : 'Participants'

  return (
    <aside className="detail-panel mobile-open">
      <div className="detail-header">
        {isAgentView && (
          <button
            className="back-btn"
            title="Back"
            onClick={() => dispatch({ type: 'SHOW_PARTICIPANTS' })}
          >
            {'\u2190'}
          </button>
        )}
        <span className="detail-title">{title}</span>
        <button
          className="close-btn"
          title="Close"
          onClick={() => dispatch({ type: 'CLOSE_DETAIL' })}
        >
          {'\u2715'}
        </button>
      </div>
      <div className="detail-body">
        {isAgentView ? <AgentDetail /> : <Participants />}
      </div>
    </aside>
  )
}
