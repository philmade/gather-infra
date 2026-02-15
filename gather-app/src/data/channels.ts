export interface Message {
  id: string
  author: string
  avatarClass: string
  avatarLetter: string
  time: string
  text: string
  isBot?: boolean
  isSystem?: boolean
}

export interface DateSeparator {
  id: string
  label: string
  isDateSeparator: true
}

export type MessageItem = Message | DateSeparator

export interface Channel {
  id: string
  name: string
  topic: string
  unread?: number
}

export interface DirectMessage {
  id: string
  name: string
  displayName: string
  avatarClass: string
  avatarLetter: string
  presence: 'online' | 'away' | 'offline'
  isBot?: boolean
}

export const channels: Channel[] = [
  { id: 'general', name: 'general', topic: 'Company-wide announcements and work-based matters' },
  { id: 'engineering', name: 'engineering', topic: 'Code, architecture, and infrastructure', unread: 3 },
  { id: 'design', name: 'design', topic: 'UI/UX design and assets' },
  { id: 'marketing', name: 'marketing', topic: 'Campaigns, content, and launches' },
  { id: 'ops', name: 'ops', topic: 'Operations, monitoring, and incidents', unread: 1 },
]

export const directMessages: DirectMessage[] = [
  { id: 'dm-sarah', name: 'sarah', displayName: 'sarah', avatarClass: 'avatar-bg-3', avatarLetter: 'S', presence: 'online' },
  { id: 'dm-marcus', name: 'marcus', displayName: 'marcus', avatarClass: 'avatar-bg-7', avatarLetter: 'M', presence: 'away' },
  { id: 'dm-buyclaw', name: 'buyclaw', displayName: 'BuyClaw', avatarClass: 'avatar-bg-2', avatarLetter: 'B', presence: 'online', isBot: true },
  { id: 'dm-reviewclaw', name: 'reviewclaw', displayName: 'ReviewClaw', avatarClass: 'avatar-bg-9', avatarLetter: 'R', presence: 'offline', isBot: true },
]

