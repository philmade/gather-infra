import { createContext, useContext, useReducer, type ReactNode, type Dispatch } from 'react'

export interface WorkspaceState {
  mode: 'workspace' | 'network'
  activeChannel: string
  detailView: 'participants' | 'agent-detail'
  detailOpen: boolean
  selectedAgent: string | null
  deployModal: { open: boolean; step: number }
  settingsOpen: boolean
  settingsTab: string
  networkTab: string
  webtopOpen: boolean
  webtopAgent: string | null
}

export type WorkspaceAction =
  | { type: 'SET_MODE'; mode: 'workspace' | 'network' }
  | { type: 'SET_CHANNEL'; channel: string }
  | { type: 'SHOW_PARTICIPANTS' }
  | { type: 'SHOW_AGENT_DETAIL'; agentId: string }
  | { type: 'TOGGLE_DETAIL' }
  | { type: 'CLOSE_DETAIL' }
  | { type: 'OPEN_DEPLOY' }
  | { type: 'CLOSE_DEPLOY' }
  | { type: 'DEPLOY_NEXT' }
  | { type: 'DEPLOY_PREV' }
  | { type: 'DEPLOY_SET_STEP'; step: number }
  | { type: 'OPEN_SETTINGS' }
  | { type: 'CLOSE_SETTINGS' }
  | { type: 'SET_SETTINGS_TAB'; tab: string }
  | { type: 'SET_NETWORK_TAB'; tab: string }
  | { type: 'OPEN_WEBTOP'; agentName: string }
  | { type: 'CLOSE_WEBTOP' }

const initialState: WorkspaceState = {
  mode: 'workspace',
  activeChannel: 'general',
  detailView: 'participants',
  detailOpen: true,
  selectedAgent: null,
  deployModal: { open: false, step: 1 },
  settingsOpen: false,
  settingsTab: 'profile',
  networkTab: 'feed',
  webtopOpen: false,
  webtopAgent: null,
}

function reducer(state: WorkspaceState, action: WorkspaceAction): WorkspaceState {
  switch (action.type) {
    case 'SET_MODE':
      return { ...state, mode: action.mode }
    case 'SET_CHANNEL':
      return { ...state, activeChannel: action.channel }
    case 'SHOW_PARTICIPANTS':
      return { ...state, detailView: 'participants', selectedAgent: null }
    case 'SHOW_AGENT_DETAIL':
      return { ...state, detailView: 'agent-detail', selectedAgent: action.agentId, detailOpen: true }
    case 'TOGGLE_DETAIL':
      return { ...state, detailOpen: !state.detailOpen }
    case 'CLOSE_DETAIL':
      return { ...state, detailOpen: false }
    case 'OPEN_DEPLOY':
      return { ...state, deployModal: { open: true, step: 1 } }
    case 'CLOSE_DEPLOY':
      return { ...state, deployModal: { open: false, step: 1 } }
    case 'DEPLOY_NEXT':
      return { ...state, deployModal: { ...state.deployModal, step: Math.min(state.deployModal.step + 1, 3) } }
    case 'DEPLOY_PREV':
      return { ...state, deployModal: { ...state.deployModal, step: Math.max(state.deployModal.step - 1, 1) } }
    case 'DEPLOY_SET_STEP':
      return { ...state, deployModal: { ...state.deployModal, step: action.step } }
    case 'OPEN_SETTINGS':
      return { ...state, settingsOpen: true }
    case 'CLOSE_SETTINGS':
      return { ...state, settingsOpen: false }
    case 'SET_SETTINGS_TAB':
      return { ...state, settingsTab: action.tab }
    case 'SET_NETWORK_TAB':
      return { ...state, networkTab: action.tab }
    case 'OPEN_WEBTOP':
      return { ...state, webtopOpen: true, webtopAgent: action.agentName }
    case 'CLOSE_WEBTOP':
      return { ...state, webtopOpen: false, webtopAgent: null }
  }
}

interface WorkspaceContextValue {
  state: WorkspaceState
  dispatch: Dispatch<WorkspaceAction>
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState)
  return (
    <WorkspaceContext.Provider value={{ state, dispatch }}>
      {children}
    </WorkspaceContext.Provider>
  )
}

export function useWorkspace() {
  const ctx = useContext(WorkspaceContext)
  if (!ctx) throw new Error('useWorkspace must be used within WorkspaceProvider')
  return ctx
}
