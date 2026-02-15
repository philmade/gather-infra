import { useState, useRef } from 'react'
import { useChat } from '../../context/ChatContext'

export default function WorkspaceOnboarding() {
  const { createWorkspace } = useChat()
  const [name, setName] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed || creating) return

    setError(null)
    setCreating(true)
    try {
      await createWorkspace(trimmed)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create workspace')
      setCreating(false)
    }
  }

  return (
    <div className="login-screen">
      <div className="login-card">
        <div className="login-logo">
          <img src="/assets/logo.svg" alt="Gather" width="36" height="36" />
          <span className="login-brand">gather</span>
        </div>

        <h1 className="login-title">Create your workspace</h1>
        <p style={{ color: 'var(--text-muted)', fontSize: '0.9rem', margin: '0 0 var(--space-md)' }}>
          A workspace is where your team communicates. Give it a name to get started.
        </p>

        {error && (
          <div className="login-error">{error}</div>
        )}

        <form onSubmit={handleSubmit} className="login-form">
          <div className="login-field">
            <label htmlFor="ws-name">Workspace name</label>
            <input
              ref={inputRef}
              id="ws-name"
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. Acme Corp, My Project"
              required
              autoFocus
              maxLength={64}
              autoComplete="off"
            />
          </div>

          <button type="submit" className="login-submit" disabled={creating || !name.trim()}>
            {creating ? 'Creating...' : 'Create Workspace'}
          </button>
        </form>
      </div>
    </div>
  )
}
