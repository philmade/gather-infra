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

type EventItem = NonNullable<ChatMessage['events']>[number]

// Group consecutive events by author, interleaving narration text events
// between tool call groups for readability.
interface AuthorGroup {
  author: string
  items: Array<{ kind: 'call'; call: EventItem; result?: EventItem } | { kind: 'narration'; text: string }>
}

function groupEventsByAuthor(events: NonNullable<ChatMessage['events']>): AuthorGroup[] {
  // Index tool results by tool_id for pairing
  const resultsByID = new Map<string, EventItem>()
  for (const e of events) {
    if (e.type === 'tool_result' && e.tool_id) {
      resultsByID.set(e.tool_id, e)
    }
  }

  const groups: AuthorGroup[] = []
  let current: AuthorGroup | null = null

  for (const evt of events) {
    // Skip tool_result events — they're paired with their call
    if (evt.type === 'tool_result') continue

    const author = evt.author || 'agent'

    // Start a new group when author changes
    if (!current || current.author !== author) {
      current = { author, items: [] }
      groups.push(current)
    }

    if (evt.type === 'tool_call') {
      const result = evt.tool_id ? resultsByID.get(evt.tool_id) : undefined
      current.items.push({ kind: 'call', call: evt, result })
    } else if (evt.type === 'text' && evt.text) {
      // Narration text interleaved with tool calls — show inline.
      // Skip if this is the final response text (last text event with no
      // tool calls after it) — that gets rendered as the message body.
      current.items.push({ kind: 'narration', text: evt.text })
    }
  }

  // Drop the last text-only narration in the last group — it's the final
  // response that LiveMessage renders as markdown body text.
  if (groups.length > 0) {
    const lastGroup = groups[groups.length - 1]
    while (
      lastGroup.items.length > 0 &&
      lastGroup.items[lastGroup.items.length - 1].kind === 'narration'
    ) {
      lastGroup.items.pop()
    }
    if (lastGroup.items.length === 0) groups.pop()
  }

  return groups
}

function ToolCallEvents({ events }: { events: ChatMessage['events'] }) {
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  if (!events || events.length === 0) return null

  const groups = groupEventsByAuthor(events)
  if (groups.length === 0) return null

  // Only show author labels when there are multiple distinct authors
  const distinctAuthors = new Set(groups.map(g => g.author))
  const showLabels = distinctAuthors.size > 1

  const toggle = (i: number) => {
    setExpanded(prev => {
      const next = new Set(prev)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })
  }

  let globalIdx = 0

  return (
    <div className="adk-events">
      {groups.map((group, gi) => (
        <div key={gi} className="adk-author-group">
          {showLabels && (
            <div className="adk-author-label">{group.author}</div>
          )}
          {group.items.map((item) => {
            if (item.kind === 'narration') {
              return (
                <div key={`n-${gi}-${globalIdx++}`} className="adk-event-narration">
                  {item.text}
                </div>
              )
            }
            const idx = globalIdx++
            const isExpanded = expanded.has(idx)
            const callItems = group.items.filter(it => it.kind === 'call')
            const isLastInGroup = item === callItems[callItems.length - 1]
            const prefix = isLastInGroup ? '\u2514' : '\u251C'

            return (
              <div key={idx} className="adk-event-row" onClick={() => toggle(idx)}>
                <span className="adk-event-prefix">{prefix}</span>
                <span className="adk-event-tool">{item.call.tool_name || 'tool'}</span>
                {item.result && (
                  <span className="adk-event-result">
                    {' \u2192 '}{truncateResult(item.result.result)}
                  </span>
                )}
                {isExpanded && item.call.tool_args ? (
                  <pre className="adk-event-args">{JSON.stringify(item.call.tool_args, null, 2)}</pre>
                ) : null}
              </div>
            )
          })}
        </div>
      ))}
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
