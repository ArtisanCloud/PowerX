package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	minGrantTTLSeconds      = 60
	autoApproveTTLLimit     = 6 * time.Hour
	auditTopicAuthorization = "event_fabric.authorization"
)

type resolvedCapability struct {
	Input GrantCapabilityInput
	Model *eventfabricmodel.AuthorizationCapability
}

// CreateGrant 创建新的授权 Grant。
func (s *serviceImpl) CreateGrant(ctx context.Context, req GrantCreateRequest) (*GrantResult, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	normalizedSubject, err := normalizeSubjectType(req.SubjectType)
	if err != nil {
		return nil, err
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant id is required")
	}
	if req.SubjectID == uuid.Nil {
		return nil, fmt.Errorf("subject id is required")
	}
	if len(req.Capabilities) == 0 {
		return nil, fmt.Errorf("at least one capability is required")
	}
	if req.TTLSeconds > 0 && req.TTLSeconds < minGrantTTLSeconds {
		return nil, fmt.Errorf("ttl_seconds must be >= %d", minGrantTTLSeconds)
	}

	resolvedCaps, err := s.resolveCapabilities(ctx, req.Capabilities)
	if err != nil {
		return nil, err
	}

	conditions, conditionSnapshot, err := buildGrantConditions(req.Conditions)
	if err != nil {
		return nil, err
	}

	source := normalizeGrantSource(req.Source)
	now := s.clock().UTC()
	var expiresAt *time.Time
	if req.TTLSeconds > 0 {
		val := now.Add(time.Duration(req.TTLSeconds) * time.Second)
		expiresAt = &val
	}

	notesJSON, err := marshalJSON(req.Notes)
	if err != nil {
		return nil, fmt.Errorf("marshal notes: %w", err)
	}

	grant := &eventfabricmodel.AuthorizationGrant{
		TenantUUID:   strings.TrimSpace(req.TenantID.String()),
		SubjectType:  normalizedSubject,
		SubjectID:    req.SubjectID,
		Source:       source,
		TemplateID:   req.TemplateID,
		TTLExpiresAt: expiresAt,
		Notes:        notesJSON,
	}
	if req.CreatedBy != nil {
		grant.CreatedBy = req.CreatedBy
	}

	challenged := s.shouldChallenge(req.TTLSeconds, resolvedCaps)
	if challenged {
		grant.Status = eventfabricmodel.GrantStatusPending
	} else {
		grant.Status = eventfabricmodel.GrantStatusActive
	}

	capabilityRecords, err := buildGrantCapabilities(resolvedCaps)
	if err != nil {
		return nil, err
	}

	txRepo, tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer txRepo.RollbackTx(tx)

	created, err := txRepo.CreateGrant(ctx, grant)
	if err != nil {
		return nil, err
	}

	if err := txRepo.ReplaceGrantCapabilities(ctx, tx, created.UUID, capabilityRecords); err != nil {
		return nil, err
	}
	if err := txRepo.ReplaceGrantConditions(ctx, tx, created.UUID, conditions); err != nil {
		return nil, err
	}

	var ticket *eventfabricmodel.AuthorizationApprovalTicket
	var dispatchPayload *ChallengeDispatchPayload
	if challenged {
		ticket, dispatchPayload, err = s.createChallengeTicket(ctx, txRepo, created, resolvedCaps, conditionSnapshot)
		if err != nil {
			return nil, err
		}
	}

	if err := txRepo.CommitTx(tx); err != nil {
		return nil, err
	}

	grantWithLatest, err := s.repo.GetGrantByUUID(ctx, created.UUID)
	if err != nil {
		return nil, err
	}
	if grantWithLatest == nil {
		return nil, fmt.Errorf("grant lost after creation")
	}

	capabilities, err := s.repo.ListGrantCapabilities(ctx, created.UUID)
	if err != nil {
		return nil, err
	}
	conds, err := s.repo.ListGrantConditions(ctx, created.UUID)
	if err != nil {
		return nil, err
	}

	capMap, err := s.buildCapabilityMap(ctx, capabilities)
	if err != nil {
		return nil, err
	}

	if grantWithLatest.Status == eventfabricmodel.GrantStatusActive {
		if err := s.writeGrantCache(ctx, grantWithLatest, capabilities, capMap, conds); err != nil {
			s.logger.WarnF(ctx, "[authorization] write cache failed: %v", err)
		}
	} else {
		_ = s.InvalidateGrantCache(ctx, buildGrantCacheKey(grantWithLatest))
	}

	if challenged && ticket != nil && dispatchPayload != nil {
		if err := s.dispatchChallenge(ctx, ticket, *dispatchPayload); err != nil {
			s.logger.WarnF(ctx, "[authorization] dispatch challenge failed ticket=%s err=%v", ticket.UUID, err)
		}
	}

	s.emitAudit(ctx, "grant.created", grantWithLatest, req.CreatedBy, map[string]string{
		"challenged": fmt.Sprintf("%t", challenged),
		"source":     grantWithLatest.Source,
	})

	return &GrantResult{
		Grant:         grantWithLatest,
		Capabilities:  capabilities,
		Conditions:    conds,
		CapabilityMap: capMap,
		Challenged:    challenged,
		Ticket:        ticket,
	}, nil
}

