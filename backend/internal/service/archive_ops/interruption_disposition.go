package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"golang.org/x/sys/unix"
)

const dispositionAction = "retain_and_close"

// InterruptionReview 仅证明本次读取；共享对象集合不等于完整采集清单。
type InterruptionReview struct {
	JobID          string    `json:"job_id"`
	PolicyID       string    `json:"policy_id"`
	PolicyRevision int64     `json:"policy_revision"`
	ReviewVersion  string    `json:"review_version"`
	ObjectCount    int       `json:"object_count"`
	Bytes          int64     `json:"bytes"`
	ReadOnly       bool      `json:"read_only"`
	ObservedAt     time.Time `json:"observed_at"`
}
type InterruptionProgress struct {
	Objects int   `json:"objects"`
	Bytes   int64 `json:"bytes"`
}
type RetainInterruptedInput struct {
	RequestID       string `json:"request_id"`
	ExpectedVersion string `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	Confirm         bool   `json:"confirm"`
}

// CaptureRetention 是不可完整恢复的保留记录；不授予下载、重传或删除资格。
type CaptureRetention struct {
	RequestID     string    `json:"request_id"`
	InputDigest   string    `json:"input_digest"`
	ReviewVersion string    `json:"review_version"`
	ObjectsDigest string    `json:"objects_digest"`
	ObjectCount   int       `json:"object_count"`
	Bytes         int64     `json:"bytes"`
	Reason        string    `json:"reason"`
	Operator      uint64    `json:"operator"`
	At            time.Time `json:"at"`
}
type InterruptionReceipt struct {
	EvidenceSource           string               `json:"evidence_source,omitempty"`
	RequestID                string               `json:"request_id"`
	JobID                    string               `json:"job_id"`
	State                    string               `json:"state"`
	InputDigest              string               `json:"input_digest"`
	ExpectedVersion          string               `json:"expected_version"`
	Action                   string               `json:"action"`
	Reason                   string               `json:"reason"`
	Operator                 uint64               `json:"operator"`
	CreatedAt                time.Time            `json:"created_at"`
	Result                   *CaptureRetention    `json:"result,omitempty"`
	Problem                  string               `json:"problem,omitempty"`
	CurrentRecordMatches     bool                 `json:"current_record_matches"`
	Closure                  *InterruptionClosure `json:"closure,omitempty"`
	CurrentProtectionMatches bool                 `json:"current_protection_matches"`
}
type dispositionProgressWriter func([]byte) (int, error)

func (w dispositionProgressWriter) Write(b []byte) (int, error) { return w(b) }

type retainedObject struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type dispositionRecord struct {
	InterruptionReceipt
	Epoch string `json:"epoch"`
}

func dispositionHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) dispositionCheck(ctx context.Context) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	if !auth.IsPlatformRootSession(ctx) {
		return failure(403, "ARCHIVE_USER_REQUIRED", "需要 Root 管理员用户会话")
	}
	return nil
}
func (s *Service) dispositionEpoch(ctx context.Context) (recovery.Epoch, error) {
	if s.recoveryAnchor == nil {
		return recovery.Epoch{}, failure(503, "ARCHIVE_RECOVERY_UNAVAILABLE", "原任务处置需要独立恢复证明")
	}
	return s.recoveryAnchor.Read(ctx)
}
func (s *Service) dispositionLocks(ctx context.Context, j jobRecord) (func(), error) {
	releases := []func(){}
	release := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	names := []string{"gc"}
	if j.MigratedFrom != "" {
		names = append(names, "migration-"+j.ID)
	}
	names = append(names, j.PolicyID)
	for _, name := range names {
		r, e := s.lock(ctx, name)
		if e != nil {
			release()
			return nil, failure(409, "ARCHIVE_BUSY", "执行或回收尚未排空，请保留原任务稍后核对")
		}
		releases = append(releases, r)
	}
	return release, nil
}
func (s *Service) dispositionPolicy(ctx context.Context, j jobRecord) (Policy, error) {
	if e := s.requireJobCommandsDrained(j); e != nil {
		return Policy{}, e
	}
	if j.Status != "interrupted" || !j.InterruptionPending || j.Imported || j.Recovered || j.CaptureRetention != nil || j.MigratedFrom != "" || j.OneTime && j.PolicyID != "" {
		return Policy{}, failure(409, "ARCHIVE_CAPTURE_NOT_INTERRUPTED", "仅处理已中断且未完成的原采集；完整快照请重传原任务")
	}
	p := oneTimeMediaPolicy()
	p.SourceID = j.SourceID
	if !j.OneTime {
		var e error
		p, e = s.policy(ctx, j.PolicyID)
		if e != nil {
			return p, e
		}
	}
	if p.Enabled || p.RemoteCleanupEnabled {
		return p, failure(409, "ARCHIVE_PAUSE_REQUIRED", "请先明确暂停原策略的归档和远端清理")
	}
	if p.SourceID != j.SourceID {
		return p, failure(409, "ARCHIVE_SOURCE_CHANGED", "原来源与当前策略不符")
	}
	return p, nil
}

// scanDispositionObjects 不跟随软链或硬链；只接受规范 CAS 名称，读取量／数量／时间均有界。
func (s *Service) scanDispositionObjects(ctx context.Context, progress func(InterruptionProgress) error) ([]retainedObject, string, error) {
	fd, e := unix.Open(filepath.Join(s.cfg.Directory, "objects"), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, "", e
	}
	f := os.NewFile(uintptr(fd), "cas-root")
	defer f.Close()
	var root unix.Stat_t
	if e = unix.Fstat(fd, &root); e != nil {
		return nil, "", e
	}
	dirs, e := f.ReadDir(10002)
	if e != nil && e != io.EOF {
		return nil, "", e
	}
	if len(dirs) > 256 {
		return nil, "", failure(409, "ARCHIVE_REVIEW_LIMIT", "对象分区超限")
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	if progress != nil {
		if e := progress(InterruptionProgress{}); e != nil {
			return nil, "", e
		}
	}
	refs := []retainedObject{}
	identity := []any{root.Dev, root.Ino}
	var total int64
	for _, d := range dirs {
		if len(d.Name()) != 2 || !shaPattern.MatchString(d.Name()+strings.Repeat("0", 62)) || !d.IsDir() {
			return nil, "", failure(409, "ARCHIVE_OBJECT_UNSAFE", "对象目录含未知项，停止处置")
		}
		shard, e := unix.Openat(fd, d.Name(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return nil, "", e
		}
		sf := os.NewFile(uintptr(shard), "cas-shard")
		entries, e := sf.ReadDir(10001)
		if e != nil && e != io.EOF {
			sf.Close()
			return nil, "", e
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if e = ctx.Err(); e != nil {
				sf.Close()
				return nil, "", e
			}
			name := entry.Name()
			if !shaPattern.MatchString(name) || !strings.HasPrefix(name, d.Name()) || len(refs) >= 10000 {
				sf.Close()
				return nil, "", failure(409, "ARCHIVE_OBJECT_UNSAFE", "对象身份未知或超过 10000 个核验上限")
			}
			ofd, err := unix.Openat(shard, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
			if err != nil {
				sf.Close()
				return nil, "", err
			}
			of := os.NewFile(uintptr(ofd), "cas-object")
			var before, after unix.Stat_t
			err = unix.Fstat(ofd, &before)
			if err == nil && (before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 0 || before.Size > (64<<30)-total) {
				err = failure(409, "ARCHIVE_OBJECT_UNSAFE", "对象类型、链接或核验总量不安全（上限 64 GiB）")
			}
			h := sha256.New()
			var n int64
			if err == nil {
				var seen int64
				last := time.Now()
				writer := io.Writer(h)
				if progress != nil {
					writer = io.MultiWriter(h, dispositionProgressWriter(func(b []byte) (int, error) {
						seen += int64(len(b))
						if time.Since(last) > 500*time.Millisecond {
							last = time.Now()
							if e := progress(InterruptionProgress{len(refs), total + seen}); e != nil {
								return 0, e
							}
						}
						return len(b), nil
					}))
				}
				n, err = io.Copy(writer, &contextReader{ctx, io.LimitReader(of, before.Size+1)})
			}
			if err == nil {
				err = unix.Fstat(ofd, &after)
			}
			of.Close()
			if err != nil {
				sf.Close()
				return nil, "", err
			}
			if n != before.Size || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || before.Mode != after.Mode || before.Nlink != after.Nlink || before.Mtim != after.Mtim || before.Ctim != after.Ctim || hex.EncodeToString(h.Sum(nil)) != name {
				sf.Close()
				return nil, "", failure(409, "ARCHIVE_OBJECT_CHANGED", "对象内容变化或校验不符")
			}
			total += n
			refs = append(refs, retainedObject{name, n})
			identity = append(identity, []any{name, before.Dev, before.Ino, before.Size, before.Mtim})
			if progress != nil {
				if e = progress(InterruptionProgress{len(refs), total}); e != nil {
					sf.Close()
					return nil, "", e
				}
			}
		}
		sf.Close()
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].SHA256 < refs[j].SHA256 })
	return refs, dispositionHash(identity), nil
}

func (s *Service) reviewInterruptedLocked(ctx context.Context, id string, progress func(InterruptionProgress) error) (InterruptionReview, []retainedObject, jobRecord, Policy, recovery.Epoch, error) {
	var out InterruptionReview
	j, e := s.readJob(id)
	if e != nil {
		return out, nil, j, Policy{}, recovery.Epoch{}, e
	}
	p, e := s.dispositionPolicy(ctx, j)
	if e != nil {
		return out, nil, j, p, recovery.Epoch{}, e
	}
	epoch, e := s.dispositionEpoch(ctx)
	if e != nil {
		return out, nil, j, p, epoch, e
	}
	source, e := s.PolicySourceFingerprint(j.SourceID)
	if e != nil {
		return out, nil, j, p, epoch, e
	}
	refs, identity, e := s.scanDispositionObjects(ctx, progress)
	if e != nil {
		return out, nil, j, p, epoch, e
	}
	version := dispositionHash([]any{"archive-retain/v1", s.cfg.Instance, s.hostIdentity, epoch, p, j, source, refs, identity})
	var size int64
	for _, r := range refs {
		size += r.Size
	}
	out = InterruptionReview{j.ID, j.PolicyID, p.Revision, version, len(refs), size, true, time.Now().UTC()}
	return out, refs, j, p, epoch, nil
}
func (s *Service) ReviewInterruptedCapture(ctx context.Context, id string, progress func(InterruptionProgress) error) (InterruptionReview, error) {
	var out InterruptionReview
	if e := s.dispositionCheck(ctx); e != nil {
		return out, e
	}
	if !validID(id) {
		return out, failure(400, "ARCHIVE_ID_INVALID", "任务编号必须是 UUID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	j, e := s.readJob(id)
	if e != nil {
		return out, e
	}
	release, e := s.dispositionLocks(run, j)
	if e != nil {
		return out, e
	}
	defer release()
	out, _, _, p, epoch, e := s.reviewInterruptedLocked(run, id, progress)
	if e != nil {
		return out, e
	}
	e = s.recoveryAnchor.WithEpoch(run, func(current recovery.Epoch) error {
		if current != epoch {
			return failure(409, "ARCHIVE_REVIEW_STALE", "恢复代次变化，请重新核验")
		}
		return s.guardPolicyAction(run, p, "interruption-retain", func(context.Context) error { return nil })
	})
	return out, e
}
func dispositionWrite(path string, v any) error {
	info, e := os.Lstat(filepath.Dir(path))
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return failure(503, "ARCHIVE_DISPOSITION_STORE_UNAVAILABLE", "处置目录身份或权限不可用")
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if e = atomicWrite(path, b); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func (s *Service) readDisposition(key string) (dispositionRecord, error) {
	var r dispositionRecord
	if !validID(key) {
		return r, failure(400, "ARCHIVE_REQUEST_ID_INVALID", "请求编号必须是 UUID")
	}
	e := readJSON(s.path("interruption-requests", key), &r)
	if e != nil {
		return r, e
	}
	if r.RequestID != key || !validID(r.JobID) || r.Action != dispositionAction || !shaPattern.MatchString(r.InputDigest) || !shaPattern.MatchString(r.ExpectedVersion) || r.InputDigest != dispositionHash([]any{r.JobID, RetainInterruptedInput{r.RequestID, r.ExpectedVersion, r.Action, r.Reason, true}}) || r.Operator == 0 || r.CreatedAt.IsZero() || !shaPattern.MatchString(r.Epoch) || r.State != "registered" && r.State != "retained" && r.State != "rejected" && r.State != "closed_unknown" {
		return r, failure(503, "ARCHIVE_DISPOSITION_CORRUPT", "原请求台账损坏，停止处置")
	}
	if r.State == "retained" && (r.Result == nil || r.Result.RequestID != r.RequestID || r.Result.InputDigest != r.InputDigest || r.Result.ReviewVersion != r.ExpectedVersion || r.Result.Reason != r.Reason || r.Result.Operator != r.Operator) {
		return r, failure(503, "ARCHIVE_DISPOSITION_CORRUPT", "原请求终态回执不一致")
	}
	if e := validateInterruptionClosure(r); e != nil {
		return r, e
	}
	return r, nil
}
func (s *Service) InterruptionRequest(ctx context.Context, key string) (InterruptionReceipt, error) {
	if e := s.dispositionCheck(ctx); e != nil {
		return InterruptionReceipt{}, e
	}
	r, e := s.readDisposition(key)
	if e != nil {
		if dto.StatusCode(e) == 404 {
			return s.findDispositionReceipt(ctx, key)
		}
		return InterruptionReceipt{}, e
	}
	out := r.InterruptionReceipt
	out.EvidenceSource = "request_ledger"
	out.CurrentRecordMatches = false
	out.CurrentProtectionMatches = false
	j, e := s.readJob(r.JobID)
	if e != nil {
		if dto.StatusCode(e) != 404 {
			return out, e
		}
		if out.State != "closed_unknown" {
			out.State = "unconfirmed"
		}
		out.CurrentRecordMatches = false
		return out, nil
	}
	if r.State == "closed_unknown" {
		// 历史结论不能授予恢复后的当前保护证明，也不能覆写当前任务。
		epoch, err := s.dispositionEpoch(ctx)
		if err == nil && r.Closure.JobVersion == dispositionHash(j) && r.Closure.CurrentEpoch == dispositionHash(epoch) && j.Status == "interrupted" && j.InterruptionPending && s.requireJobCommandsDrained(j) == nil {
			out.CurrentProtectionMatches = true
		}
		return out, nil
	}
	if c := j.CaptureRetention; c != nil && c.RequestID == key {
		if c.InputDigest != r.InputDigest || c.ReviewVersion != r.ExpectedVersion || c.Reason != r.Reason || c.Operator != r.Operator {
			return out, failure(503, "ARCHIVE_DISPOSITION_CONFLICT", "原请求回执与任务冲突")
		}
		out.State = "retained"
		out.Result = c
		out.CurrentRecordMatches = true
		return out, nil
	}
	if out.State == "registered" {
		out.State = "unconfirmed"
	}
	return out, nil
}

func (s *Service) RetainInterruptedCapture(ctx context.Context, id string, in RetainInterruptedInput) (InterruptionReceipt, error) {
	var out InterruptionReceipt
	if e := s.dispositionCheck(ctx); e != nil {
		return out, e
	}
	if !validID(id) || !validID(in.RequestID) || !shaPattern.MatchString(in.ExpectedVersion) || in.Action != dispositionAction || !in.Confirm || in.Reason != strings.TrimSpace(in.Reason) || !utf8.ValidString(in.Reason) || len([]rune(in.Reason)) < 5 || len(in.Reason) > 1024 || strings.ContainsAny(in.Reason, "\x00\r") {
		return out, failure(400, "ARCHIVE_DISPOSITION_INPUT_INVALID", "请核对原任务、版本，填写 5 至 1024 字节原因并明确确认")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	releaseRequest, e := s.lock(run, "interruption-"+in.RequestID)
	if e != nil {
		return out, failure(409, "ARCHIVE_BUSY", "原请求仍在处理，请只读核对")
	}
	defer releaseRequest()
	digest := dispositionHash([]any{id, in})
	old, e := s.readDisposition(in.RequestID)
	if e == nil {
		if old.JobID != id || old.InputDigest != digest {
			return out, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "原请求编号不能更换任务或输入")
		}
		return s.InterruptionRequest(ctx, in.RequestID)
	}
	if dto.StatusCode(e) != 404 {
		return out, e
	}
	history, lookup := s.findDispositionReceipt(run, in.RequestID)
	if lookup == nil {
		if history.JobID != id || history.InputDigest != digest {
			return out, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "原任务已保留此请求，编号不能更换输入")
		}
		return history, nil
	}
	if dto.StatusCode(lookup) != 404 {
		return out, lookup
	}
	registryUnlock, e := s.lock(run, "interruption-registry")
	if e != nil {
		return out, failure(409, "ARCHIVE_BUSY", "原请求登记正在处理")
	}
	defer func() {
		if registryUnlock != nil {
			registryUnlock()
		}
	}()
	ids, e := s.records("interruption-requests")
	if e != nil {
		return out, e
	}
	if len(ids) >= 10000 {
		return out, failure(409, "ARCHIVE_DISPOSITION_LIMIT", "处置回执已达上限，停止新处置")
	}
	for _, key := range ids {
		existing, err := s.readDisposition(key)
		if err != nil {
			return out, err
		}
		if existing.Closure != nil && existing.Closure.RequestID == in.RequestID {
			return out, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "请求编号已用于人工结案，不得复用")
		}
	}
	epoch, e := s.dispositionEpoch(run)
	if e != nil {
		return out, e
	}
	r := dispositionRecord{InterruptionReceipt: InterruptionReceipt{RequestID: in.RequestID, JobID: id, State: "registered", InputDigest: digest, ExpectedVersion: in.ExpectedVersion, Action: in.Action, Reason: in.Reason, Operator: reqctx.GetUserID(ctx), CreatedAt: time.Now().UTC()}, Epoch: dispositionHash(epoch)}
	if e = dispositionWrite(s.path("interruption-requests", in.RequestID), r); e != nil {
		return out, e
	}
	registryUnlock()
	registryUnlock = nil
	apply := func() error {
		j, e := s.readJob(id)
		if e != nil {
			return e
		}
		release, e := s.dispositionLocks(run, j)
		if e != nil {
			return e
		}
		defer release()
		review, refs, current, p, observed, e := s.reviewInterruptedLocked(run, id, nil)
		if e != nil {
			return e
		}
		if observed != epoch || review.ReviewVersion != in.ExpectedVersion {
			return failure(409, "ARCHIVE_REVIEW_STALE", "任务、策略、来源、内容或恢复代次已变化，请重新核验")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.recoveryAnchor.WithEpoch(run, func(now recovery.Epoch) error {
			if now != epoch {
				return failure(409, "ARCHIVE_REVIEW_STALE", "恢复代次变化，请重新核验")
			}
			return s.guardPolicyAction(run, p, "interruption-retain", func(context.Context) error {
				c := &CaptureRetention{in.RequestID, digest, in.ExpectedVersion, dispositionHash(refs), len(refs), review.Bytes, in.Reason, r.Operator, time.Now().UTC()}
				current.CaptureRetention = c
				current.RetainedObjects = refs
				current.Status = "retained"
				current.Phase = "closed_retained"
				current.Protected = true
				current.InterruptionPending = false
				current.UpdatedAt = time.Now().UTC()
				current.ExecutionState = ""
				return dispositionWrite(s.path("jobs", id), current)
			})
		})
	}
	if e = apply(); e != nil {
		// 文件提交与控制 DB 的确认不是跨存储原子事务；先只读核对已存在的终态。
		receipt, lookup := s.InterruptionRequest(ctx, in.RequestID)
		if lookup == nil && receipt.State == "retained" {
			return out, failure(503, "ARCHIVE_DISPOSITION_UNCONFIRMED", "提交确认丢失，请只读核对原请求")
		}
		r.Problem = "ARCHIVE_DISPOSITION_UNCONFIRMED"
		if dto.StatusCode(e) > 0 && dto.StatusCode(e) < 500 {
			r.State = "rejected"
			r.Problem = "ARCHIVE_DISPOSITION_REJECTED"
		}
		_ = dispositionWrite(s.path("interruption-requests", in.RequestID), r)
		return out, e
	}
	receipt, e := s.InterruptionRequest(ctx, in.RequestID)
	if e != nil {
		return out, e
	}
	if receipt.State != "retained" || !receipt.CurrentRecordMatches {
		return out, failure(503, "ARCHIVE_DISPOSITION_UNCONFIRMED", "原任务发生变化，请只读核对原请求")
	}
	r.InterruptionReceipt = receipt
	if e = dispositionWrite(s.path("interruption-requests", in.RequestID), r); e != nil {
		return out, failure(503, "ARCHIVE_DISPOSITION_UNCONFIRMED", "已提交但最终回执写入未确认，请核对原请求")
	}
	return receipt, nil
}

// retainedReferences 防止记录损坏／恢复缺口变成回收许可；只读校验元数据，不修改保护。
func retainedReferences(j jobRecord) ([]retainedObject, error) {
	if j.RetainedObjects == nil {
		j.RetainedObjects = []retainedObject{}
	}
	c := j.CaptureRetention
	if c == nil {
		if len(j.RetainedObjects) > 0 || j.Status == "retained" {
			return nil, errors.New("retained capture metadata missing")
		}
		return nil, nil
	}
	if c.InputDigest != dispositionHash([]any{j.ID, RetainInterruptedInput{c.RequestID, c.ReviewVersion, dispositionAction, c.Reason, true}}) || c.ObjectCount < 0 || c.ObjectCount > 10000 || c.Bytes < 0 || c.Bytes > 64<<30 || !shaPattern.MatchString(c.InputDigest) || !shaPattern.MatchString(c.ReviewVersion) || c.Operator == 0 || c.At.IsZero() || j.Status != "retained" || !j.Protected || j.InterruptionPending || !validID(c.RequestID) || len(j.RetainedObjects) != c.ObjectCount || dispositionHash(j.RetainedObjects) != c.ObjectsDigest {
		return nil, errors.New("retained capture evidence inconsistent")
	}
	var size int64
	last := ""
	for _, r := range j.RetainedObjects {
		if !shaPattern.MatchString(r.SHA256) || r.SHA256 <= last || r.Size < 0 {
			return nil, errors.New("retained capture reference invalid")
		}
		last = r.SHA256
		size += r.Size
	}
	if size != c.Bytes {
		return nil, errors.New("retained capture size inconsistent")
	}
	return j.RetainedObjects, nil
}

// findDispositionReceipt 在独立台账缺失时只读核对原子提交于任务中的终态；缺失仍不证明未执行。
func (s *Service) findDispositionReceipt(ctx context.Context, key string) (InterruptionReceipt, error) {
	var out *InterruptionReceipt
	ids, e := s.records("jobs")
	if e != nil {
		return InterruptionReceipt{}, e
	}
	if len(ids) > 10000 {
		return InterruptionReceipt{}, failure(503, "ARCHIVE_DISPOSITION_LIMIT", "原任务目录超限")
	}
	for _, id := range ids {
		if e := ctx.Err(); e != nil {
			return InterruptionReceipt{}, e
		}
		j, e := s.readJob(id)
		if e != nil {
			return InterruptionReceipt{}, e
		}
		c := j.CaptureRetention
		if c == nil || c.RequestID != key {
			continue
		}
		if out != nil {
			return InterruptionReceipt{}, failure(503, "ARCHIVE_DISPOSITION_CONFLICT", "原请求匹配多个任务")
		}
		out = &InterruptionReceipt{EvidenceSource: "job_record", RequestID: key, JobID: j.ID, State: "retained", InputDigest: c.InputDigest, ExpectedVersion: c.ReviewVersion, Action: dispositionAction, Reason: c.Reason, Operator: c.Operator, CreatedAt: c.At, Result: c, CurrentRecordMatches: true}
	}
	if out == nil {
		return InterruptionReceipt{}, failure(404, "ARCHIVE_DISPOSITION_UNCONFIRMED", "未核对到原请求；这不证明此前未执行")
	}
	return *out, nil
}
