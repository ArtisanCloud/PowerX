package integration_gateway

import (
	"errors"

	"github.com/ArtisanCloud/PowerX/internal/server/mcp/register"
	igdeps "github.com/ArtisanCloud/PowerX/internal/server/mcp/tools/integration_gateway/deps"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

type ToolDependencies = igdeps.ToolDependencies

const (
	listToolID   = "integration.route.list"
	invokeToolID = "integration.route.invoke"
)

// RegisterToolsWithRegistry 将 Integration Gateway 相关工具注册到 MCP 工具表。
func RegisterToolsWithRegistry(reg *register.ToolRegistry, deps ToolDependencies) error {
	if reg == nil {
		return errors.New("tool registry is required")
	}

	if err := igdeps.Set(deps); err != nil {
		return err
	}

	if err := reg.RegisterToolWithSpec(buildListToolSpec(), listRoutesTool); err != nil {
		return err
	}
	reg.RegisterToolHandler(listToolID, listRoutesTool)

	if err := reg.RegisterToolWithSpec(buildInvokeToolSpec(), invokeRouteTool); err != nil {
		return err
	}
	reg.RegisterToolHandler(invokeToolID, invokeRouteTool)

	return nil
}

func buildListToolSpec() *schemas.ToolSpec {
	return &schemas.ToolSpec{
		ID:          listToolID,
		Name:        "List Integration Routes",
		Description: "列举当前租户可用的集成网关路由（仅包含启用 MCP 通道的路由）。",
		Version:     "v1",
		AllowedRoles: []string{
			"agent",
			"system",
		},
		InputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"tenant_id": {
					Type:        "string",
					Description: "租户 ID，用于过滤租户可见的路由",
				},
				"capability_id": {
					Type:        "string",
					Description: "按能力 ID 过滤（可选）",
				},
				"channel": {
					Type:        "string",
					Description: "过滤通道（默认 mcp）",
					Enum:        []interface{}{"http", "mcp"},
				},
			},
			Required: []string{"tenant_id"},
		},
		OutputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"routes": {
					Type: "array",
					Items: &schemas.JSONSchema{
						Type: "object",
						Properties: map[string]*schemas.JSONSchema{
							"route_id":       {Type: "string"},
							"route_slug":     {Type: "string"},
							"capability_id":  {Type: "string"},
							"channels":       {Type: "array"},
							"tool_grant_ids": {Type: "array"},
							"status":         {Type: "string"},
						},
					},
				},
				"trace_id": {Type: "string"},
			},
			Required: []string{"routes", "trace_id"},
		},
	}
}

func buildInvokeToolSpec() *schemas.ToolSpec {
	return &schemas.ToolSpec{
		ID:          invokeToolID,
		Name:        "Invoke Integration Route",
		Description: "通过集成网关路由触发能力调用，并返回标准化响应与追踪信息。",
		Version:     "v1",
		AllowedRoles: []string{
			"agent",
			"system",
		},
		InputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"tenant_id": {
					Type:        "string",
					Description: "租户 ID",
				},
				"route_slug": {
					Type:        "string",
					Description: "集成路由别名",
				},
				"payload": {
					Type:        "object",
					Description: "透传给能力的 JSON 请求体",
				},
				"idempotency_key": {
					Type:        "string",
					Description: "幂等键（可选）",
				},
				"context": {
					Type:        "object",
					Description: "附加上下文信息（可选）",
				},
			},
			Required: []string{"tenant_id", "route_slug"},
		},
		OutputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"status":               {Type: "string"},
				"result":               {Type: "object"},
				"routed_capability_id": {Type: "string"},
				"routed_adapter":       {Type: "string"},
				"error_code":           {Type: "string"},
				"error_message":        {Type: "string"},
				"trace_id":             {Type: "string"},
			},
			Required: []string{"status", "trace_id"},
		},
	}
}