// UpdateGrant 更新授权 Grant。
func (s *serviceImpl) UpdateGrant(ctx context.Context, grantID uuid.UUID, req GrantUpdateRequest) (*GrantResult, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if grantID == uuid.Nil {
		return nil, fmt.Errorf("grant id is required")
	}

	existing, err := s.repo.GetGrantByUUID(ctx, grantID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrGrantNotFound
	}
	if existing.Status == eventfabricmodel.GrantStatusRevoked || existing.Status == eventfabricmodel.GrantStatusExpired {
		return nil, fmt.Errorf("grant is immutable in status=%s", existing.Status)
	}

	currentCaps, err := s.repo.ListGrantCapabilities(ctx, grantID)
	if err != nil {
		return nil, err
	}

	var newCapRecords []*eventfabricmodel.AuthorizationGrantCapability
	var resolvedCaps []resolvedCapability
	capabilitiesProvided := req.Capabilities != nil
	if capabilitiesProvided {
		if len(*req.Capabilities) == 0 {
			return nil, fmt.Errorf("grant must retain at least one capability")
		}
		resolvedCaps, err = s.resolveCapabilities(ctx, *req.Capabilities)
		if err != nil {
			return nil, err
		}
		newCapRecords, err = buildGrantCapabilities(resolvedCaps)
		if err != nil {
			return nil, err
		}
	} else {
		resolvedCaps, err = s.resolveExistingCapabilities(ctx, currentCaps)
		if err != nil {
			return nil, err
		}
	}

	var ttlSeconds int64
	if req.TTLSeconds != nil {
		ttlSeconds = *req.TTLSeconds
		if ttlSeconds > 0 && ttlSeconds < minGrantTTLSeconds {
			return nil, fmt.Errorf("ttl_seconds must be >= %d", minGrantTTLSeconds)
		}
	} else if existing.TTLExpiresAt != nil {
		ttlSeconds = int64(existing.TTLExpiresAt.Sub(s.clock().UTC()).Seconds())
		if ttlSeconds < 0 {
			ttlSeconds = 0
		}
	}

	var updatedConditions []*eventfabricmodel.AuthorizationGrantCondition
	var conditionSnapshot map[string]any
	conditionsProvided := req.Conditions != nil
	if conditionsProvided {
		updatedConditions, conditionSnapshot, err = buildGrantConditions(*req.Conditions)
		if err != nil {
			return nil, err
		}
	}

	challenged := s.shouldChallenge(ttlSeconds, resolvedCaps)

	fields := map[string]any{}
	now := s.clock().UTC()

	if req.TTLSeconds != nil {
		if ttlSeconds <= 0 {
			fields["ttl_expires_at"] = nil
		} else {
			expires := now.Add(time.Duration(ttlSeconds) * time.Second)
			fields["ttl_expires_at"] = expires
		}
	}

	if req.Notes != nil {
		notesJSON, err := marshalJSON(req.Notes)
		if err != nil {
			return nil, fmt.Errorf("marshal notes: %w", err)
		}
		fields["notes"] = notesJSON
	}

	if challenged {
		fields["status"] = eventfabricmodel.GrantStatusPending
	} else {
		fields["status"] = eventfabricmodel.GrantStatusActive
	}

	txRepo, tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer txRepo.RollbackTx(tx)

	if capabilitiesProvided {
		if err := txRepo.ReplaceGrantCapabilities(ctx, tx, grantID, newCapRecords); err != nil {
			return nil, err
		}
	}
	if conditionsProvided {
		if err := txRepo.ReplaceGrantConditions(ctx, tx, grantID, updatedConditions); err != nil {
			return nil, err
		}
	}
	if len(fields) > 0 {
		if err := txRepo.UpdateGrantFields(ctx, grantID, fields); err != nil {
			return nil, err
		}
	}
	if err := txRepo.IncrementGrantVersion(ctx, grantID); err != nil {
		return nil, err
	}

	var ticket *eventfabricmodel.AuthorizationApprovalTicket
	var dispatchPayload *ChallengeDispatchPayload
	if challenged {
		ticket, dispatchPayload, err = s.createChallengeTicket(ctx, txRepo, existing, resolvedCaps, conditionSnapshot)
		if err != nil {
			return nil, err
		}
	} else {
		// 清理之前未完成的 Challenge
		if err := s.resolvePendingChallenge(ctx, txRepo, existing.UUID, req.ActorID, false); err != nil && !errors.Is(err, ErrChallengeNotFound) {
			return nil, err
		}
		ticket = nil
	}

	if err := txRepo.CommitTx(tx); err != nil {
		return nil, err
	}

	refreshed, err := s.repo.GetGrantByUUID(ctx, grantID)
	if err != nil {
		return nil, err
	}
	if refreshed == nil {
		return nil, ErrGrantNotFound
	}

	capabilities, err := s.repo.ListGrantCapabilities(ctx, grantID)
	if err != nil {
		return nil, err
	}
	conds, err := s.repo.ListGrantConditions(ctx, grantID)
	if err != nil {
		return nil, err
	}
	capMap, err := s.buildCapabilityMap(ctx, capabilities)
	if err != nil {
		return nil, err
	}

	if refreshed.Status == eventfabricmodel.GrantStatusActive {
		if err := s.writeGrantCache(ctx, refreshed, capabilities, capMap, conds); err != nil {
			s.logger.WarnF(ctx, "[authorization] write cache failed: %v", err)
		}
	} else {
		_ = s.InvalidateGrantCache(ctx, buildGrantCacheKey(refreshed))
	}

	if challenged && ticket != nil && dispatchPayload != nil {
		if err := s.dispatchChallenge(ctx, ticket, *dispatchPayload); err != nil {
			s.logger.WarnF(ctx, "[authorization] dispatch challenge failed ticket=%s err=%v", ticket.UUID, err)
		}
	}

	var auditActor *uuid.UUID
	if req.ActorID != nil && *req.ActorID != uuid.Nil {
		actor := *req.ActorID
		auditActor = &actor
	}

	s.emitAudit(ctx, "grant.updated", refreshed, auditActor, map[string]string{
		"challenged": fmt.Sprintf("%t", challenged),
		"reason":     req.Reason,
	})

	return &GrantResult{
		Grant:         refreshed,
		Capabilities:  capabilities,
		Conditions:    conds,
		CapabilityMap: capMap,
		Challenged:    challenged,
		Ticket:        ticket,
	}, nil
}

