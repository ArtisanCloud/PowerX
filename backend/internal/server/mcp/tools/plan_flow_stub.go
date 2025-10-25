package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// PlanFlowTool 暂未实现，放置占位实现以保证编译通过。
func PlanFlowTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: "plan_flow tool not implemented"},
		},
	}, fmt.Errorf("plan_flow tool not implemented")
}
