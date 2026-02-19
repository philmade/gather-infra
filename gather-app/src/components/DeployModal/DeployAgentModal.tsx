import { useEffect, useState } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import StepIndicator from './StepIndicator'
import StepConfigure from './StepConfigure'
import StepDeploying from './StepDeploying'
import StepDone from './StepDone'

export interface DeployConfig {
  name: string
  instructions: string
}

export default function DeployAgentModal() {
  const { state, dispatch } = useWorkspace()
  const { open, step } = state.deployModal

  const [config, setConfig] = useState<DeployConfig>({
    name: '',
    instructions: '',
  })
  const [deploymentId, setDeploymentId] = useState<string | null>(null)

  // Reset config when modal opens
  useEffect(() => {
    if (open) {
      setConfig({ name: '', instructions: '' })
      setDeploymentId(null)
    }
  }, [open])

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && open) {
        dispatch({ type: 'CLOSE_DEPLOY' })
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, dispatch])

  if (!open) return null

  return (
    <div
      className="modal-overlay deploy-modal"
      onClick={(e) => {
        if (e.target === e.currentTarget) dispatch({ type: 'CLOSE_DEPLOY' })
      }}
    >
      <div className="modal-container">
        <div className="modal-header">
          <span className="modal-title">Try a Claw — Free for 30 minutes</span>
          <button className="modal-close" onClick={() => dispatch({ type: 'CLOSE_DEPLOY' })}>&times;</button>
        </div>
        <StepIndicator currentStep={step} totalSteps={3} />
        {step === 1 && <StepConfigure config={config} setConfig={setConfig} />}
        {step === 2 && <StepDeploying config={config} onDeployed={setDeploymentId} />}
        {step === 3 && <StepDone config={config} deploymentId={deploymentId} />}
      </div>
    </div>
  )
}
