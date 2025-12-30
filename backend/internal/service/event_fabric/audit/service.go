package audit

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"gorm.io/datatypes"
)

// Record 描述一次发布或订阅调用的审计数据。
type Record struct {
	ID           string
	TenantID     string
	Topic        string
	PrincipalID  string
	Action       string
	Status       string
	LatencyMs    int64
	TraceID      string
	Metadata     map[string]string
	HappenedAt   time.Time
	ErrorMessage string
}

// Service 将审计记录写入外部审计服务或日志。
type Service interface {
	Write(ctx context.Context, record Record) error
}

// Options 配置审计服务。
type Options struct {
	AuditService auditsvc.Service
	Source       string
	ResourceType string
	Clock        func() time.Time
}

type serviceImpl struct {
	svc          auditsvc.Service
	source       string
	resourceType string
	clock        func() time.Time
}

type noopService struct{}

func (noopService) Write(_ context.Context, _ Record) error { return nil }

// NewService 构建审计写入器。
func NewService(opts Options) Service {
	if opts.AuditService == nil {
		return noopService{}
	}
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = shared.DomainName
	}
	resourceType := strings.TrimSpace(opts.ResourceType)
	if resourceType == "" {
		resourceType = "event"
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &serviceImpl{
		svc:          opts.AuditService,
		source:       source,
		resourceType: resourceType,
		clock:        clock,
	}
}

func (s *serviceImpl) Write(ctx context.Context, record Record) error {
	if s.svc == nil {
		return nil
	}
	eventTime := record.HappenedAt
	if eventTime.IsZero() {
		eventTime = s.clock().UTC()
	}

	meta := map[string]any{
		"topic":        record.Topic,
		"principal_id": record.PrincipalID,
		"latency_ms":   record.LatencyMs,
		"metadata":     record.Metadata,
	}
	if record.TraceID != "" {
		meta["trace_id"] = record.TraceID
	}
	if record.ErrorMessage != "" {
		meta["error"] = record.ErrorMessage
	}

	metaBytes, _ := json.Marshal(meta)
	tenantUUID := strings.TrimSpace(record.TenantID)
	outcome := normalizeOutcome(record.Status)
	severity := severityFromOutcome(outcome, record.ErrorMessage)

	auditEvent := &dbmaudit.AuditEvent{
		OccurredAt:    eventTime,
		TenantUUID:    tenantUUID,
		CorrelationID: record.TraceID,
		Source:        s.source,
		Operation:     record.Action,
		ResourceType:  s.resourceType,
		ResourceID:    record.ID,
		ResourceName:  record.Topic,
		Outcome:       outcome,
		Severity:      severity,
		ActorUserName: record.PrincipalID,
		ActorDisplay:  record.PrincipalID,
		Meta:          datatypes.JSON(metaBytes),
	}

	return s.svc.Emit(ctx, auditEvent)
}

func normalizeOutcome(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(status)
}

func severityFromOutcome(outcome string, errMsg string) string {
	if errMsg != "" {
		return "ERROR"
	}
	if strings.Contains(outcome, "FAIL") || strings.Contains(outcome, "ERROR") {
		return "ERROR"
	}
	return "INFO"
}
