package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

// LifecycleAuditInput is an audit payload for lifecycle operations.
type LifecycleAuditInput struct {
	Action   string
	SkillID  string
	Version  string
	Operator string
	Source   string
	Reason   string
	Result   string
	TraceID  string
}

// ExecutionTraceInput is an execution trace payload.
type ExecutionTraceInput struct {
	TraceID          string
	TenantUUID       string
	SkillID          string
	Version          string
	Entrypoint       string
	InvokePath       string
	ProtocolUsed     string
	Status           string
	CapabilityID     string
	ProviderPluginID string
	AgentID          string
	SessionID        string
	MessageID        string
	ExecutorPath     string
	PluginTaskID     string
	PlanID           string
	NodeID           string
	TeamID           string
	HandoffTaskID    string
	HandoffTraceID   string
	NodeStatus       string
	RetryTrace       string
	LatencyMS        int
	ErrorCode        string
	ErrorSummary     string
	RequestPayload   interface{}
	ResponsePayload  interface{}
	FallbackUsed     bool
	AuthPass         bool
}

// AuditTraceService writes lifecycle audits and execution traces.
type AuditTraceService struct {
	traceRepo *skillrepo.SkillExecutionTraceRepository
	auditRepo *skillrepo.SkillLifecycleAuditRepository
	metrics   *Metrics
	now       func() time.Time
}

func NewAuditTraceService(
	traceRepo *skillrepo.SkillExecutionTraceRepository,
	auditRepo *skillrepo.SkillLifecycleAuditRepository,
) *AuditTraceService {
	return &AuditTraceService{
		traceRepo: traceRepo,
		auditRepo: auditRepo,
		metrics:   NewMetrics(),
		now:       time.Now,
	}
}

func (s *AuditTraceService) RecordLifecycleAudit(ctx context.Context, in LifecycleAuditInput) error {
	if s.auditRepo == nil {
		return nil
	}
	auditID := uuid.NewString()
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	if in.Result == "" {
		in.Result = "success"
	}
	record := &skillmodel.SkillLifecycleAudit{
		AuditID:      auditID,
		Action:       in.Action,
		SkillID:      strings.ToLower(strings.TrimSpace(in.SkillID)),
		Version:      strings.TrimSpace(in.Version),
		Operator:     strings.TrimSpace(in.Operator),
		TenantScope:  "global",
		Reason:       strings.TrimSpace(in.Reason),
		Result:       strings.ToLower(strings.TrimSpace(in.Result)),
		TraceID:      strings.TrimSpace(in.TraceID),
		Source:       strings.ToLower(strings.TrimSpace(in.Source)),
		ErrorSummary: "",
	}
	record.Normalize()
	if s.metrics != nil {
		s.metrics.IncLifecycle(record.Action, record.SkillID, record.Version)
	}
	_, err := s.auditRepo.Create(ctx, record)
	return err
}

func (s *AuditTraceService) RecordExecutionTrace(ctx context.Context, in ExecutionTraceInput) error {
	if s.traceRepo == nil {
		return nil
	}
	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	record := &skillmodel.SkillExecutionTrace{
		TraceID:                traceID,
		TenantUUID:             strings.ToLower(strings.TrimSpace(in.TenantUUID)),
		SkillID:                strings.ToLower(strings.TrimSpace(in.SkillID)),
		Version:                strings.TrimSpace(in.Version),
		Entrypoint:             strings.TrimSpace(in.Entrypoint),
		ProtocolUsed:           defaultString(in.ProtocolUsed, "skill"),
		InvokePath:             strings.ToLower(strings.TrimSpace(in.InvokePath)),
		Status:                 strings.ToLower(strings.TrimSpace(in.Status)),
		LatencyMS:              in.LatencyMS,
		ErrorCode:              strings.TrimSpace(in.ErrorCode),
		ErrorSummary:           strings.TrimSpace(in.ErrorSummary),
		RequestPayloadDigest:   digestJSON(in.RequestPayload),
		ResponsePayloadDigest:  digestJSON(in.ResponsePayload),
		CapabilityID:           strings.TrimSpace(in.CapabilityID),
		ProviderPluginID:       strings.TrimSpace(in.ProviderPluginID),
		AgentID:                strings.TrimSpace(in.AgentID),
		SessionID:              strings.TrimSpace(in.SessionID),
		MessageID:              strings.TrimSpace(in.MessageID),
		ExecutorPath:           strings.TrimSpace(in.ExecutorPath),
		PluginTaskID:           strings.TrimSpace(in.PluginTaskID),
		PlanID:                 strings.TrimSpace(in.PlanID),
		NodeID:                 strings.TrimSpace(in.NodeID),
		TeamID:                 strings.TrimSpace(in.TeamID),
		HandoffTaskID:          strings.TrimSpace(in.HandoffTaskID),
		HandoffTraceID:         strings.TrimSpace(in.HandoffTraceID),
		NodeStatus:             strings.ToLower(strings.TrimSpace(in.NodeStatus)),
		RetryTrace:             strings.TrimSpace(in.RetryTrace),
		FallbackUsed:           in.FallbackUsed,
		AuthorizationCheckPass: in.AuthPass,
	}
	record.Normalize()
	if s.metrics != nil {
		s.metrics.IncTrace(record.Status, record.SkillID, record.Version, record.TenantUUID)
	}
	_, err := s.traceRepo.UpsertByTraceID(ctx, record)
	return err
}

func defaultString(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	return raw
}

func digestJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}
