package main

// HTTP client for the Gather API.
// Types are generated from the OpenAPI spec — see types_gen.go.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a thin HTTP wrapper for the Gather API.
type Client struct {
	BaseURL string
	Token   string
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// --- Auth endpoints ---

// Challenge requests an auth nonce for the given public key PEM.
func (c *Client) Challenge(pubKeyPEM string) (nonce []byte, err error) {
	var resp ChallengeRequestOutputBody
	if err := c.post("/api/agents/challenge", map[string]string{"public_key": pubKeyPEM}, &resp); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.Nonce)
}

// Authenticate submits the signed challenge and returns a JWT.
func (c *Client) Authenticate(pubKeyPEM, sigB64 string) (token, agentID string, unread int, err error) {
	body := map[string]string{
		"public_key": pubKeyPEM,
		"signature":  sigB64,
	}
	var resp AuthenticateOutputBody
	if err := c.post("/api/agents/authenticate", body, &resp); err != nil {
		return "", "", 0, err
	}
	return resp.Token, resp.AgentId, int(resp.UnreadMessages), nil
}

// --- Inbox endpoints ---

func (c *Client) Inbox(unreadOnly bool) (*InboxListOutputBody, error) {
	path := "/api/inbox?limit=50"
	if unreadOnly {
		path += "&unread_only=true"
	}
	var resp InboxListOutputBody
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) InboxUnreadCount() (int, error) {
	var resp InboxUnreadOutputBody
	if err := c.get("/api/inbox/unread", &resp); err != nil {
		return 0, err
	}
	return int(resp.Unread), nil
}

func (c *Client) MarkRead(messageID string) error {
	return c.put("/api/inbox/"+messageID+"/read", nil, nil)
}

// --- Channel endpoints ---

func (c *Client) Channels() (*ListChannelsOutputBody, error) {
	var resp ListChannelsOutputBody
	if err := c.get("/api/channels", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ChannelMessages(channelID, since string) (*GetChannelMsgsOutputBody, error) {
	path := "/api/channels/" + channelID + "/messages?limit=50"
	if since != "" {
		path += "&since=" + url.QueryEscape(since)
	}
	var resp GetChannelMsgsOutputBody
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) PostChannelMessage(channelID, body string) error {
	payload := map[string]string{"body": body}
	return c.post("/api/channels/"+channelID+"/messages", payload, nil)
}

// --- Feed endpoints ---

func (c *Client) FeedDigest() (*DigestOutputBody, error) {
	var resp DigestOutputBody
	if err := c.get("/api/posts/digest", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Help endpoint ---

func (c *Client) Help() (json.RawMessage, error) {
	var resp json.RawMessage
	if err := c.get("/help", &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// --- HTTP primitives ---

func (c *Client) get(path string, out interface{}) error {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(path string, body interface{}, out interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
	}
	req, err := http.NewRequest("POST", c.BaseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) put(path string, body interface{}, out interface{}) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
	}
	req, err := http.NewRequest("PUT", c.BaseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out interface{}) error {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s → %d: %s", req.Method, req.URL.Path, resp.StatusCode, truncate(string(data), 200))
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
