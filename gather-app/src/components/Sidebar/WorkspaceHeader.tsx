import { useState, useRef, useEffect } from 'react'
import { useChat } from '../../context/ChatContext'

export default function WorkspaceHeader() {
  const { state, switchWorkspace, createWorkspace } = useChat()
  const [open, setOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [newName, setNewName] = useState('')
  const [creating, setCreating] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const activeWs = state.workspaces.find(w => w.topic === state.activeWorkspace)
  const name = activeWs?.name ?? 'Gather'

  // Close dropdown on click outside
  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setOpen(false)
        setAdding(false)
        setNewName('')
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  // Focus input when adding
  useEffect(() => {
    if (adding) inputRef.current?.focus()
  }, [adding])

  async function handleCreate() {
    const trimmed = newName.trim()
    if (!trimmed || creating) return
    setCreating(true)
    try {
      await createWorkspace(trimmed)
      setNewName('')
      setAdding(false)
      setOpen(false)
    } catch (err) {
      console.error('[WorkspaceHeader] Create failed:', err)
    } finally {
      setCreating(false)
    }
  }

  return (
    <div ref={dropdownRef} style={{ position: 'relative' }}>
      <div className="sidebar-header" onClick={() => setOpen(!open)} style={{ cursor: 'pointer' }}>
        <img src="/assets/logo.svg" alt="Gather" className="workspace-logo" />
        <span className="workspace-name">{name}</span>
        <span className="dropdown-icon" style={{ transform: open ? 'rotate(180deg)' : undefined, transition: 'transform 0.15s' }}>{'\u25BE'}</span>
      </div>

      {open && (
        <div style={{
          position: 'absolute',
          top: '100%',
          left: 0,
          right: 0,
          zIndex: 100,
          background: 'var(--bg-tertiary)',
          border: '1px solid var(--border)',
          borderRadius: '0 0 8px 8px',
          overflow: 'hidden',
          boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
        }}>
          {state.workspaces.map(ws => (
            <div
              key={ws.topic}
              onClick={() => {
                switchWorkspace(ws.topic)
                setOpen(false)
              }}
              style={{
                padding: '8px 12px',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                fontSize: '0.85rem',
                color: 'var(--text-primary)',
                background: ws.topic === state.activeWorkspace ? 'var(--accent-soft)' : undefined,
              }}
              onMouseEnter={e => {
                if (ws.topic !== state.activeWorkspace) e.currentTarget.style.background = 'var(--accent-soft)'
              }}
              onMouseLeave={e => {
                if (ws.topic !== state.activeWorkspace) e.currentTarget.style.background = ''
              }}
            >
              {ws.topic === state.activeWorkspace && <span style={{ fontSize: '0.75rem' }}>&#10003;</span>}
              <span>{ws.name}</span>
            </div>
          ))}

          {state.workspaces.length > 0 && (
            <div style={{ borderTop: '1px solid var(--border)' }} />
          )}

          {adding ? (
            <div style={{ padding: '8px 12px' }}>
              <input
                ref={inputRef}
                type="text"
                value={newName}
                onChange={e => setNewName(e.target.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter') handleCreate()
                  if (e.key === 'Escape') { setAdding(false); setNewName('') }
                }}
                placeholder="Workspace name"
                disabled={creating}
                style={{
                  width: '100%',
                  background: 'transparent',
                  border: 'none',
                  borderBottom: '1px solid var(--accent)',
                  color: 'var(--text-primary)',
                  font: 'inherit',
                  fontSize: '0.85rem',
                  outline: 'none',
                  padding: '2px 0',
                }}
              />
            </div>
          ) : (
            <div
              onClick={() => setAdding(true)}
              style={{
                padding: '8px 12px',
                cursor: 'pointer',
                fontSize: '0.85rem',
                color: 'var(--text-muted)',
              }}
              onMouseEnter={e => { e.currentTarget.style.background = 'var(--accent-soft)' }}
              onMouseLeave={e => { e.currentTarget.style.background = '' }}
            >
              + Create workspace
            </div>
          )}
        </div>
      )}
    </div>
  )
}
