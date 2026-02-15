import { useEffect, useState } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import StepIndicator from './StepIndicator'
import StepChooseType from './StepChooseType'
import StepConfigure from './StepConfigure'
import StepPayment from './StepPayment'
import StepDeploying from './StepDeploying'
import StepDone from './StepDone'

export interface DeployConfig {
  name: string
  instructions: string
  githubRepo: string
}

export default function DeployAgentModal() {
  const { state, dispatch } = useWorkspace()
  const { open, step } = state.deployModal

  const [config, setConfig] = useState<DeployConfig>({
    name: '',
    instructions: '',
    githubRepo: '',
  })
  const [deploymentId, setDeploymentId] = useState<string | null>(null)

  // Reset config when modal opens
  useEffect(() => {
    if (open) {
      setConfig({ name: '', instructions: '', githubRepo: '' })
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
          <span className="modal-title">Deploy a Claw</span>
          <button className="modal-close" onClick={() => dispatch({ type: 'CLOSE_DEPLOY' })}>&times;</button>
        </div>
        <StepIndicator currentStep={step} />
        {step === 1 && <StepChooseType />}
        {step === 2 && <StepConfigure config={config} setConfig={setConfig} />}
        {step === 3 && <StepPayment config={config} />}
        {step === 4 && <StepDeploying config={config} onDeployed={setDeploymentId} />}
        {step === 5 && <StepDone config={config} deploymentId={deploymentId} />}
      </div>
    </div>
  )
}
