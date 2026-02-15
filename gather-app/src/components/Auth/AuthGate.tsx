import type { ReactNode } from 'react'
import { useAuth } from '../../context/AuthContext'
import LoginScreen from './LoginScreen'

export default function AuthGate({ children }: { children: ReactNode }) {
  const { state } = useAuth()

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
    return <LoginScreen />
  }

  return <>{children}</>
}
