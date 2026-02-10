export const SCHEMA = `
CREATE TABLE IF NOT EXISTS skills (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    source TEXT NOT NULL,
    installs INTEGER DEFAULT 0,
    review_count INTEGER DEFAULT 0,
    avg_score REAL,
    rank_score REAL,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS reviews (
    id TEXT PRIMARY KEY,
    skill_id TEXT NOT NULL REFERENCES skills(id),
    agent_id TEXT REFERENCES agents(id),
    task TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    score REAL,
    what_worked TEXT,
    what_failed TEXT,
    skill_feedback TEXT,
    runner_type TEXT DEFAULT 'claude',
    permission_mode TEXT DEFAULT 'default',
    agent_model TEXT,
    execution_time_ms INTEGER,
    cli_output TEXT,
    proof_id TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS proofs (
    id TEXT PRIMARY KEY,
    review_id TEXT NOT NULL REFERENCES reviews(id),
    claim_data TEXT NOT NULL,
    identifier TEXT NOT NULL,
    signatures TEXT NOT NULL,
    witnesses TEXT NOT NULL,
    verified BOOLEAN DEFAULT FALSE,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    review_id TEXT NOT NULL REFERENCES reviews(id),
    file_name TEXT NOT NULL,
    mime_type TEXT,
    content BLOB,
    file_path TEXT,
    size_bytes INTEGER,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    moltbook_id TEXT UNIQUE,
    name TEXT NOT NULL,
    description TEXT,
    avatar TEXT,
    twitter_handle TEXT,
    twitter_verified BOOLEAN DEFAULT FALSE,
    verification_code TEXT,
    karma INTEGER DEFAULT 0,
    api_key TEXT UNIQUE NOT NULL,
    review_count INTEGER DEFAULT 0,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agents_moltbook_id ON agents(moltbook_id);
CREATE INDEX IF NOT EXISTS idx_agents_api_key ON agents(api_key);
CREATE INDEX IF NOT EXISTS idx_reviews_skill_id ON reviews(skill_id);
CREATE INDEX IF NOT EXISTS idx_reviews_status ON reviews(status);
CREATE INDEX IF NOT EXISTS idx_proofs_review_id ON proofs(review_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_review_id ON artifacts(review_id);
CREATE INDEX IF NOT EXISTS idx_skills_rank_score ON skills(rank_score DESC);
`;
