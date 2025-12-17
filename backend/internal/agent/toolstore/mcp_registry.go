package toolstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	listToolID   = "integration.route.list"
	invokeToolID = "integration.route.invoke"
	defaultLimit = 50
	maxLimit     = 200
)

// MCPRegistryOptions 描述构建 MCPRegistry 所需依赖。
type MCPRegistryOptions struct {
	Catalog     *capservice.RegistryService
	Invoker     *capservice.InvocationService
	Router      *router.Service
	TraceRepo   *repo.InvocationTraceRepository
	Clock       func() time.Time
	VersionLock capservice.VersionLock
}

// MCPRegistry 负责将 Capability Registry 暴露为 MCP 工具。
type MCPRegistry struct {
	catalog *capservice.RegistryService
	invoker *capservice.InvocationService
	now     func() time.Time
}

var (
	mcpRegistryMu sync.RWMutex
	globalMCP     *MCPRegistry
)

// NewMCPRegistry 构建 MCPRegistry。
func NewMCPRegistry(opts MCPRegistryOptions) (*MCPRegistry, error) {
	if opts.Catalog == nil {
		return nil, errors.New("mcp registry requires catalog service")
	}
	invoker := opts.Invoker
	if invoker == nil {
		if opts.Router == nil {
			return nil, errors.New("mcp registry requires router service when invoker is nil")
		}
		invoker = capservice.NewInvocationService(capservice.InvocationServiceOptions{
			Catalog:     opts.Catalog,
			Router:      opts.Router,
			TraceRepo:   opts.TraceRepo,
			Clock:       opts.Clock,
			VersionLock: opts.VersionLock,
		})
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &MCPRegistry{
		catalog: opts.Catalog,
		invoker: invoker,
		now:     clock,
	}, nil
}

// SetGlobalMCPRegistry 配置全局 MCPRegistry 实例，供服务器启动时引用。
func SetGlobalMCPRegistry(reg *MCPRegistry) {
	mcpRegistryMu.Lock()
	defer mcpRegistryMu.Unlock()
	globalMCP = reg
}

// GetGlobalMCPRegistry 返回全局 MCPRegistry 实例。
func GetGlobalMCPRegistry() (*MCPRegistry, error) {
	mcpRegistryMu.RLock()
	defer mcpRegistryMu.RUnlock()
	if globalMCP == nil {
		return nil, errors.New("mcp registry is not configured")
	}
	return globalMCP, nil
}

// ListToolSpec 返回注册 list 工具所需的规范。
func (m *MCPRegistry) ListToolSpec() *schemas.ToolSpec {
	return buildListToolSpec()
}

// ListToolHandler 返回 list 工具的处理函数。
func (m *MCPRegistry) ListToolHandler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return m.handleListTool
}

// InvokeToolSpec 返回 invoke 工具规范。
func (m *MCPRegistry) InvokeToolSpec() *schemas.ToolSpec {
	return buildInvokeToolSpec()
}

// InvokeToolHandler 返回 invoke 工具的处理函数。
func (m *MCPRegistry) InvokeToolHandler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return m.handleInvokeTool
}

