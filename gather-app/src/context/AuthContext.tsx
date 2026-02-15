import { createContext, useContext, useReducer, useEffect, useCallback, type ReactNode } from 'react'
import { pb } from '../lib/pocketbase'
import type { RecordModel } from 'pocketbase'

interface AuthState {
  user: RecordModel | null
  isAuthenticated: boolean
  isLoading: boolean
  error: string | null
}

type AuthAction =
  | { type: 'AUTH_START' }
  | { type: 'AUTH_SUCCESS'; user: RecordModel }
  | { type: 'AUTH_ERROR'; error: string }
  | { type: 'AUTH_LOGOUT' }
  | { type: 'AUTH_LOADED' }
  | { type: 'CLEAR_ERROR' }

interface AuthContextValue {
  state: AuthState
  signInWithEmail: (email: string, password: string) => Promise<void>
  signUpWithEmail: (email: string, password: string, name: string) => Promise<void>
  signInWithGoogle: () => Promise<void>
  signOut: () => void
  clearError: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

function authReducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case 'AUTH_START':
      return { ...state, isLoading: true, error: null }
    case 'AUTH_SUCCESS':
      return { user: action.user, isAuthenticated: true, isLoading: false, error: null }
    case 'AUTH_ERROR':
      return { ...state, isLoading: false, error: action.error }
    case 'AUTH_LOGOUT':
      return { user: null, isAuthenticated: false, isLoading: false, error: null }
    case 'AUTH_LOADED':
      return { ...state, isLoading: false }
    case 'CLEAR_ERROR':
      return { ...state, error: null }
  }
}

const initialState: AuthState = {
  user: null,
  isAuthenticated: false,
  isLoading: true,
  error: null,
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(authReducer, initialState)

  // Restore session or handle OAuth callback on mount
  useEffect(() => {
    async function init() {
      const params = new URLSearchParams(window.location.search)
      const code = params.get('code')
      const state = params.get('state')

      if (code && state) {
        await handleOAuthCallback(code, state)
      } else {
        checkExistingAuth()
      }
    }
    init()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  function checkExistingAuth() {
    if (pb.authStore.isValid && pb.authStore.record) {
      dispatch({ type: 'AUTH_SUCCESS', user: pb.authStore.record as RecordModel })
    } else {
      dispatch({ type: 'AUTH_LOADED' })
    }
  }

  async function handleOAuthCallback(code: string, stateParam: string) {
    const storedState = localStorage.getItem('pb_oauth_state')
    const codeVerifier = localStorage.getItem('pb_oauth_verifier')
    const provider = localStorage.getItem('pb_oauth_provider') || 'google'

    if (!codeVerifier) {
      dispatch({ type: 'AUTH_ERROR', error: 'OAuth session expired - please try again' })
      return
    }

    if (stateParam !== storedState) {
      dispatch({ type: 'AUTH_ERROR', error: 'OAuth state mismatch - please try again' })
      return
    }

    dispatch({ type: 'AUTH_START' })

    try {
      const redirectUrl = window.location.origin + window.location.pathname
      const authData = await pb.collection('users').authWithOAuth2Code(
        provider,
        code,
        codeVerifier,
        redirectUrl,
        { emailVisibility: true }
      )

      // Clean up
      localStorage.removeItem('pb_oauth_verifier')
      localStorage.removeItem('pb_oauth_state')
      localStorage.removeItem('pb_oauth_provider')
      window.history.replaceState({}, document.title, window.location.pathname)

      dispatch({ type: 'AUTH_SUCCESS', user: authData.record })
    } catch (err) {
      localStorage.removeItem('pb_oauth_verifier')
      localStorage.removeItem('pb_oauth_state')
      localStorage.removeItem('pb_oauth_provider')
      dispatch({ type: 'AUTH_ERROR', error: err instanceof Error ? err.message : 'OAuth failed' })
    }
  }

  const signInWithEmail = useCallback(async (email: string, password: string) => {
    dispatch({ type: 'AUTH_START' })
    try {
      const authData = await pb.collection('users').authWithPassword(email, password)
      dispatch({ type: 'AUTH_SUCCESS', user: authData.record })
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      let message: string
      if (raw.includes('fetch') || raw.includes('network') || raw.includes('ECONNREFUSED')) {
        message = 'Cannot reach PocketBase at ' + pb.baseURL + ' — is it running?'
      } else if (err && typeof err === 'object' && 'status' in err && (err as { status: number }).status === 400) {
        message = 'Invalid email or password'
      } else {
        message = raw
      }
      dispatch({ type: 'AUTH_ERROR', error: message })
    }
  }, [])

  const signUpWithEmail = useCallback(async (email: string, password: string, name: string) => {
    dispatch({ type: 'AUTH_START' })
    try {
      await pb.collection('users').create({
        email,
        password,
        passwordConfirm: password,
        name,
      })
      const authData = await pb.collection('users').authWithPassword(email, password)
      dispatch({ type: 'AUTH_SUCCESS', user: authData.record })
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      let message: string
      if (raw.includes('fetch') || raw.includes('network') || raw.includes('ECONNREFUSED')) {
        message = 'Cannot reach PocketBase at ' + pb.baseURL + ' — is it running?'
      } else if (err && typeof err === 'object' && 'data' in err) {
        const data = (err as { data: { data?: Record<string, { code?: string }> } }).data?.data
        if (data?.email?.code === 'validation_invalid_email') {
          message = 'Please enter a valid email address'
        } else if (data?.email?.code === 'validation_not_unique') {
          message = 'This email is already registered. Try signing in instead.'
        } else if (data?.password?.code) {
          message = 'Password must be at least 8 characters'
        } else {
          message = 'Sign up failed'
        }
      } else {
        message = raw || 'Sign up failed'
      }
      dispatch({ type: 'AUTH_ERROR', error: message })
    }
  }, [])

  const signInWithGoogle = useCallback(async () => {
    dispatch({ type: 'AUTH_START' })
    try {
      const authMethods = await pb.collection('users').listAuthMethods()
      const googleProvider = authMethods.oauth2?.providers?.find(
        (p: { name: string }) => p.name === 'google'
      )

      if (!googleProvider) {
        dispatch({ type: 'AUTH_ERROR', error: 'Google OAuth is not configured in PocketBase' })
        return
      }

      localStorage.setItem('pb_oauth_verifier', googleProvider.codeVerifier)
      localStorage.setItem('pb_oauth_state', googleProvider.state)
      localStorage.setItem('pb_oauth_provider', 'google')

      const redirectUrl = window.location.origin + window.location.pathname
      window.location.href = googleProvider.authURL + encodeURIComponent(redirectUrl)
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      const message = raw.includes('fetch') || raw.includes('network') || raw.includes('ECONNREFUSED')
        ? 'Cannot reach PocketBase at ' + pb.baseURL + ' — is it running?'
        : raw
      dispatch({ type: 'AUTH_ERROR', error: message })
    }
  }, [])

  const signOut = useCallback(() => {
    pb.authStore.clear()
    localStorage.removeItem('tinode_credentials')
    dispatch({ type: 'AUTH_LOGOUT' })
  }, [])

  const clearError = useCallback(() => {
    dispatch({ type: 'CLEAR_ERROR' })
  }, [])

  return (
    <AuthContext.Provider value={{ state, signInWithEmail, signUpWithEmail, signInWithGoogle, signOut, clearError }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
