import { useWorkspace } from '../../context/WorkspaceContext'

export default function StepDone() {
  const { dispatch } = useWorkspace()

  return (
    <div className="deploy-success">
      <div className="success-icon" style={{ color: 'var(--green)' }}>{'\u2713'}</div>
      <div className="success-text">DataClaw is live!</div>
      <div className="success-subtext">Your agent is running and connected to the workspace.</div>
      <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'CLOSE_DEPLOY' })}>
        Go to Agent
      </button>
    </div>
  )
}
