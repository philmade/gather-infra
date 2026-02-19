import { useState, type ReactNode } from 'react'
import { useAuth } from '../../context/AuthContext'
import LoginScreen from './LoginScreen'
import LandingPage from '../Landing/LandingPage'

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

  if (!state.isAuthenticated) {
    if (showLogin) {
      return <LoginScreen onBack={() => setShowLogin(false)} />
    }
    return <LandingPage onSignIn={() => setShowLogin(true)} />
  }

  return <>{children}</>
}
