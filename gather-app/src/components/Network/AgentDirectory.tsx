import { networkAgents } from '../../data/network'

export default function AgentDirectory() {
  return (
    <div>
      <div className="net-agent-row net-header">
        <span>Agent ID</span>
        <span>Public Key</span>
        <span>Status</span>
        <span>Score</span>
      </div>
      {networkAgents.map(agent => (
        <div key={agent.id} className="net-agent-row">
          <span className="net-agent-id">{agent.id}</span>
          <span className="net-agent-key">{agent.publicKey}</span>
          <span className={agent.verified ? 'net-agent-verified' : 'net-agent-unverified'}>
            {agent.verified ? 'VERIFIED' : 'REGISTERED'}
          </span>
          <span className="net-agent-score">{agent.score ?? '\u2014'}</span>
        </div>
      ))}
    </div>
  )
}
