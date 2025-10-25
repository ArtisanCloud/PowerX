package integration_gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	igdeps "github.com/ArtisanCloud/PowerX/internal/server/mcp/tools/integration_gateway/deps"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"github.com/mark3labs/mcp-go/mcp"
)

func listRoutesTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	deps, err := igdeps.Get()
	if err != nil {
		return nil, err
	}

	args := request.GetArguments()
	ctx, tenantID, _, traceID, err := deps.Adapter.PrepareContext(ctx, args)
	if err != nil {
		return toolErrorResult(err.Error()), nil
	}

	capabilityID := readOptionalString(args, "capability_id")
	channel := readOptionalString(args, "channel")
	if channel == "" {
		channel = "mcp"
	}

	routes, err := deps.TenantService.ListRoutes(ctx, tenantID, capabilityID, channel)
	if err != nil {
		return toolErrorResult(fmt.Sprintf("list routes failed: %v", err)), nil
	}

	items := make([]map[string]interface{}, 0, len(routes))
	for _, route := range routes {
		items = append(items, map[string]interface{}{
			"route_id":        route.RouteID.String(),
			"route_slug":      route.RouteSlug,
			"tenant_id":       route.TenantID,
			"capability_id":   route.CapabilityID,
			"tool_grant_ids":  route.ToolGrantIDs,
			"channels":        route.Channels,
			"rate_limit":      rateLimitToMap(route.RateLimit),
			"event_topics":    route.EventTopics,
			"lifecycle_state": route.LifecycleState,
			"status":          route.Status,
			"updated_at":      route.UpdatedAt.Format(time.RFC3339),
		})
	}

	body := map[string]interface{}{
		"routes":    items,
		"trace_id":  traceID,
		"tenant_id": tenantID,
	}
	if capabilityID != "" {
		body["capability_filter"] = capabilityID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return toolErrorResult(fmt.Sprintf("marshal result failed: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(payload),
			},
		},
	}, nil
}

func readOptionalString(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func rateLimitToMap(policy manager.RateLimitPolicy) map[string]interface{} {
	return map[string]interface{}{
		"limit":          policy.Limit,
		"burst":          policy.Burst,
		"window_seconds": policy.WindowSeconds,
		"scope":          policy.Scope,
	}
}
