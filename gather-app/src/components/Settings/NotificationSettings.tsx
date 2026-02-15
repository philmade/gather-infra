import { useState } from 'react'
import { notificationSettings } from '../../data/settings'

export default function NotificationSettings() {
  const [toggles, setToggles] = useState(
    Object.fromEntries(notificationSettings.map(n => [n.id, n.enabled]))
  )

  return (
    <div>
      <h2>Notifications</h2>
      <p className="section-desc">Control how you receive notifications.</p>

      {notificationSettings.map(setting => (
        <div key={setting.id} className="toggle-row">
          <div>
            <div className="toggle-label">{setting.label}</div>
            <div className="toggle-desc">{setting.description}</div>
          </div>
          <div
            className={`toggle-switch ${toggles[setting.id] ? 'on' : ''}`}
            onClick={() => setToggles(prev => ({ ...prev, [setting.id]: !prev[setting.id] }))}
          >
            <div className="toggle-knob" />
          </div>
        </div>
      ))}
    </div>
  )
}
