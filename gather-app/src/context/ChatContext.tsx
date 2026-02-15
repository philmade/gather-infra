import { createContext, useContext, useReducer, useEffect, useCallback, useRef, type ReactNode } from 'react'
import { useAuth } from './AuthContext'
import { pb } from '../lib/pocketbase'
import { TinodeClient, type ChatMessage, type ChannelInfo, type WorkspaceInfo } from '../lib/tinode'

interface ChatState {
  connected: boolean
  channels: ChannelInfo[]       // groups + DMs from Tinode
  workspaces: WorkspaceInfo[]
  activeWorkspace: string | null
  activeTopic: string | null
  messages: ChatMessage[]
  error: string | null
}

type ChatAction =
  | { type: 'CONNECTED' }
  | { type: 'DISCONNECTED' }
  | { type: 'SET_CHANNELS'; channels: ChannelInfo[] }
  | { type: 'SET_WORKSPACES'; workspaces: WorkspaceInfo[]; activeWorkspace: string | null }
  | { type: 'SET_TOPIC'; topic: string; messages: ChatMessage[] }
  | { type: 'ADD_MESSAGE'; message: ChatMessage }
  | { type: 'SET_ERROR'; error: string }
  | { type: 'RESET' }

function chatReducer(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case 'CONNECTED':
      return { ...state, connected: true, error: null }
    case 'DISCONNECTED':
      return { ...state, connected: false }
    case 'SET_CHANNELS':
      return { ...state, channels: action.channels }
    case 'SET_WORKSPACES':
      return { ...state, workspaces: action.workspaces, activeWorkspace: action.activeWorkspace }
    case 'SET_TOPIC':
      return { ...state, activeTopic: action.topic, messages: action.messages }
    case 'ADD_MESSAGE':
      if (action.message.topic !== state.activeTopic) return state
      if (state.messages.some(m => m.seq === action.message.seq)) return state
      return { ...state, messages: [...state.messages, action.message] }
    case 'SET_ERROR':
      return { ...state, error: action.error }
    case 'RESET':
      return initialState
  }
}

const initialState: ChatState = {
  connected: false,
  channels: [],
  workspaces: [],
  activeWorkspace: null,
  activeTopic: null,
  messages: [],
  error: null,
}

interface ChatContextValue {
  state: ChatState
  selectTopic: (topic: string) => Promise<void>
  sendMessage: (text: string) => Promise<void>
  createWorkspace: (name: string) => Promise<void>
  createChannel: (name: string) => Promise<void>
  getUserName: (userId: string) => string
  getMembers: () => Array<{ id: string; name: string; online: boolean; isBot: boolean }>
  myUserId: string | null
}

const ChatContext = createContext<ChatContextValue | null>(null)

