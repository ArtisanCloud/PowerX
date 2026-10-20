// Package system_recovery 以业务库外账本执行恢复；默认使用同一个 Core 的维护启动模式，兼容旧隔离演练合同。
package system_recovery

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

type Deployment struct {
	ValidationURL   string `json:"validation_url,omitempty"`
	PublicBaseURL   string `json:"public_base_url,omitempty"`
	ID              string `json:"id"`
	Root            string `json:"root"`
	ConfigFile      string `json:"config_file"`
	CoreBinary      string `json:"core_binary"`
	WebOutput       string `json:"web_output"`
	PostgresData    string `json:"postgres_data,omitempty"`
	PostgresPort    int    `json:"postgres_port,omitempty"`
	SocketDirectory string `json:"socket_directory,omitempty"`
	RedisPort       int    `json:"redis_port"`
	CorePort        int    `json:"core_port"`
	GRPCPort        int    `json:"grpc_port"`
	WebPort         int    `json:"web_port"`
	PluginPortMin   int    `json:"plugin_port_min"`
	PluginPortMax   int    `json:"plugin_port_max"`
	SourceManaged   bool   `json:"source_managed,omitempty"`
}
type Active struct {
	LastSwitchID string     `json:"last_switch_id,omitempty"`
	Generation   uint64     `json:"generation"`
	Deployment   Deployment `json:"deployment"`
	Maintenance  bool       `json:"maintenance"`
	JobID        string     `json:"job_id,omitempty"`
}

// Driver 的每一步必须可重入，且只能管理 profile 登记或自己创建的资源。
type Driver interface {
	Capture(context.Context, *snapshots.ManagedJob, Deployment) error
	Prepare(context.Context, *snapshots.ManagedJob) (Deployment, []snapshots.RecoveryCheck, error)
	Stop(context.Context, Deployment) error
	Start(context.Context, Deployment, bool) error // hold=true 禁止后台业务写入
	Verify(context.Context, Deployment) ([]snapshots.RecoveryCheck, error)
}
type Coordinator struct {
	Store   *snapshots.CoordinatorStore
	Driver  Driver
	Initial Deployment
	// SnapshotStore is also available when Core and its database are stopped.
	SnapshotStore   *snapshots.Service
	portalLoginMu   sync.Mutex
	portalLogins    []time.Time
	executorFailure atomic.Value
	// Checkpoint 是验收故障注入钩子，生产始终为 nil。
	Checkpoint func(string) error
}

var errCoordinatorAlreadyRunning = errors.New("coordinator already running")

func (c *Coordinator) activeFile() string { return filepath.Join(c.Store.Directory, "active.json") }
func (c *Coordinator) Active() (Active, error) {
	var a Active
	e := snapshots.ReadCoordinatorJSON(c.activeFile(), &a)
	if errors.Is(e, os.ErrNotExist) {
		if c.Initial.ID == "" {
			return a, errors.New("initial deployment is not configured")
		}
		a = Active{Generation: 1, Deployment: c.Initial}
		e = snapshots.WriteCoordinatorJSON(c.activeFile(), a)
	}
	return a, e
}
func (c *Coordinator) saveActive(a Active) error {
	return snapshots.WriteCoordinatorJSON(c.activeFile(), a)
}
func (c *Coordinator) candidateFile(id string) string {
	return filepath.Join(c.Store.Directory, "candidate-"+id+".json")
}
func (c *Coordinator) step(j *snapshots.ManagedJob, phase string) error {
	j.Phase = phase
	if e := c.Store.Save(*j); e != nil {
		return e
	}
	history := filepath.Join(c.Store.Directory, "history", j.ID)
	if e := os.MkdirAll(history, 0700); e != nil {
		return e
	}
	if e := snapshots.WriteCoordinatorJSON(filepath.Join(history, time.Now().UTC().Format("20060102T150405.000000000")+"-"+uuid.NewString()+".json"), struct {
		At                   time.Time `json:"at"`
		Phase                string    `json:"phase"`
		Operator             string    `json:"operator"`
		TraceID              string    `json:"trace_id"`
		WritesMayHaveResumed bool      `json:"writes_may_have_resumed"`
	}{time.Now().UTC(), phase, j.Operator, j.TraceID, j.WritesMayHaveResumed}); e != nil {
		return e
	}
	if c.Checkpoint != nil {
		return c.Checkpoint(phase)
	}
	return nil
}

