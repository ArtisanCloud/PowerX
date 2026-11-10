package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	auditpayload "github.com/ArtisanCloud/PowerX/pkg/corex/audit/payload"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
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
	admission    func(context.Context) error
	admissionMu  sync.Mutex
	lastRejected time.Time
	jobs         *repo.CapabilitySyncJobRepository
	events       *repo.CapabilityEventPublicationRepository
	traces       *repo.InvocationTraceRepository
	bus          event_bus.EventBus
	auditor      auditpkg.Auditor
	metrics      *capmetrics.CapabilityRegistryMetrics
	now          func() time.Time
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
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	// 仅修改值副本中的错误摘要；业务调用返回和原始 map 保持原样。
	input.ErrorSummary = auditpayload.SafeText(input.ErrorSummary)

	invocationUUID, traceErr := a.insertInvocationTrace(ctx, input)

	topic := chooseInvocationTopic(input.Status)
	eventPayload := a.buildInvocationPayload(input)
	payloadBytes, _ := json.Marshal(eventPayload)

	var publicationID uuid.UUID
	if a.events != nil && len(payloadBytes) > 0 {
		model := &models.CapabilityEventPublication{
			RecordClass: models.InvocationNotificationClass,
			TenantUUID:  input.TenantUUID,
			Topic:       topic,
			Payload:     datatypes.JSON(payloadBytes),
			Status:      "pending",
			TraceID:     input.TraceID,
		}
		if saved, err := a.events.Create(ctx, model); err == nil {
			publicationID = saved.UUID
		}
	}

	publicationStatus, receipt := "unconfirmed", ""
	if a.bus != nil {
		if publisher, ok := a.bus.(event_bus.AcknowledgedPublisher); ok {
			ack, err := publisher.PublishAcknowledged(topic, eventPayload, ctx)
			if err != nil {
				publicationStatus = "failed"
			} else {
				publicationStatus, receipt = ack.Status, ack.EventID
			}
		} else {
			a.bus.Publish(topic, eventPayload, ctx)
		}
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
	if publicationID != uuid.Nil && a.events != nil {
		_ = a.events.MarkPublicationResult(ctx, input.TenantUUID, publicationID, publicationStatus, receipt, a.now().UTC())
	}

	if publicationID != uuid.Nil && invocationUUID != uuid.Nil && a.traces != nil {
		_ = a.traces.LinkPublication(ctx, input.TenantUUID, invocationUUID, publicationID, publicationStatus == "published" || publicationStatus == "dispatched")
	}

	var metricErr error
	if input.ErrorSummary != "" {
		metricErr = errors.New(input.ErrorSummary)
	} else if traceErr != nil {
		metricErr = traceErr
	}
	a.emitInvocationMetric(ctx, input, metricErr)
}

func (a *AuditService) insertInvocationTrace(ctx context.Context, input InvocationAuditInput) (uuid.UUID, error) {
	if a == nil || a.traces == nil {
		return uuid.Nil, nil
	}
	request, response, summarized := baselineAuditSummary(input)
	if !summarized {
		request, response = auditpayload.Snapshot(input.RequestPayload), auditpayload.Snapshot(input.ResponsePayload)
	}
	payload := &models.InvocationTrace{
		CallerSubject:     callerSubject(ctx),
		TraceID:           input.TraceID,
		TenantUUID:        input.TenantUUID,
		PluginID:          input.PluginID,
		CapabilityID:      input.CapabilityID,
		PreferredProtocol: strings.TrimSpace(input.PreferredProtocol),
		ProtocolUsed:      strings.TrimSpace(input.ProtocolUsed),
		FallbackUsed:      input.FallbackUsed,
		Status:            strings.TrimSpace(input.Status),
		IdempotencyKey:    strings.TrimSpace(input.IdempotencyKey),
		RequestPayload:    datatypes.JSON(request),
		ResponsePayload:   datatypes.JSON(response),
		ErrorSummary:      input.ErrorSummary,
		LatencyMS:         int(input.Latency / time.Millisecond),
	}
	if models.TerminalInvocation(strings.TrimSpace(input.Status)) {
		at := a.now().UTC()
		payload.RecordClass = models.InvocationAuditClass
		payload.CompletedAt = &at
	}
	saved, err := a.traces.Create(ctx, payload)
	if err != nil {
		return uuid.Nil, err
	}
	return saved.UUID, nil
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

var ErrInvocationStorageCapacity = errors.New("invocation storage admission blocked")

// SetStorageAdmission 仅由 bootstrap 在提供请求前注入，拒绝发生在业务副作用之前。
func (a *AuditService) SetStorageAdmission(check func(context.Context) error) { a.admission = check }
func (a *AuditService) checkStorageAdmission(ctx context.Context) error {
	if a == nil || a.admission == nil {
		return nil
	}
	err := a.admission(ctx)
	if err == nil {
		return nil
	}
	a.admissionMu.Lock()
	defer a.admissionMu.Unlock()
	if time.Since(a.lastRejected) >= time.Minute {
		a.lastRejected = time.Now()
		code, _, ok := StorageAdmissionFailure(err)
		if !ok {
			code = "STORAGE_GOVERNANCE_UNAVAILABLE"
		}
		logger.WarnF(ctx, "storage.admission.invocation_rejected: reason_code=%s new capability calls are blocked before execution", code)
	}
	if _, _, ok := StorageAdmissionFailure(err); !ok {
		err = dto.NewErrorWithCode(503, "STORAGE_GOVERNANCE_UNAVAILABLE", "存储治理服务暂不可用", nil)
	}
	public := PublicStorageAdmissionError(err)
	if dto.CodeOf(public) == "STORAGE_CAPACITY_EXHAUSTED" {
		return fmt.Errorf("%w: %w", ErrInvocationStorageCapacity, public)
	}
	return public
}
