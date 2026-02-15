import { useEffect } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import StepIndicator from './StepIndicator'
import StepChooseType from './StepChooseType'
import StepConfigure from './StepConfigure'
import StepPayment from './StepPayment'
import StepDeploying from './StepDeploying'
import StepDone from './StepDone'

export default function DeployAgentModal() {
  const { state, dispatch } = useWorkspace()
  const { open, step } = state.deployModal

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
        {step === 2 && <StepConfigure />}
        {step === 3 && <StepPayment />}
        {step === 4 && <StepDeploying />}
        {step === 5 && <StepDone />}
      </div>
    </div>
  )
}
