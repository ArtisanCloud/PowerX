package authorization

import (
	"context"
	"fmt"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
)

// DecideChallenge 处理人工 Challenge 决策。
func (s *serviceImpl) DecideChallenge(ctx context.Context, ticketID uuid.UUID, decision ChallengeDecisionInput) (*ChallengeDecisionResult, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if ticketID == uuid.Nil {
		return nil, fmt.Errorf("ticket id is required")
	}

	outcome := strings.ToLower(strings.TrimSpace(decision.Decision))
	if outcome != challengeDecisionApprove && outcome != challengeDecisionReject {
		return nil, fmt.Errorf("invalid decision %q", decision.Decision)
	}

	ticket, err := s.repo.GetTicketByUUID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket == nil {
		return nil, ErrChallengeNotFound
	}
	if ticket.Status != eventfabricmodel.ApprovalStatusPending {
		return nil, ErrChallengeResolved
	}
	if ticket.GrantID == nil || *ticket.GrantID == uuid.Nil {
		return nil, fmt.Errorf("challenge ticket missing grant reference")
	}

	grant, err := s.repo.GetGrantByUUID(ctx, *ticket.GrantID)
	if err != nil {
		return nil, err
	}
	if grant == nil {
		return nil, ErrGrantNotFound
	}

	txRepo, tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer txRepo.RollbackTx(tx)

	now := s.clock().UTC()

	status := eventfabricmodel.ApprovalStatusApproved
	if outcome == challengeDecisionReject {
		status = eventfabricmodel.ApprovalStatusRejected
	}

	ticket.Status = status
	ticket.DecisionReason = strings.TrimSpace(decision.Reason)
	ticket.DecisionAt = &now
	if decision.ActorID != uuid.Nil {
		actor := decision.ActorID
		ticket.DecisionBy = &actor
	}

	updatedTicket, err := txRepo.UpdateApprovalTicket(ctx, ticket)
	if err != nil {
		return nil, err
	}

	grantFields := map[string]any{}
	if outcome == challengeDecisionApprove {
		grantFields["status"] = eventfabricmodel.GrantStatusActive
		grantFields["revoked_at"] = nil
		grantFields["revoked_by"] = nil
		grantFields["revoked_reason"] = nil
	} else {
		grantFields["status"] = eventfabricmodel.GrantStatusRevoked
		grantFields["revoked_at"] = now
		grantFields["revoked_reason"] = ticket.DecisionReason
		grantFields["ttl_expires_at"] = now
		if decision.ActorID != uuid.Nil {
			grantFields["revoked_by"] = decision.ActorID
		}
	}

	if len(grantFields) > 0 {
		if err := txRepo.UpdateGrantFields(ctx, grant.UUID, grantFields); err != nil {
			return nil, err
		}
	}
	if err := txRepo.IncrementGrantVersion(ctx, grant.UUID); err != nil {
		return nil, err
	}

	if err := txRepo.CommitTx(tx); err != nil {
		return nil, err
	}

	refreshedGrant, err := s.repo.GetGrantByUUID(ctx, grant.UUID)
	if err != nil {
		return nil, err
	}
	if refreshedGrant == nil {
		return nil, ErrGrantNotFound
	}

	capabilities, err := s.repo.ListGrantCapabilities(ctx, refreshedGrant.UUID)
	if err != nil {
		return nil, err
	}
	capMap, err := s.buildCapabilityMap(ctx, capabilities)
	if err != nil {
		return nil, err
	}
	conditions, err := s.repo.ListGrantConditions(ctx, refreshedGrant.UUID)
	if err != nil {
		return nil, err
	}

	if outcome == challengeDecisionApprove {
		if err := s.writeGrantCache(ctx, refreshedGrant, capabilities, capMap, conditions); err != nil {
			s.logger.WarnF(ctx, "[authorization] cache update after approve failed grant=%s err=%v", refreshedGrant.UUID, err)
		}
		s.emitAudit(ctx, "challenge.approved", refreshedGrant, maybeUUID(decision.ActorID), map[string]string{
			"ticket_id": ticket.UUID.String(),
		})
		s.emitAudit(ctx, "grant.updated", refreshedGrant, maybeUUID(decision.ActorID), map[string]string{
			"ticket_id": ticket.UUID.String(),
		})
	} else {
		if err := s.InvalidateGrantCache(ctx, buildGrantCacheKey(refreshedGrant)); err != nil {
			s.logger.WarnF(ctx, "[authorization] cache invalidate after reject failed grant=%s err=%v", refreshedGrant.UUID, err)
		}
		s.emitAudit(ctx, "challenge.rejected", refreshedGrant, maybeUUID(decision.ActorID), map[string]string{
			"ticket_id": ticket.UUID.String(),
			"reason":    ticket.DecisionReason,
		})
		s.emitAudit(ctx, "grant.revoked", refreshedGrant, maybeUUID(decision.ActorID), map[string]string{
			"ticket_id": ticket.UUID.String(),
			"reason":    ticket.DecisionReason,
		})
	}

	return &ChallengeDecisionResult{Ticket: updatedTicket}, nil
}

