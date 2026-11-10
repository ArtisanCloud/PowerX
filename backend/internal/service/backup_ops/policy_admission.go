package backup_ops

import (
	"context"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

// admitPolicyJob 仅在配置守卫内登记任务；长时间的 pg_dump 在事务提交后执行。
func (s *JobService) admitPolicyJob(ctx context.Context, req TriggerJobRequest, p *model.BackupPolicy) (*model.BackupJob, *model.BackupPolicy, error) {
	var saved *model.BackupJob
	var captured *model.BackupPolicy
	action := func(run context.Context, db *gorm.DB, current *model.BackupPolicy) error {
		jobs := repo.NewBackupJobRepository(db)
		if req.TriggerType == model.BackupTriggerTypeScheduled {
			last, e := jobs.GetLatestByPolicy(run, current.ID)
			if e != nil {
				return e
			}
			anchor := current.CreatedAt
			if last != nil {
				anchor = last.CreatedAt
				if last.StartedAt != nil {
					anchor = *last.StartedAt
				}
			}
			if time.Now().Before(anchor.Add(parseScheduleDuration(current.Schedule))) {
				return nil
			}
		}
		if e := jobs.FailInterrupted(run, req.PolicyID, time.Now().UTC().Add(-s.backupScriptTimeout-10*time.Minute)); e != nil {
			return e
		}
		running, e := jobs.ExistsRunningByPolicy(run, req.PolicyID)
		if e != nil {
			return e
		}
		if running {
			return ErrBackupJobAlreadyRunning
		}
		now := time.Now().UTC()
		trigger := req.TriggerType
		if trigger == "" {
			trigger = model.BackupTriggerTypeManual
		}
		row := &model.BackupJob{PolicyID: req.PolicyID, PolicyRevision: current.ControlRevision, Status: model.BackupJobStatusRunning, TriggerType: trigger, StartedAt: &now, Operator: normalizeOperator(req.Operator), TraceID: req.TraceID}
		row.Normalize()
		saved, e = jobs.Create(run, row)
		if e != nil {
			return e
		}
		copy := *current
		captured = &copy
		return nil
	}
	var e error
	if !req.OneTime && (s.requireControl || p.ControlScopeID != "") {
		if s.authority == nil {
			return nil, nil, controlFailure(503, "BACKUP_POLICY_CONTROL_UNAVAILABLE")
		}
		op := "start"
		if req.TriggerType == model.BackupTriggerTypeScheduled {
			op = "scheduled"
		}
		e = s.authority.Guard(ctx, p.ID, 0, op, action)
	} else {
		e = action(ctx, s.db, p)
	}
	return saved, captured, e
}
