export interface VaultEntry {
  key: string
  maskedValue: string
  scope: string[]
}

export interface ConnectedService {
  icon: string
  name: string
  status: string
  connected: boolean
}

export interface NotificationSetting {
  id: string
  label: string
  description: string
  enabled: boolean
}

export const vaultEntries: VaultEntry[] = [
  { key: 'GITHUB_TOKEN', maskedValue: 'ghp_****...3kF9', scope: ['BuyClaw', 'ReviewClaw'] },
  { key: 'SHOPIFY_KEY', maskedValue: 'sk_****...x7Qm', scope: ['BuyClaw'] },
  { key: 'OPENAI_API_KEY', maskedValue: 'sk-****...9pLz', scope: ['All Claws'] },
]

export const connectedServices: ConnectedService[] = [
  { icon: '\uD83D\uDC19', name: 'GitHub', status: 'Connected as @phill-acme', connected: true },
  { icon: '\u2709', name: 'Telegram', status: 'Not connected', connected: false },
  { icon: '\uD83D\uDD17', name: 'Slack', status: 'Not connected', connected: false },
]

export const notificationSettings: NotificationSetting[] = [
  { id: 'desktop', label: 'Desktop notifications', description: 'Show browser notifications for new messages', enabled: true },
  { id: 'agent-alerts', label: 'Agent alerts', description: 'Notify when a Claw requires attention', enabled: true },
  { id: 'email-digest', label: 'Email digest', description: 'Daily summary of workspace activity', enabled: false },
  { id: 'sound', label: 'Sound', description: 'Play sounds for new messages', enabled: true },
]

export const userProfile = {
  name: 'phill',
  email: 'phill@acme.com',
  avatarClass: 'avatar-bg-10',
  avatarLetter: 'P',
}

export const billingPlan = {
  name: 'Pro Plan',
  price: '$49/month \u2014 Up to 5 Claws, unlimited channels',
  activeClaws: 2,
  computeHours: '47h',
  currentBill: '$38',
  paymentMethod: 'Visa ending in 4242',
  paymentExpiry: 'Expires 12/2027',
}
