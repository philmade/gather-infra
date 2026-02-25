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
import { useWorkspace } from './context/WorkspaceContext'
import { listClaws } from './lib/api'
import { useEffect, useRef, useState } from 'react'
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

  return <>{children}</>
}

function StripeToast({ message, onDismiss }: { message: string; onDismiss: () => void }) {
  useEffect(() => {
    const t = setTimeout(onDismiss, 5000)
    return () => clearTimeout(t)
  }, [onDismiss])

  return (
    <div style={{
      position: 'fixed', top: 16, right: 16, zIndex: 10000,
      padding: '12px 20px', borderRadius: 8,
      background: 'var(--bg-secondary)', border: '1px solid var(--border)',
      boxShadow: '0 4px 12px rgba(0,0,0,0.3)', fontSize: '0.85rem',
      color: 'var(--text-primary)', cursor: 'pointer',
    }} onClick={onDismiss}>
      {message}
    </div>
  )
}

function WorkspaceLayout() {
  const { state, dispatch } = useWorkspace()
  const [stripeToast, setStripeToast] = useState<string | null>(null)
  const checkedClaws = useRef(false)

  // Auto-open deploy modal for new users with zero claws (or ?preview=deploy)
  useEffect(() => {
    if (checkedClaws.current) return
    checkedClaws.current = true

    const preview = new URLSearchParams(window.location.search).get('preview')
    if (preview === 'deploy') {
      dispatch({ type: 'OPEN_DEPLOY' })
      return
    }

    listClaws()
      .then(data => {
        if (!data.claws || data.claws.length === 0) {
          dispatch({ type: 'OPEN_DEPLOY' })
        }
      })
      .catch(() => {})
  }, [dispatch])

  // Handle Stripe redirect return
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const stripeResult = params.get('stripe')
    if (!stripeResult) return

    if (stripeResult === 'success') {
      setStripeToast('Payment successful! Your claw is now active.')
    } else if (stripeResult === 'cancel') {
      setStripeToast('Checkout cancelled. You can upgrade later from the detail panel.')
    }

    // Clean up URL params
    params.delete('stripe')
    params.delete('claw')
    const clean = params.toString()
    const newUrl = window.location.pathname + (clean ? '?' + clean : '')
    window.history.replaceState({}, '', newUrl)
  }, [])

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
      {stripeToast && <StripeToast message={stripeToast} onDismiss={() => setStripeToast(null)} />}
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
