import { useMemo, useState } from 'react'
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

function truncateResult(val: unknown, max = 80): string {
  if (val == null) return ''
  const s = typeof val === 'string' ? val : JSON.stringify(val)
  return s.length > max ? s.slice(0, max) + '...' : s
}

function ToolCallEvents({ events }: { events: ChatMessage['events'] }) {
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  if (!events || events.length === 0) return null

  // Filter to only tool calls and tool results (skip text events)
  const toolEvents = events.filter(e => e.type === 'tool_call' || e.type === 'tool_result')
  if (toolEvents.length === 0) return null

  // Pair tool_calls with their results by tool_id
  const resultsByID = new Map<string, typeof toolEvents[0]>()
  for (const e of toolEvents) {
    if (e.type === 'tool_result' && e.tool_id) {
      resultsByID.set(e.tool_id, e)
    }
  }

  const calls = toolEvents.filter(e => e.type === 'tool_call')

  const toggle = (i: number) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })
  }

  return (
    <div className="adk-events">
      {calls.map((call, i) => {
        const result = call.tool_id ? resultsByID.get(call.tool_id) : undefined
        const isExpanded = expanded.has(i)
        const isLast = i === calls.length - 1
        const prefix = isLast ? '\u2514' : '\u251C'

        return (
          <div key={i} className="adk-event-row" onClick={() => toggle(i)}>
            <span className="adk-event-prefix">{prefix}</span>
            <span className="adk-event-tool">{call.tool_name || 'tool'}</span>
            {result && (
              <span className="adk-event-result">
                {' \u2192 '}{truncateResult(result.result)}
              </span>
            )}
            {isExpanded && call.tool_args && (
              <pre className="adk-event-args">{JSON.stringify(call.tool_args, null, 2)}</pre>
            )}
          </div>
        )
      })}
    </div>
  )
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
        <ToolCallEvents events={msg.events} />
        {html
          ? <div className="message-text markdown-body" dangerouslySetInnerHTML={{ __html: html }} />
          : <div className="message-text">{msg.content}</div>
        }
      </div>
    </div>
  )
}
