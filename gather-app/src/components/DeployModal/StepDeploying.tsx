import { useEffect } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'

export default function StepDeploying() {
  const { dispatch } = useWorkspace()

  useEffect(() => {
    const timer = setTimeout(() => {
      dispatch({ type: 'DEPLOY_SET_STEP', step: 5 })
    }, 2000)
    return () => clearTimeout(timer)
  }, [dispatch])

  return (
    <div className="deploy-progress">
      <div className="loading-spinner spinner-lg progress-spinner" style={{ borderTopColor: 'var(--accent)' }} />
      <div className="progress-text">Setting up your agent...</div>
      <div style={{ color: 'var(--text-muted)', fontSize: '0.8rem', marginTop: 'var(--space-sm)' }}>
        Provisioning container, installing dependencies, connecting services
      </div>
    </div>
  )
}