// Run 一次只运行一个运维任务；恢复中的进程退出会在下一次启动按账本补偿。
func (c *Coordinator) Run(ctx context.Context) error {
	unlock, e := c.Store.Lock()
	if e != nil {
		return fmt.Errorf("%w: %v", errCoordinatorAlreadyRunning, e)
	}
	defer unlock()
	_ = os.Remove(filepath.Join(c.Store.Directory, "executor-error.json"))
	c.executorFailure.Store("")
	if _, e = c.Active(); e != nil {
		return e
	}
	if e = c.Recover(ctx); e != nil {
		return e
	}
	if driver, ok := c.Driver.(interface {
		EnsureActive(context.Context, Active) error
	}); ok {
		active, e := c.Active()
		if e != nil {
			return e
		}
		if !active.Maintenance {
			if e = driver.EnsureActive(ctx, active); e != nil {
				// A broken application must not shut down its own rescue entrance.
				if err := snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "startup-error.json"), map[string]string{"error": e.Error()}); err != nil {
					return err
				}
			} else {
				_ = os.Remove(filepath.Join(c.Store.Directory, "startup-error.json"))
			}
		}
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			_ = snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "heartbeat.json"), struct {
				At time.Time `json:"at"`
			}{time.Now().UTC()})
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	sub, e := opswatch.Subscribe(ctx, map[string][]string{"queue": {filepath.Join(c.Store.Directory, "jobs")}}, nil)
	if e != nil {
		return e
	}
	defer sub.Close()
	for {
		if e = c.RunOnce(ctx); e != nil && ctx.Err() == nil {
			return e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-sub.Changes:
			if !ok {
				return errors.New("协调器任务通知已断开")
			}
		case e := <-sub.Errors:
			if e == nil {
				e = errors.New("协调器任务通知已断开")
			}
			return e
		}
	}
}
func (c *Coordinator) RunOnce(ctx context.Context) error {
	jobs, e := c.Store.List()
	if e != nil {
		return e
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	for _, job := range jobs {
		if job.Status != "queued" {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		j := job
		j.Status = "running"
		if native, ok := c.Driver.(*Native); ok && native.Profile.InPlace && (j.Kind == "prepare" || j.Kind == "cutover" || j.Kind == "rollback" || j.Kind == "cleanup" || j.Kind == "overwrite") {
			j.Status, j.Error = "failed", "旧环境切换任务已停用，请选择快照恢复当前实例"
			if e = c.Store.Save(j); e != nil {
				return e
			}
			continue
		}
		if e = c.Store.Save(j); e != nil {
			return e
		}
		run, cancel := context.WithTimeout(ctx, 45*time.Minute)
		switch j.Kind {
		case "restore":
			e = c.restoreInstance(run, &j)
		case "capture":
			e = c.capture(run, &j)
		case "prepare":
			var d Deployment
			d, j.Checks, e = c.Driver.Prepare(run, &j)
			// Retain an owned candidate identity even on failure, so explicit cleanup
			// removes the complete partial environment rather than leaving orphan files.
			if e != nil && d.ID == j.ID && d.Root != "" && !d.SourceManaged {
				_ = snapshots.WriteCoordinatorJSON(c.candidateFile(j.ID), d)
			}
			if e == nil {
				for _, check := range j.Checks {
					if !check.Passed {
						e = errors.New("required business check failed")
						break
					}
				}
			}
			if e == nil && len(j.Checks) == 0 {
				e = errors.New("empty verification report")
			}
			if e == nil {
				e = snapshots.WriteCoordinatorJSON(c.candidateFile(j.ID), d)
			}
			if e == nil {
				j.ValidationURL = d.ValidationURL
				j.Status = "verified"
				j.Phase = "awaiting_review"
				j.BusinessVerified = true
			}
		case "cutover":
			e = c.cutover(run, &j)
		case "rollback":
			e = c.returnPrevious(run, &j)
		case "cleanup":
			e = c.cleanupEnvironment(run, &j)
		case "overwrite":
			e = c.overwrite(run, &j)
		case "resume":
			e = c.resume(run, &j)
		default:
			e = errors.New("unsupported coordinator job")
		}
		cancel()
		if e != nil {
			j.Error = e.Error()
			if j.Status == "running" {
				j.Status = "failed"
			}
			if driver, ok := c.Driver.(interface {
				CleanupFailed(context.Context, snapshots.ManagedJob) error
			}); ok {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
				cleanupErr := driver.CleanupFailed(cleanup, j)
				cancel()
				if cleanupErr != nil {
					j.Error += "; cleanup: " + cleanupErr.Error()
				}
			}
		}
		if j.Status == "running" {
			j.Status = "complete"
			j.Phase = "complete"
		}
		if write := c.Store.Save(j); write != nil {
			return write
		}
	}
	return nil
}
func (c *Coordinator) capture(ctx context.Context, j *snapshots.ManagedJob) error {
	a, e := c.Active()
	if e != nil {
		return e
	}
	if a.Maintenance {
		return errors.New("deployment is in maintenance")
	}
	if driver, ok := c.Driver.(interface {
		PreflightSource(context.Context, Deployment) error
	}); ok {
		if e = driver.PreflightSource(ctx, a.Deployment); e != nil {
			return e
		}
	}
	online, ok := c.Driver.(interface {
		CaptureOnline(context.Context, *snapshots.ManagedJob, Deployment) error
	})
	if !ok {
		return errors.New("部署驱动不支持在线快照；未停止业务服务")
	}
	if e = c.step(j, "capturing_online"); e != nil {
		return e
	}
	return online.CaptureOnline(ctx, j, a.Deployment)
}
func (c *Coordinator) cutover(ctx context.Context, j *snapshots.ManagedJob) (result error) {
	a, e := c.Active()
	if e != nil {
		return e
	}
	if a.Maintenance || a.Generation != j.ExpectedGeneration {
		return errors.New("active deployment changed; refresh and confirm again")
	}
	if driver, ok := c.Driver.(interface {
		PreflightSource(context.Context, Deployment) error
	}); ok {
		if e = driver.PreflightSource(ctx, a.Deployment); e != nil {
			return e
		}
	}
	var candidate Deployment
	if e = snapshots.ReadCoordinatorJSON(c.candidateFile(j.CandidateID), &candidate); e != nil {
		return e
	}
	if candidate.ID == a.Deployment.ID {
		return errors.New("candidate is already the active deployment")
	}
	old := a
	j.PreviousID = old.Deployment.ID
	if e = snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), old); e != nil {
		return e
	}
	// 终态前的任何失败都执行同一补偿路径；写入可能恢复后禁止无损回退承诺。
	defer func() {
		if result != nil {
			recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
			defer cancel()
			if j.WritesMayHaveResumed {
				j.Status = "attention_required"
				if holdErr := c.holdCandidate(recovery, j, candidate); holdErr != nil {
					result = fmt.Errorf("%w; candidate hold failed: %v", result, holdErr)
				}
				return
			}
			if rollback := c.rollback(recovery, j, old, candidate); rollback != nil {
				j.Status = "attention_required"
				result = fmt.Errorf("%w; compensation failed: %v", result, rollback)
			} else {
				j.Status = "rolled_back"
				j.Phase = "rolled_back"
			}
		}
	}()
	a.Maintenance = true
	a.JobID = j.ID
	if e = c.saveActive(a); e != nil {
		return e
	}
	if e = c.step(j, "maintenance"); e != nil {
		return e
	}
	if e = c.Driver.Stop(ctx, old.Deployment); e != nil {
		return e
	}
	if e = c.step(j, "writers_stopped"); e != nil {
		return e
	}
	// 必须在停写后重新采集当前状态 B，不能用准备候选之前的旧包作回退点。
	if e = c.Driver.Capture(ctx, j, old.Deployment); e != nil {
		return e
	}
	if j.RollbackSnapshotID == "" {
		return errors.New("fresh rollback snapshot was not recorded")
	}
	if e = c.step(j, "rollback_point_saved"); e != nil {
		return e
	}
	a.Deployment = candidate
	a.Generation++
	if e = c.saveActive(a); e != nil {
		return e
	}
	if e = c.step(j, "pointer_switched"); e != nil {
		return e
	}
	if driver, ok := c.Driver.(interface {
		ResetCandidateVerification(context.Context, Deployment) error
	}); ok {
		if e = driver.ResetCandidateVerification(ctx, candidate); e != nil {
			return e
		}
	}
	if e = c.Driver.Start(ctx, candidate, true); e != nil {
		return e
	}
	if e = c.step(j, "candidate_started_read_only"); e != nil {
		return e
	}
	checks, e := c.Driver.Verify(ctx, candidate)
	j.Checks = checks
	if e != nil {
		return e
	}
	if len(checks) == 0 {
		return errors.New("empty post-switch verification")
	}
	for _, check := range checks {
		if !check.Passed {
			return errors.New("post-switch business verification failed")
		}
	}
	if e = c.step(j, "candidate_verified"); e != nil {
		return e
	}
	// 先停止只读候选，再登记写入可能恢复；后续失败保留现场，不能回滚丢失新数据。
	if e = c.Driver.Stop(ctx, candidate); e != nil {
		return e
	}
	if driver, ok := c.Driver.(interface {
		ResetCandidateVerification(context.Context, Deployment) error
	}); ok {
		if e = driver.ResetCandidateVerification(ctx, candidate); e != nil {
			return e
		}
	}
	j.WritesMayHaveResumed = true
	if e = c.step(j, "resuming_writes"); e != nil {
		return e
	}
	if e = c.Driver.Start(ctx, candidate, false); e != nil {
		return e
	}
	if e = c.step(j, "verifying_active"); e != nil {
		return e
	}
	checks, e = c.Driver.Verify(ctx, candidate)
	j.Checks = checks
	if e != nil {
		return e
	}
	if len(checks) == 0 {
		return errors.New("empty active verification")
	}
	for _, check := range checks {
		if !check.Passed {
			return errors.New("active deployment verification failed")
		}
	}
	a.Maintenance = false
	a.JobID = ""
	a.LastSwitchID = j.ID
	if e = c.saveActive(a); e != nil {
		return e
	}
	j.Status = "complete"
	j.BusinessVerified = true
	return c.step(j, "complete")
}
func (c *Coordinator) rollback(ctx context.Context, j *snapshots.ManagedJob, old Active, candidate Deployment) error {
	if e := c.step(j, "rolling_back"); e != nil {
		return e
	}
	if candidate.ID != "" {
		if e := c.Driver.Stop(ctx, candidate); e != nil {
			return e
		}
	}
	old.Maintenance = true
	old.JobID = j.ID
	if e := c.saveActive(old); e != nil {
		return e
	}
	if e := c.Driver.Start(ctx, old.Deployment, true); e != nil {
		return e
	}
	checks, e := c.Driver.Verify(ctx, old.Deployment)
	if e != nil {
		return e
	}
	if len(checks) == 0 {
		return errors.New("empty rollback verification")
	}
	for _, check := range checks {
		if !check.Passed {
			return errors.New("rollback verification failed")
		}
	}
	if e = c.Driver.Stop(ctx, old.Deployment); e != nil {
		return e
	}
	if e = c.Driver.Start(ctx, old.Deployment, false); e != nil {
		return e
	}
	checks, e = c.Driver.Verify(ctx, old.Deployment)
	if e != nil {
		return e
	}
	if len(checks) == 0 {
		return errors.New("empty resumed rollback verification")
	}
	for _, check := range checks {
		if !check.Passed {
			return errors.New("resumed rollback verification failed")
		}
	}
	old.Maintenance = false
	old.JobID = ""
	return c.saveActive(old)
}

