import { useEffect } from 'react'
import { useWorkspace } from '../../context/WorkspaceContext'
import ProfileSettings from './ProfileSettings'
import NotificationSettings from './NotificationSettings'
import AuthVault from './AuthVault'
import ConnectedServices from './ConnectedServices'
import BillingSettings from './BillingSettings'

const tabs = [
  { id: 'profile', label: 'Profile' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'vault', label: 'Auth Vault' },
  { id: 'services', label: 'Connected Services' },
  { id: 'billing', label: 'Billing' },
]

export default function SettingsView() {
  const { state, dispatch } = useWorkspace()

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && state.settingsOpen) {
        dispatch({ type: 'CLOSE_SETTINGS' })
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [state.settingsOpen, dispatch])

  if (!state.settingsOpen) return null

  return (
    <div className="settings-view active">
      <div className="settings-header">
        <span className="settings-title">Settings</span>
        <button className="settings-close" onClick={() => dispatch({ type: 'CLOSE_SETTINGS' })}>
          &times;
        </button>
      </div>
      <div className="settings-body">
        <nav className="settings-nav">
          {tabs.map(tab => (
            <a
              key={tab.id}
              className={`settings-nav-item ${state.settingsTab === tab.id ? 'active' : ''}`}
              onClick={(e) => {
                e.preventDefault()
                dispatch({ type: 'SET_SETTINGS_TAB', tab: tab.id })
              }}
            >
              {tab.label}
            </a>
          ))}
        </nav>
        <div className="settings-content">
          <div className="settings-section active">
            {state.settingsTab === 'profile' && <ProfileSettings />}
            {state.settingsTab === 'notifications' && <NotificationSettings />}
            {state.settingsTab === 'vault' && <AuthVault />}
            {state.settingsTab === 'services' && <ConnectedServices />}
            {state.settingsTab === 'billing' && <BillingSettings />}
          </div>
        </div>
      </div>
    </div>
  )
}
