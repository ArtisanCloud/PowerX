package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/ArtisanCloud/PowerX/internal/eventbus"
	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// CatalogSyncEvent 表示 Capability Sync 过程中广播的事件负载。
type CatalogSyncEvent struct {
	JobID         string `json:"job_id"`
	PluginID      string `json:"plugin_id"`
	PluginName    string `json:"plugin_name"`
	PluginVersion string `json:"plugin_version"`
	CapabilityID  string `json:"capability_id"`
	HashBefore    string `json:"hash_before,omitempty"`
	HashAfter     string `json:"hash_after,omitempty"`
}

// InvocationAuditInput 聚合一次调用的审计所需字段。
type InvocationAuditInput struct {
	TraceID           string
	TenantUUID        string
	PluginID          string
	CapabilityID      string
	PreferredProtocol string
	ProtocolUsed      string
	FallbackUsed      bool
	Status            string
	IdempotencyKey    string
	RequestPayload    map[string]interface{}
	ResponsePayload   map[string]interface{}
	ErrorSummary      string
	Latency           time.Duration
}

// AuditService 负责 Capability Registry 相关的事件、审计与指标上报。
type AuditService struct {
	jobs    *repo.CapabilitySyncJobRepository
	events  *repo.CapabilityEventPublicationRepository
	traces  *repo.InvocationTraceRepository
	bus     event_bus.EventBus
	auditor auditpkg.Auditor
	metrics *capmetrics.CapabilityRegistryMetrics
	now     func() time.Time
}

// AuditServiceOptions 配置 AuditService。
type AuditServiceOptions struct {
	JobRepo   *repo.CapabilitySyncJobRepository
	EventRepo *repo.CapabilityEventPublicationRepository
	TraceRepo *repo.InvocationTraceRepository
	EventBus  event_bus.EventBus
	Auditor   auditpkg.Auditor
	Metrics   *capmetrics.CapabilityRegistryMetrics
	Clock     func() time.Time
}

// NewAuditService 构造 AuditService 实例。
func NewAuditService(opts AuditServiceOptions) *AuditService {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = capmetrics.NewCapabilityRegistryMetrics(nil)
	}
	return &AuditService{
		jobs:    opts.JobRepo,
		events:  opts.EventRepo,
		traces:  opts.TraceRepo,
		bus:     opts.EventBus,
		auditor: opts.Auditor,
		metrics: opts.Metrics,
		now:     clock,
	}
}

// PublishCatalogEvent 广播 Capability Sync 相关事件。
func (a *AuditService) PublishCatalogEvent(ctx context.Context, topic string, event CatalogSyncEvent) {
	if a == nil || a.bus == nil || strings.TrimSpace(topic) == "" {
		return
	}
	a.bus.Publish(topic, event, ctx)
	if a.auditor != nil {
		a.auditor.LogBusPublish(ctx, topic, 1)
	}
}

// RecordInvocation 记录调用 Trace、写入事件并打点。
func (a *AuditService) RecordInvocation(ctx context.Context, input InvocationAuditInput) {
	if a == nil {
		return
	}

	traceErr := a.insertInvocationTrace(ctx, input)

	topic := chooseInvocationTopic(input.Status)
	eventPayload := a.buildInvocationPayload(input)
	payloadBytes, _ := json.Marshal(eventPayload)

	var publicationID uuid.UUID
	if a.events != nil && len(payloadBytes) > 0 {
		model := &models.CapabilityEventPublication{
			TenantUUID: input.TenantUUID,
			Topic:      topic,
			Payload:    datatypes.JSON(payloadBytes),
			Status:     "pending",
			TraceID:    input.TraceID,
		}
		if saved, err := a.events.Create(ctx, model); err == nil {
			publicationID = saved.UUID
		}
	}

	if a.bus != nil {
		a.bus.Publish(topic, eventPayload, ctx)
		if a.auditor != nil {
			a.auditor.LogBusPublish(ctx, topic, 1)
		}
		if input.FallbackUsed {
			fallbackPayload := map[string]any{
				"capability_id": input.CapabilityID,
				"plugin_id":     input.PluginID,
				"tenant_uuid":   input.TenantUUID,
				"trace_id":      input.TraceID,
				"protocol":      input.ProtocolUsed,
				"latency_ms":    input.Latency.Milliseconds(),
			}
			a.bus.Publish(eventbus.TopicIntegrationGatewayInvocationFallback, fallbackPayload, ctx)
			if a.auditor != nil {
				a.auditor.LogBusPublish(ctx, eventbus.TopicIntegrationGatewayInvocationFallback, 1)
			}
		}
	}

	if publicationID != uuid.Nil && a.traces != nil {
		_ = a.traces.MarkEventPublished(ctx, input.TraceID, publicationID)
	}

	var metricErr error
	if input.ErrorSummary != "" {
		metricErr = errors.New(input.ErrorSummary)
	} else if traceErr != nil {
		metricErr = traceErr
	}
	a.emitInvocationMetric(ctx, input, metricErr)
}

