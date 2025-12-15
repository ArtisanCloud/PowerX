package agent_lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	"github.com/google/uuid"
)

// ShareAgent 创建共享记录并复制配额。
func (s *Service) ShareAgent(ctx context.Context, in ShareInput) (*AgentShare, error) {
	if s.shares == nil {
		return nil, fmt.Errorf("share repository not configured")
	}
	if in.AgentID == uuid.Nil || strings.TrimSpace(in.TenantUUID) == "" {
		return nil, fmt.Errorf("agent_id and tenant_uuid are required")
	}

	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	agentModel, err := s.profiles.GetByUUID(ctx, in.AgentID)
	if err != nil {
		return nil, err
	}
	ctx = agentinstr.WithTenant(ctx, agentModel.TenantUUID)
	agent := toAgent(agentModel, decodeToolGrantsJSON(agentModel.ToolGrants), decodeStringMap(agentModel.Metadata))

	if _, err := s.shares.FindActiveByAgentTenant(ctx, in.AgentID, in.TenantUUID); err == nil {
		return nil, ErrAgentShareExists
	}

	if err := s.shareValidator.Validate(ctx, agent, in.TenantUUID, in.Quotas, in.Metadata); err != nil {
		s.emitShareValidationFailure(ctx, nil, in.TraceID, err)
		return nil, fmt.Errorf("%w: %s", ErrShareValidationFailed, err.Error())
	}

	now := s.clock()
	record := &agentmodel.AgentShareRecord{
		AgentUUID:        in.AgentID,
		TargetTenantUUID: in.TenantUUID,
		Status:           ShareStatusPending,
		Quotas:           encodeShareQuotas(in.Quotas),
		Metadata:         encodeStringMap(in.Metadata),
		IssuedBy:         in.RequestedBy,
		ValidatedAt:      &now,
		NextReviewAt:     s.nextShareReviewAt(now),
	}

	created, err := s.shares.Create(ctx, record)
	if err != nil {
		if isUniqueIndexError(err, "idx_agent_share_unique") {
			return nil, ErrAgentShareExists
		}
		return nil, err
	}

	if err := s.quotaProvisioner.Provision(ctx, agent, in.TenantUUID, in.Quotas, in.Metadata); err != nil {
		s.failShareProvision(ctx, created, err)
		s.notifyShareFailure(ctx, toAgentShare(created), traceID, err)
		return nil, err
	}

	now = s.clock()
	created.Status = ShareStatusActive
	created.ProvisionedAt = &now
	if created.NextReviewAt == nil {
		created.NextReviewAt = s.nextShareReviewAt(now)
	}

	saved, err := s.shares.Save(ctx, created)
	if err != nil {
		return nil, err
	}
	share := toAgentShare(saved)

	s.emitShareEvent(ctx, "agent.share.issued", share, traceID)
	s.auditShareOperation(ctx, share, agent.TenantUUID, "SHARE_AGENT", "SUCCESS")
	s.notifyShareIssued(ctx, share, traceID)
	return share, nil
}

// RevokeAgentShare 撤销共享。
func (s *Service) RevokeAgentShare(ctx context.Context, in RevokeShareInput) (*AgentShare, error) {
	if s.shares == nil {
		return nil, fmt.Errorf("share repository not configured")
	}
	if in.ShareID == uuid.Nil {
		return nil, fmt.Errorf("share_id required")
	}

	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	record, err := s.shares.GetByUUID(ctx, in.ShareID)
	if err != nil {
		return nil, ErrAgentShareNotFound
	}
	if strings.EqualFold(record.Status, ShareStatusRevoked) {
		return toAgentShare(record), nil
	}

	agentModel, ownerErr := s.profiles.GetByUUID(ctx, record.AgentUUID)
	if ownerErr == nil {
		ctx = agentinstr.WithTenant(ctx, agentModel.TenantUUID)
	}

	share := toAgentShare(record)
	if err := s.quotaProvisioner.Release(ctx, share); err != nil {
		return nil, err
	}

	now := s.clock()
	record.Status = ShareStatusRevoked
	record.RevokedAt = &now
	record.RevokeReason = in.Reason
	record.RevokedBy = in.RequestedBy
	record.NextReviewAt = nil
	if _, err := s.shares.Save(ctx, record); err != nil {
		return nil, err
	}
	share = toAgentShare(record)

	s.emitShareEvent(ctx, "agent.share.revoked", share, traceID)
	ownerTenant := ""
	if ownerErr == nil {
		ownerTenant = agentModel.TenantUUID
	}
	s.auditShareOperation(ctx, share, ownerTenant, "REVOKE_AGENT_SHARE", "SUCCESS")
	s.notifyShareRevoked(ctx, share, traceID)
	return share, nil
}

