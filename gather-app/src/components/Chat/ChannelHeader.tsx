import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'
import { channels, directMessages } from '../../data/channels'

export default function ChannelHeader() {
  const { state, dispatch } = useWorkspace()
  const { state: chatState, getMembers } = useChat()
  const ch = state.activeChannel

  const isClawTopic = chatState.clawTopic != null && ch.startsWith('claw:')
  const useLive = chatState.connected && chatState.activeTopic != null

  let displayName: string
  let topic: string
  let memberCount: number | null = null
  let isP2P = false

  if (isClawTopic) {
    displayName = chatState.clawName || ch.replace('claw:', '')
    topic = 'Claw agent channel'
    isP2P = false
  } else if (useLive) {
    const channel = chatState.channels.find(c => c.topic === chatState.activeTopic)
    displayName = channel?.name ?? chatState.activeTopic!
    topic = channel?.isP2P ? 'Direct message' : ''
    isP2P = channel?.isP2P ?? false
    const members = getMembers()
    memberCount = members.length > 0 ? members.length : null
  } else {
    const isDM = ch.startsWith('dm-')
    if (isDM) {
      const dm = directMessages.find(d => d.id === ch)
      displayName = dm?.displayName ?? ch.replace('dm-', '')
      topic = 'Direct message'
      isP2P = true
    } else {
      const channel = channels.find(c => c.id === ch)
      displayName = ch
      topic = channel?.topic ?? ''
    }
  }

  return (
    <div className="channel-header">
      <div className="channel-info">
        <div className="channel-name">
          {isP2P ? displayName : <><span className="hash">#</span> {displayName}</>}
        </div>
        <div className="channel-topic">{topic}</div>
      </div>
      <div className="channel-actions">
        <span
          className="member-count"
          onClick={() => dispatch({ type: 'TOGGLE_DETAIL' })}
          style={{ cursor: 'pointer' }}
        >
          <span>{'\uD83D\uDC65'}</span> {memberCount ?? ''}
        </span>
        <button className="header-btn" title="Search">{'\uD83D\uDD0D'}</button>
        <button className="header-btn" title="Pin">{'\uD83D\uDCCC'}</button>
      </div>
    </div>
  )
}
