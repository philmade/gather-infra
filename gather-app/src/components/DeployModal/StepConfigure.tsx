import { useWorkspace } from '../../context/WorkspaceContext'

export default function StepConfigure() {
  const { dispatch } = useWorkspace()

  return (
    <div>
      <h3>Configure Your Claw</h3>
      <p>Give your agent a name and connect it to your code.</p>
      <div className="form-group">
        <label className="form-label">Agent Name</label>
        <input type="text" className="form-input" placeholder="e.g., BuyClaw, DeployClaw, SupportClaw" defaultValue="DataClaw" />
      </div>
      <div className="form-group">
        <label className="form-label">GitHub Repository (optional)</label>
        <input type="text" className="form-input" placeholder="e.g., acme/data-pipeline" />
      </div>
      <div className="form-group">
        <label className="form-label">Initial Instructions</label>
        <textarea className="form-input" rows={3} placeholder="What should this agent do? e.g., Monitor the data pipeline and alert on failures..." />
      </div>
      <div className="modal-footer">
        <button className="btn btn-secondary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_PREV' })}>Back</button>
        <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_NEXT' })}>Continue</button>
      </div>
    </div>
  )
}