func (m *MCPRegistry) handleListTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m == nil || m.catalog == nil {
		return mcpErrorResult("capability catalog unavailable"), nil
	}
	args := req.GetArguments()
	tenantUUID, err := canonicalTenant(readStringArg(args, "tenant_uuid"))
	if err != nil || tenantUUID == "" {
		return mcpErrorResult("tenant_uuid is required"), nil
	}

	channel := strings.TrimSpace(readStringArg(args, "protocol"))
	if channel == "" {
		channel = "mcp"
	}

	limit := clampPositive(readIntArg(args, "limit", defaultLimit), defaultLimit, maxLimit)
	offset := readIntArg(args, "offset", 0)

	opts := capservice.CapabilityListOptions{
		PluginID:                 strings.TrimSpace(readStringArg(args, "plugin_id")),
		Intent:                   strings.TrimSpace(readStringArg(args, "intent")),
		ToolScope:                strings.TrimSpace(readStringArg(args, "tool_scope")),
		TenantUUID:               tenantUUID,
		Protocol:                 channel,
		Status:                   []string{"published"},
		Limit:                    limit,
		Offset:                   offset,
		IncludeWorkflowTemplates: false,
		IncludeTotal:             true,
	}
	capabilityID := strings.TrimSpace(readStringArg(args, "capability_id"))
	if capabilityID != "" {
		opts.Search = capabilityID
	}
	if search := strings.TrimSpace(readStringArg(args, "search")); search != "" {
		opts.Search = search
	}

	records, total, err := m.catalog.ListCapabilities(ctx, opts)
	if err != nil {
		return mcpErrorResult(fmt.Sprintf("list capabilities failed: %v", err)), nil
	}

	items := make([]map[string]interface{}, 0, len(records))
	for _, view := range records {
		record := view.Record
		if record == nil {
			continue
		}
		if capabilityID != "" && !strings.EqualFold(record.CapabilityID, capabilityID) {
			continue
		}
		payload := map[string]interface{}{
			"capability_id":     record.CapabilityID,
			"plugin_id":         record.PluginID,
			"plugin_version":    record.PluginVersion,
			"title":             record.Title,
			"description":       strings.TrimSpace(record.Description),
			"intents":           decodeStringArray(record.Intents),
			"tool_scope":        decodeStringArray(record.ToolScope),
			"protocols":         decodeProtocolBindings(record.Protocols),
			"policy":            decodeJSONMap(record.Policy),
			"capabilities_hash": record.CapabilitiesHash,
			"protocol_hash":     record.ProtocolHash,
			"status":            record.Status,
		}
		if record.PublishedAt != nil && !record.PublishedAt.IsZero() {
			payload["published_at"] = record.PublishedAt.UTC().Format(time.RFC3339)
		}
		items = append(items, payload)
	}
	if capabilityID != "" {
		total = int64(len(items))
	}

	body := map[string]interface{}{
		"tenant_uuid":  tenantUUID,
		"protocol":     channel,
		"capabilities": items,
		"total":        total,
		"limit":        limit,
		"offset":       offset,
		"trace_id":     uuid.NewString(),
		"generated_at": m.now().UTC().Format(time.RFC3339),
	}
	return jsonResult(body)
}

func (m *MCPRegistry) handleInvokeTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m == nil || m.invoker == nil {
		return mcpErrorResult("invocation service unavailable"), nil
	}
	args := req.GetArguments()
	tenantUUID, err := canonicalTenant(readStringArg(args, "tenant_uuid"))
	if err != nil || tenantUUID == "" {
		return mcpErrorResult("tenant_uuid is required"), nil
	}

	capabilityID := strings.TrimSpace(readStringArg(args, "capability_id"))
	if capabilityID == "" {
		capabilityID = strings.TrimSpace(readStringArg(args, "route_slug"))
	}
	if capabilityID == "" {
		return mcpErrorResult("capability_id is required"), nil
	}

	payload := readMapArg(args, "payload")
	contextArgs := readMapArg(args, "context")
	preferredProtocol := strings.TrimSpace(readStringArg(args, "preferred_protocol"))
	traceID := strings.TrimSpace(readStringArg(args, "trace_id"))

	result, invokeErr := m.invoker.Invoke(ctx, capservice.InvocationInput{
		CapabilityID:      capabilityID,
		TenantUUID:        tenantUUID,
		PreferredProtocol: preferredProtocol,
		IdempotencyKey:    strings.TrimSpace(readStringArg(args, "idempotency_key")),
		TraceID:           traceID,
		Payload:           payload,
		Context:           contextArgs,
	})

	resp := map[string]interface{}{
		"tenant_uuid":     tenantUUID,
		"capability_id":   capabilityID,
		"status":          result.Status,
		"trace_id":        result.TraceID,
		"protocol_used":   result.ProtocolUsed,
		"fallback_used":   result.FallbackUsed,
		"preferred_proto": preferredProtocol,
	}
	if result.Result != nil {
		resp["result"] = result.Result
	}
	if invokeErr != nil {
		resp["status"] = "failed"
		resp["error_message"] = invokeErr.Error()
	}

	return jsonResult(resp)
}

