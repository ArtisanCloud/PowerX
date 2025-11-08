package integration_gateway

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

func toolErrorResult(message string) *mcp.CallToolResult {
	if message == "" {
		message = "unknown error"
	}
	body, _ := json.Marshal(map[string]string{
		"status":  "error",
		"message": message,
	})
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(body),
			},
		},
	}
}
