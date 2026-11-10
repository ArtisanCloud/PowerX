package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
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
	PreviewID      string                    `json:"preview_id,omitempty"`
	Cutoffs        *caprepo.RetentionCutoffs `json:"cutoffs,omitempty"`
	ExpiresAt      *time.Time                `json:"expires_at,omitempty"`
	Trigger        string                    `json:"trigger,omitempty"`
	PolicySnapshot *DatabaseRetentionPolicy  `json:"policy_snapshot,omitempty"`
	ID             string                    `json:"id"`
	Revision       int64                     `json:"revision"`
	State          string                    `json:"state"`
	At             time.Time                 `json:"at"`
	Actor          uint64                    `json:"actor"`
	Counts         caprepo.RetentionBatch    `json:"counts"`
	Problem        string                    `json:"problem,omitempty"`
}
type databaseRetentionDocument struct {
	Policy DatabaseRetentionPolicy `json:"policy"`
	Runs   []DatabaseRetentionRun  `json:"runs"`
}
type DatabaseRetentionStatus struct {
	Configuration    *DatabaseRetentionConfiguration `json:"configuration,omitempty"`
	StateConfigured  bool                            `json:"state_configured"`
	EnforceAllowed   bool                            `json:"enforcement_allowed"`
	MigrationReady   bool                            `json:"migration_ready"`
	MigrationProblem string                          `json:"migration_problem,omitempty"`
	UnconfirmedRuns  int                             `json:"unconfirmed_runs"`
	WorkerEnabled    bool                            `json:"worker_enabled"`
	Policy           DatabaseRetentionPolicy         `json:"policy"`
	Runs             []DatabaseRetentionRun          `json:"runs"`
	Ready            bool                            `json:"ready"`
	Problem          string                          `json:"problem,omitempty"`
	RelationBytes    *int64                          `json:"relation_bytes"`
	Scope            string                          `json:"scope"`
	PhysicalReclaim  string                          `json:"physical_reclaim"`
}

