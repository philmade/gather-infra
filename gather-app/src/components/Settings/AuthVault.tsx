import { useState } from 'react'
import { vaultEntries } from '../../data/settings'

export default function AuthVault() {
  const [showForm, setShowForm] = useState(false)

  return (
    <div>
      <h2>Authentication Vault</h2>
      <p className="section-desc">
        Store secrets that your Claws can access securely. Variables are encrypted at rest and only exposed to the agents you specify.
      </p>

      <div className="vault-table">
        <div className="vault-row vault-row-header">
          <span>Key</span>
          <span>Value</span>
          <span>Scoped To</span>
          <span></span>
        </div>
        {vaultEntries.map(entry => (
          <div key={entry.key} className="vault-row">
            <span className="vault-key">{entry.key}</span>
            <span className="vault-value">{entry.maskedValue}</span>
            <div className="vault-scope">
              {entry.scope.map(s => (
                <span key={s} className="badge badge-purple">{s}</span>
              ))}
            </div>
            <div className="vault-actions">
              <button title="Edit">{'\u270E'}</button>
              <button title="Delete">{'\uD83D\uDDD1'}</button>
            </div>
          </div>
        ))}
      </div>

      {!showForm && (
        <button className="btn btn-secondary btn-sm" onClick={() => setShowForm(true)}>
          + Add Variable
        </button>
      )}

      {showForm && (
        <div className="vault-add-form active">
          <div className="form-group">
            <label className="form-label">Key</label>
            <input type="text" className="form-input" placeholder="e.g., API_TOKEN" />
          </div>
          <div className="form-group">
            <label className="form-label">Value</label>
            <input type="password" className="form-input" placeholder="Enter secret value" />
          </div>
          <div className="form-group">
            <label className="form-label">Agent Scope</label>
            <select className="form-select" multiple style={{ height: 'auto', minHeight: '80px' }}>
              <option defaultChecked>All Claws</option>
              <option>BuyClaw</option>
              <option>ReviewClaw</option>
            </select>
          </div>
          <div style={{ display: 'flex', gap: 'var(--space-sm)', marginTop: 'var(--space-sm)' }}>
            <button className="btn btn-primary btn-sm">Save</button>
            <button className="btn btn-ghost btn-sm" onClick={() => setShowForm(false)}>Cancel</button>
          </div>
        </div>
      )}
    </div>
  )
}
