import { useEffect, useRef } from 'react'
import { channelMessages } from '../../data/channels'
import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'
import Message from './Message'
import LiveMessage from './LiveMessage'

export default function MessageList() {
  const { state } = useWorkspace()
  const { state: chatState } = useChat()
  const ref = useRef<HTMLDivElement>(null)

  // Scroll to bottom on channel switch or new messages
  useEffect(() => {
    if (ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight
    }
  }, [state.activeChannel, chatState.messages.length, chatState.clawMessages.length])

  // Claw REST channel view
  if (chatState.clawTopic) {
    return (
      <div className="message-list" ref={ref}>
        {chatState.clawMessages.map(msg => (
          <LiveMessage key={`${msg.ts}-${msg.seq}`} msg={msg} />
        ))}
        {chatState.clawMessages.length === 0 && (
          <div className="message-system"><span>No messages yet. Say something!</span></div>
        )}
      </div>
    )
  }

  // When connected to Tinode, always show live data
  if (chatState.connected) {
    return (
      <div className="message-list" ref={ref}>
        {chatState.activeTopic ? (
          <>
            {chatState.messages.map(msg => (
              <LiveMessage key={msg.seq} msg={msg} />
            ))}
            {chatState.messages.length === 0 && (
              <div className="message-system"><span>No messages yet. Say something!</span></div>
            )}
          </>
        ) : (
          <div className="message-system"><span>Select a channel to start chatting</span></div>
        )}
      </div>
    )
  }

  // Fallback to mock only when not connected
  const messages = channelMessages[state.activeChannel] ?? []
  return (
    <div className="message-list" ref={ref}>
      {messages.map(item => (
        <Message key={item.id} item={item} />
      ))}
    </div>
  )
}
