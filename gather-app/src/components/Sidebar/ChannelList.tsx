import { useEffect, useState, useRef } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'
import { channels as mockChannels } from '../../data/channels'

export default function ChannelList() {
  const { state, dispatch } = useWorkspace()
  const { state: chatState, selectTopic, createChannel } = useChat()
  const [adding, setAdding] = useState(false)
  const [newName, setNewName] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const liveChannels = chatState.channels.filter(c => !c.isP2P)

  // Auto-select first channel when live channels load
  useEffect(() => {
    if (chatState.connected && liveChannels.length > 0 && !chatState.activeTopic) {
      const first = liveChannels[0]
      dispatch({ type: 'SET_CHANNEL', channel: first.topic })
      selectTopic(first.topic)
    }
  }, [chatState.connected, liveChannels.length]) // eslint-disable-line react-hooks/exhaustive-deps

  // Focus input when add mode opens
  useEffect(() => {
    if (adding) inputRef.current?.focus()
  }, [adding])

  async function handleAddChannel() {
    const trimmed = newName.trim().toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-|-$/g, '')
    if (!trimmed) { setAdding(false); return }

    try {
      await createChannel(trimmed)
      setNewName('')
      setAdding(false)
    } catch (err) {
      console.error('[ChannelList] Create channel failed:', err)
    }
  }

  // When connected to Tinode, always show live data (even if empty)
  if (chatState.connected) {
    return (
      <>
        <div className="sidebar-section-label">
          Channels
          <span
            className="section-action"
            title="Add channel"
            onClick={() => setAdding(true)}
            style={{ cursor: 'pointer' }}
          >+</span>
        </div>
        {liveChannels.map(ch => {
          const isActive = state.activeChannel === ch.topic
          const hasUnread = ch.unread > 0
          return (
            <div
              key={ch.topic}
              className={`sidebar-item ${isActive ? 'active' : ''} ${hasUnread ? 'has-unread' : ''}`}
              onClick={() => {
                dispatch({ type: 'SET_MODE', mode: 'workspace' })
                dispatch({ type: 'SET_CHANNEL', channel: ch.topic })
                selectTopic(ch.topic)
              }}
            >
              <span className="channel-hash">#</span>
              <span className="item-name">{ch.name}</span>
              {hasUnread && <span className="unread-badge">{ch.unread}</span>}
            </div>
          )
        })}
        {liveChannels.length === 0 && !adding && (
          <div className="sidebar-item" style={{ opacity: 0.5, fontSize: '12px' }}>No channels yet</div>
        )}
        {adding && (
          <div className="sidebar-item" style={{ padding: '2px 8px 2px 12px' }}>
            <span className="channel-hash">#</span>
            <input
              ref={inputRef}
              type="text"
              value={newName}
              onChange={e => setNewName(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter') handleAddChannel()
                if (e.key === 'Escape') { setAdding(false); setNewName('') }
              }}
              onBlur={handleAddChannel}
              placeholder="channel-name"
              style={{
                background: 'transparent',
                border: 'none',
                borderBottom: '1px solid var(--accent)',
                color: 'var(--text-primary)',
                font: 'inherit',
                fontSize: '0.85rem',
                outline: 'none',
                padding: '2px 0',
                width: '100%',
              }}
            />
          </div>
        )}
      </>
    )
  }

  // Fallback to mock data only when not connected
  return (
    <>
      <div className="sidebar-section-label">
        Channels
        <span className="section-action" title="Add channel">+</span>
      </div>
      {mockChannels.map(ch => {
        const isActive = state.activeChannel === ch.id
        return (
          <div
            key={ch.id}
            className={`sidebar-item ${isActive ? 'active' : ''}`}
            onClick={() => {
              dispatch({ type: 'SET_MODE', mode: 'workspace' })
              dispatch({ type: 'SET_CHANNEL', channel: ch.id })
            }}
          >
            <span className="channel-hash">#</span>
            <span className="item-name">{ch.name}</span>
            {ch.unread && ch.unread > 0 && <span className="unread-badge">{ch.unread}</span>}
          </div>
        )
      })}
    </>
  )
}
