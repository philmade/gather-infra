import { useWorkspace } from '../../context/WorkspaceContext'
import type { DeployConfig } from './DeployAgentModal'

interface Props {
  config: DeployConfig
  setConfig: (c: DeployConfig) => void
}

export default function StepConfigure({ config, setConfig }: Props) {
  const { dispatch } = useWorkspace()

  return (
    <div>
      <h3>Configure Your Claw</h3>
      <p>Give your agent a name and connect it to your code.</p>
      <div className="form-group">
        <label className="form-label">Agent Name</label>
        <input
          type="text"
          className="form-input"
          placeholder="e.g., ResearchClaw, DeployClaw, SupportClaw"
          value={config.name}
          onChange={e => setConfig({ ...config, name: e.target.value })}
        />
      </div>
      <div className="form-group">
        <label className="form-label">GitHub Repository (optional)</label>
        <input
          type="text"
          className="form-input"
          placeholder="e.g., acme/data-pipeline"
          value={config.githubRepo}
          onChange={e => setConfig({ ...config, githubRepo: e.target.value })}
        />
      </div>
      <div className="form-group">
        <label className="form-label">Initial Instructions</label>
        <textarea
          className="form-input"
          rows={3}
          placeholder="What should this agent do? e.g., Monitor the data pipeline and alert on failures..."
          value={config.instructions}
          onChange={e => setConfig({ ...config, instructions: e.target.value })}
        />
      </div>
      <div className="modal-footer">
        <button className="btn btn-secondary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_PREV' })}>Back</button>
        <button
          className="btn btn-primary btn-sm"
          onClick={() => dispatch({ type: 'DEPLOY_NEXT' })}
          disabled={!config.name.trim()}
        >
          Continue
        </button>
      </div>
    </div>
  )
}
