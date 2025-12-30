package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
)

const (
	DecisionAllow     = "allow"
	DecisionBlock     = "block"
	DecisionChallenge = "challenge"
)

// EvaluateRequest 描述授权评估输入。
type EvaluateRequest struct {
	TenantID    uuid.UUID
	SubjectType string
	SubjectID   uuid.UUID
	Capability  string
	Resource    string
	ContextTags []string
	Attributes  map[string]string
	RequestID   string
}

// EvaluateResult 返回授权评估结果。
type EvaluateResult struct {
	Decision     string
	Reason       string
	GrantVersion uint64
	Challenge    *ChallengeInfo
	AuditEventID string
	CacheHit     bool
}

// ChallengeInfo 描述 Challenge 工单信息。
type ChallengeInfo struct {
	TicketID     uuid.UUID
	Status       string
	SLAExpiresAt time.Time
}

// GrantSnapshot 提供授权快照。
type GrantSnapshot struct {
	GrantID      uuid.UUID
	TenantUUID   string
	SubjectType  string
	SubjectID    uuid.UUID
	Status       string
	Source       string
	Version      uint64
	TTLExpires   *time.Time
	Capabilities []CapabilitySnapshot
	Conditions   ConditionsSnapshot
}

// CapabilitySnapshot 描述授权能力。
type CapabilitySnapshot struct {
	CapabilityID     uuid.UUID
	Namespace        string
	Action           string
	DefaultRateLimit map[string]any
	CustomRateLimit  map[string]any
}

// ConditionsSnapshot 描述授权条件。
type ConditionsSnapshot struct {
	Resources   []string
	ContextTags []string
	TimeWindow  *TimeWindowSnapshot
}

// TimeWindowSnapshot 定义时间窗口。
type TimeWindowSnapshot struct {
	Start time.Time
	End   time.Time
}

