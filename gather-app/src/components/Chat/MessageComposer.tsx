import { useState, useRef, type KeyboardEvent } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'
import { channels, directMessages } from '../../data/channels'

export default function MessageComposer() {
  const { state } = useWorkspace()
  const { state: chatState, sendMessage } = useChat()
  const [text, setText] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const ch = state.activeChannel
  const useLive = chatState.connected && chatState.activeTopic != null

  let placeholder: string
  if (useLive) {
    const channel = chatState.channels.find(c => c.topic === chatState.activeTopic)
    placeholder = channel?.isP2P
      ? `Message ${channel.name}`
      : `Message #${channel?.name ?? chatState.activeTopic}`
  } else {
    const isDM = ch.startsWith('dm-')
    if (isDM) {
      const dm = directMessages.find(d => d.id === ch)
      placeholder = `Message ${dm?.displayName ?? ch.replace('dm-', '')}`
    } else {
      const channel = channels.find(c => c.id === ch)
      placeholder = `Message #${channel?.name ?? ch}`
    }
  }

  async function handleSend() {
    const trimmed = text.trim()
    if (!trimmed || !useLive) return
    setText('')
    try {
      await sendMessage(trimmed)
    } catch (err) {
      console.warn('[Composer] Send failed:', err)
    }
    textareaRef.current?.focus()
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <div className="message-composer">
      <div className="composer-box">
        <textarea
          ref={textareaRef}
          className="composer-input"
          placeholder={placeholder}
          rows={1}
          value={text}
          onChange={e => setText(e.target.value)}
          onKeyDown={handleKeyDown}
        />
        <div className="composer-toolbar">
          <button className="toolbar-btn" title="Attach file">{'\uD83D\uDCCE'}</button>
          <button className="toolbar-btn" title="Emoji">{'\uD83D\uDE42'}</button>
          <button className="toolbar-btn" title="Mention">@</button>
          <button
            className="send-btn"
            onClick={handleSend}
            disabled={!text.trim() || !useLive}
          >
            Send
          </button>
        </div>
      </div>
    </div>
  )
}