// RevokeGrant 撤销 Grant。
func (s *serviceImpl) RevokeGrant(ctx context.Context, grantID uuid.UUID, actor uuid.UUID, reason string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	if grantID == uuid.Nil {
		return fmt.Errorf("grant id is required")
	}

	grant, err := s.repo.GetGrantByUUID(ctx, grantID)
	if err != nil {
		return err
	}
	if grant == nil {
		return ErrGrantNotFound
	}
	if grant.Status == eventfabricmodel.GrantStatusRevoked {
		return nil
	}

	now := s.clock().UTC()
	fields := map[string]any{
		"status":         eventfabricmodel.GrantStatusRevoked,
		"revoked_at":     now,
		"revoked_reason": reason,
		"revoked_by":     actor,
		"ttl_expires_at": now,
	}

	if err := s.repo.UpdateGrantFields(ctx, grantID, fields); err != nil {
		return err
	}
	if err := s.repo.IncrementGrantVersion(ctx, grantID); err != nil {
		return err
	}

	grant.Status = eventfabricmodel.GrantStatusRevoked
	grant.RevokedAt = &now
	grant.RevokedReason = reason
	if actor != uuid.Nil {
		grant.RevokedBy = &actor
	}
	grant.TTLExpiresAt = &now

	if err := s.InvalidateGrantCache(ctx, buildGrantCacheKey(grant)); err != nil {
		s.logger.WarnF(ctx, "[authorization] invalidate cache failed grant=%s err=%v", grant.UUID, err)
	}

	var auditActor *uuid.UUID
	if actor != uuid.Nil {
		auditActor = &actor
	}

	s.emitAudit(ctx, "grant.revoked", grant, auditActor, map[string]string{
		"reason": reason,
	})

	return nil
}

