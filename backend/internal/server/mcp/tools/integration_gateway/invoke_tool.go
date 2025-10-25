package integration_gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	igdeps "github.com/ArtisanCloud/PowerX/internal/server/mcp/tools/integration_gateway/deps"
	tenantservice "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"github.com/mark3labs/mcp-go/mcp"
)

func invokeRouteTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	deps, err := igdeps.Get()
	if err != nil {
		return nil, err
	}

	args := request.GetArguments()
	ctx, tenantID, actor, traceID, err := deps.Adapter.PrepareContext(ctx, args)
	if err != nil {
		return toolErrorResult(err.Error()), nil
	}

	routeSlug := readOptionalString(args, "route_slug")
	if routeSlug == "" {
		return toolErrorResult("route_slug is required"), nil
	}

	payload, ok := args["payload"].(map[string]interface{})
	if !ok {
		payload = map[string]interface{}{}
	}

	contextArgs, _ := args["context"].(map[string]interface{})
	input := tenantservice.InvokeInput{
		TenantID:       tenantID,
		RouteSlug:      routeSlug,
		Channel:        "mcp",
		Payload:        payload,
		Context:        contextArgs,
		IdempotencyKey: readOptionalString(args, "idempotency_key"),
		TraceID:        traceID,
		Actor:          actor,
	}

	result, invokeErr := deps.TenantService.Invoke(ctx, input)
	response := map[string]interface{}{
		"status":               string(result.Status),
		"routed_capability_id": result.RoutedCapabilityID,
		"routed_adapter":       result.RoutedAdapter,
		"result":               result.Result,
		"trace_id":             result.TraceID,
		"duration_ms":          result.Duration.Milliseconds(),
	}

	if result.RateLimit != nil {
		response["rate_limit"] = map[string]interface{}{
			"scope":       result.RateLimit.Scope,
			"retry_after": result.RateLimit.RetryAfter.String(),
			"remaining":   result.RateLimit.Remaining,
		}
	}
	if result.DispatchedAt.IsZero() {
		response["dispatched_at"] = time.Now().UTC().Format(time.RFC3339)
	} else {
		response["dispatched_at"] = result.DispatchedAt.Format(time.RFC3339)
	}
	if result.ErrorCode != "" {
		response["error_code"] = result.ErrorCode
	}
	if result.ErrorMessage != "" {
		response["error_message"] = result.ErrorMessage
	}
	if invokeErr != nil && result.ErrorMessage == "" {
		response["error_message"] = invokeErr.Error()
	}

	payloadBytes, err := json.Marshal(response)
	if err != nil {
		return toolErrorResult(fmt.Sprintf("marshal result failed: %v", err)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(payloadBytes),
			},
		},
	}, nil
}
