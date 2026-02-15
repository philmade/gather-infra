/**
 * Tinode client wrapper — adapted from gather-chat/frontend/js/tinode-client.js
 * Thin typed layer around tinode-sdk for React usage.
 */

// tinode-sdk ships as UMD only — named exports work via Vite's CJS interop
import { Tinode } from 'tinode-sdk'

export interface TinodeConfig {
  host: string
  secure: boolean
  apiKey: string
}

export interface ChatMessage {
  seq: number
  from: string
  content: string
  ts: string
  isOwn: boolean
  topic: string
}

export interface ChannelInfo {
  topic: string
  name: string
  unread: number
  isP2P: boolean
  online?: boolean
  isBot?: boolean
  touched?: string
  parent?: string
}

export interface WorkspaceInfo {
  topic: string
  name: string
  slug: string
  owner: string
}

function detectHost(): { host: string; secure: boolean } {
  const hostname = window.location.hostname
  const isProduction = hostname !== 'localhost' && hostname !== '127.0.0.1'
  return {
    host: isProduction ? window.location.host : 'localhost:6060',
    secure: isProduction || window.location.protocol === 'https:',
  }
}

export class TinodeClient {
  private client: InstanceType<typeof Tinode>
  private meTopic: ReturnType<InstanceType<typeof Tinode>['getMeTopic']> | null = null
  private currentTopic: ReturnType<InstanceType<typeof Tinode>['getTopic']> | null = null
  myUserId: string | null = null
  isConnected = false
  isLoggedIn = false

  // Callbacks
  onMessage: ((msg: ChatMessage) => void) | null = null
  onChannelsUpdated: (() => void) | null = null

  constructor(apiKey: string, config?: Omit<TinodeConfig, 'apiKey'>) {
    const hostConfig = config || detectHost()
    this.client = new Tinode({
      appName: 'GatherApp',
      host: hostConfig.host,
      apiKey,
      secure: hostConfig.secure,
      transport: 'ws',
    })

    this.client.onConnect = () => { this.isConnected = true }
    this.client.onDisconnect = () => { this.isConnected = false; this.isLoggedIn = false }
  }

  async connect() {
    return this.client.connect()
  }

  async login(username: string, password: string) {
    await this.client.loginBasic(username, password)
    this.myUserId = this.client.getCurrentUserID()
    this.isLoggedIn = true
  }

  async subscribeToMe() {
    this.meTopic = this.client.getMeTopic()

    const subsLoaded = new Promise<void>((resolve) => {
      this.meTopic!.onSubsUpdated = () => {
        this.onChannelsUpdated?.()
        resolve()
      }
    })

    this.meTopic.onContactUpdate = () => {
      this.onChannelsUpdated?.()
    }

    const query = this.meTopic.startMetaQuery().withSub().withDesc().build()
    await this.meTopic.subscribe(query)

    await Promise.race([subsLoaded, new Promise(r => setTimeout(r, 2000))])
  }

  /** Refresh contacts on an already-subscribed me topic (no re-subscribe) */
  async refreshMe() {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const me = this.meTopic as any
    if (!me?.isSubscribed?.()) {
      return this.subscribeToMe()
    }

    const subsLoaded = new Promise<void>((resolve) => {
      this.meTopic!.onSubsUpdated = () => {
        this.onChannelsUpdated?.()
        resolve()
      }
    })

    const query = this.meTopic!.startMetaQuery().withSub().build()
    me.getMeta(query)

    await Promise.race([subsLoaded, new Promise(r => setTimeout(r, 2000))])
  }

  getChannels(): ChannelInfo[] {
    if (!this.meTopic) return []
    const groups: ChannelInfo[] = []
    const dms: ChannelInfo[] = []

    this.meTopic.contacts((sub: {
      topic?: string
      public?: { fn?: string; bot?: boolean; type?: string }
      seq?: number
      read?: number
      online?: boolean
      touched?: string
      updated?: string
    }) => {
      if (!sub?.topic) return
      const pub = sub.public || (this.client.getTopic(sub.topic)?.public as typeof sub.public)
      const name = pub?.fn || sub.topic
      const unread = (sub.seq || 0) - (sub.read || 0)
      const isP2P = sub.topic.startsWith('usr')
      const isGroup = sub.topic.startsWith('grp')

      // Skip workspace-type topics
      if (pub?.type === 'workspace') return

      const item: ChannelInfo = {
        topic: sub.topic,
        name,
        unread: Math.max(0, unread),
        isP2P,
        online: sub.online,
        isBot: !!pub?.bot || (name ? /bot|agent/i.test(name) : false),
        touched: sub.touched || sub.updated,
      }

      if (isP2P) dms.push(item)
      else if (isGroup) groups.push(item)
    })

    const byTouch = (a: ChannelInfo, b: ChannelInfo) =>
      new Date(b.touched || '').getTime() - new Date(a.touched || '').getTime()
    groups.sort(byTouch)
    dms.sort(byTouch)

    return [...groups, ...dms]
  }

