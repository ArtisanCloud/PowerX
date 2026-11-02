package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/google/uuid"
)

// DatabaseRetentionPolicy 单独授权两张调用审计表；不影响业务事件队列和 SQL 日志。
type DatabaseRetentionPolicy struct {
	Revision          int64     `json:"revision"`
	OwnerInstance     string    `json:"owner_instance"`
	Enabled           bool      `json:"enabled"`
	SuccessDays       int       `json:"success_days"`
	FailureDays       int       `json:"failure_days"`
	PendingHours      int       `json:"pending_hours"`
	QuarantineDays    int       `json:"quarantine_days"`
	BatchLimit        int       `json:"batch_limit"`
	MaxBytes          int64     `json:"max_bytes"`
	AdoptLegacy       bool      `json:"adopt_legacy"`
	RejectNewWhenFull bool      `json:"reject_new_when_full"`
	UpdatedAt         time.Time `json:"updated_at"`
	Actor             uint64    `json:"actor"`
}
type DatabaseRetentionRun struct {
	PolicySnapshot *DatabaseRetentionPolicy `json:"policy_snapshot,omitempty"`
	ID             string                   `json:"id"`
	Revision       int64                    `json:"revision"`
	State          string                   `json:"state"`
	At             time.Time                `json:"at"`
	Actor          uint64                   `json:"actor"`
	Counts         caprepo.RetentionBatch   `json:"counts"`
	Problem        string                   `json:"problem,omitempty"`
}
type databaseRetentionDocument struct {
	Policy DatabaseRetentionPolicy `json:"policy"`
	Runs   []DatabaseRetentionRun  `json:"runs"`
}
type DatabaseRetentionStatus struct {
	StateConfigured  bool                    `json:"state_configured"`
	EnforceAllowed   bool                    `json:"enforcement_allowed"`
	MigrationReady   bool                    `json:"migration_ready"`
	MigrationProblem string                  `json:"migration_problem,omitempty"`
	UnconfirmedRuns  int                     `json:"unconfirmed_runs"`
	WorkerEnabled    bool                    `json:"worker_enabled"`
	Policy           DatabaseRetentionPolicy `json:"policy"`
	Runs             []DatabaseRetentionRun  `json:"runs"`
	Ready            bool                    `json:"ready"`
	Problem          string                  `json:"problem,omitempty"`
	RelationBytes    *int64                  `json:"relation_bytes"`
	Scope            string                  `json:"scope"`
	PhysicalReclaim  string                  `json:"physical_reclaim"`
}