func (s *Service) failShareProvision(ctx context.Context, record *agentmodel.AgentShareRecord, cause error) {
	if record == nil {
		return
	}
	record = markShareError(record, cause.Error())
	if _, saveErr := s.shares.Save(ctx, record); saveErr != nil && s.instr != nil {
		s.instr.Logger(ctx).WarnF(ctx, "failed to persist share error state: %v", saveErr)
	}
}

func (s *Service) emitShareEvent(ctx context.Context, topic string, share *AgentShare, traceID string) {
	if s.bus == nil || share == nil {
		return
	}
	payload := map[string]any{
		"share_id":    share.ID.String(),
		"agent_id":    share.AgentID.String(),
		"tenant_uuid": share.TenantUUID,
		"status":      share.Status,
		"trace_id":    traceID,
		"revoked_by":  share.RevokedBy,
	}
	s.bus.Publish(topic, payload, ctx)
}

func (s *Service) emitShareValidationFailure(ctx context.Context, share *AgentShare, traceID string, reason error) {
	if share != nil {
		s.emitShareEvent(ctx, "agent.share.validation_failed", share, traceID)
	}
	if s.notifier == nil {
		return
	}
	_ = s.notifier.Send(ctx, imnotify.Message{
		Title:    "Agent share validation failed",
		Content:  reason.Error(),
		Severity: "critical",
		TraceID:  traceID,
	})
}

func (s *Service) notifyShareIssued(ctx context.Context, share *AgentShare, traceID string) {
	if s.notifier == nil || share == nil {
		return
	}
	msg := imnotify.Message{
		Title:    "Agent share issued",
		Content:  fmt.Sprintf("Agent %s shared to tenant %s", share.AgentID, share.TenantUUID),
		Severity: "info",
		TraceID:  traceID,
		Metadata: map[string]any{
			"share_id": share.ID.String(),
		},
	}
	_ = s.notifier.Send(ctx, msg)
}

func (s *Service) notifyShareRevoked(ctx context.Context, share *AgentShare, traceID string) {
	if s.notifier == nil || share == nil {
		return
	}
	msg := imnotify.Message{
		Title:    "Agent share revoked",
		Content:  fmt.Sprintf("Share %s revoked: %s", share.ID, share.Reason),
		Severity: "warning",
		TraceID:  traceID,
		Metadata: map[string]any{
			"share_id": share.ID.String(),
		},
	}
	_ = s.notifier.Send(ctx, msg)
}

func (s *Service) notifyShareFailure(ctx context.Context, share *AgentShare, traceID string, cause error) {
	if s.notifier == nil {
		return
	}
	msg := imnotify.Message{
		Title:    "Agent share provisioning failed",
		Content:  cause.Error(),
		Severity: "critical",
		TraceID:  traceID,
	}
	if share != nil {
		msg.Metadata = map[string]any{
			"share_id": share.ID.String(),
		}
	}
	_ = s.notifier.Send(ctx, msg)
}

func (s *Service) auditShareOperation(ctx context.Context, share *AgentShare, tenantUUID, operation, outcome string) {
	if s.instr == nil || share == nil {
		return
	}
	meta := map[string]any{
		"target_tenant_uuid": share.TenantUUID,
		"share_id":           share.ID.String(),
		"status":             share.Status,
	}
	s.instr.AuditLifecycleEvent(ctx, tenantUUID, share.AgentID.String(), operation, outcome, meta)
}

func (s *Service) nextShareReviewAt(now time.Time) *time.Time {
	interval := s.config.ShareReviewInterval
	if interval <= 0 {
		interval = 30 * 24 * time.Hour
	}
	next := now.Add(interval)
	return &next
}

func markShareError(record *agentmodel.AgentShareRecord, reason string) *agentmodel.AgentShareRecord {
	if record == nil {
		return nil
	}
	record.Status = ShareStatusError
	record.ValidationFail = true
	record.ValidationError = reason
	return record
}

func isUniqueIndexError(err error, indexName string) bool {
	if err == nil {
		return false
	}
	if indexName == "" {
		indexName = "idx_agent_share_unique"
	}
	return strings.Contains(strings.ToLower(err.Error()), strings.ToLower(indexName))
}
