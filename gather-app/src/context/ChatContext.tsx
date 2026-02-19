import { createContext, useContext, useReducer, useEffect, useCallback, useRef, type ReactNode } from 'react'
import { useAuth } from './AuthContext'
import { pb } from '../lib/pocketbase'
import { TinodeClient, type ChatMessage, type ChannelInfo, type WorkspaceInfo } from '../lib/tinode'
import { getClawMessages, sendClawMessage as apiSendClawMessage } from '../lib/api'

interface ChatState {
  connected: boolean
  channels: ChannelInfo[]       // groups + DMs from Tinode
  workspaces: WorkspaceInfo[]
  activeWorkspace: string | null
  activeTopic: string | null
  messages: ChatMessage[]
  clawTopic: string | null      // "claw:{id}" when viewing a claw channel
  clawName: string | null       // display name of the active claw
  clawMessages: ChatMessage[]   // messages from claw REST API
  clawTyping: boolean           // true while waiting for claw LLM response
  error: string | null
}

type ChatAction =
  | { type: 'CONNECTED' }
  | { type: 'DISCONNECTED' }
  | { type: 'SET_CHANNELS'; channels: ChannelInfo[] }
  | { type: 'SET_WORKSPACES'; workspaces: WorkspaceInfo[]; activeWorkspace: string | null }
  | { type: 'SET_TOPIC'; topic: string; messages: ChatMessage[] }
  | { type: 'ADD_MESSAGE'; message: ChatMessage }
  | { type: 'SET_CLAW_TOPIC'; clawId: string; clawName: string; messages: ChatMessage[] }
  | { type: 'ADD_CLAW_MESSAGE'; message: ChatMessage }
  | { type: 'SET_CLAW_TYPING'; typing: boolean }
  | { type: 'CLEAR_CLAW' }
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
      return { ...state, activeTopic: action.topic, messages: action.messages, clawTopic: null, clawName: null, clawMessages: [] }
    case 'ADD_MESSAGE':
      if (action.message.topic !== state.activeTopic) return state
      if (state.messages.some(m => m.seq === action.message.seq)) return state
      return { ...state, messages: [...state.messages, action.message] }
    case 'SET_CLAW_TOPIC':
      return { ...state, clawTopic: `claw:${action.clawId}`, clawName: action.clawName, clawMessages: action.messages, clawTyping: false, activeTopic: null }
    case 'ADD_CLAW_MESSAGE':
      if (state.clawMessages.some(m => m.seq === action.message.seq)) return state
      return { ...state, clawMessages: [...state.clawMessages, action.message] }
    case 'SET_CLAW_TYPING':
      return { ...state, clawTyping: action.typing }
    case 'CLEAR_CLAW':
      return { ...state, clawTopic: null, clawName: null, clawMessages: [], clawTyping: false }
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
  clawTopic: null,
  clawName: null,
  clawMessages: [],
  clawTyping: false,
  error: null,
}

interface ChatContextValue {
  state: ChatState
  selectTopic: (topic: string) => Promise<void>
  selectClawTopic: (clawId: string, clawName?: string) => Promise<void>
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
  const clawPollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const clawIdRef = useRef<string | null>(null)
  const clawSeenIdsRef = useRef<Set<string>>(new Set())
  const clawSendingRef = useRef(false)

  // Clean up claw polling on unmount
  useEffect(() => {
    return () => {
      if (clawPollRef.current) clearInterval(clawPollRef.current)
    }
  }, [])

