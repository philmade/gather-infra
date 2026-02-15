import { feedMessages } from '../../data/network'

const typeClass: Record<string, string> = {
  CHANNEL: 'net-type-channel',
  INBOX: 'net-type-inbox',
  SYSTEM: 'net-type-system',
}

export default function MessageFeed() {
  return (
    <div>
      {feedMessages.map(msg => (
        <div key={msg.id} className="net-message">
          <div className="net-message-header">
            <span className="net-message-from">{msg.from}</span>
            <span className="net-message-arrow">{'\u2192'}</span>
            <span className="net-message-to">{msg.to}</span>
            <span className={`net-message-type ${typeClass[msg.type]}`}>{msg.type}</span>
            <span className="net-message-time">{msg.time}</span>
          </div>
          <div className="net-message-body">
            {msg.fields.map((f, i) => (
              <span key={i}>
                <span className="net-field">{f.label}:</span>{' '}
                <span className="net-value">{f.value}</span>
                {i < msg.fields.length - 1 && <br />}
              </span>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