// GetGrant 返回 Grant 详情。
func (s *serviceImpl) GetGrant(ctx context.Context, grantID uuid.UUID, withRelations bool) (*GrantDetail, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if grantID == uuid.Nil {
		return nil, fmt.Errorf("grant id is required")
	}

	grant, err := s.repo.GetGrantByUUID(ctx, grantID)
	if err != nil {
		return nil, err
	}
	if grant == nil {
		return nil, ErrGrantNotFound
	}

	detail := &GrantDetail{Grant: grant}
	if !withRelations {
		return detail, nil
	}

	capabilities, err := s.repo.ListGrantCapabilities(ctx, grantID)
	if err != nil {
		return nil, err
	}
	conds, err := s.repo.ListGrantConditions(ctx, grantID)
	if err != nil {
		return nil, err
	}
	capMap, err := s.buildCapabilityMap(ctx, capabilities)
	if err != nil {
		return nil, err
	}
	ticket, err := s.repo.GetLatestTicketByGrant(ctx, grant.UUID)
	if err != nil {
		return nil, err
	}

	detail.Capabilities = capabilities
	detail.Conditions = conds
	detail.CapabilityMap = capMap
	detail.Ticket = ticket
	if ticket != nil && ticket.DecisionBy != nil {
		detail.ApprovedBy = ticket.DecisionBy
	}
	return detail, nil
}

// ListGrants 查询 Grant 列表。
func (s *serviceImpl) ListGrants(ctx context.Context, filter GrantFilter) ([]*GrantSummary, int64, error) {
	if err := s.ensureReady(); err != nil {
		return nil, 0, err
	}
	filters := map[string]any{}
	if len(filter.Status) > 0 {
		filters["status"] = filter.Status
	}
	if filter.SubjectType != "" {
		filters["subject_type"] = filter.SubjectType
	}
	if filter.SubjectID != uuid.Nil {
		filters["subject_id"] = filter.SubjectID
	}

	grants, total, err := s.repo.ListGrants(ctx, filter.TenantID, filters, filter.Page, filter.PageSize)
	if err != nil {
		return nil, 0, err
	}

	summaries := make([]*GrantSummary, 0, len(grants))
	for _, g := range grants {
		summaries = append(summaries, &GrantSummary{Grant: g})
	}
	return summaries, total, nil
}

func (s *serviceImpl) resolveCapabilities(ctx context.Context, inputs []GrantCapabilityInput) ([]resolvedCapability, error) {
	unique := make(map[string]GrantCapabilityInput, len(inputs))
	order := make([]string, 0, len(inputs))
	for _, input := range inputs {
		ns := strings.TrimSpace(input.Namespace)
		action := strings.TrimSpace(input.Action)
		if ns == "" || action == "" {
			return nil, fmt.Errorf("capability namespace/action cannot be empty")
		}
		key := capabilityKey(ns, action)
		if _, exists := unique[key]; !exists {
			order = append(order, key)
		}
		unique[key] = GrantCapabilityInput{
			Namespace:       ns,
			Action:          action,
			CustomRateLimit: input.CustomRateLimit,
		}
	}

	result := make([]resolvedCapability, 0, len(order))
	for _, key := range order {
		input := unique[key]
		capability, err := s.repo.GetCapabilityByNamespaceAction(ctx, input.Namespace, input.Action)
		if err != nil {
			return nil, err
		}
		if capability == nil {
			return nil, fmt.Errorf("%w: %s.%s", ErrCapabilityNotFound, input.Namespace, input.Action)
		}
		result = append(result, resolvedCapability{
			Input: input,
			Model: capability,
		})
	}
	return result, nil
}