func (a *AuditService) insertInvocationTrace(ctx context.Context, input InvocationAuditInput) error {
	if a == nil || a.traces == nil {
		return nil
	}
	payload := &models.InvocationTrace{
		TraceID:           input.TraceID,
		TenantUUID:        input.TenantUUID,
		PluginID:          input.PluginID,
		CapabilityID:      input.CapabilityID,
		PreferredProtocol: strings.TrimSpace(input.PreferredProtocol),
		ProtocolUsed:      strings.TrimSpace(input.ProtocolUsed),
		FallbackUsed:      input.FallbackUsed,
		Status:            strings.TrimSpace(input.Status),
		IdempotencyKey:    strings.TrimSpace(input.IdempotencyKey),
		RequestPayload:    marshalJSONValue(input.RequestPayload),
		ResponsePayload:   marshalJSONValue(input.ResponsePayload),
		ErrorSummary:      input.ErrorSummary,
		LatencyMS:         int(input.Latency / time.Millisecond),
	}
	if _, err := a.traces.Create(ctx, payload); err != nil {
		return err
	}
	return nil
}

func (a *AuditService) buildInvocationPayload(input InvocationAuditInput) map[string]any {
	status := normalizeStatus(input.Status)
	payload := map[string]any{
		"capability_id":      input.CapabilityID,
		"plugin_id":          input.PluginID,
		"tenant_uuid":        input.TenantUUID,
		"preferred_protocol": strings.TrimSpace(input.PreferredProtocol),
		"protocol_used":      strings.TrimSpace(input.ProtocolUsed),
		"fallback_used":      input.FallbackUsed,
		"trace_id":           input.TraceID,
		"latency_ms":         input.Latency.Milliseconds(),
		"status":             status,
	}
	if input.IdempotencyKey != "" {
		payload["idempotency_key"] = input.IdempotencyKey
	}
	if input.ErrorSummary != "" {
		payload["error"] = input.ErrorSummary
	}
	return payload
}

func (a *AuditService) emitInvocationMetric(ctx context.Context, input InvocationAuditInput, err error) {
	if a == nil || a.metrics == nil {
		return
	}
	result := capmetrics.ResultSuccess
	switch normalizeStatus(input.Status) {
	case "failed", "error", "denied":
		result = capmetrics.ResultFailed
	case "fallback":
		result = capmetrics.ResultFallback
	}
	sample := capmetrics.CapabilityInvocationSample{
		Labels: capmetrics.CapabilityInvocationLabels{
			CapabilityID: input.CapabilityID,
			PluginID:     input.PluginID,
			Protocol:     strings.TrimSpace(input.ProtocolUsed),
			TenantUUID:   input.TenantUUID,
			TraceID:      input.TraceID,
			Result:       result,
			Fallback:     input.FallbackUsed,
		},
		Latency: input.Latency,
		Err:     err,
	}
	a.metrics.ObserveInvocation(ctx, sample)
}

func chooseInvocationTopic(status string) string {
	status = normalizeStatus(status)
	if status == "failed" || status == "error" || status == "denied" {
		return eventbus.TopicIntegrationGatewayInvocationFailed
	}
	return eventbus.TopicIntegrationGatewayInvocationSucceeded
}

func normalizeStatus(status string) string {
	trimmed := strings.TrimSpace(strings.ToLower(status))
	if trimmed == "" {
		return "completed"
	}
	return trimmed
}

func marshalJSONValue(value interface{}) datatypes.JSON {
	if value == nil {
		return datatypes.JSON([]byte("null"))
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("null"))
	}
	return datatypes.JSON(bytes)
}
