import { useWorkspace } from '../../context/WorkspaceContext'
import type { DeployConfig } from './DeployAgentModal'

const tierLabels: Record<string, { name: string; price: string }> = {
  lite: { name: 'Claw Lite', price: '$27/quarter' },
  pro: { name: 'Claw Pro', price: '$81/quarter' },
  max: { name: 'Claw Max', price: '$216/quarter' },
}

interface Props {
  config: DeployConfig
}

export default function StepPayment({ config }: Props) {
  const { dispatch } = useWorkspace()
  const tier = tierLabels[config.clawType] || tierLabels.lite

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
          <span style={{ color: 'var(--text-secondary)' }}>Plan</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>{tier.name}</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-sm)' }}>
          <span style={{ color: 'var(--text-secondary)' }}>LLM Backend</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>GLM-5 (z.ai)</span>
        </div>
        <div style={{ borderTop: '1px solid var(--border)', paddingTop: 'var(--space-sm)', display: 'flex', justifyContent: 'space-between' }}>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>Price</span>
          <span style={{ color: 'var(--accent)', fontWeight: 700, fontSize: '0.85rem' }}>{tier.price}</span>
        </div>
        <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: 'var(--space-xs)' }}>
          30-minute free trial. Your claw deploys immediately. Upgrade from the detail panel to keep it running.
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
