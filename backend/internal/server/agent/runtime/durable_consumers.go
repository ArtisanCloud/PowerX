package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const adminLocatorOffset uint64 = 1 << 63

type durableConsumers struct {
	sessions       *sessions.Service
	serviceLoader  InvokePlanningInputLoader
	admin          *AdminRunService
	archiveStore   *agent_run.RedisStore
	archiveObjects agent_run.ReportObjectStore
	archivePolicy  *agent_run.ArchivePolicy
	archiveEnv     string
	archiveQueue   agent_run.ArchivedTaskQueue
}

func NewDurableConsumers(service *sessions.Service, loader InvokePlanningInputLoader, admin *AdminRunService) *durableConsumers {
	return &durableConsumers{sessions: service, serviceLoader: loader, admin: admin}
}
func (c *durableConsumers) Load(ctx context.Context, ref agent_run.TaskRef) (InvokePlanningInput, error) {
	if !validPlanArtifactRef(ref) {
		return InvokePlanningInput{}, agent_run.ErrInvalid
	}
	if c.admin != nil {
		_, err := c.admin.repo.Get(ctx, uuid.MustParse(ref.TenantUUID), ref.Env, uuid.MustParse(ref.RunID))
		if err == nil {
			return c.admin.Loader.Load(ctx, ref)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return InvokePlanningInput{}, err
		}
	}
	return c.serviceLoader.Load(ctx, ref)
}
func (c *durableConsumers) FinalizeRun(ctx context.Context, run agent_run.Snapshot) error {
	if c.admin != nil {
		tenant, err := uuid.Parse(run.TenantUUID)
		if err != nil {
			return agent_run.ErrInvalid
		}
		id, err := uuid.Parse(run.RunID)
		if err != nil {
			return agent_run.ErrInvalid
		}
		_, err = c.admin.repo.Get(ctx, tenant, run.Env, id)
		if err == nil {
			if err := c.admin.FinalizeRun(ctx, run); err != nil {
				return err
			}
			archived, err := c.archive(ctx, run)
			if err != nil || c.archiveStore == nil {
				return err
			}
			if err := c.admin.repo.MarkRunArchived(ctx, tenant, run.Env, id, archived.ArchiveKey, archived.ArchivedAt); err != nil {
				return err
			}
			return c.scheduleExpiry(ctx, archived, true)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	if err := c.sessions.FinalizeRun(ctx, run); err != nil {
		return err
	}
	archived, err := c.archive(ctx, run)
	if err != nil || c.archiveStore == nil {
		return err
	}
	if err := c.sessions.RecordArchivedRun(ctx, archived); err != nil {
		return err
	}
	return c.scheduleExpiry(ctx, archived, false)
}
func (c *durableConsumers) ListUnfinishedRuns(ctx context.Context, after uint64, limit int) ([]agent_run.LocatedRun, error) {
	if limit < 1 || limit > 200 {
		return nil, agent_run.ErrInvalid
	}
	rows := []agent_run.LocatedRun{}
	if after < adminLocatorOffset {
		found, err := c.sessions.ListUnfinishedRuns(ctx, after, limit)
		if err != nil {
			return nil, err
		}
		for _, row := range found {
			if row.Cursor >= adminLocatorOffset {
				return nil, agent_run.ErrInvalid
			}
		}
		rows = append(rows, found...)
		if len(rows) == limit || c.admin == nil {
			return rows, nil
		}
		after = adminLocatorOffset
	}
	if c.admin == nil {
		return rows, nil
	}
	found, err := c.admin.ListUnfinishedRuns(ctx, after-adminLocatorOffset, limit-len(rows))
	if err != nil {
		return nil, err
	}
	for _, row := range found {
		if row.Cursor >= adminLocatorOffset {
			return nil, agent_run.ErrInvalid
		}
		row.Cursor += adminLocatorOffset
		rows = append(rows, row)
	}
	return rows, nil
}

// ConfigureArchive 接入终态对象归档及 SQL 定位补写，失败由同一扫描器恢复。
func (c *durableConsumers) ConfigureArchive(store *agent_run.RedisStore, objects agent_run.ReportObjectStore) error {
	if store == nil || objects == nil || c.sessions == nil {
		return agent_run.ErrInvalid
	}
	c.archiveStore, c.archiveObjects = store, objects
	c.sessions.EnableArchiveRecovery(objects)
	if c.admin != nil {
		c.admin.archiveRecovery = true
	}
	return nil
}

func (c *durableConsumers) archive(ctx context.Context, run agent_run.Snapshot) (agent_run.Snapshot, error) {
	if c.archiveStore == nil {
		return agent_run.Snapshot{}, nil
	}
	if _, err := c.archiveStore.ArchiveRun(ctx, run, c.archiveObjects); err != nil {
		return agent_run.Snapshot{}, err
	}
	return c.archiveStore.Get(ctx, run.TenantUUID, run.Env, run.RunID)
}

func (c *durableConsumers) ConfigureArchiveLifecycle(env string, policy agent_run.ArchivePolicy) error {
	if c.archiveStore == nil || policy.Validate() != nil {
		return agent_run.ErrInvalid
	}
	c.archivePolicy, c.archiveEnv = &policy, env
	gate := func(ctx context.Context) error { return c.archiveStore.CheckArchiveAdmission(ctx, env, policy) }
	if err := c.sessions.ConfigureArchiveLifecycle(gate); err != nil {
		return err
	}
	if c.admin != nil {
		c.admin.retentionRecovery = true
		c.admin.AdmissionGate = gate
	}
	return nil
}

// MaintainRuns 每轮恢复扫描刷新共享受理门禁，无 SQL 心跳或逐请求聚合。
func (c *durableConsumers) MaintainRuns(ctx context.Context) error {
	if c.archivePolicy == nil {
		return nil
	}
	count, oldest, err := c.sessions.ArchiveBacklog(ctx)
	if err != nil {
		return err
	}
	if c.admin != nil {
		n, at, err := c.admin.repo.ArchiveBacklog(ctx, c.archiveEnv)
		if err != nil {
			return err
		}
		count += n
		if at != nil && (oldest == nil || at.Before(*oldest)) {
			oldest = at
		}
	}
	return c.archiveStore.PublishArchiveHealth(ctx, c.archiveEnv, agent_run.ArchiveHealth{Pending: count, OldestAt: oldest}, *c.archivePolicy)
}
func (c *durableConsumers) scheduleExpiry(ctx context.Context, run agent_run.Snapshot, admin bool) error {
	if c.archivePolicy == nil {
		return nil
	}
	at := run.ArchivedAt.Add(c.archivePolicy.HotRetention)
	if c.archiveQueue != nil {
		report, err := agent_run.ReadRunReport(ctx, run, c.archiveObjects)
		if err != nil {
			return err
		}
		ids := []string{}
		for _, task := range report.Tasks {
			if task.Attempt > 100 {
				return agent_run.ErrInvalid
			}
			for attempt := uint64(1); attempt <= task.Attempt; attempt++ {
				ids = append(ids, fmt.Sprintf("%s:%d:%s:%d", run.RunID, task.Revision, task.TaskID, attempt))
			}
		}
		if err := c.archiveQueue.RetireRun(ctx, run.TenantUUID+":"+run.Env, agent_run.AgentTaskSubscriber, "agent-run-workers-v1", run.RunID, ids, at); err != nil {
			return err
		}
	}

	if err := c.archiveStore.ExpireArchivedRun(ctx, run, c.archiveObjects, at); err != nil {
		return err
	}
	if admin {
		return c.admin.repo.MarkHotExpiry(ctx, uuid.MustParse(run.TenantUUID), run.Env, uuid.MustParse(run.RunID), run.ArchiveKey, at)
	}
	return c.sessions.RecordHotExpiry(ctx, run, at)
}

// RecoverArchivedRun 仅修复到期确认索引，永不重建 Redis 或重新投递任务。
func (c *durableConsumers) RecoverArchivedRun(ctx context.Context, identity agent_run.Snapshot) (bool, error) {
	if c.archivePolicy == nil {
		return false, nil
	}
	if c.admin != nil {
		row, err := c.admin.repo.Get(ctx, uuid.MustParse(identity.TenantUUID), identity.Env, uuid.MustParse(identity.RunID))
		if err == nil {
			if row.FinishedAt == nil || row.ArchivedAt == nil || row.ArchiveKey == "" {
				return false, nil
			}
			report, err := agent_run.ReadRunReport(ctx, adminIdentity(row), c.archiveObjects)
			if err != nil {
				return false, err
			}
			if report.Snapshot.Status != row.Status || identity.SessionID != report.Snapshot.SessionID || identity.MessageID != report.Snapshot.MessageID || identity.TraceID != report.Snapshot.TraceID || !identity.DeadlineAt.Equal(report.Snapshot.DeadlineAt) {
				return false, agent_run.ErrConflict
			}
			run := report.Snapshot
			run.ArchiveKey = row.ArchiveKey
			run.ArchivedAt = *row.ArchivedAt
			return true, c.scheduleExpiry(ctx, run, true)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	}
	run, err := c.sessions.ArchivedRecoverySnapshot(ctx, identity)
	if err != nil {
		return false, err
	}
	return true, c.scheduleExpiry(ctx, run, false)
}

func (c *durableConsumers) ConfigureArchiveQueue(queue agent_run.ArchivedTaskQueue) {
	c.archiveQueue = queue
}
