package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
)

// Adapter 提供 MCP 工具与租户服务之间的上下文适配。
type Adapter struct {
	inst *instrumentation.Instrumentation
}

// AdapterOptions 描述构造 Adapter 所需依赖。
type AdapterOptions struct {
	Instrumentation *instrumentation.Instrumentation
}

// NewAdapter 构造上下文适配器，默认使用空实现的观测器。
func NewAdapter(opts AdapterOptions) *Adapter {
	inst := opts.Instrumentation
	if inst == nil {
		inst = instrumentation.NewInstrumentation(nil)
	}
	return &Adapter{inst: inst}
}

// PrepareContext 从工具参数中提取租户、调用者等信息，并构造带 trace 的上下文。
func (a *Adapter) PrepareContext(ctx context.Context, args map[string]interface{}) (context.Context, string, string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	tenantID := readStringArg(args, "tenant_id")
	if tenantID == "" {
		return ctx, "", "", "", errors.New("tenant_id is required")
	}

	actor := readStringArg(args, "actor")
	if actor == "" {
		actor = "mcp-client"
	}

	traceInput := readStringArg(args, "trace_id")
	if traceInput != "" {
		ctx = context.WithValue(ctx, "trace_id", traceInput)
	}

	ctx = instrumentation.WithTenant(ctx, tenantID)
	ctx, traceID := instrumentation.EnsureTraceContext(ctx)

	return ctx, tenantID, actor, traceID, nil
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
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
