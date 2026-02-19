import { useWorkspace } from '../../context/WorkspaceContext'
import type { DeployConfig } from './DeployAgentModal'

interface Props {
  config: DeployConfig
  deploymentId: string | null
}

export default function StepDone({ config }: Props) {
  const { dispatch } = useWorkspace()

  return (
    <div className="deploy-success">
      <div className="success-icon" style={{ color: 'var(--green)' }}>{'\u2713'}</div>
      <div className="success-text">{config.name} is on its way!</div>
      <div className="success-subtext">
        Your claw is being provisioned now. It'll appear in the sidebar
        in a moment — click on it to start chatting.
      </div>
      <div style={{ fontSize: '0.8rem', color: 'var(--text-muted)', margin: 'var(--space-md) 0', lineHeight: 1.5 }}>
        You have 30 minutes to try it out, completely free.
        After that it will be removed — unless you upgrade to a paid plan.
      </div>
      <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'CLOSE_DEPLOY' })}>
        Start Chatting
      </button>
    </div>
  )
}
