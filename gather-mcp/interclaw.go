package main

// RegisterInterClawTools adds inter-claw communication tools to the registry.
func RegisterInterClawTools(reg *Registry) {
	reg.Register(&Tool{
		ID:          "peer.message",
		Category:    "peer",
		Name:        "peer.message",
		Description: "Send a message to another claw by name",
		Params: []ToolParam{
			{Name: "claw", Type: "string", Required: true, Description: "Target claw name (e.g. 'webclawman')"},
			{Name: "text", Type: "string", Required: true, Description: "Message text to send"},
		},
		Source: "interclaw",
	})
	reg.Register(&Tool{
		ID:          "peer.page",
		Category:    "peer",
		Name:        "peer.page",
		Description: "Fetch another claw's public web page",
		Params: []ToolParam{
			{Name: "claw", Type: "string", Required: true, Description: "Target claw name"},
		},
		Source: "interclaw",
	})
}
