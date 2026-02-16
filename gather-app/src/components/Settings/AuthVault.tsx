import { useState, useEffect } from 'react'
import { pb } from '../../lib/pocketbase'

interface VaultRecord {
  id: string
  key: string
  value: string
  user_id: string
}

const LLM_VARS = [
  { key: 'CLAW_LLM_API_KEY', label: 'API Key', placeholder: 'Your LLM provider API key' },
  { key: 'CLAW_LLM_API_URL', label: 'API URL', placeholder: 'e.g. https://api.openai.com/v1' },
  { key: 'CLAW_LLM_MODEL', label: 'Model', placeholder: 'e.g. gpt-4o, claude-sonnet-4-5-20250929' },
]

function maskValue(v: string): string {
  if (!v || v.length < 8) return '****'
  return v.slice(0, 4) + '****' + v.slice(-4)
}

export default function AuthVault() {
  const [entries, setEntries] = useState<VaultRecord[]>([])
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [formValue, setFormValue] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)

  async function loadVault() {
    try {
      const records = await pb.collection('claw_secrets').getFullList<VaultRecord>({
        sort: 'key',
      })
      setEntries(records)
    } catch (e: any) {
      console.error('Vault load failed:', e)
      setError('Failed to load vault: ' + e.message)
    }
  }

  useEffect(() => {
    loadVault().finally(() => setLoading(false))
  }, [])

  function getEntry(key: string): VaultRecord | undefined {
    return entries.find(e => e.key === key)
  }

  async function handleSave(key: string) {
    if (!formValue.trim()) return
    setError('')
    setSaving(true)
    try {
      const userId = pb.authStore.record?.id
      if (!userId) { setError('Not authenticated'); return }

      const existing = getEntry(key)
      if (existing) {
        await pb.collection('claw_secrets').update(existing.id, { value: formValue })
      } else {
        await pb.collection('claw_secrets').create({
          user_id: userId,
          key,
          value: formValue,
          scope: [],
        })
      }
      setEditingKey(null)
      setFormValue('')
      await loadVault()
    } catch (e: any) {
      console.error('Vault save failed:', e)
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }

  async function handleClear(key: string) {
    const existing = getEntry(key)
    if (!existing) return
    setError('')
    try {
      await pb.collection('claw_secrets').delete(existing.id)
      await loadVault()
    } catch (e: any) {
      console.error('Vault delete failed:', e)
      setError(e.message)
    }
  }

  if (loading) return <div>Loading...</div>

  return (
    <div>
      <h2>LLM Provider</h2>
      <p className="section-desc">
        Configure the LLM provider for all your Claws. These credentials are injected
        as environment variables when a Claw is deployed. Changes take effect on next deploy.
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

      <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
        {LLM_VARS.map(v => {
          const entry = getEntry(v.key)
          const isEditing = editingKey === v.key

          return (
            <div key={v.key} style={{
              padding: '12px 16px',
              background: 'var(--color-surface-2, #1a1a2e)',
              borderRadius: '8px',
              border: '1px solid var(--color-border, #333)',
            }}>
              <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: isEditing ? '8px' : 0,
              }}>
                <div>
                  <div style={{ fontSize: '0.85rem', fontWeight: 600 }}>{v.label}</div>
                  <div style={{
                    fontFamily: 'var(--font-mono, monospace)',
                    fontSize: '0.8rem',
                    opacity: 0.5,
                    marginTop: '2px',
                  }}>
                    {v.key}
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  {entry && !isEditing && (
                    <span style={{
                      fontFamily: 'var(--font-mono, monospace)',
                      fontSize: '0.85rem',
                      opacity: 0.7,
                    }}>
                      {maskValue(entry.value)}
                    </span>
                  )}
                  {!isEditing && (
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={() => { setEditingKey(v.key); setFormValue('') }}
                    >
                      {entry ? 'Change' : 'Set'}
                    </button>
                  )}
                  {entry && !isEditing && (
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={() => handleClear(v.key)}
                      style={{ opacity: 0.5 }}
                    >
                      Clear
                    </button>
                  )}
                </div>
              </div>

              {isEditing && (
                <div style={{ display: 'flex', gap: '8px' }}>
                  <input
                    type="password"
                    className="form-input"
                    placeholder={v.placeholder}
                    value={formValue}
                    onChange={e => setFormValue(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleSave(v.key)}
                    autoFocus
                    style={{ flex: 1 }}
                  />
                  <button
                    className="btn btn-primary btn-sm"
                    onClick={() => handleSave(v.key)}
                    disabled={saving || !formValue.trim()}
                  >
                    {saving ? '...' : 'Save'}
                  </button>
                  <button
                    className="btn btn-ghost btn-sm"
                    onClick={() => { setEditingKey(null); setFormValue('') }}
                  >
                    Cancel
                  </button>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
