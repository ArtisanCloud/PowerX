package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReportingService 提供审计事件查询能力。
type ReportingService interface {
	Query(ctx context.Context, filter ReportingFilter) (ReportingResult, error)
}

// ReportingServiceOptions 描述 ReportingService 的依赖。
type ReportingServiceOptions struct {
	AuditDB                 *gorm.DB
	AuthorizationRepository *eventfabricrepo.AuthorizationRepository
	Logger                  *pxlog.Logger
}

// ReportingFilter 控制查询过滤条件。
type ReportingFilter struct {
	TenantUUID  uuid.UUID
	SubjectID   *uuid.UUID
	SubjectType string
	Capability  string
	Decision    string
	From        time.Time
	To          time.Time
	Page        int
	PageSize    int
	NoLimit     bool
}

// ReportingResult 返回查询结果。
type ReportingResult struct {
	Items     []ReportingEvent `json:"items"`
	Total     int              `json:"total"`
	Page      int              `json:"page,omitempty"`
	PageSize  int              `json:"pageSize,omitempty"`
	TotalPage int              `json:"pages,omitempty"`
}

// ReportingEvent 描述单条审计事件。
type ReportingEvent struct {
	ID          uint64            `json:"id"`
	OccurredAt  time.Time         `json:"occurredAt"`
	Operation   string            `json:"operation"`
	Outcome     string            `json:"outcome"`
	Source      string            `json:"source"`
	Category    string            `json:"category"`
	TenantUUID  string            `json:"tenant_uuid"`
	SubjectType string            `json:"subjectType,omitempty"`
	SubjectID   string            `json:"subjectId,omitempty"`
	Capability  string            `json:"capability,omitempty"`
	Decision    string            `json:"decision,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	GrantID     string            `json:"grantId,omitempty"`
	Actor       string            `json:"actor,omitempty"`
	LatencyMs   int64             `json:"latencyMs,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type reportingService struct {
	auditDB  *gorm.DB
	authRepo *eventfabricrepo.AuthorizationRepository
	logger   *pxlog.Logger
}

