import { useWorkspace } from '../../context/WorkspaceContext'
import AgentDirectory from './AgentDirectory'
import MessageFeed from './MessageFeed'
import ProofChain from './ProofChain'
import SkillsRegistry from './SkillsRegistry'

const tabs = ['feed', 'skills', 'agents', 'proofs'] as const

export default function NetworkView() {
  const { state, dispatch } = useWorkspace()

  return (
    <div className="network-view">
      <div className="network-header">
        <span className="network-title">gather.is <span className="net-live-dot" /></span>
        <span className="net-event-count">147 events (24h)</span>
        <div className="network-tabs">
          {tabs.map(tab => (
            <button
              key={tab}
              className={`network-tab ${state.networkTab === tab ? 'active' : ''}`}
              onClick={() => dispatch({ type: 'SET_NETWORK_TAB', tab })}
            >
              {tab.charAt(0).toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </div>
      </div>
      <div className="network-content">
        {state.networkTab === 'agents' && <AgentDirectory />}
        {state.networkTab === 'feed' && <MessageFeed />}
        {state.networkTab === 'proofs' && <ProofChain />}
        {state.networkTab === 'skills' && <SkillsRegistry />}
      </div>
    </div>
  )
}
