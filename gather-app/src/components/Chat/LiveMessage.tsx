import { useMemo } from 'react'
import { marked } from 'marked'
import { useChat } from '../../context/ChatContext'
import type { ChatMessage } from '../../lib/tinode'

// Configure marked for chat: no async, breaks enabled
marked.setOptions({ async: false, breaks: true })

function formatTime(ts: string): string {
  const d = new Date(ts)
  return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', hour12: true })
}

function hashColor(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = ((h << 5) - h + s.charCodeAt(i)) | 0
  return (Math.abs(h) % 10) + 1
}

export default function LiveMessage({ msg }: { msg: ChatMessage }) {
  const { getUserName } = useChat()
  const name = getUserName(msg.from)
  const initial = name.charAt(0).toUpperCase()
  const colorClass = `avatar-bg-${hashColor(msg.from)}`

  const html = useMemo(() => {
    if (msg.isOwn) return null // user messages stay plain text
    return marked.parse(msg.content) as string
  }, [msg.content, msg.isOwn])

  return (
    <div className="message">
      <div className={`message-avatar ${colorClass}`}>
        {initial}
      </div>
      <div className="message-content">
        <div className="message-header">
          <span className="message-author">{name}</span>
          <span className="message-time">{formatTime(msg.ts)}</span>
        </div>
        {html
          ? <div className="message-text markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
          : <div className="message-text">{msg.content}</div>
        }
      </div>
    </div>
  )
}