// NewReportingService 构建审计查询服务。
func NewReportingService(opts ReportingServiceOptions) ReportingService {
	if opts.AuditDB == nil {
		panic("authorization reporting requires audit db")
	}
	if opts.AuthorizationRepository == nil {
		panic("authorization reporting requires authorization repository")
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	return &reportingService{
		auditDB:  opts.AuditDB,
		authRepo: opts.AuthorizationRepository,
		logger:   logger,
	}
}

func (s *reportingService) Query(ctx context.Context, filter ReportingFilter) (ReportingResult, error) {
	if filter.TenantUUID == uuid.Nil {
		return ReportingResult{}, fmt.Errorf("tenant uuid is required")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 && !filter.NoLimit {
		filter.PageSize = 20
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && filter.To.Before(filter.From) {
		return ReportingResult{}, fmt.Errorf("time range invalid: to before from")
	}

	events, err := s.loadAuditEvents(ctx, filter)
	if err != nil {
		return ReportingResult{}, err
	}
	if len(events) == 0 {
		return ReportingResult{Items: []ReportingEvent{}, Total: 0, Page: filter.Page, PageSize: filter.PageSize, TotalPage: 0}, nil
	}

	transformer := newEventTransformer(ctx, s.authRepo, filter, s.logger)
	reportItems := make([]ReportingEvent, 0, len(events))
	for _, evt := range events {
		item, ok := transformer.transform(evt)
		if !ok {
			continue
		}
		reportItems = append(reportItems, item)
	}

	total := len(reportItems)
	if total == 0 {
		return ReportingResult{Items: []ReportingEvent{}, Total: 0, Page: filter.Page, PageSize: filter.PageSize, TotalPage: 0}, nil
	}

	if filter.NoLimit {
		return ReportingResult{
			Items:    reportItems,
			Total:    total,
			Page:     0,
			PageSize: 0,
		}, nil
	}

	start := (filter.Page - 1) * filter.PageSize
	if start >= total {
		return ReportingResult{
			Items:     []ReportingEvent{},
			Total:     total,
			Page:      filter.Page,
			PageSize:  filter.PageSize,
			TotalPage: pages(total, filter.PageSize),
		}, nil
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}
	return ReportingResult{
		Items:     reportItems[start:end],
		Total:     total,
		Page:      filter.Page,
		PageSize:  filter.PageSize,
		TotalPage: pages(total, filter.PageSize),
	}, nil
}

func (s *reportingService) loadAuditEvents(ctx context.Context, filter ReportingFilter) ([]auditmodel.AuditEvent, error) {
	db := s.auditDB.WithContext(ctx).
		Where("resource_name = ?", auditTopicAuthorization).
		Where("(operation LIKE ? OR operation LIKE ?)", "GRANT.%", "EVALUATION.%")
	if !filter.From.IsZero() {
		db = db.Where("occurred_at >= ?", filter.From)
	}
	if !filter.To.IsZero() {
		db = db.Where("occurred_at <= ?", filter.To)
	}

	var rows []auditmodel.AuditEvent
	if err := db.Order("occurred_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type eventTransformer struct {
	ctx       context.Context
	repo      *eventfabricrepo.AuthorizationRepository
	filter    ReportingFilter
	logger    *pxlog.Logger
	grantPool map[uuid.UUID]*eventfabricmodel.AuthorizationGrant
	tenantKey string
}

func newEventTransformer(ctx context.Context, repo *eventfabricrepo.AuthorizationRepository, filter ReportingFilter, logger *pxlog.Logger) *eventTransformer {
	return &eventTransformer{
		ctx:       ctx,
		repo:      repo,
		filter:    filter,
		logger:    logger,
		tenantKey: canonicalTenantKey(filter.TenantUUID.String()),
		grantPool: make(map[uuid.UUID]*eventfabricmodel.AuthorizationGrant),
	}
}

func (t *eventTransformer) transform(evt auditmodel.AuditEvent) (ReportingEvent, bool) {
	metaEnvelope, err := parseAuditMeta(evt.Meta)
	if err != nil {
		if t.logger != nil {
			t.logger.WarnF(t.ctx, "[authorization.reporting] parse audit meta failed id=%d err=%v", evt.ID, err)
		}
		return ReportingEvent{}, false
	}

	category := classifyOperation(evt.Operation)
	if category == "" {
		return ReportingEvent{}, false
	}

	meta := toStringMap(metaEnvelope.Metadata)
	grantID := extractGrantID(evt, category, meta)
	var grant *eventfabricmodel.AuthorizationGrant
	if grantID != uuid.Nil {
		grant = t.loadGrant(grantID)
		if grant == nil {
			return ReportingEvent{}, false
		}
		tenantKey := canonicalTenantKey(grant.TenantUUID)
		if tenantKey == "" {
			return ReportingEvent{}, false
		}
		if tenantKey != t.tenantKey {
			return ReportingEvent{}, false
		}
		if t.filter.SubjectID != nil && grant.SubjectID != *t.filter.SubjectID {
			return ReportingEvent{}, false
		}
		if t.filter.SubjectType != "" && !strings.EqualFold(grant.SubjectType, t.filter.SubjectType) {
			return ReportingEvent{}, false
		}
	} else {
		// 无 Grant ID 的事件无法校验租户
		return ReportingEvent{}, false
	}

	if t.filter.Capability != "" {
		if !strings.EqualFold(meta["capability"], t.filter.Capability) {
			return ReportingEvent{}, false
		}
	}
	if t.filter.Decision != "" {
		if !strings.EqualFold(meta["decision"], t.filter.Decision) {
			return ReportingEvent{}, false
		}
	}

	tenantKey := canonicalTenantKey(grant.TenantUUID)
	event := ReportingEvent{
		ID:          evt.ID,
		OccurredAt:  evt.OccurredAt,
		Operation:   evt.Operation,
		Outcome:     evt.Outcome,
		Source:      evt.Source,
		Category:    category,
		TenantUUID:  tenantKey,
		SubjectType: strings.ToLower(grant.SubjectType),
		SubjectID:   grant.SubjectID.String(),
		Capability:  meta["capability"],
		Decision:    strings.ToLower(meta["decision"]),
		Reason:      meta["reason"],
		GrantID:     grant.UUID.String(),
		Actor:       evt.ActorDisplay,
		LatencyMs:   int64(metaEnvelope.LatencyMS),
		Metadata:    meta,
	}
	return event, true
}

func (t *eventTransformer) loadGrant(id uuid.UUID) *eventfabricmodel.AuthorizationGrant {
	if grant, ok := t.grantPool[id]; ok {
		return grant
	}
	grant, err := t.repo.GetGrantByUUID(t.ctx, id)
	if err != nil && err != gorm.ErrRecordNotFound {
		if t.logger != nil {
			t.logger.WarnF(t.ctx, "[authorization.reporting] load grant failed grant=%s err=%v", id, err)
		}
		return nil
	}
	if grant != nil {
		t.grantPool[id] = grant
	}
	return grant
}

type auditMetaEnvelope struct {
	Topic       string         `json:"topic"`
	PrincipalID string         `json:"principal_id"`
	LatencyMS   float64        `json:"latency_ms"`
	Metadata    map[string]any `json:"metadata"`
}

func parseAuditMeta(data []byte) (auditMetaEnvelope, error) {
	if len(data) == 0 {
		return auditMetaEnvelope{}, nil
	}
	var env auditMetaEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return auditMetaEnvelope{}, err
	}
	return env, nil
}

func toStringMap(input map[string]any) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		switch vv := v.(type) {
		case string:
			out[k] = vv
		case fmt.Stringer:
			out[k] = vv.String()
		case json.Number:
			out[k] = vv.String()
		default:
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}

func extractGrantID(evt auditmodel.AuditEvent, category string, meta map[string]string) uuid.UUID {
	if idStr := meta["grant_id"]; idStr != "" {
		if id, err := uuid.Parse(idStr); err == nil {
			return id
		}
	}
	if category == "grant" {
		if id, err := uuid.Parse(evt.ResourceID); err == nil {
			return id
		}
	}
	return uuid.Nil
}

func classifyOperation(op string) string {
	op = strings.ToUpper(strings.TrimSpace(op))
	switch {
	case strings.HasPrefix(op, "GRANT."):
		return "grant"
	case strings.HasPrefix(op, "EVALUATION."):
		return "evaluation"
	default:
		return ""
	}
}

func pages(total, size int) int {
	if size <= 0 {
		return 0
	}
	p := total / size
	if total%size != 0 {
		p++
	}
	return p
}