func (s *serviceImpl) Evaluate(ctx context.Context, req EvaluateRequest) (*EvaluateResult, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant id is required")
	}
	if req.SubjectID == uuid.Nil {
		return nil, fmt.Errorf("subject id is required")
	}

	subjectType, err := normalizeSubjectType(req.SubjectType)
	if err != nil {
		return nil, err
	}
	capability := strings.TrimSpace(req.Capability)
	if capability == "" {
		return nil, fmt.Errorf("capability is required")
	}
	req.SubjectType = subjectType
	req.Capability = capability
	namespace, action, err := splitCapabilityName(capability)
	if err != nil {
		return nil, err
	}

	cacheKey := GrantCacheKey{
		TenantUUID:  req.TenantID.String(),
		SubjectType: subjectType,
		SubjectID:   req.SubjectID,
	}

	start := s.clock().UTC()
	snapshot, cacheHit, err := s.loadGrantSnapshot(ctx, cacheKey, req.TenantID)
	if err != nil {
		s.emitEvaluationAlert(ctx, req, nil, "authorization.evaluation_failed", "high", err.Error(), nil)
		return nil, err
	}
	if snapshot == nil {
		reason := "no active grant"
		s.emitEvaluationAlert(ctx, req, nil, "authorization.policy_missing", "high", reason, nil)
		auditID := s.emitEvaluationAudit(ctx, req, DecisionBlock, reason, nil)
		s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionBlock)
		return &EvaluateResult{
			Decision:     DecisionBlock,
			Reason:       reason,
			AuditEventID: auditID,
			CacheHit:     cacheHit,
		}, nil
	}

	now := s.clock().UTC()
	if snapshot.TTLExpires != nil && now.After(snapshot.TTLExpires.UTC()) {
		if err := s.markGrantExpired(ctx, snapshot); err != nil {
			s.logger.WarnF(ctx, "[authorization.evaluate] mark expired failed grant=%s err=%v", snapshot.GrantID, err)
		}
		reason := "grant expired"
		s.emitEvaluationAlert(ctx, req, snapshot, "authorization.policy_expired", "medium", reason, nil)
		auditID := s.emitEvaluationAudit(ctx, req, DecisionBlock, reason, snapshot)
		s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionBlock)
		return &EvaluateResult{
			Decision:     DecisionBlock,
			Reason:       reason,
			GrantVersion: snapshot.Version,
			AuditEventID: auditID,
			CacheHit:     cacheHit,
		}, nil
	}

	capSnapshot := snapshot.matchCapability(namespace, action)
	if capSnapshot == nil {
		reason := "capability not granted"
		s.emitEvaluationAlert(ctx, req, snapshot, "authorization.policy_violation", "high", reason, map[string]string{
			"namespace": namespace,
			"action":    action,
		})
		auditID := s.emitEvaluationAudit(ctx, req, DecisionBlock, reason, snapshot)
		s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionBlock)
		return &EvaluateResult{
			Decision:     DecisionBlock,
			Reason:       reason,
			GrantVersion: snapshot.Version,
			AuditEventID: auditID,
			CacheHit:     cacheHit,
		}, nil
	}

	if err := snapshot.Conditions.validate(now, req.Resource, req.ContextTags); err != nil {
		reason := err.Error()
		s.emitEvaluationAlert(ctx, req, snapshot, "authorization.policy_violation", "medium", reason, nil)
		auditID := s.emitEvaluationAudit(ctx, req, DecisionBlock, reason, snapshot)
		s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionBlock)
		return &EvaluateResult{
			Decision:     DecisionBlock,
			Reason:       reason,
			GrantVersion: snapshot.Version,
			AuditEventID: auditID,
			CacheHit:     cacheHit,
		}, nil
	}

	if snapshot.Status == eventfabricmodel.GrantStatusPending {
		ticket, err := s.repo.GetPendingTicketByGrant(ctx, snapshot.GrantID)
		if err != nil {
			s.logger.WarnF(ctx, "[authorization.evaluate] load pending ticket failed grant=%s err=%v", snapshot.GrantID, err)
			ticket = nil
		}
		reason := "challenge pending approval"
		s.emitEvaluationAlert(ctx, req, snapshot, "authorization.challenge_required", "medium", reason, map[string]string{
			"ticket_id": ticketIDString(ticket),
		})
		auditID := s.emitEvaluationAudit(ctx, req, DecisionChallenge, reason, snapshot)
		s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionChallenge)
		return &EvaluateResult{
			Decision:     DecisionChallenge,
			Reason:       reason,
			GrantVersion: snapshot.Version,
			AuditEventID: auditID,
			CacheHit:     cacheHit,
			Challenge:    convertTicket(ticket),
		}, nil
	}

	policy := selectRateLimit(capSnapshot)
	if policy.Limit > 0 && s.limiter != nil {
		rlKey := buildRateLimitKey(snapshot, namespace, action)
		result, rlErr := s.limiter.Allow(ctx, rlKey, policy)
		if rlErr != nil {
			s.emitEvaluationAlert(ctx, req, snapshot, "authorization.rate_limit_failed", "medium", rlErr.Error(), nil)
			auditID := s.emitEvaluationAudit(ctx, req, DecisionBlock, "rate limiter failure", snapshot)
			s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionBlock)
			return &EvaluateResult{
				Decision:     DecisionBlock,
				Reason:       "rate limiter failure",
				GrantVersion: snapshot.Version,
				AuditEventID: auditID,
				CacheHit:     cacheHit,
			}, nil
		}
		if !result.Allowed {
			reason := "rate limit exceeded"
			meta := map[string]string{
				"remaining": fmt.Sprintf("%d", result.Remaining),
				"interval":  policy.Interval.String(),
			}
			s.emitEvaluationAlert(ctx, req, snapshot, "authorization.rate_limited", "medium", reason, meta)
			auditID := s.emitEvaluationAudit(ctx, req, DecisionBlock, reason, snapshot)
			s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionBlock)
			return &EvaluateResult{
				Decision:     DecisionBlock,
				Reason:       reason,
				GrantVersion: snapshot.Version,
				AuditEventID: auditID,
				CacheHit:     cacheHit,
			}, nil
		}
	}

	if err := s.updateLastEvaluatedAt(ctx, snapshot.GrantID, now); err != nil {
		s.logger.WarnF(ctx, "[authorization.evaluate] update last evaluated failed grant=%s err=%v", snapshot.GrantID, err)
	}

	auditID := s.emitEvaluationAudit(ctx, req, DecisionAllow, "authorized", snapshot)
	s.recordEvaluationMetrics(ctx, start, cacheHit, DecisionAllow)
	return &EvaluateResult{
		Decision:     DecisionAllow,
		Reason:       "authorized",
		GrantVersion: snapshot.Version,
		AuditEventID: auditID,
		CacheHit:     cacheHit,
	}, nil
}

