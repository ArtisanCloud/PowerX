package system_recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

func checksPassed(checks []snapshots.RecoveryCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

// returnPrevious 是用户明确确认的回退，不沿用故障时“无损自动回退”的承诺。
// 回退前保存当前 A 的独立回退快照，保留验证期间可能产生的新数据。
func (c *Coordinator) returnPrevious(ctx context.Context, j *snapshots.ManagedJob) (result error) {
	a, e := c.Active()
	if e != nil {
		return e
	}
	parent, e := c.Store.Get(j.CandidateID)
	if e != nil {
		return e
	}
	if a.Maintenance || a.Generation != j.ExpectedGeneration || a.LastSwitchID != parent.ID || parent.Status != "complete" {
		return errors.New("当前环境或回退点已变化，请刷新后重新确认")
	}
	var old Active
	if e = snapshots.ReadCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+parent.ID+".json"), &old); e != nil {
		return e
	}
	before := a
	changed := false
	defer func() {
		if result != nil {
			j.Status = "attention_required"
			current := before
			if changed {
				current = old
			}
			current.Maintenance = true
			current.JobID = j.ID
			_ = c.saveActive(current)
		}
	}()
	if e = snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), before); e != nil {
		return e
	}
	a.Maintenance = true
	a.JobID = j.ID
	if e = c.saveActive(a); e != nil {
		return e
	}
	if e = c.step(j, "maintenance"); e != nil {
		return e
	}
	if e = c.Driver.Stop(ctx, before.Deployment); e != nil {
		return e
	}
	if e = c.Driver.Capture(ctx, j, before.Deployment); e != nil {
		return e
	}
	if j.RollbackSnapshotID == "" {
		return errors.New("回退前未生成当前状态保护快照")
	}
	if parent.Kind == "overwrite" {
		driver, ok := c.Driver.(interface {
			RevertReplacement(context.Context, snapshots.ManagedJob, string) error
		})
		if !ok {
			return errors.New("部署驱动不支持原地覆盖回退")
		}
		if e = driver.RevertReplacement(ctx, parent, j.ID); e != nil {
			return e
		}
	}
	old.Generation = before.Generation + 1
	old.Maintenance = true
	old.JobID = j.ID
	old.LastSwitchID = ""
	changed = true
	if e = c.saveActive(old); e != nil {
		return e
	}
	j.PreviousID = before.Deployment.ID
	if e = c.step(j, "returning_previous"); e != nil {
		return e
	}
	// 写入可能恢复之前的错误保留维护现场；不能把尚未核验的旧环境开放给业务。
	defer func() {
		if result != nil {
			j.Status = "attention_required"
			_ = c.saveActive(Active{Generation: old.Generation, Deployment: old.Deployment, Maintenance: true, JobID: j.ID})
		}
	}()
	if e = c.Driver.Start(ctx, old.Deployment, true); e != nil {
		return e
	}
	j.Checks, e = c.Driver.Verify(ctx, old.Deployment)
	if e != nil {
		return e
	}
	if !checksPassed(j.Checks) {
		return errors.New("原环境只读验证未通过")
	}
	if e = c.Driver.Stop(ctx, old.Deployment); e != nil {
		return e
	}
	j.WritesMayHaveResumed = true
	if e = c.step(j, "resuming_writes"); e != nil {
		return e
	}
	if e = c.Driver.Start(ctx, old.Deployment, false); e != nil {
		return e
	}
	j.Checks, e = c.Driver.Verify(ctx, old.Deployment)
	if e != nil {
		return e
	}
	if !checksPassed(j.Checks) {
		return errors.New("原环境恢复写入后的业务验证未通过")
	}
	old.Maintenance = false
	old.JobID = ""
	if e = c.saveActive(old); e != nil {
		return e
	}
	j.BusinessVerified = true
	j.Status = "complete"
	return c.step(j, "complete")
}