  async subscribeTopic(topicName: string) {
    // Leave current topic
    if (this.currentTopic?.isSubscribed()) {
      await this.currentTopic.leave(false)
    }

    const topic = this.client.getTopic(topicName)
    this.currentTopic = topic

    topic.onData = (data: { seq: number; from: string; content: unknown; ts: string }) => {
      if (!data.from) return
      this.onMessage?.({
        seq: data.seq,
        from: data.from,
        content: extractContent(data.content),
        ts: data.ts,
        isOwn: data.from === this.myUserId,
        topic: topicName,
      })
    }

    const query = topic.startMetaQuery().withLaterData(50).withLaterSub().withDesc().build()
    await topic.subscribe(query)
    return topic
  }

  getCachedMessages(): ChatMessage[] {
    if (!this.currentTopic) return []
    const msgs: ChatMessage[] = []
    this.currentTopic.messages((msg: { seq: number; from: string; content: unknown; ts: string }) => {
      msgs.push({
        seq: msg.seq,
        from: msg.from,
        content: extractContent(msg.content),
        ts: msg.ts,
        isOwn: msg.from === this.myUserId,
        topic: this.currentTopic!.name,
      })
    })
    return msgs
  }

  async sendMessage(text: string) {
    if (!this.currentTopic?.isSubscribed()) {
      throw new Error('Not subscribed to any topic')
    }
    return this.currentTopic.publish({ txt: text }, false)
  }

  noteKeyPress() {
    if (this.currentTopic?.isSubscribed()) {
      this.currentTopic.noteKeyPress()
    }
  }

  getUserName(userId: string): string {
    if (userId === this.myUserId && this.meTopic) {
      return (this.meTopic as { public?: { fn?: string } }).public?.fn || 'Me'
    }
    // Try current topic's user data
    if (this.currentTopic) {
      const user = this.currentTopic.userDesc(userId)
      if (user?.public?.fn) return user.public.fn
    }
    // Try meTopic contacts
    if (this.meTopic) {
      let name: string | null = null
      this.meTopic.contacts((sub: { topic?: string; public?: { fn?: string } }) => {
        if (sub.topic === userId && sub.public?.fn) {
          name = sub.public.fn
        }
      })
      if (name) return name
    }
    return userId
  }

  getTopicMembers(): Array<{ id: string; name: string; online: boolean; isBot: boolean }> {
    if (!this.currentTopic) return []
    const members: Array<{ id: string; name: string; online: boolean; isBot: boolean }> = []
    this.currentTopic.subscribers((sub: { user: string; public?: { fn?: string; bot?: boolean }; online?: boolean }) => {
      const name = sub.public?.fn || sub.user
      members.push({
        id: sub.user,
        name,
        online: sub.online || false,
        isBot: !!sub.public?.bot || /bot|agent/i.test(name),
      })
    })
    return members
  }

  // ── Workspace methods (Tinode-native, from gather-chat) ──

  getWorkspaces(): WorkspaceInfo[] {
    if (!this.meTopic) return []
    const workspaces: WorkspaceInfo[] = []
    this.meTopic.contacts((sub: { topic?: string; public?: Record<string, unknown> }) => {
      if (!sub?.topic?.startsWith('grp')) return
      const pub = sub.public || (this.client.getTopic(sub.topic)?.public as Record<string, unknown>)
      if (pub?.type !== 'workspace') return
      workspaces.push({
        topic: sub.topic,
        name: String(pub.fn || pub.name || sub.topic),
        slug: String(pub.slug || ''),
        owner: String(pub.owner || ''),
      })
    })
    return workspaces
  }

  async createWorkspace(name: string): Promise<string> {
    const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
    const topic = this.client.getTopic('new')

    await topic.subscribe(
      topic.startMetaQuery().withDesc().withSub().build()
    )

    await topic.setMeta({
      desc: {
        public: { fn: name, type: 'workspace', name, slug, owner: this.myUserId },
      },
      tags: ['workspace', `slug:${slug}`],
    })

    const workspaceId = topic.name
    console.log('[Tinode] Created workspace:', workspaceId, name)

    // Create default #general channel
    await this.createChannel(workspaceId, 'general')

    return workspaceId
  }

  async createChannel(workspaceId: string, name: string): Promise<string> {
    const topic = this.client.getTopic('new')

    await topic.subscribe(
      topic.startMetaQuery().withDesc().withSub().build()
    )

    await topic.setMeta({
      desc: {
        public: { fn: name, type: 'channel', name, parent: workspaceId },
        defacs: { auth: 'JRWPS', anon: 'N' },
      },
      tags: ['channel', `parent:${workspaceId}`],
    })

    const channelId = topic.name
    console.log('[Tinode] Created channel:', channelId, `#${name}`, 'in', workspaceId)

    // Send welcome message
    if (topic.isSubscribed()) {
      await topic.publish(
        { txt: `Welcome to #${name}! This is your workspace's default channel.` },
        false,
      )
    }

    return channelId
  }

  disconnect() {
    this.client?.disconnect()
  }
}

function extractContent(content: unknown): string {
  if (!content) return ''
  if (typeof content === 'string') return content
  const c = content as Record<string, unknown>
  if (c.txt) return String(c.txt)
  if (c.text) return String(c.text)
  if (c.content) return extractContent(c.content)
  if (c.message) return String(c.message)
  if (c.body) return String(c.body)
  return JSON.stringify(content)
}
