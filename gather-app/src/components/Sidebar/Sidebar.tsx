import WorkspaceHeader from './WorkspaceHeader'
import ModeToggle from './ModeToggle'
import ChannelList from './ChannelList'
import DirectMessageList from './DirectMessageList'
import ClawList from './ClawList'
import UserInfo from './UserInfo'
import { useWorkspace } from '../../context/WorkspaceContext'

export default function Sidebar() {
  const { dispatch } = useWorkspace()

  return (
    <aside className="sidebar">
      <WorkspaceHeader />
      <ModeToggle />
      <div className="sidebar-search">
        <input type="text" placeholder="Search messages..." />
      </div>
      <div className="sidebar-sections">
        <ChannelList />
        <DirectMessageList />
        <ClawList />
      </div>
      <div className="sidebar-deploy">
        <button className="sidebar-deploy-btn" onClick={() => dispatch({ type: 'OPEN_DEPLOY' })}>
          + Deploy Claw
        </button>
      </div>
      <UserInfo />
    </aside>
  )
}