func (c *Coordinator) holdCandidate(ctx context.Context, j *snapshots.ManagedJob, candidate Deployment) error {
	active, e := c.Active()
	if e != nil {
		return e
	}
	if active.Deployment.ID != candidate.ID {
		return errors.New("active identity changed while holding candidate")
	}
	active.Maintenance = true
	active.JobID = j.ID
	if e = c.saveActive(active); e != nil {
		return e
	}
	if e = c.Driver.Stop(ctx, candidate); e != nil {
		return e
	}
	return c.Driver.Start(ctx, candidate, true)
}

// Recover 在持有 worker 锁后执行；未恢复写入的切换统一回到原部署。
func (c *Coordinator) Recover(ctx context.Context) error {
	jobs, e := c.Store.List()
	if e != nil {
		return e
	}
	for _, job := range jobs {
		j := job
		if j.Status != "running" {
			continue
		}
		_, priorErr := os.Lstat(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"))
		if j.Kind == "restore" && errors.Is(priorErr, os.ErrNotExist) {
			j.Status, j.Phase, j.Error = "failed", "interrupted", "恢复预检查中断，当前实例未被覆盖"
			if n, ok := c.Driver.(*Native); ok {
				root := filepath.Join(n.Profile.CandidateDirectory, j.ID)
				if canonicalNewPath(root) && root != n.Profile.Initial.Root {
					if err := StopCandidateProcesses(ctx, root); err != nil {
						return err
					}
					if err := os.RemoveAll(root); err != nil {
						return err
					}
				}
			}
		} else if (j.Kind == "overwrite" || j.Kind == "restore") && !j.WritesMayHaveResumed {
			var old Active
			err := snapshots.ReadCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), &old)
			if err == nil {
				driver, ok := c.Driver.(interface {
					RevertReplacement(context.Context, snapshots.ManagedJob, string) error
				})
				if !ok {
					err = errors.New("部署驱动不支持原地覆盖补偿")
				} else {
					err = driver.RevertReplacement(ctx, j, "interrupted-"+j.ID)
				}
				if err == nil {
					err = c.rollback(ctx, &j, old, Deployment{})
				}
			}
			if err == nil {
				j.Status = "rolled_back"
				j.Phase = "rolled_back"
				j.Error = "原地覆盖在恢复写入前中断，已恢复原完整环境"
			} else {
				j.Status = "attention_required"
				j.Error = "原地覆盖中断，原环境补偿失败：" + err.Error()
			}
		} else if j.Kind == "overwrite" || j.Kind == "restore" || j.Kind == "rollback" {
			a, err := c.Active()
			if err != nil {
				return err
			}
			j.Status = "attention_required"
			j.Error = "环境恢复中断，已保留维护状态和回退快照；请核对独立任务后处理，未自动丢弃可能产生的新数据"
			if err = c.holdCandidate(ctx, &j, a.Deployment); err != nil {
				j.Error += "；停写失败：" + err.Error()
			}
		} else if j.Kind == "resume" {
			var candidate Deployment
			if j.CandidateID == c.Initial.ID {
				candidate = c.Initial
			} else if e = snapshots.ReadCoordinatorJSON(c.candidateFile(j.CandidateID), &candidate); e != nil {
				return e
			}
			j.Status = "attention_required"
			j.Error = "继续候选的任务中断，保留当前数据并恢复维护"
			if e = c.holdCandidate(ctx, &j, candidate); e != nil {
				j.Error += "；停写失败：" + e.Error()
			}
		} else if j.Kind == "capture" && j.Phase == "capturing_online" {
			// 在线采集没有停止来源；协调器崩溃只能结束备份，不能重启业务。
			j.Status = "interrupted"
			j.Error = "在线备份中断，原站继续运行；可重新创建快照"
		} else if j.Kind == "cutover" || j.Kind == "capture" {
			var old Active
			if e = snapshots.ReadCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), &old); e != nil {
				active, activeErr := c.Active()
				if activeErr == nil && !active.Maintenance && active.JobID != j.ID {
					j.Status = "interrupted"
					j.Error = "协调器在修改部署前中断，原站保持不变"
				} else {
					j.Status = "attention_required"
					j.Error = "interrupted before source identity was recorded"
				}
			} else if j.WritesMayHaveResumed {
				j.Status = "attention_required"
				j.Error = "进程中断时写入可能已恢复，保留现场，必须人工核对新数据"
				var candidate Deployment
				if e = snapshots.ReadCoordinatorJSON(c.candidateFile(j.CandidateID), &candidate); e != nil {
					return e
				}
				if e = c.holdCandidate(ctx, &j, candidate); e != nil {
					j.Error += "；候选停写失败：" + e.Error()
				}
			} else {
				var candidate Deployment
				if j.CandidateID != "" {
					_ = snapshots.ReadCoordinatorJSON(c.candidateFile(j.CandidateID), &candidate)
				}
				if e = c.rollback(ctx, &j, old, candidate); e != nil {
					j.Status = "attention_required"
					j.Error = e.Error()
				} else {
					j.Status = "rolled_back"
					j.Phase = "rolled_back"
					j.Error = "协调器中断后已恢复原部署"
				}
			}
		} else {
			j.Status = "interrupted"
			j.Error = "候选准备中断；源环境未切换，候选目录保留"
		}
		if driver, ok := c.Driver.(interface {
			CleanupFailed(context.Context, snapshots.ManagedJob) error
		}); ok {
			if cleanupErr := driver.CleanupFailed(ctx, j); cleanupErr != nil {
				j.Error += "; cleanup: " + cleanupErr.Error()
			}
		}
		if e = c.Store.Save(j); e != nil {
			return e
		}
	}
	return nil
}
