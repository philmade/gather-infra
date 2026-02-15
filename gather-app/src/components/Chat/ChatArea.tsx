import ChannelHeader from './ChannelHeader'
import MessageList from './MessageList'
import MessageComposer from './MessageComposer'
import { useChat } from '../../context/ChatContext'

export default function ChatArea() {
  const { state: chatState } = useChat()

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
