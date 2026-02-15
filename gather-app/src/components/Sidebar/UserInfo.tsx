import { useWorkspace } from '../../context/WorkspaceContext'
import { useAuth } from '../../context/AuthContext'

export default function UserInfo() {
  const { dispatch } = useWorkspace()
  const { state: authState, signOut } = useAuth()

  const user = authState.user
  const displayName = user?.name || user?.username || user?.email?.split('@')[0] || 'User'
  const initial = displayName.charAt(0).toUpperCase()

  return (
    <div className="sidebar-footer">
      <div className="sidebar-footer-item" onClick={() => dispatch({ type: 'OPEN_SETTINGS' })}>
        <span>{'\u2699'}</span>
        <span>Settings</span>
      </div>
      <div className="sidebar-footer-item" onClick={signOut}>
        <span>{'\u2192'}</span>
        <span>Sign Out</span>
      </div>
      <div className="sidebar-user">
        <div className="user-avatar avatar-bg-10">{initial}</div>
        <div>
          <div className="user-name">{displayName}</div>
          <div className="user-status">Active</div>
        </div>
      </div>
    </div>
  )
}
