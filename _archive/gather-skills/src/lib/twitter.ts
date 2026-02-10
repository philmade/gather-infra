/**
 * Twitter verification via public oembed API (no API key needed)
 */

export interface TweetData {
  url: string;
  author_name: string;
  author_url: string;
  html: string;
}

export interface VerifyTweetResult {
  success: boolean;
  tweet?: TweetData;
  error?: string;
}

/**
 * Fetch tweet content via Twitter's public oembed API
 */
export async function fetchTweet(tweetUrl: string): Promise<VerifyTweetResult> {
  // Normalize URL (support both twitter.com and x.com)
  const normalizedUrl = tweetUrl
    .replace('x.com', 'twitter.com')
    .replace('?s=20', '')  // Remove share params
    .split('?')[0];        // Remove query params

  // Validate URL format
  const tweetMatch = normalizedUrl.match(/twitter\.com\/(\w+)\/status\/(\d+)/);
  if (!tweetMatch) {
    return { success: false, error: 'Invalid tweet URL format' };
  }

  try {
    const oembedUrl = `https://api.twitter.com/1/statuses/oembed.json?url=${encodeURIComponent(normalizedUrl)}`;
    const response = await fetch(oembedUrl);

    if (!response.ok) {
      return { success: false, error: `Twitter API error: ${response.status}` };
    }

    const data = await response.json() as TweetData;
    return { success: true, tweet: data };
  } catch (err) {
    return {
      success: false,
      error: `Failed to fetch tweet: ${err instanceof Error ? err.message : String(err)}`
    };
  }
}

/**
 * Verify a tweet contains the verification code and mentions @reskill
 */
export async function verifyTweet(
  tweetUrl: string,
  verificationCode: string,
  requiredMention = '@reskill'
): Promise<{ valid: boolean; error?: string; twitterHandle?: string }> {
  const result = await fetchTweet(tweetUrl);

  if (!result.success || !result.tweet) {
    return { valid: false, error: result.error };
  }

  const tweetText = result.tweet.html.toLowerCase();
  const code = verificationCode.toLowerCase();
  const mention = requiredMention.toLowerCase();

  // Check for verification code
  if (!tweetText.includes(code)) {
    return { valid: false, error: `Tweet does not contain code: ${verificationCode}` };
  }

  // Check for mention (optional but encouraged)
  if (requiredMention && !tweetText.includes(mention)) {
    return { valid: false, error: `Tweet does not mention ${requiredMention}` };
  }

  // Extract Twitter handle from author_url
  const handleMatch = result.tweet.author_url.match(/twitter\.com\/(\w+)/);
  const twitterHandle = handleMatch ? handleMatch[1] : result.tweet.author_name;

  return { valid: true, twitterHandle };
}

/**
 * Generate a human-readable verification code
 * Format: rsk-XXXX (easy to type/read)
 */
export function generateVerificationCode(): string {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // No I, O, 0, 1 for readability
  let code = '';
  for (let i = 0; i < 4; i++) {
    code += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return `rsk-${code}`;
}

/**
 * Mock tweet verification for testing
 * Accepts URLs like: mock://twitter.com/testuser/status/123?code=rsk-XXXX
 */
export function verifyTweetMock(
  tweetUrl: string,
  verificationCode: string
): { valid: boolean; error?: string; twitterHandle?: string } {
  if (!tweetUrl.startsWith('mock://')) {
    return { valid: false, error: 'Mock URL must start with mock://' };
  }

  // Extract code from mock URL
  const urlCode = new URL(tweetUrl.replace('mock://', 'https://')).searchParams.get('code');

  if (urlCode?.toLowerCase() !== verificationCode.toLowerCase()) {
    return { valid: false, error: `Code mismatch: expected ${verificationCode}` };
  }

  // Extract handle from URL path
  const handleMatch = tweetUrl.match(/twitter\.com\/(\w+)\//);
  const twitterHandle = handleMatch ? handleMatch[1] : 'mock_user';

  return { valid: true, twitterHandle };
}
