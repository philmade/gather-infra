import { useEffect, useState } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { pb } from '../../lib/pocketbase'
import type { DeployConfig } from './DeployAgentModal'

interface Props {
  config: DeployConfig
  onDeployed: (id: string) => void
}

export default function StepDeploying({ config, onDeployed }: Props) {
  const { dispatch } = useWorkspace()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function deploy() {
      try {
        const resp = await fetch(pb.baseURL + '/api/claws', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${pb.authStore.token}`,
          },
          body: JSON.stringify({
            name: config.name.trim(),
            instructions: config.instructions.trim(),
            claw_type: 'lite',
          }),
        })

        if (cancelled) return

        if (!resp.ok) {
          const body = await resp.text()
          throw new Error(`Deploy failed: ${resp.status} — ${body}`)
        }

        const data = await resp.json()
        if (cancelled) return

        onDeployed(data.id)
        dispatch({ type: 'DEPLOY_SET_STEP', step: 3 })
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      }
    }

    deploy()
    return () => { cancelled = true }
  }, [config, dispatch, onDeployed])

  if (error) {
    return (
      <div style={{ textAlign: 'center', padding: 'var(--space-lg)' }}>
        <div style={{ color: 'var(--red)', marginBottom: 'var(--space-md)', fontSize: '0.85rem' }}>{error}</div>
        <button className="btn btn-secondary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_PREV' })}>
          Go Back
        </button>
      </div>
    )
  }

  return (
    <div className="deploy-progress">
      <div className="loading-spinner spinner-lg progress-spinner" style={{ borderTopColor: 'var(--accent)' }} />
      <div className="progress-text">Deploying {config.name}...</div>
      <div style={{ color: 'var(--text-muted)', fontSize: '0.8rem', marginTop: 'var(--space-sm)' }}>
        Setting up your container. This takes a few seconds.
      </div>
    </div>
  )
}