export function ChatProvider({ children }: { children: ReactNode }) {
  const { state: authState } = useAuth()
  const [state, dispatch] = useReducer(chatReducer, initialState)
  const clientRef = useRef<TinodeClient | null>(null)

  // Connect to Tinode after PocketBase auth
  useEffect(() => {
    if (!authState.isAuthenticated) {
      clientRef.current?.disconnect()
      clientRef.current = null
      dispatch({ type: 'RESET' })
      return
    }

    let cancelled = false
    const client = new TinodeClient()
    clientRef.current = client

    client.onMessage = (msg) => {
      if (!cancelled) dispatch({ type: 'ADD_MESSAGE', message: msg })
    }

    client.onChannelsUpdated = () => {
      if (!cancelled) dispatch({ type: 'SET_CHANNELS', channels: client.getChannels() })
    }

    async function init() {
      try {
        console.log('[Chat] Step 1: Connecting to Tinode WebSocket...')
        await client.connect()
        if (cancelled) { client.disconnect(); return }
        console.log('[Chat] Step 2: Connected. Fetching credentials...')

        const resp = await fetch(pb.baseURL + '/api/tinode/credentials', {
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
        })
        if (cancelled) { client.disconnect(); return }

        if (!resp.ok) {
          const body = await resp.text()
          throw new Error(`Tinode credentials: HTTP ${resp.status} — ${body}`)
        }

        const creds = await resp.json() as { login: string; password: string }
        console.log('[Chat] Step 3: Got credentials, logging in as', creds.login)

        await client.login(creds.login, creds.password)
        if (cancelled) { client.disconnect(); return }
        console.log('[Chat] Step 4: Logged in. Subscribing to me...')

        dispatch({ type: 'CONNECTED' })

        await client.subscribeToMe()
        if (cancelled) { client.disconnect(); return }

        // Load workspaces and channels
        const workspaces = client.getWorkspaces()
        const activeWorkspace = workspaces[0]?.topic ?? null
        dispatch({ type: 'SET_WORKSPACES', workspaces, activeWorkspace })

        const channels = client.getChannels()
        console.log('[Chat] Step 5: Found', workspaces.length, 'workspaces,', channels.length, 'channels')
        dispatch({ type: 'SET_CHANNELS', channels })

      } catch (err) {
        if (!cancelled) {
          let msg: string
          if (err instanceof Error) {
            msg = err.message
          } else if (err instanceof Event) {
            msg = 'WebSocket connection failed'
          } else {
            msg = String(err)
          }
          console.error('[Chat] FAILED:', msg, err)
          dispatch({ type: 'SET_ERROR', error: msg })
        }
      }
    }

    init()

    return () => {
      cancelled = true
      client.disconnect()
    }
  }, [authState.isAuthenticated])

  const selectTopic = useCallback(async (topic: string) => {
    const client = clientRef.current
    if (!client?.isLoggedIn) return

    try {
      await client.subscribeTopic(topic)
      const messages = client.getCachedMessages()
      dispatch({ type: 'SET_TOPIC', topic, messages })
    } catch (err) {
      console.warn('[Chat] Subscribe failed:', err)
    }
  }, [])

  const sendMessage = useCallback(async (text: string) => {
    const client = clientRef.current
    if (!client?.isLoggedIn) return
    await client.sendMessage(text)
  }, [])

  const getUserName = useCallback((userId: string) => {
    return clientRef.current?.getUserName(userId) ?? userId
  }, [])

  const getMembers = useCallback(() => {
    return clientRef.current?.getTopicMembers() ?? []
  }, [])

  const refreshChannels = useCallback(async () => {
    const client = clientRef.current
    if (!client?.isLoggedIn) return

    await client.subscribeToMe()

    const workspaces = client.getWorkspaces()
    const activeWorkspace = workspaces[0]?.topic ?? null
    dispatch({ type: 'SET_WORKSPACES', workspaces, activeWorkspace })

    const channels = client.getChannels()
    dispatch({ type: 'SET_CHANNELS', channels })
    return channels
  }, [])

  const createWorkspace = useCallback(async (name: string) => {
    const client = clientRef.current
    if (!client?.isLoggedIn) throw new Error('Not connected')

    await client.createWorkspace(name)
    const channels = await refreshChannels()

    // Auto-select first channel
    if (channels && channels.length > 0) {
      await client.subscribeTopic(channels[0].topic)
      const messages = client.getCachedMessages()
      dispatch({ type: 'SET_TOPIC', topic: channels[0].topic, messages })
    }
  }, [refreshChannels])

  const createChannel = useCallback(async (name: string) => {
    const client = clientRef.current
    if (!client?.isLoggedIn || !state.activeWorkspace) throw new Error('Not connected')

    const channelId = await client.createChannel(state.activeWorkspace, name)
    await refreshChannels()

    // Auto-select the new channel
    await client.subscribeTopic(channelId)
    const messages = client.getCachedMessages()
    dispatch({ type: 'SET_TOPIC', topic: channelId, messages })
  }, [state.activeWorkspace, refreshChannels])

  const myUserId = clientRef.current?.myUserId ?? null

  return (
    <ChatContext.Provider value={{ state, selectTopic, sendMessage, createWorkspace, createChannel, getUserName, getMembers, myUserId }}>
      {children}
    </ChatContext.Provider>
  )
}

export function useChat() {
  const ctx = useContext(ChatContext)
  if (!ctx) throw new Error('useChat must be used within ChatProvider')
  return ctx
}
