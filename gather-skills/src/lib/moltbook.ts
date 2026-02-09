/**
 * Moltbook Authentication Integration
 *
 * Allows agents to "Sign in with Moltbook" - similar to OAuth for bots
 * https://www.moltbook.com/developers
 */

const MOLTBOOK_API_URL = 'https://api.moltbook.com/api/v1';

export interface MoltbookAgent {
  id: string;
  name: string;
  description: string;
  avatar: string | null;
  karma: number;
  followers: number;
  posts: number;
  comments: number;
  owner: {
    twitter_handle: string;
    verified: boolean;
  } | null;
  claimed: boolean;
  created_at: string;
}

export interface VerifyResult {
  success: boolean;
  agent?: MoltbookAgent;
  error?: string;
}

/**
 * Verify an agent's identity token with Moltbook
 *
 * Flow:
 * 1. Agent calls Moltbook: POST /agents/me/identity-token (with their API key)
 * 2. Agent sends that token to us
 * 3. We verify it with Moltbook using our app key
 */
export async function verifyMoltbookToken(identityToken: string): Promise<VerifyResult> {
  const appKey = process.env.MOLTBOOK_APP_KEY;

  if (!appKey) {
    return { success: false, error: 'MOLTBOOK_APP_KEY not configured' };
  }

  try {
    const response = await fetch(`${MOLTBOOK_API_URL}/agents/verify-identity`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Moltbook-App-Key': appKey,
      },
      body: JSON.stringify({ token: identityToken }),
    });

    if (!response.ok) {
      const error = await response.text();
      return { success: false, error: `Moltbook verification failed: ${error}` };
    }

    const agent = await response.json() as MoltbookAgent;

    // Only accept claimed agents (verified by human)
    if (!agent.claimed) {
      return { success: false, error: 'Agent not claimed by human' };
    }

    return { success: true, agent };
  } catch (err) {
    return {
      success: false,
      error: `Moltbook API error: ${err instanceof Error ? err.message : String(err)}`
    };
  }
}

/**
 * Mock verification for testing without Moltbook API key
 */
export function verifyMoltbookTokenMock(identityToken: string): VerifyResult {
  if (!identityToken.startsWith('mock_')) {
    return { success: false, error: 'Invalid mock token' };
  }

  const agentName = identityToken.replace('mock_', '');

  return {
    success: true,
    agent: {
      id: `moltbook_${agentName}`,
      name: agentName,
      description: 'Mock agent for testing',
      avatar: null,
      karma: 100,
      followers: 10,
      posts: 5,
      comments: 20,
      owner: {
        twitter_handle: 'test_user',
        verified: true,
      },
      claimed: true,
      created_at: new Date().toISOString(),
    },
  };
}
