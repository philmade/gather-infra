import { AuthProvider } from './context/AuthContext'
import AuthGate from './components/Auth/AuthGate'
import { ChatProvider, useChat } from './context/ChatContext'
import { WorkspaceProvider } from './context/WorkspaceContext'
import Sidebar from './components/Sidebar/Sidebar'
import ChatArea from './components/Chat/ChatArea'
import NetworkView from './components/Network/NetworkView'
import DetailPanel from './components/Detail/DetailPanel'
import WebTopFullscreen from './components/WebTopFullscreen'
import DeployAgentModal from './components/DeployModal/DeployAgentModal'
import SettingsView from './components/Settings/SettingsView'
import WorkspaceOnboarding from './components/Onboarding/WorkspaceOnboarding'
import { useWorkspace } from './context/WorkspaceContext'
import type { ReactNode } from 'react'

function ChatGate({ children }: { children: ReactNode }) {
  const { state } = useChat()

  // Still connecting — show loading (unless there's an error)
  if (!state.connected) {
    return (
      <div className="login-screen">
        {state.error ? (
          <div className="login-card">
            <div className="login-error">{state.error}</div>
            <p style={{ color: 'var(--text-muted)', fontSize: '0.85rem', textAlign: 'center' }}>
              Could not connect to chat. Try refreshing the page.
            </p>
          </div>
        ) : (
          <div className="login-loading">
            <div className="login-spinner" />
          </div>
        )}
      </div>
    )
  }

  // Connected but no workspaces — onboarding
  if (state.workspaces.length === 0) {
    return <WorkspaceOnboarding />
  }

  return <>{children}</>
}

function WorkspaceLayout() {
  const { state } = useWorkspace()

  const workspaceClass = [
    'workspace',
    state.mode === 'network' ? 'mode-network' : '',
    !state.detailOpen ? 'detail-collapsed' : '',
  ].filter(Boolean).join(' ')

  return (
    <>
      <div className={workspaceClass}>
        <Sidebar />
        <ChatArea />
        <NetworkView />
        {state.detailOpen && <DetailPanel />}
      </div>
      <WebTopFullscreen />
      <DeployAgentModal />
      <SettingsView />
    </>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <AuthGate>
        <ChatProvider>
          <ChatGate>
            <WorkspaceProvider>
              <WorkspaceLayout />
            </WorkspaceProvider>
          </ChatGate>
        </ChatProvider>
      </AuthGate>
    </AuthProvider>
  )
}
