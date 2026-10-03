package agent_session

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/google/uuid"
)

// EnableArchiveRecovery 由正式 Worker 装配启用终态归档补写扫描。
func (s *Service) EnableArchiveRecovery() { s.archiveRecovery = true }

func (s *Service) RecordArchivedRun(ctx context.Context, run agent_run.Snapshot) error {
	tenant, err := uuid.Parse(run.TenantUUID)
	if err != nil {
		return ErrInvalid
	}
	id, err := uuid.Parse(run.RunID)
	if err != nil {
		return ErrInvalid
	}
	row, err := s.repo.DurableInvocation(ctx, tenant, id)
	if err != nil {
		return translate(err)
	}
	if row.FinishedAt == nil || row.RunEnv != run.Env || row.SessionUUID.String() != run.SessionID ||
		row.MessageUUID.String() != run.MessageID || row.TraceUUID.String() != run.TraceID ||
		!row.DeadlineAt.Equal(run.DeadlineAt) || row.Status != run.Status {
		return ErrConflict
	}
	owner := repo.SessionOwner{TenantUUID: tenant, PluginID: row.PluginID, ServiceActor: row.ServiceActor}
	return translate(s.repo.MarkRunArchived(ctx, owner, run.Env, row.SessionUUID, id, run.ArchiveKey, run.ArchivedAt))
}
