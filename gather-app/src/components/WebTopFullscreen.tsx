import { useEffect } from 'react'
import { useWorkspace } from '../context/WorkspaceContext'

export default function WebTopFullscreen() {
  const { state, dispatch } = useWorkspace()

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && state.webtopOpen) {
        dispatch({ type: 'CLOSE_WEBTOP' })
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [state.webtopOpen, dispatch])

  if (!state.webtopOpen) return null

  return (
    <div className="webtop-fullscreen active">
      <div className="webtop-fs-header">
        <span className="fs-agent-name">{state.webtopAgent}</span>
        <span className="fs-agent-status status-running">
          <span className="status-dot" /> Running
        </span>
        <button
          className="btn btn-secondary btn-sm exit-fs-btn"
          onClick={() => dispatch({ type: 'CLOSE_WEBTOP' })}
        >
          Exit Fullscreen
        </button>
      </div>
      <div className="webtop-fs-body">
        <div className="webtop-fs-placeholder">
          <div className="fs-icon">{'\uD83D\uDDA5'}</div>
          <div className="fs-text">Agent Browser Environment</div>
          <div className="fs-subtext">Streaming WebTop will render here</div>
        </div>
      </div>
    </div>
  )
}
