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

// restoreInstance has one target and one consent. Staging is a disposable
// preflight, never a second user-managed environment or a separate cutover.
func (c *Coordinator) restoreInstance(ctx context.Context, j *snapshots.ManagedJob) (result error) {
	a, e := c.Active()
	if e != nil {
		return e
	}
	if a.Maintenance || a.Generation != j.ExpectedGeneration {
		return errors.New("当前实例已经变化，请刷新后重新确认恢复")
	}
	driver, ok := c.Driver.(interface {
		PreflightRestore(context.Context, *snapshots.ManagedJob, Deployment) ([]snapshots.RecoveryCheck, error)
		ReplaceCurrent(context.Context, *snapshots.ManagedJob, Deployment) (Deployment, []snapshots.RecoveryCheck, error)
		RevertReplacement(context.Context, snapshots.ManagedJob, string) error
		FinishReplacement(context.Context, snapshots.ManagedJob) error
	})
	if !ok {
		return errors.New("当前部署尚未接入原地恢复执行器")
	}
	j.CandidateID = a.Deployment.ID // private maintenance-resume identity, not a selectable environment
	if e = c.step(j, "preflight_restore"); e != nil {
		return e
	}
	j.Checks, e = driver.PreflightRestore(ctx, j, a.Deployment)
	if e != nil {
		return e
	}
	if !checksPassed(j.Checks) {
		return errors.New("恢复预检查未通过，当前实例未改变")
	}
	old := a
	if e = snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), old); e != nil {
		return e
	}
	a.Maintenance, a.JobID = true, j.ID
	if e = c.saveActive(a); e != nil {
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
			if err := c.holdCandidate(cleanup, j, a.Deployment); err != nil {
				result = fmt.Errorf("%w; 保持维护失败: %v", result, err)
			}
			return
		}
		if err := driver.RevertReplacement(cleanup, *j, "failed-"+j.ID); err != nil {
			j.Status = "attention_required"
			result = fmt.Errorf("%w; 恢复补偿失败: %v", result, err)
			return
		}
		if err := c.rollback(cleanup, j, old, Deployment{}); err != nil {
			j.Status = "attention_required"
			result = fmt.Errorf("%w; 原实例重新启动失败: %v", result, err)
		} else {
			j.Status, j.Phase = "rolled_back", "rolled_back"
		}
	}()
	if e = c.step(j, "maintenance"); e != nil {
		return e
	}
	if e = c.Driver.Stop(ctx, old.Deployment); e != nil {
		return e
	}
	if e = c.Driver.Capture(ctx, j, old.Deployment); e != nil {
		return e
	}
	if j.RollbackSnapshotID == "" {
		return errors.New("未保存恢复前保护快照，已阻止覆盖")
	}
	if e = c.step(j, "rollback_point_saved"); e != nil {
		return e
	}
	d, checks, e := driver.ReplaceCurrent(ctx, j, old.Deployment)
	j.Checks = append(j.Checks, checks...)
	if e != nil {
		return e
	}
	if d.ID != old.Deployment.ID || d.Root != old.Deployment.Root || d.ConfigFile != old.Deployment.ConfigFile {
		return errors.New("恢复执行器改变了固定实例身份或正式配置路径")
	}
	if !checksPassed(checks) {
		return errors.New("恢复后的业务验证失败")
	}
	a.Deployment = d
	a.Generation++
	a.LastSwitchID = ""
	if e = c.saveActive(a); e != nil {
		return e
	}
	if e = c.step(j, "instance_replaced"); e != nil {
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
	checks, e = c.Driver.Verify(ctx, d)
	j.Checks = append(j.Checks, checks...)
	if e != nil {
		return e
	}
	if !checksPassed(checks) {
		return errors.New("恢复后的正常运行检查失败，已保留维护现场")
	}
	a.Maintenance, a.JobID = false, ""
	if e = c.saveActive(a); e != nil {
		return e
	}
	j.BusinessVerified, j.Status = true, "complete"
	if e = c.step(j, "complete"); e != nil {
		return e
	}
	// Once complete, the rollback snapshot is authoritative. A stopped copy of
	// the old installation is not exposed or retained as a switchable environment.
	if err := driver.FinishReplacement(ctx, *j); err != nil {
		j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "temporary_cleanup", Passed: false, Detail: "恢复成功；旧临时文件尚待清理：" + err.Error()})
	}
	return nil
}

func (n *Native) PreflightRestore(ctx context.Context, j *snapshots.ManagedJob, current Deployment) ([]snapshots.RecoveryCheck, error) {
	if !n.Profile.InPlace || current.ID != n.Profile.Initial.ID || current.Root != n.Profile.Initial.Root || current.ConfigFile != n.Profile.Initial.ConfigFile {
		return nil, errors.New("实例尚未接入固定目标原地恢复；请核对部署 profile")
	}
	if e := n.ownedEnvironment(current); e != nil {
		return nil, e
	}
	staging := *j
	staging.ID = j.ID
	d, checks, err := n.Prepare(ctx, &staging)
	root := filepath.Join(n.Profile.CandidateDirectory, j.ID)
	if d.Root != "" && d.Root != root {
		return checks, errors.New("恢复预检查目录身份异常")
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	if e := StopCandidateProcesses(cleanup, root); e != nil {
		return checks, errors.Join(err, e)
	}
	if !canonicalNewPath(root) || root == current.Root {
		return checks, errors.New("恢复预检查目录越界")
	}
	if e := os.RemoveAll(root); e != nil {
		return checks, errors.Join(err, e)
	}
	return checks, err
}

func (n *Native) FinishReplacement(ctx context.Context, j snapshots.ManagedJob) error {
	var journal replacementJournal
	if e := snapshots.ReadCoordinatorJSON(n.replacementFile(j.ID), &journal); e != nil {
		return e
	}
	if e := n.ownedEnvironment(journal.Current); e != nil {
		return e
	}
	expected := filepath.Join(n.Profile.CandidateDirectory, "retained-"+j.ID)
	if journal.BackupRoot != expected || !canonicalNewPath(expected) || j.RollbackSnapshotID == "" {
		return errors.New("临时文件清理身份不匹配")
	}
	if e := StopCandidateProcesses(ctx, expected); e != nil {
		return e
	}
	return os.RemoveAll(expected)
}