func (s *serviceImpl) GetGrantSnapshot(ctx context.Context, tenantID uuid.UUID, subjectType string, subjectID uuid.UUID) (*GrantSnapshot, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant id is required")
	}
	if subjectID == uuid.Nil {
		return nil, fmt.Errorf("subject id is required")
	}
	normalizedType, err := normalizeSubjectType(subjectType)
	if err != nil {
		return nil, err
	}
	cacheKey := GrantCacheKey{
		TenantUUID:  tenantID.String(),
		SubjectType: normalizedType,
		SubjectID:   subjectID,
	}
	snapshot, _, err := s.loadGrantSnapshot(ctx, cacheKey, tenantID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, ErrGrantNotFound
	}
	return snapshot, nil
}

func (s *serviceImpl) loadGrantSnapshot(ctx context.Context, key GrantCacheKey, tenantID uuid.UUID) (*GrantSnapshot, bool, error) {
	if s.cache != nil {
		entry, err := s.cache.Get(ctx, key, 0)
		if err != nil {
			return nil, false, err
		}
		if entry != nil {
			snapshot, err := snapshotFromCacheEntry(entry)
			if err == nil {
				return snapshot, true, nil
			}
			s.logger.WarnF(ctx, "[authorization.cache] decode grant cache failed key=%s err=%v", key.String(), err)
			_ = s.cache.Invalidate(ctx, key)
		}
	}
	snapshot, err := s.fetchGrantSnapshotFromDB(ctx, tenantID, key)
	if err != nil {
		return nil, false, err
	}
	return snapshot, false, nil
}

func (s *serviceImpl) fetchGrantSnapshotFromDB(ctx context.Context, tenantID uuid.UUID, key GrantCacheKey) (*GrantSnapshot, error) {
	if s.repo == nil {
		return nil, ErrServiceUnavailable
	}

	grants, err := s.repo.GetGrantBySubject(ctx, tenantID, key.SubjectType, key.SubjectID, []string{
		eventfabricmodel.GrantStatusActive,
		eventfabricmodel.GrantStatusPending,
	})
	if err != nil {
		return nil, err
	}
	var grant *eventfabricmodel.AuthorizationGrant
	for _, candidate := range grants {
		if candidate == nil {
			continue
		}
		if candidate.Status == eventfabricmodel.GrantStatusActive {
			grant = candidate
			break
		}
		if grant == nil {
			grant = candidate
		}
	}
	if grant == nil {
		return nil, nil
	}

	tenantUUID := tenantUUIDFromGrant(grant)
	if grant.TTLExpiresAt != nil && s.clock().UTC().After(grant.TTLExpiresAt.UTC()) {
		if err := s.markGrantExpired(ctx, &GrantSnapshot{
			GrantID:     grant.UUID,
			TenantUUID:  tenantUUID,
			SubjectType: grant.SubjectType,
			SubjectID:   grant.SubjectID,
			Status:      grant.Status,
			Version:     grant.Version,
			TTLExpires:  grant.TTLExpiresAt,
		}); err != nil {
			s.logger.WarnF(ctx, "[authorization.evaluate] expire grant from db failed grant=%s err=%v", grant.UUID, err)
		}
		return nil, nil
	}

	capabilities, err := s.repo.ListGrantCapabilities(ctx, grant.UUID)
	if err != nil {
		return nil, err
	}
	capMap, err := s.buildCapabilityMap(ctx, capabilities)
	if err != nil {
		return nil, err
	}
	conditions, err := s.repo.ListGrantConditions(ctx, grant.UUID)
	if err != nil {
		return nil, err
	}

	payload := buildGrantCachePayload(grant, capabilities, capMap, conditions)
	blob, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	expiresAt := grant.TTLExpiresAt
	if expiresAt == nil {
		defaultExpire := s.clock().UTC().Add(24 * time.Hour)
		expiresAt = &defaultExpire
	}
	entry := &GrantCacheEntry{
		Version:   grant.Version,
		ExpiresAt: *expiresAt,
		Payload:   blob,
	}
	if s.cache != nil {
		if err := s.cache.Set(ctx, key, entry); err != nil {
			s.logger.WarnF(ctx, "[authorization.cache] set grant cache failed key=%s err=%v", key.String(), err)
		}
	}
	snapshot, err := snapshotFromCacheEntry(entry)
	if err != nil {
		return nil, err
	}
	snapshot.TenantUUID = tenantUUID
	return snapshot, nil
}

