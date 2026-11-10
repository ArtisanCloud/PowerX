package archive_ops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// recordedPending 仅识别尚未形成完整终态的本地执行，不把历史成功当作当前资格。
func recordedPending(j jobRecord) bool {
	return !j.Imported && !j.Recovered && (j.Status == "running" || j.Status == "success" && j.RemoteState == "pending")
}
func interruptionPending(j jobRecord) bool {
	return !j.Imported && !j.Recovered && (j.InterruptionPending || j.Status == "interrupted")
}

func (s *Service) beginExecution(j *jobRecord) (func(), error) {
	if e := s.requireCommandsDrained(true); e != nil {
		return nil, e
	}
	executionID := uuid.NewString()
	if e := s.initializeCommandRecord(j.ID, executionID, true); e != nil {
		return nil, e
	}
	j.ExecutionID = executionID
	j.CommandProtocol = 2
	s.executions.Store(j.ID, j.ExecutionID)
	id, execution := j.ID, j.ExecutionID
	return func() { s.executions.CompareAndDelete(id, execution) }, nil
}

// observedJob 的执行状态仅来自当前 Service 的匹配执行凭据；GET 不接管锁或写文件。
func (s *Service) observedJob(j jobRecord) Job {
	out := j.Job
	out.CommandDrainState = s.jobCommandState(j)
	out.ExecutionState = "recorded"
	if token, ok := s.executions.Load(j.ID); ok && j.ExecutionID != "" && token == j.ExecutionID {
		out.ExecutionState = "active"
	} else if interruptionPending(j) {
		out.ExecutionState = "interrupted"
	} else if recordedPending(j) {
		out.ExecutionState = "unconfirmed"
	}
	return out
}

// archiveManifest 不向历史副本传递当前执行归属／待处理门禁；中断时间仍可追溯。
func archiveManifest(j Job) Job {
	j.ExecutionState = ""
	j.CommandDrainState = ""
	j.InterruptionPending = false
	return j
}

// markInterruptedLocked 的调用方必须持有原策略锁，以及迁移任务的专用锁。
// 这里只记录“执行已失去所有者”，不宣称之前未写入，不删除或补造内容。
func (s *Service) markInterruptedLocked(j *jobRecord) error {
	if !recordedPending(*j) {
		return nil
	}
	at := time.Now().UTC()
	j.InterruptedAt = &at
	j.InterruptedPhase = j.Phase
	j.InterruptionPending = true
	j.Phase = "interrupted"
	if j.Status == "running" {
		j.Status = "interrupted"
		j.Error = "采集执行已中断；原任务和本地内容保留，需核对后处理"
	} else {
		j.RemoteState = "unconfirmed"
		j.RemoteError = "远端执行已中断，可能已收到部分内容；请核对并重试原任务"
	}
	return s.saveJob(j)
}

// reconcileInterrupted 仅在原组件锁可取得时标记失去执行所有者，不认证外部命令排空。
// 本机命令需独立的序列证明；其他进程仍持有原策略锁时不改记录。
// 在启动和既有调度 Tick 内调用；不增加队列、轮询或业务数据库依赖。
func (s *Service) reconcileInterrupted(ctx context.Context) error {
	ids, e := s.records("jobs")
	if e != nil {
		return e
	}
	if len(ids) > 10000 {
		return failure(503, "ARCHIVE_EXECUTION_CATALOG_UNAVAILABLE", "执行记录超限，停止新归档")
	}
	for _, id := range ids {
		if e := ctx.Err(); e != nil {
			return e
		}
		j, e := s.readJob(id)
		if e != nil {
			return e
		}
		if !recordedPending(j) {
			continue
		}
		var releaseMigration func()
		if j.MigratedFrom != "" {
			releaseMigration, e = s.lock(ctx, "migration-"+j.ID)
			if e != nil {
				if errors.Is(e, ErrBusy) {
					continue
				}
				return e
			}
		}
		release, e := s.lock(ctx, j.PolicyID)
		if e != nil {
			if releaseMigration != nil {
				releaseMigration()
			}
			if errors.Is(e, ErrBusy) {
				continue
			}
			return e
		}
		current, e := s.readJob(id)
		if e == nil {
			e = s.markInterruptedLocked(&current)
		}
		release()
		if releaseMigration != nil {
			releaseMigration()
		}
		if e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) policyHasInterrupted(ctx context.Context, policyID string) (bool, error) {
	return s.policyHasInterruptedExcept(ctx, policyID, "")
}
func (s *Service) policyHasInterruptedExcept(ctx context.Context, policyID, except string) (bool, error) {
	ids, e := s.records("jobs")
	if e != nil {
		return false, e
	}
	if len(ids) > 10000 {
		return false, failure(503, "ARCHIVE_EXECUTION_CATALOG_UNAVAILABLE", "执行记录超限，停止新归档")
	}
	for _, id := range ids {
		if e := ctx.Err(); e != nil {
			return false, e
		}
		j, e := s.readJob(id)
		if e != nil {
			return false, e
		}
		if id != except && j.PolicyID == policyID && (interruptionPending(j) || recordedPending(j)) {
			return true, nil
		}
	}
	return false, nil
}