func (s *Service) databaseDocument(raw []byte) (databaseRetentionDocument, error) {
	d := databaseRetentionDocument{Policy: DatabaseRetentionPolicy{OwnerInstance: s.opts.Identity.InstanceID, SuccessDays: 30, FailureDays: 90, PendingHours: 24, QuarantineDays: 30, BatchLimit: 200, MaxBytes: 8 << 30}, Runs: []DatabaseRetentionRun{}}
	if len(raw) > 0 && (json.Unmarshal(raw, &d) != nil || d.Policy.OwnerInstance != s.opts.Identity.InstanceID) {
		return d, ErrStateUnavailable
	}
	return d, nil
}
func (s *Service) DatabaseRetention(ctx context.Context) (DatabaseRetentionStatus, error) {
	out := DatabaseRetentionStatus{StateConfigured: s.opts.Budget != nil, EnforceAllowed: s.opts.DatabaseRetentionAllowed, WorkerEnabled: s.opts.LifecycleWorkerEnabled, Scope: "instance_registered_tenants_invocation_audit_only", PhysicalReclaim: "deleted_rows_reusable_after_vacuum_not_immediate_filesystem_release"}
	if rootActor(ctx) == 0 {
		return out, ErrRootRequired
	}
	// 观察模式同样返回可读的就绪状态；不创建私有状态，不假设默认草稿已经生效。
	var raw []byte
	if s.opts.Budget != nil {
		var err error
		raw, err = s.opts.Budget.Document(ctx, "database-retention", nil)
		if err != nil {
			return out, err
		}
	}
	d, err := s.databaseDocument(raw)
	if err != nil {
		return out, err
	}
	out.Policy = d.Policy
	out.Runs = d.Runs
	for _, run := range d.Runs {
		if run.State == "unconfirmed" {
			out.UnconfirmedRuns++
		}
	}
	if err = s.dbRetention.Ready(ctx); err != nil {
		out.MigrationProblem = err.Error()
	} else {
		out.MigrationReady = true
	}
	if n, e := s.dbRetention.RelationBytes(ctx); e == nil {
		out.RelationBytes = &n
	}
	switch {
	case !out.StateConfigured:
		out.Problem = "database_retention_runtime_not_configured"
	case !out.EnforceAllowed:
		out.Problem = "database_retention_enforcement_not_allowed"
	case !out.MigrationReady:
		out.Problem = out.MigrationProblem
	default:
		out.Ready = true
	}
	return out, nil
}
func validDatabasePolicy(p DatabaseRetentionPolicy) bool {
	return p.SuccessDays >= 1 && p.SuccessDays <= 3650 && p.FailureDays >= p.SuccessDays && p.FailureDays <= 3650 && p.PendingHours >= 1 && p.PendingHours <= 720 && p.QuarantineDays >= 1 && p.QuarantineDays <= 3650 && p.BatchLimit >= 1 && p.BatchLimit <= 1000 && p.MaxBytes >= 64<<20 && p.MaxBytes <= 1<<50
}
func (s *Service) SaveDatabaseRetention(ctx context.Context, revision int64, p DatabaseRetentionPolicy) (DatabaseRetentionPolicy, error) {
	actor := rootActor(ctx)
	if actor == 0 {
		return p, ErrRootRequired
	}
	if s.opts.Budget == nil || s.dbRetention == nil {
		return p, ErrStateUnavailable
	}
	if !validDatabasePolicy(p) {
		return p, ErrPolicyInvalid
	}
	if p.Enabled || p.RejectNewWhenFull {
		if !s.opts.DatabaseRetentionAllowed {
			return p, ErrStateUnavailable
		}
		if p.Enabled {
			if err := s.dbRetention.Ready(ctx); err != nil {
				return p, err
			}
		}
	}
	_, err := s.opts.Budget.Document(ctx, "database-retention", func(raw []byte) ([]byte, error) {
		d, e := s.databaseDocument(raw)
		if e != nil {
			return nil, e
		}
		if revision != d.Policy.Revision {
			return nil, ErrPolicyConflict
		}
		p.Revision = revision + 1
		p.OwnerInstance = s.opts.Identity.InstanceID
		p.Actor = actor
		p.UpdatedAt = s.opts.Now().UTC()
		d.Policy = p
		return json.Marshal(d)
	})
	if err == nil {
		s.PublishChange("database_policy_changed")
	}
	return p, err
}
func (s *Service) databaseBatch(ctx context.Context, revision int64, key string, apply, scheduled bool) (DatabaseRetentionRun, error) {
	out := DatabaseRetentionRun{ID: key, Revision: revision, At: s.opts.Now().UTC(), Actor: rootActor(ctx), State: "preview"}
	if !scheduled && out.Actor == 0 {
		return out, ErrRootRequired
	}
	if s.opts.Budget == nil || s.dbRetention == nil {
		return out, ErrStateUnavailable
	}
	if apply {
		if _, err := uuid.Parse(key); err != nil {
			return out, ErrPolicyInvalid
		}
	}
	// 保持主机元数据锁直到有界事务和结果写入，避免跨实例竞争与策略变更竞态。
	_, err := s.opts.Budget.DocumentTransaction(ctx, "database-retention", func(raw []byte, persist func([]byte) error) ([]byte, error) {
		d, e := s.databaseDocument(raw)
		if e != nil {
			return nil, e
		}
		p := d.Policy
		snapshot := p
		out.PolicySnapshot = &snapshot
		for _, old := range d.Runs {
			if old.ID == key && apply {
				out = old
				return nil, nil
			}
		}
		if !scheduled && p.Revision != revision {
			return nil, ErrPolicyConflict
		}
		out.Revision = p.Revision
		if apply && (!p.Enabled || !s.opts.DatabaseRetentionAllowed) {
			if scheduled {
				return nil, nil
			}
			return nil, ErrStateUnavailable
		}
		if e = s.dbRetention.Ready(ctx); e != nil {
			return nil, e
		}
		c := caprepo.RetentionCutoffs{Success: out.At.AddDate(0, 0, -p.SuccessDays), Failure: out.At.AddDate(0, 0, -p.FailureDays), Pending: out.At.Add(-time.Duration(p.PendingHours) * time.Hour), Quarantine: out.At.AddDate(0, 0, -p.QuarantineDays), Limit: p.BatchLimit, AdoptLegacy: p.AdoptLegacy}
		// 先写操作意图；若事务完成后最终写入失败，下次不自动重放未知结果。
		if apply {
			unknown := 0
			for _, old := range d.Runs {
				if old.State == "unconfirmed" {
					unknown++
				}
			}
			if unknown >= 100 {
				return nil, ErrStateUnavailable
			}
			out.State = "unconfirmed"
			d.Runs = append([]DatabaseRetentionRun{out}, d.Runs...)
			if len(d.Runs) > 100 {
				kept := append([]DatabaseRetentionRun(nil), d.Runs[:100]...)
				for _, old := range d.Runs[100:] {
					if old.State == "unconfirmed" {
						kept = append(kept, old)
					}
				}
				d.Runs = kept
			}
			intent, e := json.Marshal(d)
			if e != nil {
				return nil, e
			}
			if e = persist(intent); e != nil {
				return nil, e
			}
		}
		out.Counts, e = s.dbRetention.Batch(ctx, c, apply)
		if !apply {
			return nil, e
		}
		out.State = "completed"
		if e != nil {
			out.State = "failed"
			out.Problem = "database_batch_rolled_back"
		}
		d.Runs[0] = out
		return json.Marshal(d)
	})
	if err == nil && apply {
		s.PublishChange("database_retention_completed")
	}
	return out, err
}
func (s *Service) PreviewDatabaseRetention(ctx context.Context, revision int64) (DatabaseRetentionRun, error) {
	return s.databaseBatch(ctx, revision, "", false, false)
}
func (s *Service) RunDatabaseRetention(ctx context.Context, revision int64, key string) (DatabaseRetentionRun, error) {
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.databaseBatch(run, revision, key, true, false)
}

