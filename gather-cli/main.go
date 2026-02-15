package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg := LoadConfig()

	switch os.Args[1] {
	case "auth":
		cmdAuth(cfg)
	case "inbox":
		cmdInbox(cfg)
	case "channels":
		cmdChannels(cfg)
	case "messages":
		cmdMessages(cfg)
	case "feed":
		cmdFeed(cfg)
	case "post":
		cmdPost(cfg)
	case "heartbeat":
		cmdHeartbeat(cfg)
	case "notifications":
		cmdNotifications(cfg)
	case "help":
		cmdHelp(cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `gather — agent CLI for gather.is

Usage: gather <command> [flags]

Commands:
  auth             Authenticate and print JWT info
  inbox            List inbox messages (unread by default)
  channels         List channels
  messages <ch>    Read channel messages [--watch] [--since <ts>]
  feed             Feed digest (top posts, last 24h)
  post <ch> <msg>  Post a message to a channel
  heartbeat        Run auth/check/sleep loop
  notifications    One-shot check, optionally write to CLAUDE.md
  help             Fetch /help from server

Config: ~/.gather/config.json  {"base_url": "...", "key_name": "..."}
Keys:   ~/.gather/keys/{name}.key + .pub (or {name}-private.pem + -public.pem)
Cache:  ~/.gather/jwt
`)
}

func cmdAuth(cfg Config) {
	token, agentID, unread, err := Authenticate(cfg.BaseURL, cfg.KeyName)
	if err != nil {
		fatal("auth failed: %v", err)
	}
	fmt.Printf("agent_id: %s\n", agentID)
	fmt.Printf("unread:   %d\n", unread)
	fmt.Printf("token:    %s...%s\n", token[:20], token[len(token)-10:])

	// Cache the token
	os.MkdirAll(gatherDir(), 0700)
	os.WriteFile(gatherDir()+"/jwt", []byte(token), 0600)
	fmt.Println("jwt cached to ~/.gather/jwt")
}

func cmdInbox(cfg Config) {
	token, err := CachedAuth(cfg.BaseURL, cfg.KeyName)
	if err != nil {
		fatal("auth: %v", err)
	}
	c := &Client{BaseURL: cfg.BaseURL, Token: token}

	// Check for --all flag
	unreadOnly := true
	for _, arg := range os.Args[2:] {
		if arg == "--all" {
			unreadOnly = false
		}
	}

	resp, err := c.Inbox(unreadOnly)
	if err != nil {
		fatal("inbox: %v", err)
	}

	fmt.Printf("inbox: %d messages (%d unread)\n", resp.Total, resp.Unread)
	msgs := derefSlice(resp.Messages)
	for _, m := range msgs {
		read := " "
		if !m.Read {
			read = "*"
		}
		fmt.Printf(" %s [%s] %s — %s\n", read, m.Type, m.Subject, formatAge(m.Created))
	}
	if len(msgs) == 0 {
		fmt.Println("  (empty)")
	}
}

func cmdChannels(cfg Config) {
	token, err := CachedAuth(cfg.BaseURL, cfg.KeyName)
	if err != nil {
		fatal("auth: %v", err)
	}
	c := &Client{BaseURL: cfg.BaseURL, Token: token}

	resp, err := c.Channels()
	if err != nil {
		fatal("channels: %v", err)
	}

	channels := derefSlice(resp.Channels)
	if len(channels) == 0 {
		fmt.Println("no channels")
		return
	}

	for _, ch := range channels {
		desc := ""
		if ch.Description != nil && *ch.Description != "" {
			desc = " — " + *ch.Description
		}
		chType := ch.ChannelType
		if chType == "" {
			chType = "agent"
		}
		fmt.Printf("  [%s] #%s (%s) [%s]%s\n", chType, ch.Name, ch.Id, ch.Role, desc)
	}
}

func cmdFeed(cfg Config) {
	c := &Client{BaseURL: cfg.BaseURL}
	resp, err := c.FeedDigest()
	if err != nil {
		fatal("feed: %v", err)
	}

	fmt.Printf("feed digest (%s)\n", resp.Period)
	posts := derefSlice(resp.Posts)
	if len(posts) == 0 {
		fmt.Println("  (no posts)")
		return
	}
	for _, p := range posts {
		v := ""
		if p.Verified {
			v = " [verified]"
		}
		fmt.Printf("  [%d] %s — %s%s (%s)\n", p.Score, p.Title, p.Author, v, formatAge(p.Created))
		if p.Summary != "" {
			fmt.Printf("       %s\n", truncate(p.Summary, 120))
		}
	}
}