// ProcessExpiredChallenges 将超过 SLA 的 Challenge 自动拒绝。
func (s *serviceImpl) ProcessExpiredChallenges(ctx context.Context, tenantID uuid.UUID, before time.Time) (int, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	if tenantID == uuid.Nil {
		return 0, fmt.Errorf("tenant id is required")
	}
	if before.IsZero() {
		before = s.clock().UTC()
	}

	tickets, err := s.repo.ListTicketsByStatus(ctx, tenantID, []string{eventfabricmodel.ApprovalStatusPending}, before)
	if err != nil {
		return 0, err
	}

	var processed int
	for _, ticket := range tickets {
		ok, handleErr := s.processExpiredChallenge(ctx, ticket, before)
		if handleErr != nil {
			s.logger.WarnF(ctx, "[authorization] process challenge timeout failed ticket=%v err=%v", challengeTicketIDString(ticket), handleErr)
			continue
		}
		if ok {
			processed++
		}
	}

	return processed, nil
}

func (s *serviceImpl) ProcessExpiredChallengeTicket(ctx context.Context, ticketID uuid.UUID, before time.Time) (bool, error) {
	if err := s.ensureReady(); err != nil {
		return false, err
	}
	if ticketID == uuid.Nil {
		return false, ErrChallengeNotFound
	}
	if before.IsZero() {
		before = s.clock().UTC()
	}
	ticket, err := s.repo.GetTicketByUUID(ctx, ticketID)
	if err != nil {
		return false, err
	}
	if ticket == nil {
		return false, ErrChallengeNotFound
	}
	return s.processExpiredChallenge(ctx, ticket, before)
}

