import type { Message as MessageType, DateSeparator, MessageItem } from '../../data/channels'

function DateSep({ label }: { label: string }) {
  return (
    <div className="date-separator"><span>{label}</span></div>
  )
}

function SystemMessage({ text }: { text: string }) {
  return (
    <div className="message-system"><span>{text}</span></div>
  )
}

function ChatMessage({ msg }: { msg: MessageType }) {
  return (
    <div className="message">
      <div className={`message-avatar ${msg.isBot ? 'bot-avatar' : ''} ${msg.avatarClass}`}>
        {msg.avatarLetter}
      </div>
      <div className="message-content">
        <div className="message-header">
          <span className="message-author">
            {msg.author}
            {msg.isBot && <span className="bot-badge">BOT</span>}
          </span>
          <span className="message-time">{msg.time}</span>
        </div>
        <div className="message-text" dangerouslySetInnerHTML={{ __html: msg.text }} />
      </div>
    </div>
  )
}

export default function Message({ item }: { item: MessageItem }) {
  if ('isDateSeparator' in item) {
    return <DateSep label={(item as DateSeparator).label} />
  }
  const msg = item as MessageType
  if (msg.isSystem) {
    return <SystemMessage text={msg.text} />
  }
  return <ChatMessage msg={msg} />
}