func (s *serviceImpl) resolveExistingCapabilities(ctx context.Context, records []*eventfabricmodel.AuthorizationGrantCapability) ([]resolvedCapability, error) {
	if len(records) == 0 {
		return nil, nil
	}
	capMap, err := s.buildCapabilityMap(ctx, records)
	if err != nil {
		return nil, err
	}
	result := make([]resolvedCapability, 0, len(records))
	for _, record := range records {
		model := capMap[record.CapabilityID]
		if model == nil {
			continue
		}
		input := GrantCapabilityInput{
			Namespace: model.Namespace,
			Action:    model.Action,
		}
		if len(record.CustomRateLimit) > 0 {
			var custom map[string]any
			_ = json.Unmarshal(record.CustomRateLimit, &custom)
			input.CustomRateLimit = custom
		}
		result = append(result, resolvedCapability{
			Input: input,
			Model: model,
		})
	}
	return result, nil
}

func buildGrantCapabilities(resolved []resolvedCapability) ([]*eventfabricmodel.AuthorizationGrantCapability, error) {
	items := make([]*eventfabricmodel.AuthorizationGrantCapability, 0, len(resolved))
	for _, item := range resolved {
		record := &eventfabricmodel.AuthorizationGrantCapability{
			CapabilityID: item.Model.UUID,
		}
		if len(item.Input.CustomRateLimit) > 0 {
			blob, err := json.Marshal(item.Input.CustomRateLimit)
			if err != nil {
				return nil, fmt.Errorf("marshal custom rate limit: %w", err)
			}
			record.CustomRateLimit = datatypes.JSON(blob)
		}
		items = append(items, record)
	}
	return items, nil
}

func buildGrantConditions(req GrantConditionsInput) ([]*eventfabricmodel.AuthorizationGrantCondition, map[string]any, error) {
	conditions := make([]*eventfabricmodel.AuthorizationGrantCondition, 0, 3)
	snapshot := map[string]any{}

	if len(req.Resources) > 0 {
		payload := map[string]any{"resources": req.Resources}
		blob, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal resources: %w", err)
		}
		conditions = append(conditions, &eventfabricmodel.AuthorizationGrantCondition{
			Type:       eventfabricmodel.GrantConditionTypeResource,
			Expression: datatypes.JSON(blob),
		})
		snapshot["resources"] = req.Resources
	}

	if len(req.ContextTags) > 0 {
		payload := map[string]any{"context_tags": req.ContextTags}
		blob, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal context tags: %w", err)
		}
		conditions = append(conditions, &eventfabricmodel.AuthorizationGrantCondition{
			Type:       eventfabricmodel.GrantConditionTypeContextTag,
			Expression: datatypes.JSON(blob),
		})
		snapshot["context_tags"] = req.ContextTags
	}

	if req.TimeWindow != nil {
		if req.TimeWindow.End.Before(req.TimeWindow.Start) {
			return nil, nil, fmt.Errorf("time_window end must be after start")
		}
		payload := map[string]any{
			"start": req.TimeWindow.Start.UTC(),
			"end":   req.TimeWindow.End.UTC(),
		}
		blob, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal time window: %w", err)
		}
		conditions = append(conditions, &eventfabricmodel.AuthorizationGrantCondition{
			Type:       eventfabricmodel.GrantConditionTypeTimeWindow,
			Expression: datatypes.JSON(blob),
		})
		snapshot["time_window"] = payload
	}

	return conditions, snapshot, nil
}

func (s *serviceImpl) shouldChallenge(ttlSeconds int64, capabilities []resolvedCapability) bool {
	if len(capabilities) == 0 {
		return false
	}
	for _, cap := range capabilities {
		level := strings.ToLower(cap.Model.RiskLevel)
		if level != eventfabricmodel.RiskLevelLow {
			return true
		}
	}
	if ttlSeconds <= 0 {
		return false
	}
	return time.Duration(ttlSeconds)*time.Second > autoApproveTTLLimit
}

