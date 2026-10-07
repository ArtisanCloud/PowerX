package agent_session

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type DurableOutput struct {
	Content          string
	ResponseEnvelope json.RawMessage
}

// DurableOutputReader 由 Runtime 验证归档计划、任务结果及报告合同。
type DurableOutputReader interface {
	ReadFinalOutput(context.Context, agent_run.Snapshot) (DurableOutput, error)
}

func (s *Service) ConfigureDurableOutput(reader DurableOutputReader) error {
	if s == nil || reader == nil || s.executor != nil {
		return ErrInvalid
	}
	s.finalOutput = reader
	return nil
}

func (s *Service) ListUnfinishedRuns(ctx context.Context, after uint64, limit int) ([]agent_run.LocatedRun, error) {
	if s == nil || s.repo == nil || s.durableRuns == nil {
		return nil, ErrDependency
	}
	rows, err := s.repo.ListDurableLocators(ctx, s.runEnv, after, limit, s.archiveRecovery, s.retentionRecovery)
	if err != nil {
		return nil, translate(err)
	}
	result := make([]agent_run.LocatedRun, 0, len(rows))
	for _, row := range rows {
		if row.AdmissionState == "pending_create" {
			owner := repo.SessionOwner{TenantUUID: row.TenantUUID, PluginID: row.PluginID, ServiceActor: row.ServiceActor}
			err := s.repo.WithSession(ctx, owner, row.SessionUUID, func(tx *repo.ServiceSessionRepository, _ *m.ServiceSession) error {
				current, err := tx.Invocation(ctx, owner, row.SessionUUID, row.UUID)
				if err != nil {
					return err
				}
				if current.AdmissionState != "pending_create" || current.FinishedAt != nil {
					return nil
				}
				recovered, err := s.durableRuns.RecoverAdmission(ctx, durableIdentity(owner, current))
				if err != nil {
					return err
				}
				if recovered.PlanRevision != 0 || (recovered.Status != "accepted" && recovered.Status != "failed") {
					return ErrDependency
				}
				return tx.MarkInvocationAdmitted(ctx, owner, row.SessionUUID, row.UUID)
			})
			if err != nil {
				return nil, translate(err)
			}
		}
		identity := durableIdentity(repo.SessionOwner{TenantUUID: row.TenantUUID}, &row)
		result = append(result, agent_run.LocatedRun{Cursor: row.ID, Identity: identity})
	}
	return result, nil
}

// FinalizeRun 在父会话锁内原子写终态和唯一助手消息；失败可由扫描器安全补写。
func (s *Service) FinalizeRun(ctx context.Context, identity agent_run.Snapshot) error {
	if s == nil || s.repo == nil || s.durableRuns == nil || s.finalOutput == nil {
		return ErrDependency
	}
	tenant, err := uuid.Parse(identity.TenantUUID)
	if err != nil {
		return ErrInvalid
	}
	id, err := uuid.Parse(identity.RunID)
	if err != nil {
		return ErrInvalid
	}
	anchor, err := s.repo.DurableInvocation(ctx, tenant, id)
	if err != nil {
		return translate(err)
	}
	run, err := s.durableRuns.Get(ctx, identity.TenantUUID, anchor.RunEnv, identity.RunID)
	if err != nil {
		return ErrDependency
	}
	if !durableTerminalStatus(run.Status) || run.SessionID != anchor.SessionUUID.String() || run.MessageID != anchor.MessageUUID.String() ||
		run.TraceID != anchor.TraceUUID.String() || run.Env != identity.Env || !run.DeadlineAt.Equal(anchor.DeadlineAt) {
		return ErrConflict
	}
	if anchor.FinishedAt != nil {
		return nil
	}
	var output DurableOutput
	if run.Status == "completed" {
		output, err = s.finalOutput.ReadFinalOutput(ctx, run)
		if err != nil {
			return err
		}
		if strings.TrimSpace(output.Content) == "" && (len(output.ResponseEnvelope) == 0 || !json.Valid(output.ResponseEnvelope) || string(output.ResponseEnvelope) == "null") {
			return ErrDependency
		}
	}
	owner := repo.SessionOwner{TenantUUID: tenant, PluginID: anchor.PluginID, ServiceActor: anchor.ServiceActor}
	return translate(s.repo.WithSession(ctx, owner, anchor.SessionUUID, func(tx *repo.ServiceSessionRepository, session *m.ServiceSession) error {
		current, err := tx.Invocation(ctx, owner, anchor.SessionUUID, id)
		if err != nil {
			return err
		}
		if current.FinishedAt != nil {
			return nil
		}
		if current.AdmissionState != "admitted" || current.RunEnv != run.Env {
			return ErrConflict
		}
		if run.Status == "completed" && session.Status != "deleted" {
			session.Revision++
			message := &m.ServiceMessage{Role: "assistant", Content: output.Content, ResponseEnvelope: datatypes.JSON(output.ResponseEnvelope), Sequence: session.Revision,
				IdempotencyKey: "invoke:" + id.String(), RequestHash: id.String(), IdempotencyExpiresAt: current.IdempotencyExpiresAt}
			if err := tx.InsertMessage(ctx, session, message); err != nil {
				return err
			}
			session.PendingTask = nil
			session.PendingTaskExpiresAt = nil
			if err := tx.SaveSession(ctx, owner, session); err != nil {
				return err
			}
		}
		current.Status = run.Status
		current.Output = output.Content
		current.ResponseEnvelope = datatypes.JSON(output.ResponseEnvelope)
		current.FinishedAt = &run.UpdatedAt
		if run.Status != "completed" {
			current.ReasonCode = "agent_run." + run.Status
		}
		return tx.FinishDurableInvocation(ctx, owner, current)
	}))
}
