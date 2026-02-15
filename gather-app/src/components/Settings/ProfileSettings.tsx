import { userProfile } from '../../data/settings'

export default function ProfileSettings() {
  return (
    <div>
      <h2>Profile</h2>
      <p className="section-desc">Manage your account details.</p>

      <div className="profile-card">
        <div className={`profile-avatar ${userProfile.avatarClass}`}>{userProfile.avatarLetter}</div>
        <div className="profile-info">
          <div className="profile-name">{userProfile.name}</div>
          <div className="profile-email">{userProfile.email}</div>
        </div>
      </div>

      <div className="form-group">
        <label className="form-label">Display Name</label>
        <input type="text" className="form-input" defaultValue={userProfile.name} />
      </div>
      <div className="form-group">
        <label className="form-label">Email</label>
        <input type="email" className="form-input" defaultValue={userProfile.email} />
      </div>
      <div style={{ marginTop: 'var(--space-md)' }}>
        <button className="btn btn-primary btn-sm">Save Changes</button>
      </div>
    </div>
  )
}
