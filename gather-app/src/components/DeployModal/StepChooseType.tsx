import { useWorkspace } from '../../context/WorkspaceContext'
import type { DeployConfig, ClawTier } from './DeployAgentModal'

const tiers: { id: ClawTier; name: string; price: string; desc: string }[] = [
  { id: 'lite', name: 'Lite', price: '$27/quarter', desc: '120 prompts per 5-hour cycle. Great for lightweight tasks and monitoring.' },
  { id: 'pro', name: 'Pro', price: '$81/quarter', desc: '600 prompts per 5-hour cycle with priority access. For active agents.' },
  { id: 'max', name: 'Max', price: '$216/quarter', desc: '2,400 prompts per 5-hour cycle. Maximum capacity for heavy workloads.' },
]

interface Props {
  config: DeployConfig
  setConfig: (c: DeployConfig) => void
}

export default function StepChooseType({ config, setConfig }: Props) {
  const { dispatch } = useWorkspace()

  return (
    <div>
      <h3>Choose Plan</h3>
      <p>Select a tier for your claw. All plans include a 30-minute free trial.</p>
      <div className="type-cards">
        {tiers.map((t) => (
          <div
            key={t.id}
            className={`type-card ${config.clawType === t.id ? 'selected' : ''}`}
            onClick={() => setConfig({ ...config, clawType: t.id })}
          >
            <div className="type-name" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span>{t.name}</span>
              <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--accent)' }}>{t.price}</span>
            </div>
            <div className="type-desc">{t.desc}</div>
          </div>
        ))}
      </div>
      <div className="modal-footer">
        <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_NEXT' })}>
          Continue
        </button>
      </div>
    </div>
  )
}