func buildListToolSpec() *schemas.ToolSpec {
	return &schemas.ToolSpec{
		ID:          listToolID,
		Name:        "List Published Capabilities",
		Description: "列举指定租户可用且启用 MCP 通道的能力条目，直接来源于 Capability Registry 缓存。",
		Version:     "v1",
		AllowedRoles: []string{
			"agent",
			"system",
		},
		InputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"tenant_uuid": {
					Type:        "string",
					Description: "租户 UUID（必填，用于策略过滤）",
				},
				"plugin_id": {
					Type:        "string",
					Description: "按插件过滤（可选）",
				},
				"intent": {
					Type:        "string",
					Description: "按 intent 过滤（可选）",
				},
				"tool_scope": {
					Type:        "string",
					Description: "按 tool_scope 过滤（可选）",
				},
				"protocol": {
					Type:        "string",
					Description: "协议通道（默认 mcp）",
				},
				"capability_id": {
					Type:        "string",
					Description: "按能力 ID 精确过滤（可选）",
				},
				"limit": {
					Type:        "integer",
					Description: "分页大小，默认 50",
				},
				"offset": {
					Type:        "integer",
					Description: "分页偏移，默认 0",
				},
			},
			Required: []string{"tenant_uuid"},
		},
		OutputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"capabilities": {Type: "array"},
				"tenant_uuid":  {Type: "string"},
				"protocol":     {Type: "string"},
				"trace_id":     {Type: "string"},
			},
		},
	}
}

func buildInvokeToolSpec() *schemas.ToolSpec {
	return &schemas.ToolSpec{
		ID:          invokeToolID,
		Name:        "Invoke Capability via Registry",
		Description: "基于 Capability Registry 与 Selector 触发能力调用，并返回标准化追踪结果。",
		Version:     "v1",
		AllowedRoles: []string{
			"agent",
			"system",
		},
		InputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"tenant_uuid": {
					Type:        "string",
					Description: "租户 UUID",
				},
				"capability_id": {
					Type:        "string",
					Description: "要调用的能力 ID",
				},
				"payload": {
					Type:        "object",
					Description: "能力请求体（JSON）",
				},
				"idempotency_key": {
					Type:        "string",
					Description: "幂等键（可选）",
				},
				"preferred_protocol": {
					Type:        "string",
					Description: "期望协议（可选）",
				},
				"context": {
					Type:        "object",
					Description: "扩展上下文（可选）",
				},
			},
			Required: []string{"tenant_uuid", "capability_id"},
		},
		OutputSchema: &schemas.JSONSchema{
			Type: "object",
			Properties: map[string]*schemas.JSONSchema{
				"status":        {Type: "string"},
				"trace_id":      {Type: "string"},
				"protocol_used": {Type: "string"},
				"result":        {Type: "object"},
			},
		},
	}
}

func canonicalTenant(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("tenant uuid missing")
	}
	return reqctx.CanonicalTenantUUID(raw)
}

func readStringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func readIntArg(args map[string]interface{}, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	value, ok := args[key]
	if !ok || value == nil {
		return fallback
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	case string:
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			if i, err := strconv.Atoi(trimmed); err == nil {
				return i
			}
		}
	}
	return fallback
}

func readMapArg(args map[string]interface{}, key string) map[string]interface{} {
	if args == nil {
		return map[string]interface{}{}
	}
	value, _ := args[key]
	if value == nil {
		return map[string]interface{}{}
	}
	if out, ok := value.(map[string]interface{}); ok {
		return out
	}
	return map[string]interface{}{}
}

func clampPositive(value, min, max int) int {
	if value <= 0 {
		value = min
	}
	if value > max {
		value = max
	}
	return value
}

func decodeStringArray(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func decodeProtocolBindings(data []byte) []models.ProtocolBinding {
	if len(data) == 0 {
		return nil
	}
	var bindings []models.ProtocolBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil
	}
	return bindings
}

func decodeJSONMap(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func jsonResult(body map[string]interface{}) (*mcp.CallToolResult, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return mcpErrorResult(fmt.Sprintf("marshal response failed: %v", err)), nil
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

func mcpErrorResult(message string) *mcp.CallToolResult {
	if strings.TrimSpace(message) == "" {
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
