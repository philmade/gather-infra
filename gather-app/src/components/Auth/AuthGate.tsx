import { useState, type ReactNode } from 'react'
import { useAuth } from '../../context/AuthContext'
import LoginScreen from './LoginScreen'
import LandingPage from '../Landing/LandingPage'

const previewParam = new URLSearchParams(window.location.search).get('preview')

export default function AuthGate({ children }: { children: ReactNode }) {
  const { state } = useAuth()
  const [showLogin, setShowLogin] = useState(false)

  if (state.isLoading) {
    return (
      <div className="login-screen">
        <div className="login-loading">
          <div className="login-spinner" />
        </div>
      </div>
    )
  }

  // ?preview=landing — show landing page even when logged in
  if (previewParam === 'landing') {
    return <LandingPage onSignIn={() => setShowLogin(true)} />
  }

  if (!state.isAuthenticated) {
    // Pending invite — skip landing, go straight to login with banner
    if (state.pendingInvite) {
      return <LoginScreen inviteBanner={state.pendingInvite} />
    }

    if (showLogin) {
      return <LoginScreen onBack={() => setShowLogin(false)} />
    }
    return <LandingPage onSignIn={() => setShowLogin(true)} />
  }

  return <>{children}</>
}
