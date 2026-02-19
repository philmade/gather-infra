import { useWorkspace } from '../../context/WorkspaceContext'
import type { DeployConfig } from './DeployAgentModal'

interface Props {
  config: DeployConfig
  deploymentId: string | null
}

export default function StepDone({ config, deploymentId }: Props) {
  const { dispatch } = useWorkspace()

  return (
    <div className="deploy-success">
      <div className="success-icon" style={{ color: 'var(--green)' }}>{'\u2713'}</div>
      <div className="success-text">{config.name} is queued!</div>
      <div className="success-subtext">
        Your claw deployment has been created. Container provisioning will begin shortly.
      </div>
      <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: 'var(--space-xs)' }}>
        30-minute trial started. Upgrade from the detail panel to keep it running.
      </div>
      {deploymentId && (
        <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: 'var(--space-xs)' }}>
          Deployment ID: {deploymentId}
        </div>
      )}
      <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'CLOSE_DEPLOY' })}>
        Done
      </button>
    </div>
  )
}
