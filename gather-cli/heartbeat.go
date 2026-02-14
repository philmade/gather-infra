package main

import (
	"fmt"
	"time"
)

// RunHeartbeat runs the auth → check → sleep loop.
func RunHeartbeat(baseURL, keyName string, interval time.Duration, claudeMD string) {
	fmt.Printf("heartbeat: starting (interval %s, key %q)\n", interval, keyName)
	if claudeMD != "" {
		fmt.Printf("heartbeat: will write notifications to %s\n", claudeMD)
	}

	lastCheck := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	for {
		now := time.Now().Format("15:04")
		token, agentID, unread, err := Authenticate(baseURL, keyName)
		if err != nil {
			fmt.Printf("[%s] auth FAILED: %v\n", now, err)
			fmt.Printf("[%s] sleeping %s before retry\n", now, interval)
			time.Sleep(interval)
			continue
		}

		c := &Client{BaseURL: baseURL, Token: token}

		var summary []string
		summary = append(summary, fmt.Sprintf("auth ok (agent %s)", agentID))
		summary = append(summary, fmt.Sprintf("%d unread", unread))

		// Fetch inbox if there are unread messages
		var inboxMsgs []InboxMessage
		if unread > 0 {
			resp, err := c.Inbox(true)
			if err != nil {
				fmt.Printf("[%s] inbox error: %v\n", now, err)
			} else if resp.Messages != nil {
				inboxMsgs = *resp.Messages
				for _, m := range inboxMsgs {
					fmt.Printf("  inbox: [%s] %s\n", m.Type, m.Subject)
				}
			}
		}

		// Fetch channels and new messages
		channelMsgs := make(map[string][]ChannelMsg)
		chResp, err := c.Channels()
		if err != nil {
			fmt.Printf("[%s] channels error: %v\n", now, err)
		} else if chResp.Channels != nil {
			newMsgCount := 0
			for _, ch := range *chResp.Channels {
				msgs, err := c.ChannelMessages(ch.Id, lastCheck)
				if err != nil || msgs.Messages == nil {
					continue
				}
				if len(*msgs.Messages) > 0 {
					channelMsgs[ch.Name] = *msgs.Messages
					newMsgCount += len(*msgs.Messages)
					for _, m := range *msgs.Messages {
						age := formatAge(m.Created)
						fmt.Printf("  #%s: %s — %q (%s)\n", ch.Name, m.AuthorName, truncate(m.Body, 80), age)
					}
				}
			}
			if newMsgCount > 0 {
				summary = append(summary, fmt.Sprintf("%d channel msgs", newMsgCount))
			}
		}

		// Write notifications to CLAUDE.md if requested
		if claudeMD != "" {
			WriteNotifications(claudeMD, inboxMsgs, channelMsgs)
		}

		fmt.Printf("[%s] %s\n", now, joinParts(summary))

		lastCheck = time.Now().UTC().Format(time.RFC3339)
		time.Sleep(interval)
	}
}

func joinParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " | "
		}
		result += p
	}
	return result
}

func formatAge(created string) string {
	t, err := time.Parse("2006-01-02 15:04:05.000Z", created)
	if err != nil {
		t, err = time.Parse(time.RFC3339, created)
		if err != nil {
			return created
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
