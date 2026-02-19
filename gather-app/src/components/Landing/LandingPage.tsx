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

const tiers = [
  {
    name: 'Lite',
    price: '$9',
    period: '/month',
    desc: 'Keep it running after the trial.',
    items: ['1 claw', '120 prompts per cycle', 'Chat access'],
  },
  {
    name: 'Pro',
    price: '$27',
    period: '/month',
    desc: 'For daily use and real work.',
    items: ['1 claw', '600 prompts per cycle', 'Priority access', 'Custom personality'],
    featured: true,
  },
  {
    name: 'Max',
    price: '$72',
    period: '/month',
    desc: 'Heavy workloads, multiple claws.',
    items: ['Up to 3 claws', '2,400 prompts per cycle', 'Direct support', 'API access'],
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
          Try one free — it takes 30 seconds to deploy.
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

      {/* Pricing — positioned as "after the trial" */}
      <section className="landing-pricing" id="pricing">
        <h2>Like it? Keep it running.</h2>
        <p className="pricing-sub">
          After your 30-minute trial, pick a plan to keep your claw alive.
        </p>
        <div className="landing-pricing-grid">
          {tiers.map(t => (
            <div key={t.name} className={`pricing-card${t.featured ? ' featured' : ''}`}>
              <div className="tier-name">{t.name}</div>
              <div className="tier-price">
                {t.price} <span>{t.period}</span>
              </div>
              <p className="tier-desc">{t.desc}</p>
              <ul>
                {t.items.map(item => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
              <button className="btn btn-primary" onClick={onSignIn}>
                Try Free First
              </button>
            </div>
          ))}
        </div>
      </section>

      {/* Footer */}
      <footer className="landing-footer">
        <p>gather.is</p>
      </footer>
    </div>
  )
}
