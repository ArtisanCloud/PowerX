package archive_ops

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

const interruptionClosureAction = "close_unknown_keep_protection"

// InterruptionRequestReview 核对原请求和当前保护；不证明此前未执行或采集完整。
type InterruptionRequestReview struct {
	RequestID              string    `json:"request_id"`
	JobID                  string    `json:"job_id"`
	InputDigest            string    `json:"input_digest"`
	ReviewVersion          string    `json:"review_version"`
	RegisteredEpochMatches bool      `json:"registered_epoch_matches"`
	InterruptionPending    bool      `json:"interruption_pending"`
	OriginalLocksAvailable bool      `json:"original_locks_available"`
	PolicyRevision         int64     `json:"policy_revision"`
	ReadOnly               bool      `json:"read_only"`
	ObservedAt             time.Time `json:"observed_at"`
}
type CloseInterruptionRequestInput struct {
	RequestID       string `json:"request_id"`
	ExpectedVersion string `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	Confirm         bool   `json:"confirm"`
}

// InterruptionClosure 与原台账同次落盘；仅结案未知请求，不修改任务及其保护。
type InterruptionClosure struct {
	RequestID       string    `json:"request_id"`
	InputDigest     string    `json:"input_digest"`
	ReviewVersion   string    `json:"review_version"`
	Action          string    `json:"action"`
	Reason          string    `json:"reason"`
	Operator        uint64    `json:"operator"`
	At              time.Time `json:"at"`
	RegisteredEpoch string    `json:"registered_epoch"`
	CurrentEpoch    string    `json:"current_epoch"`
	JobVersion      string    `json:"job_version"`
}

func closureInputDigest(original, input string, in CloseInterruptionRequestInput) string {
	return dispositionHash([]any{original, input, in})
}
func validateInterruptionClosure(r dispositionRecord) error {
	c := r.Closure
	if c == nil && r.State != "closed_unknown" {
		return nil
	}
	if c == nil || r.State != "closed_unknown" || r.Result != nil || !validID(c.RequestID) || c.RequestID == r.RequestID || c.Action != interruptionClosureAction || !shaPattern.MatchString(c.ReviewVersion) || !shaPattern.MatchString(c.JobVersion) || !shaPattern.MatchString(c.CurrentEpoch) || c.RegisteredEpoch != r.Epoch || c.Operator == 0 || c.At.IsZero() || !closureReasonValid(c.Reason) || c.InputDigest != closureInputDigest(r.RequestID, r.InputDigest, CloseInterruptionRequestInput{c.RequestID, c.ReviewVersion, c.Action, c.Reason, true}) {
		return failure(503, "ARCHIVE_DISPOSITION_CORRUPT", "人工结案证据损坏，保留原保护并停止处置")
	}
	return nil
}
func closureReasonValid(reason string) bool {
	return reason == strings.TrimSpace(reason) && utf8.ValidString(reason) && len([]rune(reason)) >= 5 && len(reason) <= 1024 && !strings.ContainsAny(reason, "\x00\r")
}
func (s *Service) reviewInterruptionRequestLocked(ctx context.Context, key string) (InterruptionRequestReview, dispositionRecord, jobRecord, Policy, recovery.Epoch, error) {
	var out InterruptionRequestReview
	r, e := s.readDisposition(key)
	if e != nil {
		return out, r, jobRecord{}, Policy{}, recovery.Epoch{}, e
	}
	if r.State != "registered" || r.Closure != nil || r.Result != nil {
		return out, r, jobRecord{}, Policy{}, recovery.Epoch{}, failure(409, "ARCHIVE_REQUEST_NOT_UNCONFIRMED", "仅可核对已登记且无终态的原请求")
	}
	j, e := s.readJob(r.JobID)
	if e != nil {
		return out, r, j, Policy{}, recovery.Epoch{}, e
	}
	p, e := s.dispositionPolicy(ctx, j)
	if e != nil {
		return out, r, j, p, recovery.Epoch{}, e
	}
	epoch, e := s.dispositionEpoch(ctx)
	if e != nil {
		return out, r, j, p, epoch, e
	}
	version := dispositionHash([]any{"archive-request-closure/v1", s.cfg.Instance, s.hostIdentity, r, j, p, epoch})
	out = InterruptionRequestReview{key, r.JobID, r.InputDigest, version, r.Epoch == dispositionHash(epoch), j.InterruptionPending, true, p.Revision, true, time.Now().UTC()}
	return out, r, j, p, epoch, nil
}

// ReviewInterruptionRequest 只读取得当前元数据与锁证明，不写入台账或原任务。
func (s *Service) ReviewInterruptionRequest(ctx context.Context, key string) (InterruptionRequestReview, error) {
	var out InterruptionRequestReview
	if !validID(key) {
		return out, failure(400, "ARCHIVE_REQUEST_ID_INVALID", "请求编号必须是 UUID")
	}
	if e := s.dispositionCheck(ctx); e != nil {
		return out, e
	}
	release, e := s.lock(ctx, "interruption-"+key)
	if e != nil {
		return out, failure(409, "ARCHIVE_BUSY", "原请求仍在执行，请保留原编号")
	}
	defer release()
	r, e := s.readDisposition(key)
	if e != nil {
		return out, e
	}
	j, e := s.readJob(r.JobID)
	if e != nil {
		return out, e
	}
	unlock, e := s.dispositionLocks(ctx, j)
	if e != nil {
		return out, e
	}
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	out, _, _, p, epoch, e := s.reviewInterruptionRequestLocked(ctx, key)
	if e != nil {
		return out, e
	}
	e = s.recoveryAnchor.WithEpoch(ctx, func(now recovery.Epoch) error {
		if now != epoch {
			return failure(409, "ARCHIVE_REVIEW_STALE", "恢复代次变化，请重新核对")
		}
		return s.guardPolicyAction(ctx, p, "interruption-retain", func(context.Context) error { return nil })
	})
	return out, e
}

// CloseInterruptionRequest 原子记录未知结论。重放只读，任务和 CAS 从不在此变更。
func (s *Service) CloseInterruptionRequest(ctx context.Context, key string, in CloseInterruptionRequestInput) (InterruptionReceipt, error) {
	var out InterruptionReceipt
	if e := s.dispositionCheck(ctx); e != nil {
		return out, e
	}
	if !validID(key) || !validID(in.RequestID) || key == in.RequestID || !shaPattern.MatchString(in.ExpectedVersion) || in.Action != interruptionClosureAction || !in.Confirm || !closureReasonValid(in.Reason) {
		return out, failure(400, "ARCHIVE_CLOSURE_INPUT_INVALID", "请确认原请求、核对版本及人工结案原因")
	}
	release, e := s.lock(ctx, "interruption-"+key)
	if e != nil {
		return out, failure(409, "ARCHIVE_BUSY", "原请求仍在处理，请只读核对")
	}
	defer release()
	r, e := s.readDisposition(key)
	if e != nil {
		return out, e
	}
	digest := closureInputDigest(key, r.InputDigest, in)
	if r.Closure != nil {
		if r.Closure.RequestID != in.RequestID || r.Closure.InputDigest != digest {
			return out, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "结案编号或输入与原结论不符")
		}
		return s.InterruptionRequest(ctx, key)
	}
	j, e := s.readJob(r.JobID)
	if e != nil {
		return out, e
	}
	unlock, e := s.dispositionLocks(ctx, j)
	if e != nil {
		return out, e
	}
	defer unlock()
	review, current, job, p, epoch, e := s.reviewInterruptionRequestLocked(ctx, key)
	if e != nil {
		return out, e
	}
	if review.ReviewVersion != in.ExpectedVersion {
		return out, failure(409, "ARCHIVE_REVIEW_STALE", "原台账、任务、策略或恢复代次变化，请重新核对")
	}
	// 全目录登记锁防止另一个原请求使用同一结案 UUID；扫描有界，不创建私有队列。
	registry, e := s.lock(ctx, "interruption-registry")
	if e != nil {
		return out, failure(409, "ARCHIVE_BUSY", "处置台账正在登记")
	}
	defer registry()
	ids, e := s.records("interruption-requests")
	if e != nil {
		return out, e
	}
	if len(ids) > 10000 {
		return out, failure(503, "ARCHIVE_DISPOSITION_LIMIT", "处置台账超限")
	}
	for _, id := range ids {
		if e = ctx.Err(); e != nil {
			return out, e
		}
		other, err := s.readDisposition(id)
		if err != nil {
			return out, err
		}
		if other.RequestID == in.RequestID || other.Closure != nil && other.Closure.RequestID == in.RequestID {
			return out, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "结案编号已登记，不得复用")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Legacy 配置也必须在持有配置互斥锁后重查；等待锁期间的修改不能沿用旧暂停证明。
	checked, _, _, _, _, err := s.reviewInterruptionRequestLocked(ctx, key)
	if err != nil {
		return out, err
	}
	if checked.ReviewVersion != in.ExpectedVersion {
		return out, failure(409, "ARCHIVE_REVIEW_STALE", "等待配置锁期间核对上下文发生变化")
	}
	e = s.recoveryAnchor.WithEpoch(ctx, func(now recovery.Epoch) error {
		if now != epoch {
			return failure(409, "ARCHIVE_REVIEW_STALE", "恢复代次变化，请重新核对")
		}
		return s.guardPolicyAction(ctx, p, "interruption-retain", func(context.Context) error {
			current.State = "closed_unknown"
			current.CurrentRecordMatches = false
			current.CurrentProtectionMatches = false
			current.Closure = &InterruptionClosure{in.RequestID, digest, in.ExpectedVersion, in.Action, in.Reason, reqctx.GetUserID(ctx), time.Now().UTC(), current.Epoch, dispositionHash(epoch), dispositionHash(job)}
			return dispositionWrite(s.path("interruption-requests", key), current)
		})
	})
	if e != nil {
		return out, failure(503, "ARCHIVE_CLOSURE_UNCONFIRMED", "结案确认未完成，请按原请求只读核对；保护保持")
	}
	return s.InterruptionRequest(ctx, key)
}
