package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// InvocationServiceOptions 配置能力调用服务。
type InvocationServiceOptions struct {
	Catalog     *RegistryService
	Router      *router.Service
	TraceRepo   *repo.InvocationTraceRepository
	EventRepo   *repo.CapabilityEventPublicationRepository
	EventBus    event_bus.EventBus
	Auditor     auditpkg.Auditor
	Metrics     *capmetrics.CapabilityRegistryMetrics
	Audit       *AuditService
	Clock       func() time.Time
	VersionLock VersionLock
}

// InvocationService 负责触发能力调用并记录追踪。
type InvocationService struct {
	catalog     *RegistryService
	router      *router.Service
	traces      *repo.InvocationTraceRepository
	audit       *AuditService
	versionLock VersionLock
	now         func() time.Time
}

// InvocationInput 描述调用��求。
type InvocationInput struct {
	CapabilityID      string
	TenantUUID        string
	PreferredProtocol string
	IdempotencyKey    string
	TraceID           string
	Payload           map[string]interface{}
	Context           map[string]interface{}
}

// InvocationResult 描述调用结果。
type InvocationResult struct {
	TraceID      string
	Status       string
	ProtocolUsed string
	FallbackUsed bool
	Result       map[string]interface{}
}

// NewInvocationService 构造调用服务实例。
func NewInvocationService(opts InvocationServiceOptions) *InvocationService {
	if opts.Catalog == nil {
		panic("capability invocation service requires catalog service")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	audit := opts.Audit
	if audit == nil {
		audit = NewAuditService(AuditServiceOptions{
			EventRepo: opts.EventRepo,
			TraceRepo: opts.TraceRepo,
			EventBus:  opts.EventBus,
			Auditor:   opts.Auditor,
			Metrics:   opts.Metrics,
			Clock:     clock,
		})
	}
	return &InvocationService{
		catalog:     opts.Catalog,
		router:      opts.Router,
		traces:      opts.TraceRepo,
		audit:       audit,
		versionLock: opts.VersionLock,
		now:         clock,
	}
}

// VersionLock 描述版本锁执行器行为，用于校验 capabilities_hash。
type VersionLock interface {
	Enforce(ctx context.Context, tenantUUID, capabilityID, hash string) error
	IsUpgradeError(err error) bool
}

// ErrManualUpgradeRequired 表示能力调用前需要管理员确认升级。
var ErrManualUpgradeRequired = errors.New("capability manual upgrade required")

// Invoke 触发能力调用并记录追踪。
func (s *InvocationService) Invoke(ctx context.Context, in InvocationInput) (InvocationResult, error) {
	var result InvocationResult
	if s == nil {
		return result, errors.New("invocation service unavailable")
	}
	if s.router == nil {
		return result, errors.New("invocation router unavailable")
	}
	capabilityID := strings.TrimSpace(in.CapabilityID)
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	if capabilityID == "" || tenantUUID == "" {
		return result, errors.New("capability_id and tenant_uuid are required")
	}

	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	result.TraceID = traceID

	view, err := s.catalog.GetCapability(ctx, capabilityID, false)
	if err != nil {
		return result, err
	}
	record := view.Record
	if record == nil {
		return result, fmt.Errorf("capability %s not found", capabilityID)
	}
	if !strings.EqualFold(strings.TrimSpace(record.Status), "published") {
		return result, fmt.Errorf("capability %s is not published", capabilityID)
	}

	if s.versionLock != nil && strings.TrimSpace(record.CapabilitiesHash) != "" {
		if err := s.versionLock.Enforce(ctx, tenantUUID, capabilityID, strings.TrimSpace(record.CapabilitiesHash)); err != nil {
			if s.versionLock.IsUpgradeError(err) {
				return result, fmt.Errorf("%w: %v", ErrManualUpgradeRequired, err)
			}
			return result, err
		}
	}

	payloadBytes, err := json.Marshal(in.Payload)
	if err != nil {
		return result, fmt.Errorf("invalid payload: %w", err)
	}
	stickyKey := extractStickyKey(in.Context)

	start := s.now()
	routerResult, invokeErr := s.router.Invoke(ctx, router.InvokeRequest{
		CapabilityID: capabilityID,
		TenantUUID:   tenantUUID,
		Payload:      payloadBytes,
		StickyKey:    stickyKey,
	})
	latency := s.now().Sub(start)

	responseMap := decodeJSONPayload(routerResult.Payload)
	if invokeErr != nil {
		result.Status = "failed"
		result.ProtocolUsed = routerResult.Transport
		result.FallbackUsed = routerResult.FallbackUsed
		if s.audit != nil {
			s.audit.RecordInvocation(ctx, InvocationAuditInput{
				TraceID:           traceID,
				TenantUUID:        tenantUUID,
				PluginID:          record.PluginID,
				CapabilityID:      capabilityID,
				PreferredProtocol: in.PreferredProtocol,
				ProtocolUsed:      routerResult.Transport,
				FallbackUsed:      routerResult.FallbackUsed,
				Status:            result.Status,
				IdempotencyKey:    strings.TrimSpace(in.IdempotencyKey),
				RequestPayload:    in.Payload,
				ResponsePayload:   responseMap,
				ErrorSummary:      invokeErr.Error(),
				Latency:           latency,
			})
		}
		return result, invokeErr
	}

	result.Status = "completed"
	result.ProtocolUsed = routerResult.Transport
	result.FallbackUsed = routerResult.FallbackUsed
	result.Result = responseMap

	if s.audit != nil {
		s.audit.RecordInvocation(ctx, InvocationAuditInput{
			TraceID:           traceID,
			TenantUUID:        tenantUUID,
			PluginID:          record.PluginID,
			CapabilityID:      capabilityID,
			PreferredProtocol: in.PreferredProtocol,
			ProtocolUsed:      routerResult.Transport,
			FallbackUsed:      routerResult.FallbackUsed,
			Status:            result.Status,
			IdempotencyKey:    strings.TrimSpace(in.IdempotencyKey),
			RequestPayload:    in.Payload,
			ResponsePayload:   responseMap,
			Latency:           latency,
		})
	}

	return result, nil
}

// GetTrace 根据 trace_id 查询调用记录。
func (s *InvocationService) GetTrace(ctx context.Context, traceID string) (*models.InvocationTrace, error) {
	if s == nil || s.traces == nil {
		return nil, errors.New("invocation trace repository unavailable")
	}
	return s.traces.GetByTraceID(ctx, traceID)
}

func extractStickyKey(ctx map[string]interface{}) string {
	if len(ctx) == 0 {
		return ""
	}
	const workflowKey = "workflow_instance_id"
	if value, ok := ctx[workflowKey]; ok {
		if str, ok := value.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func decodeJSONPayload(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
