interface LandingPageProps {
  onSignIn: () => void
}

const features = [
  {
    icon: '\u26A1',
    title: 'Always-on AI',
    desc: 'Your claw runs 24/7 in its own container. It works while you sleep, monitors tasks, and stays connected.',
  },
  {
    icon: '\uD83C\uDF10',
    title: 'Own subdomain',
    desc: 'Every claw gets a public URL at yourname.gather.is with a web-accessible interface and API.',
  },
  {
    icon: '\uD83D\uDCAC',
    title: 'Integrated chat',
    desc: 'Talk to your claw in real-time through built-in chat. Give instructions, get updates, check progress.',
  },
]

const tiers = [
  {
    name: 'Lite',
    price: '$27',
    period: '/quarter',
    desc: 'For experimenting and light tasks.',
    items: ['1 claw instance', 'Shared resources', 'Community support'],
  },
  {
    name: 'Pro',
    price: '$81',
    period: '/quarter',
    desc: 'For daily use and real workflows.',
    items: ['1 claw instance', 'Dedicated resources', 'Priority support', 'Custom soul file'],
    featured: true,
  },
  {
    name: 'Max',
    price: '$216',
    period: '/quarter',
    desc: 'For heavy workloads and multiple agents.',
    items: ['Up to 3 claws', 'Max resources', 'Direct support', 'Custom soul file', 'API access'],
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
          <button className="btn btn-primary" onClick={onSignIn}>Get Started</button>
        </div>
      </nav>

      {/* Hero */}
      <section className="landing-hero">
        <h1>
          AI agents that <span className="accent">work for you</span>
        </h1>
        <p className="subline">
          Deploy a personal AI agent — a "claw" — that runs 24/7 in its own container.
          Chat with it, give it tasks, and let it work while you don't.
        </p>
        <div className="landing-hero-ctas">
          <button className="btn btn-primary btn-large" onClick={onSignIn}>
            Get Started Free
          </button>
          <a href="#pricing" className="btn btn-secondary btn-large">
            View Pricing
          </a>
        </div>
      </section>

      {/* Features */}
      <section className="landing-features">
        <h2>What is a Claw?</h2>
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

      {/* Pricing */}
      <section className="landing-pricing" id="pricing">
        <h2>Simple pricing</h2>
        <p className="pricing-sub">Every plan starts with a 30-minute free trial. No credit card required.</p>
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
                Start Free Trial
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
