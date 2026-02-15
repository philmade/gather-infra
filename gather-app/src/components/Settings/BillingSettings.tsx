import { billingPlan } from '../../data/settings'

export default function BillingSettings() {
  return (
    <div>
      <h2>Billing</h2>
      <p className="section-desc">Manage your subscription and payment methods.</p>

      <div className="plan-card">
        <div className="plan-name">{billingPlan.name}</div>
        <div className="plan-price">{billingPlan.price}</div>
      </div>

      <h3 style={{ fontSize: '0.95rem', color: 'var(--text-primary)', margin: 'var(--space-lg) 0 var(--space-sm)' }}>
        Usage This Month
      </h3>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 'var(--space-sm)', marginBottom: 'var(--space-lg)' }}>
        <div className="card stat-card">
          <div className="stat-value">{billingPlan.activeClaws}</div>
          <div className="stat-label">Active Claws</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value">{billingPlan.computeHours}</div>
          <div className="stat-label">Compute Hours</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value">{billingPlan.currentBill}</div>
          <div className="stat-label">Current Bill</div>
        </div>
      </div>

      <h3 style={{ fontSize: '0.95rem', color: 'var(--text-primary)', marginBottom: 'var(--space-sm)' }}>
        Payment Method
      </h3>
      <div className="service-row">
        <div className="service-icon">{'\uD83D\uDCB3'}</div>
        <div className="service-info">
          <div className="service-name">{billingPlan.paymentMethod}</div>
          <div className="service-status">{billingPlan.paymentExpiry}</div>
        </div>
        <button className="btn btn-ghost btn-sm">Update</button>
      </div>
    </div>
  )
}
