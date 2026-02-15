import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'
import { directMessages as mockDMs } from '../../data/channels'

export default function DirectMessageList() {
  const { state, dispatch } = useWorkspace()
  const { state: chatState, selectTopic } = useChat()

  const liveDMs = chatState.channels.filter(c => c.isP2P)

  if (chatState.connected) {
    return (
      <>
        <div className="sidebar-section-label" style={{ marginTop: 'var(--space-md)' }}>
          Direct Messages
          <span className="section-action" title="New message">+</span>
        </div>
        {liveDMs.map(dm => {
          const isActive = state.activeChannel === dm.topic
          return (
            <div
              key={dm.topic}
              className={`sidebar-item ${isActive ? 'active' : ''}`}
              onClick={() => {
                dispatch({ type: 'SET_CHANNEL', channel: dm.topic })
                selectTopic(dm.topic)
              }}
            >
              <div className={`dm-avatar avatar-bg-${hashColor(dm.topic)}`}>
                {dm.name.charAt(0).toUpperCase()}
                <span className={`presence-dot presence-${dm.online ? 'online' : 'offline'}`} />
              </div>
              <span className="item-name">{dm.name}</span>
              {dm.isBot && <span className="bot-indicator">BOT</span>}
              {dm.unread > 0 && <span className="unread-badge">{dm.unread}</span>}
            </div>
          )
        })}
      </>
    )
  }

  // Fallback to mock
  return (
    <>
      <div className="sidebar-section-label" style={{ marginTop: 'var(--space-md)' }}>
        Direct Messages
        <span className="section-action" title="New message">+</span>
      </div>
      {mockDMs.map(dm => (
        <div
          key={dm.id}
          className={`sidebar-item ${state.activeChannel === dm.id ? 'active' : ''}`}
          onClick={() => dispatch({ type: 'SET_CHANNEL', channel: dm.id })}
        >
          <div className={`dm-avatar ${dm.avatarClass}`}>
            {dm.avatarLetter}
            <span className={`presence-dot presence-${dm.presence}`} />
          </div>
          <span className="item-name">{dm.displayName}</span>
          {dm.isBot && <span className="bot-indicator">BOT</span>}
        </div>
      ))}
    </>
  )
}

function hashColor(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = ((h << 5) - h + s.charCodeAt(i)) | 0
  return (Math.abs(h) % 10) + 1
}