func (c *Coordinator) cleanupEnvironment(ctx context.Context, j *snapshots.ManagedJob) error {
	a, e := c.Active()
	if e != nil {
		return e
	}
	if a.Maintenance || a.Generation != j.ExpectedGeneration {
		return errors.New("部署已变化或正在维护，不能清理")
	}
	var candidate Deployment
	if e = snapshots.ReadCoordinatorJSON(c.candidateFile(j.CandidateID), &candidate); e != nil {
		return e
	}
	if candidate.ID == a.Deployment.ID || candidate.ID == c.Initial.ID {
		return errors.New("当前环境和初始环境不能清理")
	}
	if a.LastSwitchID != "" {
		var previous Active
		if snapshots.ReadCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+a.LastSwitchID+".json"), &previous) == nil && previous.Deployment.ID == candidate.ID {
			return errors.New("该环境仍是当前切换的回退环境，不能清理")
		}
	}
	driver, ok := c.Driver.(interface {
		DeleteEnvironment(context.Context, Deployment) error
	})
	if !ok {
		return errors.New("部署驱动没有整套清理能力")
	}
	if e = c.step(j, "cleaning_environment"); e != nil {
		return e
	}
	if e = driver.DeleteEnvironment(ctx, candidate); e != nil {
		return e
	}
	prior, e := c.Store.Get(j.CandidateID)
	if e != nil {
		return e
	}
	prior.Status = "deleted"
	prior.Phase = "deleted"
	prior.BusinessVerified = false
	prior.ValidationURL = ""
	if e = c.Store.Save(prior); e != nil {
		return e
	}
	if e = os.Remove(c.candidateFile(j.CandidateID)); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	j.Status = "complete"
	return c.step(j, "complete")
}

func (c *Coordinator) overwrite(ctx context.Context, j *snapshots.ManagedJob) (result error) {
	a, e := c.Active()
	if e != nil {
		return e
	}
	if a.Maintenance || a.Generation != j.ExpectedGeneration {
		return errors.New("部署已变化，请刷新后重新确认原地覆盖")
	}
	driver, ok := c.Driver.(interface {
		ReplaceCurrent(context.Context, *snapshots.ManagedJob, Deployment) (Deployment, []snapshots.RecoveryCheck, error)
		RevertReplacement(context.Context, snapshots.ManagedJob, string) error
	})
	if !ok {
		return errors.New("当前部署不支持本地原地覆盖")
	}
	allowed := false
	for _, target := range c.Store.Targets {
		if target.ID == j.TargetID && target.Environment == "dev" && target.AllowOverwrite {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("原地覆盖只允许已登记的本地开发环境")
	}
	old := a
	if e = snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), old); e != nil {
		return e
	}
	j.PreviousID = old.Deployment.ID
	a.Maintenance = true
	a.JobID = j.ID
	if e = c.saveActive(a); e != nil {
		return e
	}
	if e = c.step(j, "maintenance"); e != nil {
		return e
	}
	defer func() {
		if result == nil {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		if j.WritesMayHaveResumed {
			j.Status = "attention_required"
			_ = c.holdCandidate(cleanup, j, a.Deployment)
			return
		}
		if undo := driver.RevertReplacement(cleanup, *j, "failed-"+j.ID); undo != nil {
			j.Status = "attention_required"
			result = fmt.Errorf("%w; 原环境回退失败: %v", result, undo)
			return
		}
		if undo := c.rollback(cleanup, j, old, Deployment{}); undo != nil {
			j.Status = "attention_required"
			result = fmt.Errorf("%w; 原环境启动失败: %v", result, undo)
		} else {
			j.Status = "rolled_back"
			j.Phase = "rolled_back"
		}
	}()
	if e = c.Driver.Stop(ctx, old.Deployment); e != nil {
		return e
	}
	if e = c.Driver.Capture(ctx, j, old.Deployment); e != nil {
		return e
	}
	if j.RollbackSnapshotID == "" {
		return errors.New("原地覆盖前未生成当前状态保护快照")
	}
	d, checks, e := driver.ReplaceCurrent(ctx, j, old.Deployment)
	j.Checks = checks
	if e != nil {
		return e
	}
	if !checksPassed(checks) {
		return errors.New("原地恢复未通过业务验证")
	}
	a.Deployment = d
	a.Generation++
	a.LastSwitchID = j.ID
	if e = c.saveActive(a); e != nil {
		return e
	}
	if e = c.Driver.Stop(ctx, d); e != nil {
		return e
	}
	j.WritesMayHaveResumed = true
	if e = c.step(j, "resuming_writes"); e != nil {
		return e
	}
	if e = c.Driver.Start(ctx, d, false); e != nil {
		return e
	}
	j.Checks, e = c.Driver.Verify(ctx, d)
	if e != nil {
		return e
	}
	if !checksPassed(j.Checks) {
		return errors.New("原地恢复写入后的验证失败")
	}
	a.Maintenance = false
	a.JobID = ""
	if e = c.saveActive(a); e != nil {
		return e
	}
	j.BusinessVerified = true
	j.Status = "complete"
	return c.step(j, "complete")
}
