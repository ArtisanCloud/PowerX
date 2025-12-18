package engine

import (
	"context"
	"errors"
	"strings"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
)

// selectorInvoker describes the subset of Selector behavior used by workflow engine.
type selectorInvoker interface {
	Invoke(ctx context.Context, req capservice.CapabilityInvokeRequest) (capservice.CapabilityInvokeResponse, error)
}

// CapabilityStepAdapter 将 Workflow 节点与 Capability Selector 连接。
type CapabilityStepAdapter struct {
	selector  selectorInvoker
	telemetry executionTelemetry
}

type executionTelemetry interface {
	ObserveWorkflowExecution(ctx context.Context, input WorkflowExecutionTelemetryInput)
}

// CapabilityStepInput 描述一次节点执行所需的上下文。
type CapabilityStepInput struct {
	CapabilityID          string
	TenantUUID            string
	Intent                string
	ToolScope             string
	ToolGrantIDs          []string
	PreferredProtocol     string
	IdempotencyKey        string
	TraceID               string
	TemplateID            string
	RequiresManualUpgrade bool
	Payload               map[string]interface{}
	Context               map[string]interface{}
}

// NewCapabilityStepAdapter 创建适配器实例。
func NewCapabilityStepAdapter(invoker selectorInvoker, telemetry executionTelemetry) *CapabilityStepAdapter {
	if invoker == nil {
		return nil
	}
	return &CapabilityStepAdapter{
		selector:  invoker,
		telemetry: telemetry,
	}
}

// InvokeCapability 将节点配置转换为 CapabilityInvokeRequest 并调用 Selector。
func (a *CapabilityStepAdapter) InvokeCapability(ctx context.Context, input CapabilityStepInput) (capservice.CapabilityInvokeResponse, error) {
	var resp capservice.CapabilityInvokeResponse
	if a == nil || a.selector == nil {
		return resp, errors.New("capability step adapter unavailable")
	}
	capabilityID := strings.TrimSpace(input.CapabilityID)
	tenantUUID := strings.TrimSpace(input.TenantUUID)
	if capabilityID == "" || tenantUUID == "" {
		return resp, errors.New("capability_id and tenant_uuid are required")
	}

	req := capservice.CapabilityInvokeRequest{
		CapabilityID:      capabilityID,
		TenantUUID:        tenantUUID,
		Intent:            strings.TrimSpace(input.Intent),
		ToolScope:         strings.TrimSpace(input.ToolScope),
		ToolGrantIDs:      normalizeStrings(input.ToolGrantIDs),
		PreferredProtocol: strings.TrimSpace(input.PreferredProtocol),
		IdempotencyKey:    strings.TrimSpace(input.IdempotencyKey),
		TraceID:           strings.TrimSpace(input.TraceID),
		Payload:           cloneMap(input.Payload),
		Context:           cloneMap(input.Context),
	}

	resp, err := a.selector.Invoke(ctx, req)
	a.reportExecution(ctx, input, resp, err)
	return resp, err
}

func (a *CapabilityStepAdapter) reportExecution(ctx context.Context, input CapabilityStepInput, resp capservice.CapabilityInvokeResponse, invokeErr error) {
	if a == nil || a.telemetry == nil {
		return
	}
	protocol := strings.TrimSpace(resp.ProtocolUsed)
	if protocol == "" {
		protocol = strings.TrimSpace(input.PreferredProtocol)
	}
	status := strings.TrimSpace(resp.Status)
	if status == "" {
		if invokeErr != nil {
			status = "failed"
		} else {
			status = "completed"
		}
	}
	a.telemetry.ObserveWorkflowExecution(ctx, WorkflowExecutionTelemetryInput{
		CapabilityID: strings.TrimSpace(input.CapabilityID),
		TemplateID:   strings.TrimSpace(input.TemplateID),
		TenantUUID:   strings.TrimSpace(input.TenantUUID),
		Protocol:     protocol,
		Status:       status,
		NeedsUpgrade: input.RequiresManualUpgrade,
		Err:          invokeErr,
	})
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, val := range values {
		if trimmed := strings.TrimSpace(val); trimmed != "" {
			key := strings.ToLower(trimmed)
			if _, exists := set[key]; exists {
				continue
			}
			set[key] = struct{}{}
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
