import { useWorkspace } from '../../context/WorkspaceContext'

export default function StepPayment() {
  const { dispatch } = useWorkspace()

  return (
    <div>
      <h3>Payment</h3>
      <p>Review pricing and confirm deployment.</p>
      <div className="card" style={{ marginBottom: 'var(--space-md)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-sm)' }}>
          <span style={{ color: 'var(--text-secondary)' }}>Claw Agent</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>$29/mo</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 'var(--space-sm)' }}>
          <span style={{ color: 'var(--text-secondary)' }}>Compute (estimated)</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>$12/mo</span>
        </div>
        <div style={{ borderTop: '1px solid var(--border)', paddingTop: 'var(--space-sm)', display: 'flex', justifyContent: 'space-between' }}>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>Total</span>
          <span style={{ color: 'var(--accent)', fontWeight: 700, fontSize: '1.1rem' }}>$41/mo</span>
        </div>
      </div>
      <div className="form-group">
        <label className="form-label">Payment Method</label>
        <select className="form-select" defaultValue="card">
          <option value="card">Credit Card ending in 4242</option>
          <option value="bch">BCH Wallet</option>
          <option value="new">Add new payment method...</option>
        </select>
      </div>
      <div className="modal-footer">
        <button className="btn btn-secondary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_PREV' })}>Back</button>
        <button className="btn btn-primary btn-sm" onClick={() => dispatch({ type: 'DEPLOY_NEXT' })}>Deploy Now</button>
      </div>
    </div>
  )
}
