package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const notifyHeader = "## Gather Notifications"
const notifyMarker = "<!-- Auto-updated by gather-cli."

// WriteNotifications finds or creates the ## Gather Notifications section in a
// CLAUDE.md file and replaces its content with current notifications. The rest
// of the file is untouched.
func WriteNotifications(claudeMDPath string, inbox []InboxMessage, channelMsgs map[string][]ChannelMsg) {
	// Build notification lines
	var lines []string
	for name, msgs := range channelMsgs {
		for _, m := range msgs {
			age := formatAge(m.Created)
			lines = append(lines, fmt.Sprintf("- [%s] #%s: %s — %q", age, name, m.AuthorName, truncate(m.Body, 100)))
		}
	}
	for _, m := range inbox {
		age := formatAge(m.Created)
		lines = append(lines, fmt.Sprintf("- [%s] inbox: %s", age, m.Subject))
	}

	if len(lines) == 0 {
		lines = append(lines, "- No new notifications.")
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	section := notifyHeader + "\n" +
		fmt.Sprintf("%s Last check: %s -->\n", notifyMarker, timestamp) +
		strings.Join(lines, "\n") + "\n"

	// Read existing file
	existing, err := os.ReadFile(claudeMDPath)
	if err != nil {
		// File doesn't exist — create it with just the section
		os.WriteFile(claudeMDPath, []byte(section), 0644)
		return
	}

	content := string(existing)

	// Find existing section and replace it
	idx := strings.Index(content, notifyHeader)
	if idx >= 0 {
		// Find the end of the section: next ## heading or EOF
		rest := content[idx+len(notifyHeader):]
		endIdx := strings.Index(rest, "\n## ")
		if endIdx >= 0 {
			// Replace section, keep everything after
			content = content[:idx] + section + rest[endIdx+1:]
		} else {
			// Section goes to EOF
			content = content[:idx] + section
		}
	} else {
		// Append section at end
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + section
	}

	os.WriteFile(claudeMDPath, []byte(content), 0644)
}