func (s *Service) databaseDocument(raw []byte) (databaseRetentionDocument, error) {
	d := databaseRetentionDocument{Policy: DatabaseRetentionPolicy{OwnerInstance: s.opts.Identity.InstanceID, SuccessDays: 30, FailureDays: 90, PendingHours: 24, QuarantineDays: 30, BatchLimit: 200, MaxBytes: 8 << 30}, Runs: []DatabaseRetentionRun{}}
	if len(raw) > 0 && (json.Unmarshal(raw, &d) != nil || d.Policy.OwnerInstance != s.opts.Identity.InstanceID) {
		return d, ErrStateUnavailable
	}
	return d, nil
}
func (s *Service) DatabaseRetention(ctx context.Context) (DatabaseRetentionStatus, error) {
	out := DatabaseRetentionStatus{Runs: []DatabaseRetentionRun{}, StateConfigured: s.opts.Budget != nil, EnforceAllowed: s.opts.DatabaseRetentionAllowed, WorkerEnabled: s.opts.LifecycleWorkerEnabled, Scope: "instance_registered_tenants_invocation_audit_only", PhysicalReclaim: "deleted_rows_reusable_after_vacuum_not_immediate_filesystem_release"}
	if !auth.IsPlatformRootSession(ctx) {
		return out, ErrRootRequired
	}
	// 观察模式同样返回可读的就绪状态；不创建私有状态，不假设默认草稿已经生效。
	var raw []byte
	if s.opts.Budget != nil {
		var err error
		raw, err = readArchiveControlProjection(ctx, s.opts.Budget, "database-retention")
		if err != nil {
			return out, err
		}
	}
	d, err := s.databaseDocument(raw)
	if err != nil {
		return out, err
	}
	out.Policy = d.Policy
	if s.opts.Control != nil {
		p, c, e := s.loadRetentionControl(ctx)
		out.Configuration = &c
		if e != nil {
			out.Problem = c.Problem
			out.Policy.Enabled = false
		} else {
			out.Policy = p
		}
		if s.opts.DB != nil {
			rows, e := s.retentionLedger().List(ctx)
			if e != nil {
				out.Problem = "DATABASE_RETENTION_LEDGER_UNAVAILABLE"
			} else {
				out.Runs = []DatabaseRetentionRun{}
				for _, row := range rows {
					r, e := decodeRetentionRun(&row)
					if e != nil {
						out.Problem = "DATABASE_RETENTION_LEDGER_UNAVAILABLE"
						break
					}
					out.Runs = append(out.Runs, r)
				}
				out.Runs = append(out.Runs, d.Runs...)
			}
		}
	} else {
		out.Problem = "STORAGE_CONTROL_SCHEMA_NOT_READY"
		if s.opts.Budget == nil {
			out.Problem = "database_retention_runtime_not_configured"
		}
		out.Policy.Enabled = false
	}
	if out.Configuration == nil {
		out.Runs = d.Runs
	}
	for _, run := range d.Runs {
		if run.State == "unconfirmed" {
			out.UnconfirmedRuns++
		}
	}
	if out.Problem == "" && s.opts.Control != nil && s.opts.DB != nil && !s.retentionLedger().Ready(ctx) {
		out.Problem = "DATABASE_RETENTION_LEDGER_UNAVAILABLE"
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
	case out.Problem != "":
	case out.Configuration == nil || out.Configuration.Authority != "database":
		out.Problem = "DATABASE_RETENTION_ADOPTION_REQUIRED"
	case !out.Configuration.ExecutionReady:
		out.Problem = "DATABASE_RETENTION_PUBLICATION_PENDING"
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

// SaveDatabaseRetention 的旧文件写入口已停用，必须显式接管并使用版本化变更。
func (s *Service) SaveDatabaseRetention(ctx context.Context, revision int64, p DatabaseRetentionPolicy) (DatabaseRetentionPolicy, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return p, ErrRootRequired
	}
	return p, controlError(428, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
}
func (s *Service) RunDatabaseRetention(ctx context.Context, revision int64, key string) (DatabaseRetentionRun, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return DatabaseRetentionRun{}, ErrRootRequired
	}
	return DatabaseRetentionRun{}, controlError(428, "DATABASE_RETENTION_PREVIEW_REQUIRED")
}

// CheckDatabaseAdmission 在能力调用产生副作用前阻止新的调用；绝不事后丢审计。
func (s *Service) CheckDatabaseAdmission(ctx context.Context) error {
	if s.opts.Budget == nil && s.opts.Control == nil {
		return nil
	}
	p, configuration, err := s.loadRetentionControl(ctx)
	if err != nil {
		return admissionUnavailable(err)
	}
	if configuration.Problem == "DATABASE_RETENTION_SOURCE_CHANGED" {
		return admissionUnavailable(controlError(503, configuration.Problem))
	}
	if configuration.EffectiveCapacity != nil {
		p = configuration.EffectiveCapacity.policy()
	}
	if !p.RejectNewWhenFull {
		return nil
	}
	n, err := s.dbRetention.RelationBytes(ctx)
	if err != nil {
		return admissionUnavailable(err)
	}
	if n >= p.MaxBytes {
		return controlError(503, "STORAGE_CAPACITY_EXHAUSTED")
	}
	snap, err := s.opts.Budget.Status(ctx)
	if err != nil {
		return admissionUnavailable(err)
	}
	if snap.AvailableBytes-snap.ReservedBytes <= snap.CriticalFreeBytes {
		return controlError(503, "STORAGE_CAPACITY_EXHAUSTED")
	}
	// 与大文件任务共用整盘预算；预留中的空间同样不能再次承诺给新调用。
	if snap.MaxUsedBytes > 0 && snap.UsedBytes+snap.ReservedBytes >= snap.MaxUsedBytes {
		return controlError(503, "STORAGE_CAPACITY_EXHAUSTED")
	}
	return nil
}

// runDatabaseRetentionCycle 在一个调度窗口内推进多批，避免 200 行/15 分钟的固定吞吐令存量永远追不上增长。
// 保留每批事务、意图与主机锁；总计至多 100 批/60 秒，仍无法回收时由容量门禁和运维处理。
func (s *Service) runDatabaseRetentionCycle(ctx context.Context, onError func(error)) {
	cycle, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	policy, configuration, e := s.loadRetentionControl(cycle)
	if e != nil {
		if onError != nil {
			onError(e)
		}
		return
	}
	if configuration.Authority != "database" || !configuration.ExecutionReady || !policy.Enabled {
		return
	}
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