func snapshotFromCacheEntry(entry *GrantCacheEntry) (*GrantSnapshot, error) {
	if entry == nil {
		return nil, nil
	}
	var doc grantCacheDocument
	if err := json.Unmarshal(entry.Payload, &doc); err != nil {
		return nil, err
	}
	return doc.toSnapshot(entry.Version)
}

func (doc grantCacheDocument) toSnapshot(version uint64) (*GrantSnapshot, error) {
	grantID, err := uuid.Parse(doc.GrantID)
	if err != nil {
		return nil, fmt.Errorf("invalid grant id: %w", err)
	}
	tenantValue := canonicalTenantKey(doc.TenantUUID)
	if tenantValue == "" {
		return nil, fmt.Errorf("invalid tenant uuid")
	}
	subjectID, err := uuid.Parse(doc.SubjectID)
	if err != nil {
		return nil, fmt.Errorf("invalid subject id: %w", err)
	}
	snapshot := &GrantSnapshot{
		GrantID:     grantID,
		TenantUUID:  tenantValue,
		SubjectType: doc.SubjectType,
		SubjectID:   subjectID,
		Status:      doc.Status,
		Source:      doc.Source,
		Version:     version,
		TTLExpires:  doc.TTLExpiresAt,
		Conditions:  doc.Conditions.toSnapshot(),
	}
	for _, capDoc := range doc.Capabilities {
		capID, err := uuid.Parse(capDoc.CapabilityID)
		if err != nil {
			continue
		}
		snapshot.Capabilities = append(snapshot.Capabilities, CapabilitySnapshot{
			CapabilityID:     capID,
			Namespace:        strings.ToLower(capDoc.Namespace),
			Action:           strings.ToLower(capDoc.Action),
			DefaultRateLimit: capDoc.DefaultRateLimit,
			CustomRateLimit:  capDoc.CustomRateLimit,
		})
	}
	return snapshot, nil
}

func (c grantCacheConditionsDocument) toSnapshot() ConditionsSnapshot {
	snapshot := ConditionsSnapshot{
		Resources:   append([]string(nil), c.Resources...),
		ContextTags: append([]string(nil), c.ContextTags...),
	}
	if c.TimeWindow != nil {
		snapshot.TimeWindow = &TimeWindowSnapshot{
			Start: c.TimeWindow.Start,
			End:   c.TimeWindow.End,
		}
	}
	return snapshot
}

func (s *serviceImpl) markGrantExpired(ctx context.Context, snapshot *GrantSnapshot) error {
	if snapshot == nil || snapshot.GrantID == uuid.Nil {
		return fmt.Errorf("grant snapshot required")
	}
	txRepo, tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer txRepo.RollbackTx(tx)

	now := s.clock().UTC()
	fields := map[string]any{
		"status":         eventfabricmodel.GrantStatusExpired,
		"ttl_expires_at": now,
	}
	if err := txRepo.UpdateGrantFields(ctx, snapshot.GrantID, fields); err != nil {
		return err
	}
	if err := txRepo.IncrementGrantVersion(ctx, snapshot.GrantID); err != nil {
		return err
	}
	if err := txRepo.CommitTx(tx); err != nil {
		return err
	}

	grantKey := GrantCacheKey{
		TenantUUID:  snapshot.TenantUUID,
		SubjectType: snapshot.SubjectType,
		SubjectID:   snapshot.SubjectID,
	}
	_ = s.InvalidateGrantCache(ctx, grantKey)
	return nil
}

func (snapshot *GrantSnapshot) matchCapability(namespace, action string) *CapabilitySnapshot {
	if snapshot == nil {
		return nil
	}
	ns := strings.ToLower(namespace)
	act := strings.ToLower(action)
	for _, item := range snapshot.Capabilities {
		if item.Namespace == ns && item.Action == act {
			return &item
		}
	}
	return nil
}

