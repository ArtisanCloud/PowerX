package agent_lifecycle

import (
	"context"
	"fmt"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
)

// RunShareCompliance 扫描需要复核的共享记录并执行验证/撤销。
func (s *Service) RunShareCompliance(ctx context.Context, limit int) error {
	if s.shares == nil || s.shareValidator == nil {
		return nil
	}
	now := s.clock()
	records, err := s.shares.ListDueForReview(ctx, now, limit)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if err := s.revalidateShare(ctx, &rec); err != nil {
			pxlog.Warn(ctx, fmt.Sprintf("share compliance failed for %s: %v", rec.UUID.String(), err))
		}
	}
	return nil
}

func (s *Service) revalidateShare(ctx context.Context, record *agentmodel.AgentShareRecord) error {
	if record == nil {
		return nil
	}
	agentModel, err := s.profiles.GetByUUID(ctx, record.AgentUUID)
	if err != nil {
		return err
	}
	agent := toAgent(agentModel, decodeToolGrantsJSON(agentModel.ToolGrants), decodeStringMap(agentModel.Metadata))
	share := toAgentShare(record)
	if err := s.shareValidator.Validate(ctx, agent, share.TenantID, share.Quotas, share.Metadata); err != nil {
		s.handleShareValidationFailure(ctx, share, err, "share-compliance")
		return nil
	}
	now := s.clock()
	record.ValidatedAt = &now
	record.ValidationFail = false
	record.ValidationError = ""
	record.NextReviewAt = s.nextShareReviewAt(now)
	_, err = s.shares.Save(ctx, record)
	return err
}

func (s *Service) handleShareValidationFailure(ctx context.Context, share *AgentShare, reason error, traceID string) {
	if share == nil {
		return
	}
	s.emitShareValidationFailure(ctx, share, traceID, reason)
	s.auditShareOperation(ctx, share, "", "SHARE_VALIDATION_FAILED", "DENIED")
	_ = s.shares.MarkValidationFailure(ctx, share.ID, reason.Error())

	if share.Status == ShareStatusActive {
		_, _ = s.RevokeAgentShare(ctx, RevokeShareInput{
			ShareID:     share.ID,
			Reason:      reason.Error(),
			RequestedBy: "share-compliance",
			TraceID:     traceID,
		})
	}
}

// ScheduleShareReview 允许外部手动触发单个共享的复核。
func (s *Service) ScheduleShareReview(ctx context.Context, shareID uuid.UUID) error {
	if shareID == uuid.Nil || s.shares == nil {
		return nil
	}
	record, err := s.shares.GetByUUID(ctx, shareID)
	if err != nil {
		return err
	}
	record.NextReviewAt = timePtr(s.clock())
	_, err = s.shares.Save(ctx, record)
	return err
}

func timePtr(t time.Time) *time.Time {
	return &t
}
