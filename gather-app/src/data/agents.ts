export interface Agent {
  id: string
  name: string
  status: 'running' | 'idle' | 'stopped'
  uptime: string
  lastActive: string
  connectedRepo: string
  tasksDone: number
}

export interface Participant {
  name: string
  avatarClass: string
  avatarLetter: string
  presence: 'online' | 'away' | 'offline'
  isYou?: boolean
}

export const humanParticipants: Participant[] = [
  { name: 'phill', avatarClass: 'avatar-bg-10', avatarLetter: 'P', presence: 'online', isYou: true },
  { name: 'sarah', avatarClass: 'avatar-bg-3', avatarLetter: 'S', presence: 'online' },
  { name: 'marcus', avatarClass: 'avatar-bg-7', avatarLetter: 'M', presence: 'away' },
]

export const agents: Agent[] = [
  {
    id: 'buyclaw',
    name: 'BuyClaw',
    status: 'running',
    uptime: '4h 23m',
    lastActive: '2m ago',
    connectedRepo: 'acme/orders',
    tasksDone: 23,
  },
  {
    id: 'reviewclaw',
    name: 'ReviewClaw',
    status: 'idle',
    uptime: '1h 05m',
    lastActive: '15m ago',
    connectedRepo: 'acme/skills',
    tasksDone: 7,
  },
]