func (c ConditionsSnapshot) validate(now time.Time, resource string, tags []string) error {
	if len(c.Resources) > 0 {
		match := false
		for _, allowed := range c.Resources {
			if strings.EqualFold(strings.TrimSpace(resource), strings.TrimSpace(allowed)) {
				match = true
				break
			}
		}
		if !match {
			return errors.New("resource not allowed")
		}
	}
	if len(c.ContextTags) > 0 {
		reqTags := make(map[string]struct{}, len(tags))
		for _, tag := range tags {
			if trimmed := strings.TrimSpace(strings.ToLower(tag)); trimmed != "" {
				reqTags[trimmed] = struct{}{}
			}
		}
		for _, required := range c.ContextTags {
			if _, ok := reqTags[strings.TrimSpace(strings.ToLower(required))]; !ok {
				return fmt.Errorf("missing required context tag %s", required)
			}
		}
	}
	if c.TimeWindow != nil {
		if now.Before(c.TimeWindow.Start) || now.After(c.TimeWindow.End) {
			return errors.New("outside permitted time window")
		}
	}
	return nil
}

func selectRateLimit(cap *CapabilitySnapshot) RateLimitPolicy {
	if cap == nil {
		return RateLimitPolicy{}
	}
	if policy := decodeRateLimit(cap.CustomRateLimit); policy.Limit > 0 {
		return policy
	}
	return decodeRateLimit(cap.DefaultRateLimit)
}

