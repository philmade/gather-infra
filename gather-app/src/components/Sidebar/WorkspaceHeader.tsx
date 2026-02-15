import { useChat } from '../../context/ChatContext'

export default function WorkspaceHeader() {
  const { state } = useChat()

  const activeWs = state.workspaces.find(w => w.topic === state.activeWorkspace)
  const name = activeWs?.name ?? 'Workspace'

  return (
    <div className="sidebar-header">
      <img src="/assets/logo.svg" alt="Gather" className="workspace-logo" />
      <span className="workspace-name">{name}</span>
      <span className="dropdown-icon">{'\u25BE'}</span>
    </div>
  )
}
