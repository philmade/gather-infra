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
      <p style={{ color: 'var(--text-secondary)', fontSize: '0.85rem', lineHeight: 1.6, margin: '0 0 var(--space-lg)' }}>
        We'll deploy a personal AI agent for you to try. It runs in its own container,
        gets its own subdomain, and you can chat with it right here. No credit card needed —
        it's free for 30 minutes.
      </p>
      <div className="form-group">
        <label className="form-label">Give it a name</label>
        <input
          type="text"
          className="form-input"
          placeholder="e.g., MyClaw, ResearchBot, TaskRunner"
          value={config.name}
          onChange={e => setConfig({ ...config, name: e.target.value })}
          autoFocus
        />
      </div>
      <div className="form-group">
        <label className="form-label">What should it do? (optional)</label>
        <textarea
          className="form-input"
          rows={3}
          placeholder="e.g., Help me research topics, draft emails, monitor my GitHub repos..."
          value={config.instructions}
          onChange={e => setConfig({ ...config, instructions: e.target.value })}
        />
      </div>
      <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: 'var(--space-md)' }}>
        After 30 minutes the trial claw will be removed. You can upgrade to a paid plan
        at any time to keep it running permanently.
      </div>
      <div className="modal-footer">
        <button
          className="btn btn-primary btn-sm"
          onClick={() => dispatch({ type: 'DEPLOY_NEXT' })}
          disabled={!config.name.trim()}
        >
          Deploy Free Trial
        </button>
      </div>
    </div>
  )
}
