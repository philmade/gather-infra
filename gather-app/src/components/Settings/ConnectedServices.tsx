import { connectedServices } from '../../data/settings'

export default function ConnectedServices() {
  return (
    <div>
      <h2>Connected Services</h2>
      <p className="section-desc">Connect external services to your workspace.</p>

      {connectedServices.map(service => (
        <div key={service.name} className="service-row">
          <div className="service-icon">{service.icon}</div>
          <div className="service-info">
            <div className="service-name">{service.name}</div>
            <div className="service-status">{service.status}</div>
          </div>
          <button className={`btn ${service.connected ? 'btn-ghost' : 'btn-secondary'} btn-sm`}>
            {service.connected ? 'Disconnect' : 'Connect'}
          </button>
        </div>
      ))}
    </div>
  )
}
