package observability_ops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	inst "github.com/ArtisanCloud/PowerX/internal/service/observability_ops/instrumentation"
	auditrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Operation    string
	Outcome      string
	Severity     string
	Detail       map[string]any
}

type AuditWriter interface {
	Write(ctx context.Context, rec AuditRecord) error
}

type UnifiedAuditWriter struct {
	repo    *auditrepo.AuditEventRepository
	metrics *inst.Recorder
}

func NewUnifiedAuditWriter(db *gorm.DB) *UnifiedAuditWriter {
	return &UnifiedAuditWriter{
		repo:    auditrepo.NewAuditEventRepository(db),
		metrics: inst.NewRecorder("powerx.service.observability_ops"),
	}
}

func (w *UnifiedAuditWriter) Write(ctx context.Context, rec AuditRecord) error {
	startedAt := time.Now()
	var writeErr error
	defer func() { w.metrics.Observe(ctx, "audit_write", startedAt, writeErr) }()

	payload, err := json.Marshal(rec.Detail)
	if err != nil {
		writeErr = fmt.Errorf("marshal audit detail: %w", err)
		return writeErr
	}

	now := time.Now().UTC()
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	traceID := strings.TrimSpace(reqctx.GetTraceID(ctx))
	userID := int64(reqctx.GetUserID(ctx))
	memberID := int64(reqctx.GetMemberID(ctx))

	auditRows := []struct {
		OccurredAt   time.Time
		Source       string
		Operation    string
		ResourceType string
		ResourceID   string
		ActorUserID  *int64
		ActorMember  *int64
		TenantUUID   string
		TraceID      string
		Outcome      string
		Severity     string
		Meta         []byte
	}{
		{
			OccurredAt:   now,
			Source:       "platform_ops",
			Operation:    strings.TrimSpace(strings.ToUpper(rec.Operation)),
			ResourceType: strings.TrimSpace(strings.ToLower(rec.ResourceType)),
			ResourceID:   strings.TrimSpace(rec.ResourceID),
			ActorUserID:  nullableInt64(userID),
			ActorMember:  nullableInt64(memberID),
			TenantUUID:   tenantUUID,
			TraceID:      traceID,
			Outcome:      normalizeOrDefault(strings.ToUpper(rec.Outcome), "SUCCESS"),
			Severity:     normalizeOrDefault(strings.ToUpper(rec.Severity), "INFO"),
			Meta:         payload,
		},
	}

	models := make([]auditrepoRow, 0, len(auditRows))
	for _, row := range auditRows {
		models = append(models, auditrepoRow{
			OccurredAt:   row.OccurredAt,
			Source:       row.Source,
			Operation:    row.Operation,
			ResourceType: row.ResourceType,
			ResourceID:   row.ResourceID,
			ActorUserID:  row.ActorUserID,
			ActorMember:  row.ActorMember,
			TenantUUID:   row.TenantUUID,
			Correlation:  row.TraceID,
			Outcome:      row.Outcome,
			Severity:     row.Severity,
			Meta:         row.Meta,
		})
	}
	writeErr = w.repo.InsertBatch(ctx, toAuditEvents(models))
	return writeErr
}

type auditrepoRow struct {
	OccurredAt   time.Time
	Source       string
	Operation    string
	ResourceType string
	ResourceID   string
	ActorUserID  *int64
	ActorMember  *int64
	TenantUUID   string
	Correlation  string
	Outcome      string
	Severity     string
	Meta         []byte
}

func nullableInt64(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

func normalizeOrDefault(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}