// CheckDatabaseAdmission 在能力调用产生副作用前阻止新的调用；绝不事后丢审计。
func (s *Service) CheckDatabaseAdmission(ctx context.Context) error {
	if s.opts.Budget == nil {
		return nil
	}
	raw, err := s.opts.Budget.Document(ctx, "database-retention", nil)
	if err != nil {
		return err
	}
	d, err := s.databaseDocument(raw)
	if err != nil {
		return err
	}
	p := d.Policy
	if !p.RejectNewWhenFull {
		return nil
	}
	n, err := s.dbRetention.RelationBytes(ctx)
	if err != nil {
		return errors.New("database_capacity_unknown")
	}
	if n >= p.MaxBytes {
		return errors.New("database_capacity_exhausted")
	}
	snap, err := s.opts.Budget.Status(ctx)
	if err != nil {
		return err
	}
	if snap.AvailableBytes-snap.ReservedBytes <= snap.CriticalFreeBytes {
		return errors.New("host_capacity_exhausted")
	}
	// 与大文件任务共用整盘预算；预留中的空间同样不能再次承诺给新调用。
	if snap.MaxUsedBytes > 0 && snap.UsedBytes+snap.ReservedBytes >= snap.MaxUsedBytes {
		return errors.New("host_capacity_exhausted")
	}
	return nil
}

// runDatabaseRetentionCycle 在一个调度窗口内推进多批，避免 200 行/15 分钟的固定吞吐令存量永远追不上增长。
// 保留每批事务、意图与主机锁；总计至多 100 批/60 秒，仍无法回收时由容量门禁和运维处理。
func (s *Service) runDatabaseRetentionCycle(ctx context.Context, onError func(error)) {
	cycle, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for batch := 0; batch < 100 && cycle.Err() == nil; batch++ {
		run, cancelBatch := context.WithTimeout(cycle, 10*time.Second)
		out, err := s.databaseBatch(run, 0, uuid.NewString(), true, true)
		cancelBatch()
		if err != nil {
			if onError != nil {
				onError(err)
			}
			return
		}
		if out.State == "failed" {
			if onError != nil {
				onError(errors.New("database_retention_batch_rolled_back"))
			}
			return
		}
		if out.Counts.Traces+out.Counts.Events+out.Counts.Quarantined == 0 {
			return
		}
	}
}