func (s *serviceImpl) buildCapabilityMap(ctx context.Context, records []*eventfabricmodel.AuthorizationGrantCapability) (map[uuid.UUID]*eventfabricmodel.AuthorizationCapability, error) {
	ids := make([]uuid.UUID, 0, len(records))
	seen := make(map[uuid.UUID]struct{}, len(records))
	for _, record := range records {
		if record == nil || record.CapabilityID == uuid.Nil {
			continue
		}
		if _, ok := seen[record.CapabilityID]; ok {
			continue
		}
		seen[record.CapabilityID] = struct{}{}
		ids = append(ids, record.CapabilityID)
	}
	if len(ids) == 0 {
		return map[uuid.UUID]*eventfabricmodel.AuthorizationCapability{}, nil
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	result := make(map[uuid.UUID]*eventfabricmodel.AuthorizationCapability, len(ids))
	for _, id := range ids {
		record, err := s.repo.GetCapabilityByUUID(ctx, id)
		if err != nil {
			return nil, err
		}
		if record != nil {
			result[id] = record
		}
	}
	return result, nil
}

func (s *serviceImpl) writeGrantCache(ctx context.Context, grant *eventfabricmodel.AuthorizationGrant, capabilities []*eventfabricmodel.AuthorizationGrantCapability, capMap map[uuid.UUID]*eventfabricmodel.AuthorizationCapability, conditions []*eventfabricmodel.AuthorizationGrantCondition) error {
	if s.cache == nil || grant == nil {
		return nil
	}
	payload := buildGrantCachePayload(grant, capabilities, capMap, conditions)
	blob, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal cache payload: %w", err)
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
	return s.cache.Set(ctx, buildGrantCacheKey(grant), entry)
}

func buildGrantCachePayload(grant *eventfabricmodel.AuthorizationGrant, capabilities []*eventfabricmodel.AuthorizationGrantCapability, capMap map[uuid.UUID]*eventfabricmodel.AuthorizationCapability, conditions []*eventfabricmodel.AuthorizationGrantCondition) map[string]any {
	tenantUUID := tenantUUIDFromGrant(grant)
	payload := map[string]any{
		"grant_id":     grant.UUID.String(),
		"tenant_uuid":  tenantUUID,
		"subject_type": grant.SubjectType,
		"subject_id":   grant.SubjectID.String(),
		"status":       grant.Status,
		"source":       grant.Source,
		"version":      grant.Version,
	}
	if grant.TTLExpiresAt != nil {
		payload["ttl_expires_at"] = grant.TTLExpiresAt.UTC()
	}

	items := make([]map[string]any, 0, len(capabilities))
	for _, record := range capabilities {
		capability := capMap[record.CapabilityID]
		item := map[string]any{
			"capability_id": record.CapabilityID.String(),
		}
		if capability != nil {
			item["namespace"] = capability.Namespace
			item["action"] = capability.Action
			if len(capability.DefaultRateLimit) > 0 {
				var defaults map[string]any
				if err := json.Unmarshal(capability.DefaultRateLimit, &defaults); err == nil {
					item["default_rate_limit"] = defaults
				}
			}
		}
		if len(record.CustomRateLimit) > 0 {
			var custom map[string]any
			if err := json.Unmarshal(record.CustomRateLimit, &custom); err == nil {
				item["custom_rate_limit"] = custom
			}
		}
		items = append(items, item)
	}
	payload["capabilities"] = items

	if len(conditions) > 0 {
		conditionPayload := map[string]any{}
		for _, cond := range conditions {
			if cond == nil {
				continue
			}
			switch cond.Type {
			case eventfabricmodel.GrantConditionTypeResource:
				var data map[string]any
				if err := json.Unmarshal(cond.Expression, &data); err == nil {
					conditionPayload["resources"] = data["resources"]
				}
			case eventfabricmodel.GrantConditionTypeContextTag:
				var data map[string]any
				if err := json.Unmarshal(cond.Expression, &data); err == nil {
					conditionPayload["context_tags"] = data["context_tags"]
				}
			case eventfabricmodel.GrantConditionTypeTimeWindow:
				var data map[string]any
				if err := json.Unmarshal(cond.Expression, &data); err == nil {
					conditionPayload["time_window"] = data
				}
			}
		}
		payload["conditions"] = conditionPayload
	}

	return payload
}

func buildGrantCacheKey(grant *eventfabricmodel.AuthorizationGrant) GrantCacheKey {
	return GrantCacheKey{
		TenantUUID:  tenantUUIDFromGrant(grant),
		SubjectType: grant.SubjectType,
		SubjectID:   grant.SubjectID,
	}
}

func capabilityKey(namespace, action string) string {
	return strings.ToLower(namespace) + "|" + strings.ToLower(action)
}

func normalizeSubjectType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SubjectTypeAgent:
		return SubjectTypeAgent, nil
	case SubjectTypePlugin:
		return SubjectTypePlugin, nil
	default:
		return "", fmt.Errorf("unsupported subject type %q", value)
	}
}

func normalizeGrantSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case eventfabricmodel.GrantSourceTenantTemplate:
		return eventfabricmodel.GrantSourceTenantTemplate
	case eventfabricmodel.GrantSourceSessionTemp:
		return eventfabricmodel.GrantSourceSessionTemp
	default:
		return eventfabricmodel.GrantSourceSystemTemplate
	}
}

func marshalJSON(val map[string]any) (datatypes.JSON, error) {
	if len(val) == 0 {
		return nil, nil
	}
	blob, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(blob), nil
}

func (s *serviceImpl) emitAudit(ctx context.Context, action string, grant *eventfabricmodel.AuthorizationGrant, actor *uuid.UUID, meta map[string]string) {
	if s.audit == nil || grant == nil {
		return
	}
	if meta == nil {
		meta = map[string]string{}
	}
	tenantUUID := tenantUUIDFromGrant(grant)
	meta["tenant_uuid"] = tenantUUID
	meta["subject_id"] = grant.SubjectID.String()
	meta["subject_type"] = strings.ToLower(grant.SubjectType)
	meta["grant_id"] = grant.UUID.String()
	meta["grant_status"] = grant.Status
	meta["grant_version"] = fmt.Sprintf("%d", grant.Version)
	record := eventaudit.Record{
		ID:          grant.UUID.String(),
		TenantID:    tenantUUID,
		Topic:       auditTopicAuthorization,
		PrincipalID: "",
		Action:      strings.ToUpper(action),
		Status:      "SUCCESS",
		Metadata:    meta,
		HappenedAt:  s.clock().UTC(),
	}
	if actor != nil && *actor != uuid.Nil {
		record.PrincipalID = actor.String()
	}
	if err := s.audit.Write(ctx, record); err != nil {
		s.logger.WarnF(ctx, "[authorization] audit failed action=%s grant=%s err=%v", action, grant.UUID, err)
	}
}

func (s *serviceImpl) dispatchChallenge(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket, payload ChallengeDispatchPayload) error {
	if s.dispatcher == nil {
		return nil
	}
	return s.dispatcher.DispatchChallenge(ctx, ticket, payload)
}

func (s *serviceImpl) createChallengeTicket(ctx context.Context, repo *eventfabricrepo.AuthorizationRepository, grant *eventfabricmodel.AuthorizationGrant, caps []resolvedCapability, snapshot map[string]any) (*eventfabricmodel.AuthorizationApprovalTicket, *ChallengeDispatchPayload, error) {
	if repo == nil || grant == nil {
		return nil, nil, fmt.Errorf("challenge creation requires repository and grant")
	}

	fingerprint := uuid.New()
	slaExpires := s.clock().UTC().Add(s.challengeSLA)
	tenantUUID := tenantUUIDFromGrant(grant)
	ticketPayload := map[string]any{
		"grant_id":     grant.UUID.String(),
		"tenant_uuid":  tenantUUID,
		"subject_id":   grant.SubjectID.String(),
		"subject_type": grant.SubjectType,
	}

	capNames := make([]string, 0, len(caps))
	for _, cap := range caps {
		name := fmt.Sprintf("%s.%s", cap.Model.Namespace, cap.Model.Action)
		capNames = append(capNames, name)
	}
	ticketPayload["capabilities"] = capNames
	if snapshot != nil {
		ticketPayload["conditions"] = snapshot
	}
	blob, err := json.Marshal(ticketPayload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal challenge snapshot: %w", err)
	}

	ticket := &eventfabricmodel.AuthorizationApprovalTicket{
		TenantUUID:         tenantUUID,
		GrantID:            &grant.UUID,
		RequestFingerprint: fingerprint,
		Status:             eventfabricmodel.ApprovalStatusPending,
		SLAExpiresAt:       slaExpires,
		PayloadSnapshot:    datatypes.JSON(blob),
	}

	createdTicket, err := repo.CreateApprovalTicket(ctx, ticket)
	if err != nil {
		return nil, nil, err
	}

	payload := ChallengeDispatchPayload{
		GrantUUID:          &grant.UUID,
		TenantUUID:         tenantUUID,
		RequestFingerprint: fingerprint,
		SubjectType:        grant.SubjectType,
		SubjectID:          grant.SubjectID.String(),
		Capabilities:       capNames,
		Conditions:         snapshot,
		SLAExpiresAt:       slaExpires,
		IssuedAt:           createdTicket.CreatedAt,
		Metadata: map[string]any{
			"source": grant.Source,
		},
	}

	return createdTicket, &payload, nil
}