func decodeRateLimit(raw map[string]any) RateLimitPolicy {
	if len(raw) == 0 {
		return RateLimitPolicy{}
	}
	limit := toUint(raw["limit"])
	burst := toUint(raw["burst"])
	intervalSeconds := toUint(raw["interval_seconds"])
	interval := time.Duration(intervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	return RateLimitPolicy{
		Limit:    limit,
		Burst:    burst,
		Interval: interval,
	}
}

func toUint(v any) uint64 {
	switch val := v.(type) {
	case float64:
		if val < 0 {
			return 0
		}
		return uint64(val)
	case float32:
		if val < 0 {
			return 0
		}
		return uint64(val)
	case int:
		if val < 0 {
			return 0
		}
		return uint64(val)
	case int64:
		if val < 0 {
			return 0
		}
		return uint64(val)
	case uint64:
		return val
	case json.Number:
		f, _ := val.Float64()
		if f < 0 {
			return 0
		}
		return uint64(f)
	default:
		return 0
	}
}

func buildRateLimitKey(snapshot *GrantSnapshot, namespace, action string) string {
	if snapshot == nil {
		return ""
	}
	tenantUUID := tenantUUIDFromSnapshot(snapshot)
	return fmt.Sprintf("%s:%s:%s:%s", strings.ToLower(tenantUUID),
		strings.ToLower(snapshot.SubjectType), strings.ToLower(snapshot.SubjectID.String()),
		capabilityKey(namespace, action))
}

func (s *serviceImpl) updateLastEvaluatedAt(ctx context.Context, grantID uuid.UUID, now time.Time) error {
	if grantID == uuid.Nil {
		return nil
	}
	return s.repo.UpdateGrantFields(ctx, grantID, map[string]any{
		"last_evaluated_at": now,
	})
}

func (s *serviceImpl) recordEvaluationMetrics(ctx context.Context, start time.Time, cacheHit bool, decision string) {
	if s.metrics == nil {
		return
	}
	latency := s.clock().UTC().Sub(start)
	if latency < 0 {
		latency = 0
	}
	s.metrics.ObserveAuthorizationEvaluation(ctx, decision, cacheHit, latency)
}

func (s *serviceImpl) emitEvaluationAlert(ctx context.Context, req EvaluateRequest, snapshot *GrantSnapshot, alertType, severity, reason string, metadata map[string]string) {
	if s.alerts == nil {
		return
	}
	tenantUUID := req.TenantID.String()
	if tenantUUID == "" && snapshot != nil {
		tenantUUID = strings.TrimSpace(snapshot.TenantUUID)
	}
	event := AlertEvent{
		Type:        alertType,
		Severity:    severity,
		TenantUUID:  tenantUUID,
		SubjectType: strings.ToLower(req.SubjectType),
		SubjectID:   req.SubjectID.String(),
		Capability:  req.Capability,
		Reason:      reason,
		RequestID:   req.RequestID,
		Metadata:    metadata,
		Timestamp:   s.clock().UTC(),
	}
	if snapshot != nil {
		event.GrantID = snapshot.GrantID.String()
		if event.SubjectType == "" {
			event.SubjectType = strings.ToLower(snapshot.SubjectType)
		}
		if snapUUID := tenantUUIDFromSnapshot(snapshot); snapUUID != "" {
			event.TenantUUID = snapUUID
		}
		if event.SubjectID == "" && snapshot.SubjectID != uuid.Nil {
			event.SubjectID = snapshot.SubjectID.String()
		}
	}
	s.alerts.Emit(ctx, event)
}

func (s *serviceImpl) emitEvaluationAudit(ctx context.Context, req EvaluateRequest, decision, reason string, snapshot *GrantSnapshot) string {
	if s.audit == nil {
		return ""
	}
	eventID := uuid.New().String()
	requestTenantUUID := req.TenantID.String()
	meta := map[string]string{
		"decision":     decision,
		"reason":       reason,
		"capability":   req.Capability,
		"tenant_uuid":  requestTenantUUID,
		"subject_id":   req.SubjectID.String(),
		"subject_type": strings.ToLower(req.SubjectType),
	}
	if snapshot != nil {
		meta["grant_id"] = snapshot.GrantID.String()
		meta["grant_version"] = fmt.Sprintf("%d", snapshot.Version)
		meta["grant_status"] = snapshot.Status
		if snapUUID := tenantUUIDFromSnapshot(snapshot); snapUUID != "" {
			meta["tenant_uuid"] = snapUUID
		}
	}
	if req.Resource != "" {
		meta["resource"] = req.Resource
	}
	if len(req.ContextTags) > 0 {
		meta["context_tags"] = strings.Join(req.ContextTags, ",")
	}
	record := eventaudit.Record{
		ID:         eventID,
		TenantID:   meta["tenant_uuid"],
		Topic:      auditTopicAuthorization,
		Action:     fmt.Sprintf("evaluation.%s", decision),
		Status:     strings.ToUpper(decision),
		Metadata:   meta,
		HappenedAt: s.clock().UTC(),
	}
	if err := s.audit.Write(ctx, record); err != nil {
		s.logger.WarnF(ctx, "[authorization.audit] write evaluation failed decision=%s err=%v", decision, err)
	}
	return eventID
}

func splitCapabilityName(capability string) (string, string, error) {
	tokens := strings.SplitN(strings.TrimSpace(capability), ".", 2)
	if len(tokens) != 2 {
		return "", "", fmt.Errorf("invalid capability %q", capability)
	}
	return strings.TrimSpace(tokens[0]), strings.TrimSpace(tokens[1]), nil
}

func (doc grantCacheDocument) validate() error {
	if strings.TrimSpace(doc.GrantID) == "" {
		return fmt.Errorf("missing grant id")
	}
	if strings.TrimSpace(doc.TenantUUID) == "" {
		return fmt.Errorf("missing tenant uuid")
	}
	if strings.TrimSpace(doc.SubjectID) == "" {
		return fmt.Errorf("missing subject id")
	}
	return nil
}

type grantCacheDocument struct {
	GrantID      string                         `json:"grant_id"`
	TenantUUID   string                         `json:"tenant_uuid"`
	SubjectType  string                         `json:"subject_type"`
	SubjectID    string                         `json:"subject_id"`
	Status       string                         `json:"status"`
	Source       string                         `json:"source"`
	Version      uint64                         `json:"version"`
	TTLExpiresAt *time.Time                     `json:"ttl_expires_at"`
	Capabilities []grantCacheCapabilityDocument `json:"capabilities"`
	Conditions   grantCacheConditionsDocument   `json:"conditions"`
}

type grantCacheCapabilityDocument struct {
	CapabilityID     string         `json:"capability_id"`
	Namespace        string         `json:"namespace"`
	Action           string         `json:"action"`
	DefaultRateLimit map[string]any `json:"default_rate_limit"`
	CustomRateLimit  map[string]any `json:"custom_rate_limit"`
}

type grantCacheConditionsDocument struct {
	Resources   []string                      `json:"resources"`
	ContextTags []string                      `json:"context_tags"`
	TimeWindow  *grantCacheTimeWindowDocument `json:"time_window"`
}

type grantCacheTimeWindowDocument struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func ticketIDString(ticket *eventfabricmodel.AuthorizationApprovalTicket) string {
	if ticket == nil {
		return ""
	}
	return ticket.UUID.String()
}

func convertTicket(ticket *eventfabricmodel.AuthorizationApprovalTicket) *ChallengeInfo {
	if ticket == nil {
		return nil
	}
	return &ChallengeInfo{
		TicketID:     ticket.UUID,
		Status:       ticket.Status,
		SLAExpiresAt: ticket.SLAExpiresAt,
	}
}
