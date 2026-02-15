declare module 'tinode-sdk' {
  class Tinode {
    constructor(config: {
      appName: string
      host: string
      apiKey: string
      secure: boolean
      transport: string
    })
    onConnect: (() => void) | null
    onDisconnect: (() => void) | null
    connect(): Promise<void>
    loginBasic(username: string, password: string): Promise<void>
    getCurrentUserID(): string
    getMeTopic(): MeTopic
    getTopic(name: string): Topic
    disconnect(): void
  }

  interface MeTopic {
    public?: { fn?: string }
    onSubsUpdated: (() => void) | null
    onContactUpdate: (() => void) | null
    startMetaQuery(): MetaQueryBuilder
    subscribe(query: unknown): Promise<void>
    contacts(callback: (sub: any) => void): void
  }

  interface Topic {
    name: string
    public?: unknown
    onData: ((data: any) => void) | null
    startMetaQuery(): MetaQueryBuilder
    subscribe(query: unknown): Promise<void>
    leave(unsub: boolean): Promise<void>
    isSubscribed(): boolean
    publish(content: unknown, noEcho: boolean): Promise<void>
    setMeta(meta: { desc?: { public?: unknown; private?: unknown; defacs?: { auth?: string; anon?: string } }; tags?: string[] }): Promise<void>
    noteKeyPress(): void
    messages(callback: (msg: any) => void): void
    userDesc(userId: string): { public?: { fn?: string } } | undefined
    subscribers(callback: (sub: any) => void): void
  }

  interface MetaQueryBuilder {
    withSub(): MetaQueryBuilder
    withDesc(): MetaQueryBuilder
    withLaterData(limit: number): MetaQueryBuilder
    withLaterSub(): MetaQueryBuilder
    build(): unknown
  }

  export { Tinode }
}