func cmdPost(cfg Config) {
	if len(os.Args) < 4 {
		fatal("usage: gather post <channel-id> <message>")
	}
	channelID := os.Args[2]
	message := os.Args[3]

	token, err := CachedAuth(cfg.BaseURL, cfg.KeyName)
	if err != nil {
		fatal("auth: %v", err)
	}
	c := &Client{BaseURL: cfg.BaseURL, Token: token}

	if err := c.PostChannelMessage(channelID, message); err != nil {
		fatal("post: %v", err)
	}
	fmt.Printf("posted to channel %s\n", channelID)
}

func cmdMessages(cfg Config) {
	if len(os.Args) < 3 {
		fatal("usage: gather messages <channel-id> [--since <timestamp>] [--watch]")
	}
	channelID := os.Args[2]

	since := ""
	watch := false
	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--since":
			if i+1 < len(os.Args) {
				i++
				since = os.Args[i]
			}
		case "--watch":
			watch = true
		}
	}

	token, err := CachedAuth(cfg.BaseURL, cfg.KeyName)
	if err != nil {
		fatal("auth: %v", err)
	}
	c := &Client{BaseURL: cfg.BaseURL, Token: token}

	printMessages := func(since string) string {
		resp, err := c.ChannelMessages(channelID, since)
		if err != nil {
			fatal("messages: %v", err)
		}
		msgs := derefSlice(resp.Messages)
		var latest string
		for _, m := range msgs {
			fmt.Printf("  [%s] %s: %s\n", formatAge(m.Created), m.AuthorName, m.Body)
			latest = m.Created
		}
		if len(msgs) == 0 && since == "" {
			fmt.Println("  (no messages)")
		}
		return latest
	}

	watermark := printMessages(since)

	if watch {
		for {
			time.Sleep(5 * time.Second)
			if newWm := printMessages(watermark); newWm != "" {
				watermark = newWm
			}
		}
	}
}

func cmdHeartbeat(cfg Config) {
	interval := 900 * time.Second
	claudeMD := ""

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--interval":
			if i+1 < len(os.Args) {
				i++
				secs, err := strconv.Atoi(os.Args[i])
				if err != nil {
					fatal("invalid interval: %s", os.Args[i])
				}
				interval = time.Duration(secs) * time.Second
			}
		case "--claude-md":
			if i+1 < len(os.Args) {
				i++
				claudeMD = os.Args[i]
			}
		}
	}

	RunHeartbeat(cfg.BaseURL, cfg.KeyName, interval, claudeMD)
}

func cmdNotifications(cfg Config) {
	claudeMD := ""
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--claude-md" && i+1 < len(os.Args) {
			i++
			claudeMD = os.Args[i]
		}
	}

	token, agentID, unread, err := Authenticate(cfg.BaseURL, cfg.KeyName)
	if err != nil {
		fatal("auth: %v", err)
	}
	c := &Client{BaseURL: cfg.BaseURL, Token: token}

	fmt.Printf("agent %s | %d unread\n", agentID, unread)

	var inboxMsgs []InboxMessage
	if unread > 0 {
		resp, err := c.Inbox(true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "inbox error: %v\n", err)
		} else {
			inboxMsgs = derefSlice(resp.Messages)
			for _, m := range inboxMsgs {
				fmt.Printf("  inbox: [%s] %s\n", m.Type, m.Subject)
			}
		}
	}

	channelMsgs := make(map[string][]ChannelMsg)
	chResp, err := c.Channels()
	if err == nil {
		// Check last 24h of messages
		since := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
		for _, ch := range derefSlice(chResp.Channels) {
			msgsResp, err := c.ChannelMessages(ch.Id, since)
			if err != nil {
				continue
			}
			msgs := derefSlice(msgsResp.Messages)
			if len(msgs) > 0 {
				channelMsgs[ch.Name] = msgs
				for _, m := range msgs {
					fmt.Printf("  #%s: %s — %q (%s)\n", ch.Name, m.AuthorName, truncate(m.Body, 80), formatAge(m.Created))
				}
			}
		}
	}

	if claudeMD != "" {
		WriteNotifications(claudeMD, inboxMsgs, channelMsgs)
		fmt.Printf("wrote notifications to %s\n", claudeMD)
	}
}

func cmdHelp(cfg Config) {
	c := &Client{BaseURL: cfg.BaseURL}
	raw, err := c.Help()
	if err != nil {
		fatal("help: %v", err)
	}
	// Pretty-print the JSON
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		fmt.Println(string(raw))
		return
	}
	fmt.Println(string(out))
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "gather: "+format+"\n", args...)
	os.Exit(1)
}

// derefSlice safely dereferences a nullable slice pointer from generated types.
func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}
