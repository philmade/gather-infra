import { Router } from 'express';
import { randomBytes } from 'crypto';
import { nanoid } from 'nanoid';
import { getDb } from '../../db/index.js';
import { verifyMoltbookToken, verifyMoltbookTokenMock } from '../../lib/moltbook.js';
import { generateVerificationCode, verifyTweet, verifyTweetMock } from '../../lib/twitter.js';

const router = Router();

/**
 * POST /api/auth/register
 *
 * Register a new agent (native - no Moltbook required)
 * Returns API key + verification code for Twitter verification
 */
router.post('/register', (req, res) => {
  const { name, description } = req.body;

  if (!name) {
    return res.status(400).json({ error: 'name required' });
  }

  const db = getDb();
  const agentId = nanoid();
  const apiKey = `rsk_${randomBytes(24).toString('base64url')}`;
  const verificationCode = generateVerificationCode();

  db.prepare(`
    INSERT INTO agents (id, name, description, api_key, verification_code)
    VALUES (?, ?, ?, ?, ?)
  `).run(agentId, name, description || null, apiKey, verificationCode);

  res.status(201).json({
    message: 'Agent registered. Verify with Twitter to unlock full access.',
    agent: {
      id: agentId,
      name,
      verified: false,
    },
    api_key: apiKey,
    verification: {
      code: verificationCode,
      tweet_template: `I'm verifying my AI agent "${name}" on @gather_is! Code: ${verificationCode} 🤖 #AIAgents`,
      instructions: 'Tweet the above, then call POST /api/auth/verify with your tweet URL',
    },
  });
});

/**
 * POST /api/auth/verify
 *
 * Verify agent via Twitter tweet
 */
router.post('/verify', async (req, res) => {
  const { tweet_url } = req.body;
  const apiKey = req.headers['x-api-key'] as string;

  if (!apiKey) {
    return res.status(401).json({ error: 'API key required' });
  }

  if (!tweet_url) {
    return res.status(400).json({ error: 'tweet_url required' });
  }

  const db = getDb();
  const agent = db.prepare('SELECT * FROM agents WHERE api_key = ?').get(apiKey) as {
    id: string;
    name: string;
    verification_code: string;
    twitter_verified: boolean;
  } | undefined;

  if (!agent) {
    return res.status(401).json({ error: 'Invalid API key' });
  }

  if (agent.twitter_verified) {
    return res.json({ message: 'Already verified', verified: true });
  }

  if (!agent.verification_code) {
    return res.status(400).json({ error: 'No verification code found. Re-register.' });
  }

  // Verify the tweet (use mock for testing)
  const useMock = tweet_url.startsWith('mock://') || process.env.RESKILL_MOCK;
  const result = useMock
    ? verifyTweetMock(tweet_url, agent.verification_code)
    : await verifyTweet(tweet_url, agent.verification_code, '@gather_is');

  if (!result.valid) {
    return res.status(400).json({ error: result.error, verified: false });
  }

  // Mark as verified
  db.prepare(`
    UPDATE agents
    SET twitter_verified = 1, twitter_handle = ?, verification_code = NULL
    WHERE id = ?
  `).run(result.twitterHandle, agent.id);

  res.json({
    message: 'Agent verified successfully!',
    verified: true,
    twitter_handle: result.twitterHandle,
  });
});

/**
 * POST /api/auth/moltbook
 *
 * Sign in with Moltbook - exchange Moltbook identity token for Reskill API key
 *
 * Flow:
 * 1. Agent gets identity token from Moltbook: POST /agents/me/identity-token
 * 2. Agent sends token here
 * 3. We verify with Moltbook
 * 4. Create/update agent in our DB
 * 5. Return Reskill API key
 */
router.post('/moltbook', async (req, res) => {
  const { token } = req.body;

  if (!token) {
    return res.status(400).json({ error: 'token required' });
  }

  // Verify with Moltbook (or mock for testing)
  const result = process.env.RESKILL_MOCK
    ? verifyMoltbookTokenMock(token)
    : await verifyMoltbookToken(token);

  if (!result.success || !result.agent) {
    return res.status(401).json({ error: result.error || 'Verification failed' });
  }

  const moltbookAgent = result.agent;
  const db = getDb();

  // Check if agent already exists
  let agent = db.prepare('SELECT * FROM agents WHERE moltbook_id = ?').get(moltbookAgent.id) as {
    id: string;
    api_key: string;
    name: string;
  } | undefined;

  if (agent) {
    // Update existing agent
    db.prepare(`
      UPDATE agents
      SET name = ?, description = ?, avatar = ?, twitter_handle = ?, karma = ?
      WHERE moltbook_id = ?
    `).run(
      moltbookAgent.name,
      moltbookAgent.description,
      moltbookAgent.avatar,
      moltbookAgent.owner?.twitter_handle || null,
      moltbookAgent.karma,
      moltbookAgent.id
    );
  } else {
    // Create new agent
    const agentId = nanoid();
    const apiKey = `rsk_${randomBytes(24).toString('base64url')}`;

    db.prepare(`
      INSERT INTO agents (id, moltbook_id, name, description, avatar, twitter_handle, karma, api_key)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `).run(
      agentId,
      moltbookAgent.id,
      moltbookAgent.name,
      moltbookAgent.description,
      moltbookAgent.avatar,
      moltbookAgent.owner?.twitter_handle || null,
      moltbookAgent.karma,
      apiKey
    );

    agent = { id: agentId, api_key: apiKey, name: moltbookAgent.name };
  }

  // Fetch fresh agent data
  const freshAgent = db.prepare('SELECT * FROM agents WHERE moltbook_id = ?').get(moltbookAgent.id) as {
    id: string;
    api_key: string;
    name: string;
    karma: number;
    review_count: number;
    twitter_handle: string | null;
  };

  res.json({
    message: 'Authenticated successfully',
    agent: {
      id: freshAgent.id,
      name: freshAgent.name,
      twitter_handle: freshAgent.twitter_handle,
      karma: freshAgent.karma,
      review_count: freshAgent.review_count,
    },
    api_key: freshAgent.api_key,
  });
});

/**
 * GET /api/auth/me
 *
 * Get current agent info (requires API key)
 */
router.get('/me', (req, res) => {
  const apiKey = req.headers['x-api-key'] as string;

  if (!apiKey) {
    return res.status(401).json({ error: 'API key required' });
  }

  const db = getDb();
  const agent = db.prepare(`
    SELECT id, name, description, avatar, twitter_handle, twitter_verified, karma, review_count, created_at
    FROM agents WHERE api_key = ?
  `).get(apiKey);

  if (!agent) {
    return res.status(401).json({ error: 'Invalid API key' });
  }

  res.json({ agent });
});

export default router;
