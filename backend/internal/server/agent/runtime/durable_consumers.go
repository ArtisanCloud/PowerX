package runtime

import (
	"context"
	"errors"

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
			return c.admin.repo.MarkRunArchived(ctx, tenant, run.Env, id, archived.ArchiveKey, archived.ArchivedAt)
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
	return c.sessions.RecordArchivedRun(ctx, archived)
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
	c.sessions.EnableArchiveRecovery()
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
