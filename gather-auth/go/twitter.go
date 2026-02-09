package auth

// Port of reskill's src/lib/twitter.ts to Go.
// Twitter verification via public oEmbed API — no API key needed.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

var tweetURLPattern = regexp.MustCompile(`twitter\.com/(\w+)/status/(\d+)`)

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
		"https://api.twitter.com/1/statuses/oembed.json?url=%s",
		url.QueryEscape(normalized),
	)

	resp, err := http.Get(oembedURL)
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

	// Extract handle from author_url
	handle := tweet.AuthorName
	if m := tweetURLPattern.FindStringSubmatch(
		strings.Replace(tweet.AuthorURL, "x.com", "twitter.com", 1),
	); len(m) > 1 {
		handle = m[1]
	}

	return VerifyTweetResult{Valid: true, TwitterHandle: handle}
}

// GenerateVerificationCode creates a human-readable code like "gtr-X4B2".
// Uses unambiguous characters (no I, O, 0, 1).
func GenerateVerificationCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 4)
	for i := range code {
		code[i] = chars[rand.Intn(len(chars))]
	}
	return "gtr-" + string(code)
}
