import { useState } from 'react'
import ChannelHeader from './ChannelHeader'
import MessageList from './MessageList'
import MessageComposer from './MessageComposer'
import { useChat } from '../../context/ChatContext'

export default function ChatArea() {
  const { state: chatState, createWorkspace } = useChat()
  const [wsName, setWsName] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // No workspace and no topic selected — welcome state
  if (!chatState.activeWorkspace && !chatState.activeTopic && !chatState.clawTopic) {
    async function handleCreate(e: React.FormEvent) {
      e.preventDefault()
      const trimmed = wsName.trim()
      if (!trimmed || creating) return
      setCreating(true)
      setError(null)
      try {
        await createWorkspace(trimmed)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to create workspace')
        setCreating(false)
      }
    }

    return (
      <main className="chat-area">
        <div className="login-screen" style={{ position: 'absolute', inset: 0 }}>
          <div className="login-card">
            <div className="login-logo">
              <img src="/assets/logo.svg" alt="Gather" width="36" height="36" />
              <span className="login-brand">gather</span>
            </div>
            <h1 className="login-title">Welcome to Gather</h1>
            <p style={{ color: 'var(--text-muted)', fontSize: '0.9rem', margin: '0 0 var(--space-md)', textAlign: 'center' }}>
              You're not in a workspace yet. Create one, or wait for a teammate to invite you.
            </p>
            {error && <div className="login-error">{error}</div>}
            <form onSubmit={handleCreate} className="login-form">
              <div className="login-field">
                <label htmlFor="welcome-ws-name">Workspace name</label>
                <input
                  id="welcome-ws-name"
                  type="text"
                  value={wsName}
                  onChange={e => setWsName(e.target.value)}
                  placeholder="e.g. Acme Corp, My Project"
                  required
                  autoFocus
                  maxLength={64}
                  autoComplete="off"
                />
              </div>
              <button type="submit" className="login-submit" disabled={creating || !wsName.trim()}>
                {creating ? 'Creating...' : 'Create Workspace'}
              </button>
            </form>
          </div>
        </div>
      </main>
    )
  }

  return (
    <main className="chat-area">
      <ChannelHeader />
      {chatState.error && (
        <div style={{ background: '#b91c1c', color: '#fff', padding: '8px 16px', fontSize: '13px' }}>
          Chat: {chatState.error}
        </div>
      )}
      <MessageList />
      <MessageComposer />
    </main>
  )
}
