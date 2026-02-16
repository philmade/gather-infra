import { useState, useEffect } from 'react'
import { listVault, createVaultEntry, updateVaultEntry, deleteVaultEntry, listClaws, type VaultEntry, type ClawDeployment } from '../../lib/api'

const SUGGESTED_VARS = [
  { key: 'CLAW_LLM_API_KEY', desc: 'API key for your LLM provider (OpenAI, Anthropic, Z.AI, etc.)' },
  { key: 'CLAW_LLM_API_URL', desc: 'LLM base URL — e.g. https://api.openai.com/v1' },
  { key: 'CLAW_LLM_MODEL', desc: 'Model name — e.g. gpt-4o, claude-sonnet-4-5-20250929' },
  { key: 'TELEGRAM_BOT_TOKEN', desc: 'Telegram Bot API token from @BotFather' },
  { key: 'GITHUB_TOKEN', desc: 'GitHub personal access token for repo access' },
]

export default function AuthVault() {
  const [entries, setEntries] = useState<VaultEntry[]>([])
  const [claws, setClaws] = useState<ClawDeployment[]>([])
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [formKey, setFormKey] = useState('')
  const [formValue, setFormValue] = useState('')
  const [formScope, setFormScope] = useState<string[]>([])
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)

  async function loadVault() {
    try {
      const data = await listVault()
      setEntries(data.entries || [])
    } catch (e: any) {
      console.error('Vault load failed:', e)
      setError('Failed to load vault: ' + e.message)
    }
  }

  async function loadClaws() {
    try {
      const data = await listClaws()
      setClaws(data.claws || [])
    } catch (e: any) {
      console.error('Claws load failed:', e)
    }
  }

  useEffect(() => {
    Promise.all([loadVault(), loadClaws()]).finally(() => setLoading(false))
  }, [])

  function resetForm() {
    setFormKey('')
    setFormValue('')
    setFormScope([])
    setShowForm(false)
    setEditingId(null)
    setError('')
  }

  function startAdd(key: string) {
    setEditingId(null)
    setFormKey(key)
    setFormValue('')
    setFormScope([])
    setShowForm(true)
    setError('')
  }

  function startEdit(entry: VaultEntry) {
    setEditingId(entry.id)
    setFormKey(entry.key)
    setFormValue('')
    setFormScope(entry.scope || [])
    setShowForm(true)
    setError('')
  }

  async function handleSave() {
    setError('')
    setSaving(true)
    try {
      if (editingId) {
        const body: { key?: string; value?: string; scope?: string[] } = { scope: formScope }
        body.key = formKey
        if (formValue) body.value = formValue
        await updateVaultEntry(editingId, body)
      } else {
        if (!formKey || !formValue) {
          setError('Key and value are required')
          setSaving(false)
          return
        }
        await createVaultEntry({ key: formKey, value: formValue, scope: formScope })
      }
      resetForm()
      await loadVault()
    } catch (e: any) {
      console.error('Vault save failed:', e)
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: string) {
    setError('')
    try {
      await deleteVaultEntry(id)
      await loadVault()
    } catch (e: any) {
      console.error('Vault delete failed:', e)
      setError(e.message)
    }
  }

  function toggleScope(clawId: string) {
    setFormScope(prev =>
      prev.includes(clawId) ? prev.filter(s => s !== clawId) : [...prev, clawId]
    )
  }

  function scopeLabel(scope: string[]) {
    if (!scope || scope.length === 0) return 'All Claws'
    return scope.map(id => {
      const claw = claws.find(c => c.id === id)
      return claw ? claw.name : id.slice(0, 8)
    }).join(', ')
  }

  const existingKeys = new Set(entries.map(e => e.key))
  const suggestions = SUGGESTED_VARS.filter(s => !existingKeys.has(s.key))

  if (loading) return <div>Loading vault...</div>

  return (
    <div>
      <h2>Authentication Vault</h2>
      <p className="section-desc">
        Store API keys and tokens that your Claws need. Each variable is injected as an
        environment variable when a Claw is deployed. To change a running Claw's config,
        update the value here then re-deploy the Claw.
      </p>

      {error && (
        <div style={{
          padding: '8px 12px',
          marginBottom: '12px',
          background: 'var(--color-error-bg, #fee)',
          color: 'var(--color-error, #c33)',
          borderRadius: '6px',
          border: '1px solid var(--color-error-border, #fcc)',
          fontSize: '0.875rem',
        }}>
          {error}
        </div>
      )}

      <div className="vault-table">
        <div className="vault-row vault-row-header">
          <span>Key</span>
          <span>Value</span>
          <span>Scoped To</span>
          <span></span>
        </div>
        {entries.map(entry => (
          <div key={entry.id} className="vault-row">
            <span className="vault-key">{entry.key}</span>
            <span className="vault-value">{entry.masked_value}</span>
            <div className="vault-scope">
              <span className="badge badge-purple">{scopeLabel(entry.scope)}</span>
            </div>
            <div className="vault-actions">
              <button title="Edit" onClick={() => startEdit(entry)}>{'\u270E'}</button>
              <button title="Delete" onClick={() => handleDelete(entry.id)}>{'\uD83D\uDDD1'}</button>
            </div>
          </div>
        ))}
        {entries.length === 0 && (
          <div className="vault-row" style={{ opacity: 0.6 }}>
            <span>No secrets stored yet — add one below</span>
          </div>
        )}
      </div>

      {suggestions.length > 0 && !showForm && (
        <div style={{
          margin: '16px 0',
          padding: '12px 16px',
          background: 'var(--color-surface-2, #1a1a2e)',
          borderRadius: '8px',
          border: '1px solid var(--color-border, #333)',
        }}>
          <div style={{ fontSize: '0.8rem', opacity: 0.7, marginBottom: '8px' }}>
            Suggested variables — click to add:
          </div>
          {suggestions.map(s => (
            <div
              key={s.key}
              onClick={() => startAdd(s.key)}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                padding: '6px 8px',
                marginBottom: '4px',
                borderRadius: '4px',
                cursor: 'pointer',
                transition: 'background 0.15s',
              }}
              onMouseEnter={e => (e.currentTarget.style.background = 'var(--color-surface-3, #252540)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
            >
              <div>
                <span style={{ fontFamily: 'var(--font-mono, monospace)', fontSize: '0.85rem' }}>
                  {s.key}
                </span>
                <span style={{ opacity: 0.5, fontSize: '0.8rem', marginLeft: '8px' }}>
                  {s.desc}
                </span>
              </div>
              <span style={{ opacity: 0.4, fontSize: '0.75rem' }}>+ Add</span>
            </div>
          ))}
        </div>
      )}

      {!showForm && (
        <button className="btn btn-secondary btn-sm" onClick={() => { resetForm(); setShowForm(true) }}>
          + Add Variable
        </button>
      )}

      {showForm && (
        <div className="vault-add-form active">
          <div className="form-group">
            <label className="form-label">Key</label>
            <input
              type="text"
              className="form-input"
              placeholder="e.g., CLAW_LLM_API_KEY"
              value={formKey}
              onChange={e => setFormKey(e.target.value)}
            />
          </div>
          <div className="form-group">
            <label className="form-label">
              Value {editingId ? '(leave blank to keep current)' : ''}
            </label>
            <input
              type="password"
              className="form-input"
              placeholder="Enter secret value"
              value={formValue}
              onChange={e => setFormValue(e.target.value)}
            />
          </div>
          <div className="form-group">
            <label className="form-label">
              Scope
              <span style={{ opacity: 0.5, fontWeight: 'normal', marginLeft: '6px', fontSize: '0.8rem' }}>
                — which Claws can use this variable?
              </span>
            </label>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={formScope.length === 0}
                  onChange={() => setFormScope([])}
                />
                All Claws
              </label>
              {claws.map(claw => (
                <label key={claw.id} style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
                  <input
                    type="checkbox"
                    checked={formScope.includes(claw.id)}
                    onChange={() => toggleScope(claw.id)}
                  />
                  {claw.name}
                </label>
              ))}
              {claws.length === 0 && (
                <span style={{ opacity: 0.5, fontSize: '0.8rem' }}>
                  No claws deployed yet — variable will apply to all future claws
                </span>
              )}
            </div>
          </div>
          <div style={{ display: 'flex', gap: 'var(--space-sm, 8px)', marginTop: 'var(--space-sm, 8px)' }}>
            <button
              className="btn btn-primary btn-sm"
              onClick={handleSave}
              disabled={saving}
            >
              {saving ? 'Saving...' : editingId ? 'Update' : 'Save'}
            </button>
            <button className="btn btn-ghost btn-sm" onClick={resetForm} disabled={saving}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
