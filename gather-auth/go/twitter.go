package auth

// Port of reskill's src/lib/twitter.ts to Go.
// Twitter verification via public oEmbed API — no API key needed.

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// TweetData holds the oEmbed response from Twitter.
type TweetData struct {
	URL        string `json:"url"`
	AuthorName string `json:"author_name"`
	AuthorURL  string `json:"author_url"`
	HTML       string `json:"html"`
}

// VerifyTweetResult is the outcome of a tweet verification attempt.
type VerifyTweetResult struct {
	Valid         bool
	TwitterHandle string
	Error         string
}

var tweetURLPattern = regexp.MustCompile(`(?:twitter|x)\.com/(\w+)/status/(\d+)`)
var profileURLPattern = regexp.MustCompile(`(?:twitter|x)\.com/(\w+)/?$`)

// FetchTweet retrieves tweet content via Twitter's public oEmbed API.
func FetchTweet(tweetURL string) (*TweetData, error) {
	// Normalize: x.com → twitter.com, strip query params
	normalized := strings.Replace(tweetURL, "x.com", "twitter.com", 1)
	if idx := strings.Index(normalized, "?"); idx != -1 {
		normalized = normalized[:idx]
	}

	if !tweetURLPattern.MatchString(normalized) {
		return nil, fmt.Errorf("invalid tweet URL format")
	}

	oembedURL := fmt.Sprintf(
		"https://publish.twitter.com/oembed?url=%s&format=json",
		url.QueryEscape(normalized),
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(oembedURL)
	if err != nil {
		return nil, fmt.Errorf("fetch oembed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter API error: %d", resp.StatusCode)
	}

	var data TweetData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode oembed: %w", err)
	}

	return &data, nil
}

// VerifyTweet checks that a tweet contains the verification code and required mention.
func VerifyTweet(tweetURL, verificationCode, requiredMention string) VerifyTweetResult {
	tweet, err := FetchTweet(tweetURL)
	if err != nil {
		return VerifyTweetResult{Valid: false, Error: err.Error()}
	}

	text := strings.ToLower(tweet.HTML)
	code := strings.ToLower(verificationCode)
	mention := strings.ToLower(requiredMention)

	if !strings.Contains(text, code) {
		return VerifyTweetResult{
			Valid: false,
			Error: fmt.Sprintf("tweet does not contain code: %s", verificationCode),
		}
	}

	if requiredMention != "" && !strings.Contains(text, mention) {
		return VerifyTweetResult{
			Valid: false,
			Error: fmt.Sprintf("tweet does not mention %s", requiredMention),
		}
	}

	// Extract handle from author_url (profile URL like https://twitter.com/username)
	authorURL := tweet.AuthorURL
	if authorURL == "" {
		authorURL = tweet.URL // fallback to tweet URL
	}
	normalized := strings.Replace(authorURL, "x.com", "twitter.com", 1)

	var handle string
	if m := profileURLPattern.FindStringSubmatch(normalized); len(m) >= 2 {
		handle = m[1]
	} else if m := tweetURLPattern.FindStringSubmatch(normalized); len(m) >= 2 {
		handle = m[1]
	}

	if handle == "" {
		return VerifyTweetResult{
			Valid: false,
			Error: fmt.Sprintf("could not extract twitter handle from author URL: %s", tweet.AuthorURL),
		}
	}

	return VerifyTweetResult{Valid: true, TwitterHandle: handle}
}

// GenerateVerificationCode creates a human-readable code like "gtr-X4B2".
// Uses unambiguous characters (no I, O, 0, 1).
func GenerateVerificationCode() (string, error) {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 4)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", fmt.Errorf("generate verification code: %w", err)
		}
		code[i] = chars[n.Int64()]
	}
	return "gtr-" + string(code), nil
}
