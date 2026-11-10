package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// RunRequestReceipt 只证明原请求已有持久任务，不证明任务完成或当前执行资格。
// 不返回目标、凭据、文件内容或请求摘要，不依赖当前策略仍然启用。
type RunRequestReceipt struct {
	ExecutionState      string    `json:"execution_state"`
	InterruptionPending bool      `json:"interruption_pending"`
	RequestID           string    `json:"request_id"`
	PolicyID            string    `json:"policy_id"`
	JobID               string    `json:"job_id"`
	PolicyRevision      int64     `json:"policy_revision"`
	SourceID            string    `json:"source_id"`
	Status              string    `json:"status"`
	Phase               string    `json:"phase"`
	RemoteState         string    `json:"remote_state"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// RunRequest 仅从已有任务核对 UUID。未找到、目录损坏或冲突均不能证明从未执行，
// 因而不创建任务、不重试传输、不重写记录，也不解除任何执行保护。
func (s *Service) RunRequest(ctx context.Context, policyID, requestID string) (RunRequestReceipt, error) {
	var out RunRequestReceipt
	if e := s.check(ctx); e != nil {
		return out, e
	}
	if !validID(policyID) || !validID(requestID) {
		return out, failure(400, "ARCHIVE_REQUEST_ID_INVALID", "策略和原请求编号必须是 UUID")
	}
	ids, e := s.records("jobs")
	if e != nil || len(ids) > 10000 {
		return out, failure(503, "ARCHIVE_REQUEST_LOOKUP_UNAVAILABLE", "原请求暂不能核对，请保留编号")
	}
	hash := sha256.Sum256([]byte(requestID))
	want := hex.EncodeToString(hash[:])
	var match *jobRecord
	for _, id := range ids {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		j, e := s.readJob(id)
		if e != nil {
			return out, failure(503, "ARCHIVE_REQUEST_LOOKUP_UNAVAILABLE", "原请求清单不完整，请保留编号")
		}
		if j.RequestKey != want {
			continue
		}
		if match != nil {
			return out, failure(503, "ARCHIVE_REQUEST_AMBIGUOUS", "原请求匹配多个任务，需人工核对")
		}
		copy := j
		match = &copy
	}
	if match == nil {
		return out, failure(404, "ARCHIVE_REQUEST_UNCONFIRMED", "未核对到原请求；这不证明此前未执行，请保留编号")
	}
	if match.PolicyID != policyID || match.OneTime || match.Imported || match.Recovered || match.MigratedFrom != "" {
		return out, failure(409, "ARCHIVE_REQUEST_CONTEXT_CONFLICT", "原请求归属不匹配，请保留编号并核对")
	}
	if match.PolicyRevision < 1 || !labelPattern.MatchString(match.SourceID) || match.Status == "" || match.Phase == "" || match.RemoteState == "" || match.CreatedAt.IsZero() || match.UpdatedAt.IsZero() {
		return out, failure(503, "ARCHIVE_REQUEST_EVIDENCE_INCOMPLETE", "原请求证据不完整，请保留编号并核对")
	}
	return RunRequestReceipt{ExecutionState: s.observedJob(*match).ExecutionState, InterruptionPending: match.InterruptionPending, RequestID: requestID, PolicyID: match.PolicyID, JobID: match.ID, PolicyRevision: match.PolicyRevision, SourceID: match.SourceID, Status: match.Status, Phase: match.Phase, RemoteState: match.RemoteState, CreatedAt: match.CreatedAt, UpdatedAt: match.UpdatedAt}, nil
}

// StartAtRevision 将手动操作绑定页面已查看的策略版本；后台调度仍走既有入口。
func (s *Service) StartAtRevision(ctx context.Context, policyID, key string, revision int64, operator, trace string) (Job, error) {
	if e := s.check(ctx); e != nil {
		return Job{}, e
	}
	if revision < 1 {
		return Job{}, failure(400, "ARCHIVE_POLICY_REVISION_REQUIRED", "请刷新并核对当前策略版本")
	}
	p, e := s.policy(ctx, policyID)
	if e != nil {
		return Job{}, e
	}
	if p.Revision != revision {
		return Job{}, failure(409, "ARCHIVE_POLICY_REVISION_CONFLICT", "策略版本已变化，请先核对原请求")
	}
	if s.policyControlRequired() && !p.Enabled {
		return Job{}, failure(409, "ARCHIVE_POLICY_DISABLED", "策略已暂停")
	}
	return s.startPolicyAtRevision(ctx, p, key, operator, trace, false, revision)
}
