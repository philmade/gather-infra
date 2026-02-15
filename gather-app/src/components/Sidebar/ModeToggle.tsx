import { useWorkspace } from '../../context/WorkspaceContext'

export default function ModeToggle() {
  const { state, dispatch } = useWorkspace()

  return (
    <div className="mode-toggle">
      <button
        className={`mode-toggle-btn ${state.mode === 'workspace' ? 'active' : ''}`}
        onClick={() => dispatch({ type: 'SET_MODE', mode: 'workspace' })}
      >
        Workspace
      </button>
      <button
        className={`mode-toggle-btn ${state.mode === 'network' ? 'active' : ''}`}
        onClick={() => dispatch({ type: 'SET_MODE', mode: 'network' })}
      >
        Network
      </button>
    </div>
  )
}