  // Connect to Tinode after PocketBase auth
  useEffect(() => {
    if (!authState.isAuthenticated) {
      clientRef.current?.disconnect()
      clientRef.current = null
      dispatch({ type: 'RESET' })
      return
    }

    let cancelled = false

    async function init() {
      try {
        // Step 1: Fetch credentials (includes API key) before creating client
        console.log('[Chat] Step 1: Fetching Tinode credentials...')
        const resp = await fetch(pb.baseURL + '/api/tinode/credentials', {
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
        })
        if (cancelled) return

        if (!resp.ok) {
          const body = await resp.text()
          throw new Error(`Tinode credentials: HTTP ${resp.status} — ${body}`)
        }

        const creds = await resp.json() as { login: string; password: string; apiKey: string }
        if (cancelled) return

        // Step 2: Create client with server-provided API key
        console.log('[Chat] Step 2: Connecting to Tinode WebSocket...')
        const client = new TinodeClient(creds.apiKey)
        clientRef.current = client

        client.onMessage = (msg) => {
          if (!cancelled) dispatch({ type: 'ADD_MESSAGE', message: msg })
        }

        client.onChannelsUpdated = () => {
          if (!cancelled) dispatch({ type: 'SET_CHANNELS', channels: client.getChannels() })
        }

        await client.connect()
        if (cancelled) { client.disconnect(); return }
        console.log('[Chat] Step 3: Connected. Logging in as', creds.login)

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
      clientRef.current?.disconnect()
      clientRef.current = null
    }
  }, [authState.isAuthenticated])

  const selectTopic = useCallback(async (topic: string) => {
    // Clear claw polling when switching to a Tinode topic
    if (clawPollRef.current) {
      clearInterval(clawPollRef.current)
      clawPollRef.current = null
    }
    clawIdRef.current = null

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

  const selectClawTopic = useCallback(async (clawId: string, clawName?: string) => {
    // Stop any existing claw polling
    if (clawPollRef.current) {
      clearInterval(clawPollRef.current)
      clawPollRef.current = null
    }
    clawIdRef.current = clawId

    // Track seen message IDs to prevent duplicates
    clawSeenIdsRef.current = new Set()
    let lastTs = ''

    // Fetch initial messages
    try {
      const data = await getClawMessages(clawId)
      const raw = data.messages || []
      for (const m of raw) clawSeenIdsRef.current.add(m.id)
      // Initialize watermark from newest message (API returns newest first)
      if (raw.length > 0) lastTs = raw[0].created

      const msgs: ChatMessage[] = raw.reverse().map((m, i) => ({
        seq: i,
        from: m.author_name,
        content: m.body,
        ts: m.created,
        isOwn: m.author_id.startsWith('user:'),
        topic: `claw:${clawId}`,
      }))
      dispatch({ type: 'SET_CLAW_TOPIC', clawId, clawName: clawName || clawId, messages: msgs })
    } catch (err) {
      console.warn('[Chat] Failed to fetch claw messages:', err)
      dispatch({ type: 'SET_CLAW_TOPIC', clawId, clawName: clawName || clawId, messages: [] })
    }

    // Poll for new messages every 3s
    clawPollRef.current = setInterval(async () => {
      if (clawIdRef.current !== clawId) return
      try {
        const data = await getClawMessages(clawId, lastTs || undefined)
        const newMsgs = (data.messages || []).filter(m => {
          if (clawSeenIdsRef.current.has(m.id)) return false
          // Skip user messages while a send is in flight (prevents echo)
          if (clawSendingRef.current && m.author_id.startsWith('user:')) return false
          return true
        })
        if (newMsgs.length > 0) {
          for (const m of newMsgs) clawSeenIdsRef.current.add(m.id)
          lastTs = newMsgs[0].created
          const msgs: ChatMessage[] = newMsgs.reverse().map((m, i) => ({
            seq: Date.now() + i,
            from: m.author_name,
            content: m.body,
            ts: m.created,
            isOwn: m.author_id.startsWith('user:'),
            topic: `claw:${clawId}`,
          }))
          for (const msg of msgs) {
            dispatch({ type: 'ADD_CLAW_MESSAGE', message: msg })
          }
        }
      } catch { /* silent */ }
    }, 3000)
  }, [])

  const sendMessage = useCallback(async (text: string) => {
    // Route to claw REST if a claw topic is active
    if (clawIdRef.current && state.clawTopic) {
      // Show user's message immediately (optimistic)
      const userMsg: ChatMessage = {
        seq: Date.now(),
        from: 'You',
        content: text,
        ts: new Date().toISOString(),
        isOwn: true,
        topic: state.clawTopic,
      }
      dispatch({ type: 'ADD_CLAW_MESSAGE', message: userMsg })
      dispatch({ type: 'SET_CLAW_TYPING', typing: true })
      clawSendingRef.current = true

      try {
        const data = await apiSendClawMessage(clawIdRef.current, text)
        // Mark both user message and claw reply as seen so poll doesn't duplicate
        if (data.user_message_id) clawSeenIdsRef.current.add(data.user_message_id)
        clawSeenIdsRef.current.add(data.message.id)
        const clawReply: ChatMessage = {
          seq: Date.now() + 1,
          from: data.message.author_name,
          content: data.message.body,
          ts: data.message.created,
          isOwn: false,
          topic: state.clawTopic,
        }
        dispatch({ type: 'SET_CLAW_TYPING', typing: false })
        dispatch({ type: 'ADD_CLAW_MESSAGE', message: clawReply })
      } catch (err) {
        console.warn('[Chat] Failed to send claw message:', err)
        dispatch({ type: 'SET_CLAW_TYPING', typing: false })
      } finally {
        clawSendingRef.current = false
      }
      return
    }

    const client = clientRef.current
    if (!client?.isLoggedIn) return
    await client.sendMessage(text)
  }, [state.clawTopic])

  const getUserName = useCallback((userId: string) => {
    return clientRef.current?.getUserName(userId) ?? userId
  }, [])

  const getMembers = useCallback(() => {
    return clientRef.current?.getTopicMembers() ?? []
  }, [])

  const refreshChannels = useCallback(async () => {
    const client = clientRef.current
    if (!client?.isLoggedIn) return

    await client.refreshMe()

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
    <ChatContext.Provider value={{ state, selectTopic, selectClawTopic, sendMessage, createWorkspace, createChannel, getUserName, getMembers, myUserId }}>
      {children}
    </ChatContext.Provider>
  )
}

export function useChat() {
  const ctx = useContext(ChatContext)
  if (!ctx) throw new Error('useChat must be used within ChatProvider')
  return ctx
}