export const channelMessages: Record<string, MessageItem[]> = {
  general: [
    { id: 'ds-1', label: 'Yesterday', isDateSeparator: true },
    { id: 'g1', author: 'sarah', avatarClass: 'avatar-bg-3', avatarLetter: 'S', time: '4:32 PM', text: 'Hey team, just pushed the new landing page updates. Can someone review the copy before we go live?' },
    { id: 'g2', author: 'phill', avatarClass: 'avatar-bg-10', avatarLetter: 'P', time: '4:45 PM', text: "I'll take a look. Also, I'm going to deploy BuyClaw to help with the order processing backlog." },
    { id: 'ds-2', label: 'Today', isDateSeparator: true },
    { id: 'g3', author: '', avatarClass: '', avatarLetter: '', time: '', text: 'phill deployed BuyClaw to this workspace', isSystem: true },
    { id: 'g4', author: 'BuyClaw', avatarClass: 'avatar-bg-2', avatarLetter: 'B', time: '9:15 AM', text: "I'm online and ready. I've connected to the Shopify API and found 23 pending orders. Starting batch processing now.", isBot: true },
    { id: 'g5', author: 'marcus', avatarClass: 'avatar-bg-7', avatarLetter: 'M', time: '9:30 AM', text: 'Nice, that backlog was getting out of hand. BuyClaw, can you prioritize international orders first?' },
    { id: 'g6', author: 'BuyClaw', avatarClass: 'avatar-bg-2', avatarLetter: 'B', time: '9:31 AM', text: "Got it. Reprioritizing \u2014 8 international orders moved to the front. I'll have them processed within the hour.", isBot: true },
    { id: 'g7', author: 'sarah', avatarClass: 'avatar-bg-3', avatarLetter: 'S', time: '10:12 AM', text: 'Landing page is live! Check it out: <code>gather.is</code>' },
    { id: 'g8', author: 'phill', avatarClass: 'avatar-bg-10', avatarLetter: 'P', time: '10:20 AM', text: 'Looks great. I\'m also thinking we should deploy a ReviewClaw to handle the skill reviews queue. Thoughts?' },
  ],
  engineering: [
    { id: 'ds-3', label: 'Today', isDateSeparator: true },
    { id: 'e1', author: 'marcus', avatarClass: 'avatar-bg-7', avatarLetter: 'M', time: '8:00 AM', text: 'I\'ve been profiling the API and the <code>/skills/search</code> endpoint is taking 400ms on average. The ranking query needs optimization.' },
    { id: 'e2', author: 'phill', avatarClass: 'avatar-bg-10', avatarLetter: 'P', time: '8:15 AM', text: 'We should add an index on the weighted_score column. SQLite should handle that fine for our current scale.' },
    { id: 'e3', author: '', avatarClass: '', avatarLetter: '', time: '', text: 'phill deployed ReviewClaw to this workspace', isSystem: true },
    { id: 'e4', author: 'ReviewClaw', avatarClass: 'avatar-bg-9', avatarLetter: 'R', time: '9:00 AM', text: 'Online. I have access to the skills database and review queue. 14 pending reviews detected. Beginning execution.', isBot: true },
  ],
  design: [
    { id: 'ds-4', label: 'Today', isDateSeparator: true },
    { id: 'd1', author: 'sarah', avatarClass: 'avatar-bg-3', avatarLetter: 'S', time: '11:00 AM', text: 'Working on the workspace redesign mockups. The three-column layout is coming together nicely. Will share Figma link soon.' },
  ],
  marketing: [
    { id: 'ds-5', label: 'Today', isDateSeparator: true },
    { id: 'm1', author: 'phill', avatarClass: 'avatar-bg-10', avatarLetter: 'P', time: '2:00 PM', text: 'We need to draft the launch announcement for the Claw marketplace. Target is next Friday.' },
  ],
  ops: [
    { id: 'ds-6', label: 'Today', isDateSeparator: true },
    { id: 'o0', author: '', avatarClass: '', avatarLetter: '', time: '', text: 'System: All services healthy. Uptime 99.97% (30d)', isSystem: true },
    { id: 'o1', author: 'BuyClaw', avatarClass: 'avatar-bg-2', avatarLetter: 'B', time: '11:30 AM', text: 'Order processing complete. 23/23 orders fulfilled. 0 errors. Average processing time: 4.2s per order.', isBot: true },
  ],
  'dm-sarah': [
    { id: 'ds-7', label: 'Today', isDateSeparator: true },
    { id: 'dm-s1', author: 'sarah', avatarClass: 'avatar-bg-3', avatarLetter: 'S', time: '3:00 PM', text: 'Hey, do you have a minute? I want to discuss the billing page design before I finalize it.' },
    { id: 'dm-s2', author: 'phill', avatarClass: 'avatar-bg-10', avatarLetter: 'P', time: '3:05 PM', text: "Sure, go ahead. I'm free for the next 30 min." },
  ],
  'dm-marcus': [
    { id: 'ds-8', label: 'Yesterday', isDateSeparator: true },
    { id: 'dm-m1', author: 'marcus', avatarClass: 'avatar-bg-7', avatarLetter: 'M', time: '6:00 PM', text: 'The deploy pipeline is green. Pushed the latest changes to prod.' },
  ],
  'dm-buyclaw': [
    { id: 'ds-9', label: 'Today', isDateSeparator: true },
    { id: 'dm-b1', author: 'BuyClaw', avatarClass: 'avatar-bg-2', avatarLetter: 'B', time: '9:20 AM', text: 'Daily report: I processed 23 orders today with a 100% success rate. 3 orders flagged for manual review due to address mismatches. Want me to send the details?', isBot: true },
    { id: 'dm-b2', author: 'phill', avatarClass: 'avatar-bg-10', avatarLetter: 'P', time: '9:25 AM', text: "Yes, send the flagged order IDs and I'll take a look." },
  ],
  'dm-reviewclaw': [
    { id: 'ds-10', label: 'Today', isDateSeparator: true },
    { id: 'dm-r1', author: 'ReviewClaw', avatarClass: 'avatar-bg-9', avatarLetter: 'R', time: '9:05 AM', text: "Initialized. I'm reviewing skill submissions in the queue. First up: FELMONON/skillsign. Estimated review time: 12 minutes.", isBot: true },
  ],
}
