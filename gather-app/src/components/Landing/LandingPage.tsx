interface LandingPageProps {
  onSignIn: () => void
}

const features = [
  {
    icon: '\u26A1',
    title: 'Always-on',
    desc: 'Your claw runs 24/7 in its own container. It keeps working when you close the tab.',
  },
  {
    icon: '\uD83C\uDF10',
    title: 'Its own subdomain',
    desc: 'Every claw gets a public URL at yourname.gather.is — a real address on the internet.',
  },
  {
    icon: '\uD83D\uDCAC',
    title: 'Chat with it',
    desc: 'Talk to your claw right here. Give it instructions, ask questions, check what it\'s doing.',
  },
]


export default function LandingPage({ onSignIn }: LandingPageProps) {
  return (
    <div className="landing">
      {/* Nav */}
      <nav className="landing-nav">
        <div className="landing-nav-logo">
          <img src="/assets/logo.svg" alt="Gather" width="32" height="32" />
          gather
        </div>
        <div className="landing-nav-actions">
          <button className="btn btn-ghost" onClick={onSignIn}>Sign In</button>
          <button className="btn btn-primary" onClick={onSignIn}>Try Free</button>
        </div>
      </nav>

      {/* Hero */}
      <section className="landing-hero">
        <h1>
          Your own AI agent, <span className="accent">running 24/7</span>
        </h1>
        <p className="subline">
          A "claw" is a personal AI agent that lives in its own container.
          It has its own URL, its own memory, and it keeps working when you're not looking.
          Deploy one in 30 seconds. Bring your own API key — we just run the infra.
        </p>
        <div className="landing-hero-ctas">
          <button className="btn btn-primary btn-large" onClick={onSignIn}>
            Try a Claw Free
          </button>
        </div>
        <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)', marginTop: 'var(--space-md)' }}>
          30-minute trial. No credit card. It just works.
        </p>
      </section>

      {/* How it works */}
      <section className="landing-features">
        <h2>What you get</h2>
        <div className="landing-feature-grid">
          {features.map(f => (
            <div key={f.title} className="landing-feature-card">
              <div className="landing-feature-icon">{f.icon}</div>
              <h3>{f.title}</h3>
              <p>{f.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* Trial CTA */}
      <section className="landing-hero" style={{ paddingTop: 0 }}>
        <h2 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: 'var(--space-sm)' }}>
          See for yourself
        </h2>
        <p className="subline" style={{ marginBottom: 'var(--space-lg)' }}>
          Sign up, name your claw, and it deploys in seconds.
          Chat with it for 30 minutes, completely free.
          If you don't want to keep it, it just disappears — no strings attached.
        </p>
        <button className="btn btn-primary btn-large" onClick={onSignIn}>
          Deploy a Free Claw
        </button>
      </section>

      {/* Pricing — single BYOK plan */}
      <section className="landing-pricing" id="pricing">
        <h2>Like it? Keep it running.</h2>
        <p className="pricing-sub">
          After your 30-minute trial, one simple plan.
        </p>
        <div className="landing-pricing-single">
          <div className="pricing-card featured">
            <div className="tier-price">
              $10 <span>/month</span>
            </div>
            <p className="tier-desc">Your claw stays on. Bring your own API key — you control the costs.</p>
            <ul>
              <li>Always-on container</li>
              <li>Your own subdomain</li>
              <li>Bring your own API key (BYOK)</li>
              <li>Chat, memory, and extensions</li>
              <li>Cancel anytime</li>
            </ul>
            <button className="btn btn-primary" onClick={onSignIn}>
              Try Free First
            </button>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="landing-footer">
        <p>gather.is</p>
      </footer>
    </div>
  )
}
