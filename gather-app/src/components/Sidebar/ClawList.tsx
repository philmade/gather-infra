import { useEffect, useState } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import { useChat } from '../../context/ChatContext'
import { listClaws, type ClawDeployment } from '../../lib/api'

const statusDot: Record<string, string> = {
  running: 'online',
  queued: 'idle',
  provisioning: 'idle',
  stopped: 'offline',
  failed: 'offline',
}

export default function ClawList() {
  const { state, dispatch } = useWorkspace()
  const { selectClawTopic } = useChat()
  const [claws, setClaws] = useState<ClawDeployment[]>([])

  useEffect(() => {
    async function fetch() {
      try {
        const data = await listClaws()
        setClaws(data.claws || [])
      } catch { /* silent */ }
    }
    fetch()
    const interval = setInterval(fetch, 15000)
    return () => clearInterval(interval)
  }, [])

  if (claws.length === 0) return null

  return (
    <>
      <div className="sidebar-section-label" style={{ marginTop: 'var(--space-md)' }}>
        Claws
      </div>
      {claws.map(claw => {
        const topic = `claw:${claw.id}`
        const isActive = state.activeChannel === topic
        const dotClass = statusDot[claw.status] || 'offline'
        return (
          <div
            key={claw.id}
            className={`sidebar-item ${isActive ? 'active' : ''}`}
            onClick={() => {
              dispatch({ type: 'SET_MODE', mode: 'workspace' })
              dispatch({ type: 'SET_CHANNEL', channel: topic })
              selectClawTopic(claw.id, claw.name)
              dispatch({ type: 'SHOW_AGENT_DETAIL', agentId: claw.id })
            }}
          >
            <div className={`dm-avatar avatar-bg-${hashColor(claw.id)}`}>
              C
              <span className={`presence-dot presence-${dotClass}`} />
            </div>
            <span className="item-name">{claw.name}</span>
            <span className="bot-indicator">CLAW</span>
          </div>
        )
      })}
    </>
  )
}

function hashColor(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = ((h << 5) - h + s.charCodeAt(i)) | 0
  return (Math.abs(h) % 10) + 1
}