func (s *serviceImpl) processExpiredChallenge(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket, before time.Time) (bool, error) {
	if ticket == nil || ticket.GrantID == nil || *ticket.GrantID == uuid.Nil {
		return false, ErrChallengeNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(ticket.Status), eventfabricmodel.ApprovalStatusPending) {
		return false, ErrChallengeResolved
	}
	if !ticket.SLAExpiresAt.IsZero() && ticket.SLAExpiresAt.After(before) {
		return false, nil
	}

	grant, err := s.repo.GetGrantByUUID(ctx, *ticket.GrantID)
	if err != nil {
		return false, err
	}
	if grant == nil {
		return false, nil
	}

	txRepo, tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return false, err
	}

	now := s.clock().UTC()
	timeoutReason := "challenge sla expired"
	ticket.Status = eventfabricmodel.ApprovalStatusExpired
	ticket.DecisionReason = timeoutReason
	ticket.DecisionAt = &now

	updatedTicket, err := txRepo.UpdateApprovalTicket(ctx, ticket)
	if err != nil {
		txRepo.RollbackTx(tx)
		return false, err
	}

	fields := map[string]any{
		"status":         eventfabricmodel.GrantStatusRevoked,
		"revoked_at":     now,
		"revoked_reason": timeoutReason,
		"ttl_expires_at": now,
	}
	if err := txRepo.UpdateGrantFields(ctx, grant.UUID, fields); err != nil {
		txRepo.RollbackTx(tx)
		return false, err
	}
	if err := txRepo.IncrementGrantVersion(ctx, grant.UUID); err != nil {
		txRepo.RollbackTx(tx)
		return false, err
	}

	if err := txRepo.CommitTx(tx); err != nil {
		return false, err
	}

	refreshedGrant, err := s.repo.GetGrantByUUID(ctx, grant.UUID)
	if err != nil {
		s.logger.WarnF(ctx, "[authorization] reload grant timeout failed grant=%s err=%v", grant.UUID, err)
	} else if refreshedGrant != nil {
		if err := s.InvalidateGrantCache(ctx, buildGrantCacheKey(refreshedGrant)); err != nil {
			s.logger.WarnF(ctx, "[authorization] invalidate cache timeout failed grant=%s err=%v", refreshedGrant.UUID, err)
		}
		s.emitAudit(ctx, "challenge.expired", refreshedGrant, nil, map[string]string{
			"ticket_id": ticket.UUID.String(),
		})
		s.emitAudit(ctx, "grant.revoked", refreshedGrant, nil, map[string]string{
			"ticket_id": ticket.UUID.String(),
			"reason":    timeoutReason,
		})
		s.emitEvaluationAlert(ctx, EvaluateRequest{
			TenantID:    uuid.Nil,
			SubjectType: refreshedGrant.SubjectType,
			SubjectID:   refreshedGrant.SubjectID,
		}, &GrantSnapshot{
			GrantID:     refreshedGrant.UUID,
			TenantUUID:  tenantUUIDFromGrant(refreshedGrant),
			SubjectType: refreshedGrant.SubjectType,
			SubjectID:   refreshedGrant.SubjectID,
			Status:      refreshedGrant.Status,
		}, "authorization.challenge_timeout", "high", timeoutReason, map[string]string{
			"ticket_id": ticket.UUID.String(),
		})
	}

	if s.dispatcher != nil && updatedTicket != nil {
		if err := s.dispatcher.NotifyTimeout(ctx, updatedTicket); err != nil {
			s.logger.WarnF(ctx, "[authorization] notify timeout failed ticket=%s err=%v", ticket.UUID, err)
		}
	}

	return true, nil
}

func challengeTicketIDString(ticket *eventfabricmodel.AuthorizationApprovalTicket) string {
	if ticket == nil {
		return ""
	}
	return ticket.UUID.String()
}

func (s *serviceImpl) resolvePendingChallenge(ctx context.Context, repo *eventfabricrepo.AuthorizationRepository, grantID uuid.UUID, actorID *uuid.UUID, reject bool) error {
	if repo == nil || grantID == uuid.Nil {
		return ErrChallengeNotFound
	}
	ticket, err := repo.GetPendingTicketByGrant(ctx, grantID)
	if err != nil {
		return err
	}
	if ticket == nil {
		return ErrChallengeNotFound
	}

	now := s.clock().UTC()
	if reject {
		ticket.Status = eventfabricmodel.ApprovalStatusRejected
		ticket.DecisionReason = "challenge auto-rejected"
	} else {
		ticket.Status = eventfabricmodel.ApprovalStatusExpired
		ticket.DecisionReason = "challenge auto-resolved"
	}
	ticket.DecisionAt = &now
	if actorID != nil && *actorID != uuid.Nil {
		id := *actorID
		ticket.DecisionBy = &id
	}

	_, err = repo.UpdateApprovalTicket(ctx, ticket)
	return err
}

func maybeUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
