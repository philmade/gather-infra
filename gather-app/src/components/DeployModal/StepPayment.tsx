import { useWorkspace } from '../../context/WorkspaceContext'
import type { DeployConfig } from './DeployAgentModal'

interface Props {
  config: DeployConfig
}

export default function StepPayment({ config }: Props) {
  const { dispatch } = useWorkspace()

  return (
    <div>
      <h3>Review &amp; Deploy</h3>
      <p>Review your claw configuration before deploying.</p>
      <div className="card" style={{ marginBottom: 'var(--space-md)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-sm)' }}>
          <span style={{ color: 'var(--text-secondary)' }}>Agent</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>{config.name || 'Unnamed'}</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-sm)' }}>
          <span style={{ color: 'var(--text-secondary)' }}>Type</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>PicoClaw</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-sm)' }}>
          <span style={{ color: 'var(--text-secondary)' }}>LLM Backend</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>GLM-5 (z.ai)</span>
        </div>
        <div style={{ borderTop: '1px solid var(--border)', paddingTop: 'var(--space-sm)', display: 'flex', justifyContent: 'space-between' }}>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>Pricing</span>
          <span style={{ color: 'var(--accent)', fontWeight: 700, fontSize: '0.85rem' }}>Free during beta</span>
        </div>
      </div>
      {config.instructions && (
        <div style={{ marginBottom: 'var(--space-md)', padding: 'var(--space-sm)', background: 'var(--bg-tertiary)', borderRadius: '6px', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
          <strong>Instructions:</strong> {config.instructions}
        </div>
      )}
      <div className="modal-footer">
        <button className="btn btn-secondary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_PREV' })}>Back</button>
        <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_NEXT' })}>Deploy Now</button>
      </div>
    </div>
  )
}
