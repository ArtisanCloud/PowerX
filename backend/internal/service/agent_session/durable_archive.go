package agent_session

import (
	"context"
	"errors"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/google/uuid"
)

// EnableArchiveRecovery 由正式 Worker 装配启用终态归档补写扫描。
func (s *Service) EnableArchiveRecovery(objects ...agent_run.ReportObjectStore) {
	s.archiveRecovery = true
	if len(objects) > 0 {
		s.archiveObjects = objects[0]
	}
}

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

func (s *Service) readArchivedInvocation(ctx context.Context, row *m.ServiceInvocation) (agent_run.RunReport, error) {
	if s.archiveObjects == nil || row.FinishedAt == nil || row.ArchivedAt == nil || row.ArchiveKey == "" {
		return agent_run.RunReport{}, ErrDependency
	}
	identity := durableIdentity(repo.SessionOwner{TenantUUID: row.TenantUUID}, row)
	report, err := agent_run.ReadRunReport(ctx, identity, s.archiveObjects)
	if err != nil {
		return agent_run.RunReport{}, err
	}
	if report.Snapshot.Status != row.Status {
		return agent_run.RunReport{}, ErrConflict
	}
	return report, nil
}

func (s *Service) invocationSnapshot(ctx context.Context, row *m.ServiceInvocation) (agent_run.Snapshot, error) {
	run, err := s.durableRuns.Get(ctx, row.TenantUUID.String(), row.RunEnv, row.UUID.String())
	if errors.Is(err, agent_run.ErrNotFound) {
		report, readErr := s.readArchivedInvocation(ctx, row)
		return report.Snapshot, readErr
	}
	return run, err
}

// ConfigureArchiveLifecycle 为正式共享 Worker 启用清理恢复与受理门禁。
func (s *Service) ConfigureArchiveLifecycle(gate func(context.Context) error) error {
	if s == nil || s.repo == nil || s.durableRuns == nil || s.archiveObjects == nil || gate == nil {
		return ErrInvalid
	}
	s.retentionRecovery, s.admissionGate = true, gate
	return nil
}
func (s *Service) ArchiveBacklog(ctx context.Context) (int64, *time.Time, error) {
	return s.repo.ArchiveBacklog(ctx, s.runEnv)
}
func (s *Service) RecordHotExpiry(ctx context.Context, run agent_run.Snapshot, at time.Time) error {
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
	if row.RunEnv != run.Env || row.SessionUUID.String() != run.SessionID || row.MessageUUID.String() != run.MessageID || row.TraceUUID.String() != run.TraceID || !row.DeadlineAt.Equal(run.DeadlineAt) || row.Status != run.Status || row.ArchiveKey != run.ArchiveKey {
		return ErrConflict
	}
	return translate(s.repo.MarkHotExpiry(ctx, repo.SessionOwner{TenantUUID: tenant, PluginID: row.PluginID, ServiceActor: row.ServiceActor}, run.Env, row.SessionUUID, id, run.ArchiveKey, at))
}
func (s *Service) ArchivedRecoverySnapshot(ctx context.Context, identity agent_run.Snapshot) (agent_run.Snapshot, error) {
	tenant, err := uuid.Parse(identity.TenantUUID)
	if err != nil {
		return agent_run.Snapshot{}, ErrInvalid
	}
	id, err := uuid.Parse(identity.RunID)
	if err != nil {
		return agent_run.Snapshot{}, ErrInvalid
	}
	row, err := s.repo.DurableInvocation(ctx, tenant, id)
	if err != nil {
		return agent_run.Snapshot{}, translate(err)
	}
	if row.RunEnv != identity.Env || row.SessionUUID.String() != identity.SessionID || row.MessageUUID.String() != identity.MessageID || row.TraceUUID.String() != identity.TraceID || !row.DeadlineAt.Equal(identity.DeadlineAt) {
		return agent_run.Snapshot{}, ErrConflict
	}
	report, err := s.readArchivedInvocation(ctx, row)
	if err != nil {
		return agent_run.Snapshot{}, err
	}
	run := report.Snapshot
	run.ArchiveKey = row.ArchiveKey
	run.ArchivedAt = *row.ArchivedAt
	return run, nil
}
