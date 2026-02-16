import WorkspaceHeader from './WorkspaceHeader'
import ModeToggle from './ModeToggle'
import ChannelList from './ChannelList'
import DirectMessageList from './DirectMessageList'
import ClawList from './ClawList'
import UserInfo from './UserInfo'

export default function Sidebar() {
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
      <UserInfo />
    </aside>
  )
}
