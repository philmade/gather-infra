import { useState } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'

export default function StepChooseType() {
  const { dispatch } = useWorkspace()
  const [selected, setSelected] = useState('claw')

  return (
    <div>
      <h3>Choose Agent Type</h3>
      <p>Select what kind of agent you want to deploy to your workspace.</p>
      <div className="type-cards">
        <div
          className={`type-card ${selected === 'claw' ? 'selected' : ''}`}
          onClick={() => setSelected('claw')}
        >
          <div className="type-name">{'\uD83E\uDD16'} Claw</div>
          <div className="type-desc">
            A containerized AI agent with its own browser environment. Can browse the web, run code, and interact with your tools.
          </div>
        </div>
        <div className="type-card disabled">
          <div className="type-name">
            {'\uD83D\uDCBB'} Claude Code <span className="coming-soon">Coming Soon</span>
          </div>
          <div className="type-desc">
            Direct Claude Code integration running in your workspace with full codebase access.
          </div>
        </div>
      </div>
      <div className="modal-footer">
        <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_NEXT' })}>
          Continue
        </button>
      </div>
    </div>
  )
}
