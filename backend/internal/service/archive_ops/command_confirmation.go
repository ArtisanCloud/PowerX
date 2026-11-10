package archive_ops

import (
	"context"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const confirmCommandDrainAction = "confirm_local_command_drain"

// CommandDrainReview 只读核对原命令和 OS 身份，不能从缺少 PID 单独推断已排空。
type CommandDrainReview struct {
	Subject       string                  `json:"subject,omitempty"`
	TargetID      string                  `json:"target_id,omitempty"`
	Probe         *ProbeScopeReview       `json:"probe,omitempty"`
	RecordID      string                  `json:"record_id"`
	JobID         string                  `json:"job_id"`
	ExecutionID   string                  `json:"execution_id"`
	CommandID     string                  `json:"command_id"`
	Kind          string                  `json:"kind"`
	State         string                  `json:"state"`
	ReviewVersion string                  `json:"review_version"`
	Eligible      bool                    `json:"eligible"`
	Problem       string                  `json:"problem,omitempty"`
	BootMatches   bool                    `json:"boot_matches"`
	Leader        string                  `json:"leader"`
	Group         string                  `json:"group"`
	Identity      *CommandProcessIdentity `json:"identity,omitempty"`
	ReadOnly      bool                    `json:"read_only"`
	ObservedAt    time.Time               `json:"observed_at"`
}

// ConfirmCommandDrainInput 固定动作，不接受 PID、命令、路径或进程信号。
type ConfirmCommandDrainInput struct {
	RequestID       string `json:"request_id"`
	ExpectedVersion string `json:"expected_version"`
	ExecutionID     string `json:"execution_id"`
	CommandID       string `json:"command_id"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	Confirm         bool   `json:"confirm"`
	TargetID        string `json:"target_id,omitempty"`
}

// CommandDrainReceipt 同一不可变文件既是终态回执，也是精确匹配原命令的本机证明。
type CommandDrainReceipt struct {
	TargetID             string            `json:"target_id,omitempty"`
	Probe                *ProbeScopeReview `json:"probe,omitempty"`
	RequestID            string            `json:"request_id"`
	RecordID             string            `json:"record_id"`
	JobID                string            `json:"job_id"`
	ExecutionID          string            `json:"execution_id"`
	CommandID            string            `json:"command_id"`
	State                string            `json:"state"`
	InputDigest          string            `json:"input_digest"`
	ExpectedVersion      string            `json:"expected_version"`
	Action               string            `json:"action"`
	Reason               string            `json:"reason"`
	Operator             uint64            `json:"operator"`
	At                   time.Time         `json:"at"`
	CurrentRecordMatches bool              `json:"current_record_matches"`
	Problem              string            `json:"problem,omitempty"`
}
type commandConfirmation struct {
	CommandDrainReceipt
	CommandDigest string               `json:"command_digest"`
	JobVersion    string               `json:"job_version"`
	Epoch         string               `json:"epoch"`
	Host          string               `json:"host"`
	Boot          string               `json:"boot"`
	Namespace     string               `json:"namespace"`
	Observation   commandOSObservation `json:"observation"`
}

func commandConfirmationDigest(id string, in ConfirmCommandDrainInput) string {
	return dispositionHash([]any{id, in})
}
func (s *Service) readCommandConfirmation(command string) (commandConfirmation, error) {
	var r commandConfirmation
	if !validID(command) {
		return r, failure(400, "ARCHIVE_COMMAND_ID_INVALID", "命令编号无效")
	}
	if e := readJSON(s.path("command-confirmations", command), &r); e != nil {
		return r, e
	}
	in := ConfirmCommandDrainInput{r.RequestID, r.ExpectedVersion, r.ExecutionID, r.CommandID, r.Action, r.Reason, true, r.TargetID}
	if r.CommandID != command || !validID(r.RequestID) || !validID(r.RecordID) || !validCommandReceiptSubject(r.CommandDrainReceipt) || !validID(r.ExecutionID) || r.State != "confirmed" || r.Action != confirmCommandDrainAction || !closureReasonValid(r.Reason) || r.Operator == 0 || r.At.IsZero() || !shaPattern.MatchString(r.ExpectedVersion) || r.InputDigest != commandConfirmationDigest(r.RecordID, in) || !shaPattern.MatchString(r.CommandDigest) || !shaPattern.MatchString(r.JobVersion) || !shaPattern.MatchString(r.Epoch) || !shaPattern.MatchString(r.Host) || !shaPattern.MatchString(r.Boot) || !shaPattern.MatchString(r.Namespace) || !r.Observation.BootMatches || r.Observation.Leader != "absent" || r.Observation.Group != "absent" {
		return r, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_INVALID", "人工排空证明损坏，继续保留保护")
	}
	if r.TargetID != "" {
		if r.Probe == nil || r.Probe.TargetID != r.TargetID || r.Probe.Digest != r.JobVersion || r.Probe.OriginalRevision < 0 || r.Probe.CurrentRevision < 0 || (r.Probe.TargetState != "unchanged" && r.Probe.TargetState != "changed" && r.Probe.TargetState != "deleted") || len(r.Probe.Policies) > 128 {
			return r, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_INVALID", "原目标范围证明损坏，继续保护")
		}
		for _, p := range r.Probe.Policies {
			if !validID(p.ID) || !p.Paused || p.Revision < 0 {
				return r, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_INVALID", "引用策略证明损坏")
			}
		}
	} else if r.Probe != nil {
		return r, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_INVALID", "任务证明不能携带探测范围")
	}
	return r, nil
}
func (s *Service) commandConfirmationMatches(r commandRecord, c commandConfirmation) bool {
	if (r.Protocol != 2 && r.Protocol != 3) || r.State != "running" || r.Identity == nil || !validCommandIdentity(r.Identity) || r.ID != c.RecordID || r.JobID != c.JobID || r.ExecutionID != c.ExecutionID || r.CommandID != c.CommandID || dispositionHash(r) != c.CommandDigest || r.Host != s.hostIdentity || r.Host != c.Host || r.Identity.Boot != c.Boot || r.Identity.Namespace != c.Namespace {
		return false
	}
	boot, ns, e := commandBootIdentity()
	if e != nil || boot != c.Boot || ns != c.Namespace {
		return false
	}
	epoch, e := s.dispositionEpoch(context.Background())
	if e != nil || dispositionHash(epoch) != c.Epoch {
		return false
	}
	if c.TargetID != "" {
		return r.Protocol == 3 && r.JobID == "" && validProbeBinding(r.Probe, r.ID) && r.Probe.TargetID == c.TargetID
	}
	j, e := s.readJob(c.JobID)
	return e == nil && (j.CommandProtocol == 1 || j.CommandProtocol == 2) && j.ExecutionID == c.ExecutionID && !j.Imported && !j.Recovered
}
func (s *Service) commandConfirmationCurrent(r commandRecord) bool {
	if !validID(r.CommandID) {
		return false
	}
	c, e := s.readCommandConfirmation(r.CommandID)
	return e == nil && s.commandConfirmationMatches(r, c)
}
func (s *Service) commandConfirmationReceipt(c commandConfirmation) CommandDrainReceipt {
	out := c.CommandDrainReceipt
	if out.Probe != nil {
		view := *out.Probe
		view.Snapshot = ProbeControlSnapshot{}
		view.Fingerprint = ""
		out.Probe = &view
	}
	out.CurrentRecordMatches = false
	r, e := s.readCommandRecord(c.RecordID)
	if e == nil {
		out.CurrentRecordMatches = s.commandConfirmationMatches(r, c)
	}
	return out
}
func (s *Service) findCommandConfirmation(request string) (CommandDrainReceipt, error) {
	var rejected CommandDrainReceipt
	rejectionErr := readJSON(s.path("command-confirmation-rejections", request), &rejected)
	if rejectionErr == nil {
		in := ConfirmCommandDrainInput{rejected.RequestID, rejected.ExpectedVersion, rejected.ExecutionID, rejected.CommandID, rejected.Action, rejected.Reason, true, rejected.TargetID}
		if rejected.RequestID != request || !validID(rejected.RecordID) || !validCommandReceiptSubject(rejected) || !validID(rejected.ExecutionID) || !validID(rejected.CommandID) || rejected.State != "rejected" || rejected.Action != confirmCommandDrainAction || !shaPattern.MatchString(rejected.ExpectedVersion) || rejected.InputDigest != commandConfirmationDigest(rejected.RecordID, in) || !closureReasonValid(rejected.Reason) || rejected.Operator == 0 || rejected.At.IsZero() {
			return CommandDrainReceipt{}, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_INVALID", "原请求拒绝回执损坏")
		}
		rejected.CurrentRecordMatches = false
	} else if dto.StatusCode(rejectionErr) != 404 {
		return CommandDrainReceipt{}, rejectionErr
	}
	ids, e := s.records("command-confirmations")
	if e != nil {
		return CommandDrainReceipt{}, e
	}
	if len(ids) > 10000 {
		return CommandDrainReceipt{}, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_LIMIT", "人工排空回执超限")
	}
	var out *CommandDrainReceipt
	for _, id := range ids {
		c, e := s.readCommandConfirmation(id)
		if e != nil {
			return CommandDrainReceipt{}, e
		}
		if c.RequestID != request {
			continue
		}
		if out != nil {
			return CommandDrainReceipt{}, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_INVALID", "原请求存在冲突回执")
		}
		receipt := s.commandConfirmationReceipt(c)
		out = &receipt
	}
	if rejectionErr == nil {
		if out != nil {
			return CommandDrainReceipt{}, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_INVALID", "原请求终态冲突")
		}
		return rejected, nil
	}
	if out == nil {
		return CommandDrainReceipt{}, failure(404, "ARCHIVE_COMMAND_CONFIRMATION_UNCONFIRMED", "尚未核对到原请求，这不证明之前未提交")
	}
	return *out, nil
}

// CommandConfirmationRequest 不写文件或补做动作；历史回执与当前资格分开。
func (s *Service) CommandConfirmationRequest(ctx context.Context, request string) (CommandDrainReceipt, error) {
	if e := s.dispositionCheck(ctx); e != nil {
		return CommandDrainReceipt{}, e
	}
	if !validID(request) {
		return CommandDrainReceipt{}, failure(400, "ARCHIVE_REQUEST_ID_INVALID", "请求编号必须是 UUID")
	}
	return s.findCommandConfirmation(request)
}
func (s *Service) commandReviewPolicy(ctx context.Context, r commandRecord) (jobRecord, Policy, error) {
	var j jobRecord
	var p Policy
	if r.JobID == "" {
		return j, p, failure(409, "ARCHIVE_COMMAND_SUBJECT_UNSUPPORTED", "目标探测记录暂不提供排空确认，继续保护")
	}
	j, e := s.readJob(r.JobID)
	if e != nil {
		return j, p, e
	}
	if j.Imported || j.Recovered || j.ExecutionID != r.ExecutionID || j.CommandProtocol != 1 && j.CommandProtocol != 2 {
		return j, p, failure(409, "ARCHIVE_COMMAND_CONTEXT_CHANGED", "原任务执行身份不符，继续保护")
	}
	p = oneTimeMediaPolicy()
	p.SourceID = j.SourceID
	if !j.OneTime {
		p, e = s.policy(ctx, j.PolicyID)
		if e != nil {
			return j, p, e
		}
	}
	if p.SourceID != j.SourceID || p.Enabled || p.RemoteCleanupEnabled {
		return j, p, failure(409, "ARCHIVE_PAUSE_REQUIRED", "请先明确暂停原策略的归档和远端清理")
	}
	return j, p, nil
}
func (s *Service) commandReviewLocked(ctx context.Context, id string) (CommandDrainReview, commandRecord, jobRecord, Policy, recovery.Epoch, error) {
	var out CommandDrainReview
	r, e := s.readCommandRecord(id)
	if e != nil {
		return out, r, jobRecord{}, Policy{}, recovery.Epoch{}, e
	}
	if r.JobID == "" {
		return s.probeCommandReviewLocked(ctx, r)
	}
	j, p, e := s.commandReviewPolicy(ctx, r)
	if e != nil {
		return out, r, j, p, recovery.Epoch{}, e
	}
	epoch, e := s.dispositionEpoch(ctx)
	if e != nil {
		return out, r, j, p, epoch, e
	}
	facts := observeRecordedCommand(r)
	problem := ""
	switch {
	case r.Host != s.hostIdentity:
		problem = "host_changed"
	case r.State != "running":
		problem = "command_not_unconfirmed"
	case r.Protocol != 2 || !validCommandIdentity(r.Identity):
		problem = "identity_missing"
	case !facts.BootMatches:
		problem = "boot_changed"
	case facts.Leader == "identity_mismatch":
		problem = "identity_mismatch"
	case facts.Leader != "absent" || facts.Group != "absent":
		problem = "group_not_drained"
	}
	out = CommandDrainReview{Subject: "job", RecordID: id, JobID: j.ID, ExecutionID: r.ExecutionID, CommandID: r.CommandID, Kind: r.Kind, State: r.State, ReviewVersion: dispositionHash([]any{"command-confirmation/v1", s.cfg.Instance, s.hostIdentity, r, j, p, epoch, facts}), Eligible: problem == "", Problem: problem, BootMatches: facts.BootMatches, Leader: facts.Leader, Group: facts.Group, Identity: r.Identity, ReadOnly: true, ObservedAt: time.Now().UTC()}
	return out, r, j, p, epoch, nil
}
func (s *Service) commandReviewLocks(ctx context.Context, id string) (func(), error) {
	unlock, e := s.lock(ctx, "command-"+id)
	if e != nil {
		return nil, failure(409, "ARCHIVE_COMMANDS_ACTIVE", "本机命令仍由执行器持有")
	}
	r, e := s.readCommandRecord(id)
	if e != nil {
		unlock()
		return nil, e
	}
	if r.JobID == "" {
		releases := []func(){unlock}
		done := func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		}
		names := []string{"gc", "settings"}
		if r.Probe != nil {
			names = append([]string{"probe-target-" + r.Probe.TargetID}, names...)
		}
		for _, name := range names {
			release, e := s.lock(ctx, name)
			if e != nil {
				done()
				return nil, failure(409, "ARCHIVE_BUSY", "探测、配置或回收仍在处理")
			}
			releases = append(releases, release)
		}
		return done, nil
	}
	j, e := s.readJob(r.JobID)
	if e != nil {
		unlock()
		return nil, e
	}
	release, e := s.dispositionLocks(ctx, j)
	if e != nil {
		unlock()
		return nil, e
	}
	settings, e := s.lock(ctx, "settings")
	if e != nil {
		release()
		unlock()
		return nil, failure(409, "ARCHIVE_BUSY", "归档配置正在修改")
	}
	return func() { settings(); release(); unlock() }, nil
}

// ReviewCommandDrain 只读检查，原锁可取得仍不代表进程组排空。
func (s *Service) ReviewCommandDrain(ctx context.Context, id string) (CommandDrainReview, error) {
	var out CommandDrainReview
	if e := s.dispositionCheck(ctx); e != nil {
		return out, e
	}
	if !validID(id) {
		return out, failure(400, "ARCHIVE_ID_INVALID", "原记录编号必须是 UUID")
	}
	release, e := s.commandReviewLocks(ctx, id)
	if e != nil {
		return out, e
	}
	defer release()
	s.mu.Lock()
	defer s.mu.Unlock()
	out, r, _, p, epoch, e := s.commandReviewLocked(ctx, id)
	if e != nil {
		return out, e
	}
	e = s.recoveryAnchor.WithEpoch(ctx, func(now recovery.Epoch) error {
		if now != epoch {
			return failure(409, "ARCHIVE_REVIEW_STALE", "恢复代次变化，请重新核对")
		}
		return s.guardCommandReview(ctx, r, p, out.Probe, func(context.Context) error { return nil })
	})
	return out, e
}

// ConfirmCommandDrain 唯一写入是一条不可变终态；不改原任务、原命令或任何数据对象。
func (s *Service) ConfirmCommandDrain(ctx context.Context, id string, in ConfirmCommandDrainInput) (out CommandDrainReceipt, resultErr error) {
	if e := s.dispositionCheck(ctx); e != nil {
		return out, e
	}
	if in.TargetID != "" && (!validID(in.TargetID) || id != commandProbeID(in.TargetID)) {
		return out, failure(400, "ARCHIVE_PROBE_SCOPE_INVALID", "原目标范围不符")
	}
	if !validID(id) || !validID(in.RequestID) || !validID(in.CommandID) || !validID(in.ExecutionID) || !shaPattern.MatchString(in.ExpectedVersion) || in.Action != confirmCommandDrainAction || !in.Confirm || !closureReasonValid(in.Reason) {
		return out, failure(400, "ARCHIVE_COMMAND_CONFIRMATION_INPUT_INVALID", "请核对原命令、版本、原因并明确确认")
	}
	requestUnlock, e := s.lock(ctx, "command-request-"+in.RequestID)
	if e != nil {
		return out, failure(409, "ARCHIVE_BUSY", "原确认请求仍在处理，请只读核对")
	}
	defer requestUnlock()
	digest := commandConfirmationDigest(id, in)
	history, e := s.findCommandConfirmation(in.RequestID)
	if e == nil {
		if history.RecordID != id || history.InputDigest != digest {
			return out, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "原请求不能更换输入")
		}
		return history, nil
	}
	if dto.StatusCode(e) != 404 {
		return out, e
	}
	attempted := false
	defer func() {
		if resultErr != nil && !attempted {
			// 只有尚未进入证明写入的本次调用能确定未接纳。写入开始后的任何错误仍保持未知。
			_ = s.recordCommandRejection(ctx, id, in, digest, dto.CodeOf(resultErr))
		}
	}()
	release, e := s.commandReviewLocks(ctx, id)
	if e != nil {
		return out, e
	}
	defer release()
	registry, e := s.lock(ctx, "command-confirmation-registry")
	if e != nil {
		return out, ErrBusy
	}
	defer registry()
	history, e = s.findCommandConfirmation(in.RequestID)
	if e == nil {
		if history.RecordID != id || history.InputDigest != digest {
			return out, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "原请求不能更换输入")
		}
		return history, nil
	}
	if dto.StatusCode(e) != 404 {
		return out, e
	}
	ids, e := s.records("command-confirmations")
	if e != nil {
		return out, e
	}
	if len(ids) >= 10000 {
		return out, failure(409, "ARCHIVE_COMMAND_CONFIRMATION_LIMIT", "人工确认记录已达上限")
	}
	if _, e = s.readCommandConfirmation(in.CommandID); e == nil {
		return out, failure(409, "ARCHIVE_COMMAND_ALREADY_REVIEWED", "原命令已有不可变确认，请核对原请求")
	}
	if dto.StatusCode(e) != 404 {
		return out, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	review, r, j, p, epoch, e := s.commandReviewLocked(ctx, id)
	if e != nil {
		return out, e
	}
	if review.TargetID != in.TargetID {
		return out, failure(409, "ARCHIVE_PROBE_SCOPE_CHANGED", "不能更换原命令的目标范围")
	}
	if review.ReviewVersion != in.ExpectedVersion || r.CommandID != in.CommandID || r.ExecutionID != in.ExecutionID {
		return out, failure(409, "ARCHIVE_REVIEW_STALE", "任务、命令、策略、系统观察或恢复代次发生变化")
	}
	if !review.Eligible {
		return out, failure(409, "ARCHIVE_COMMAND_NOT_DRAINED", "缺少完整启动身份或进程组仍未确认退出，继续保护")
	}
	receipt := commandConfirmation{CommandDrainReceipt: CommandDrainReceipt{RequestID: in.RequestID, RecordID: id, JobID: j.ID, ExecutionID: r.ExecutionID, CommandID: r.CommandID, State: "confirmed", InputDigest: digest, ExpectedVersion: in.ExpectedVersion, Action: in.Action, Reason: in.Reason, Operator: reqctx.GetUserID(ctx), At: time.Now().UTC()}, CommandDigest: dispositionHash(r), JobVersion: dispositionHash(j), Epoch: dispositionHash(epoch), Host: r.Host, Boot: r.Identity.Boot, Namespace: r.Identity.Namespace, Observation: commandOSObservation{true, "absent", "absent"}}
	if r.JobID == "" {
		receipt.TargetID = r.Probe.TargetID
		receipt.Probe = review.Probe
		receipt.JobVersion = review.Probe.Digest
	}
	e = s.recoveryAnchor.WithEpoch(ctx, func(now recovery.Epoch) error {
		if now != epoch {
			return failure(409, "ARCHIVE_REVIEW_STALE", "恢复代次变化，请重新核对")
		}
		return s.guardCommandReview(ctx, r, p, review.Probe, func(context.Context) error {
			attempted = true
			return dispositionWrite(s.path("command-confirmations", r.CommandID), receipt)
		})
	})
	if e != nil {
		return out, failure(503, "ARCHIVE_COMMAND_CONFIRMATION_UNCONFIRMED", "提交确认未完成，请只读核对原请求，保护不由错误答复解除")
	}
	return s.commandConfirmationReceipt(receipt), nil
}

func (s *Service) recordCommandRejection(ctx context.Context, id string, in ConfirmCommandDrainInput, digest, problem string) error {
	registry, e := s.lock(context.Background(), "command-rejection-registry")
	if e != nil {
		return e
	}
	defer registry()
	ids, e := s.records("command-confirmation-rejections")
	if e != nil {
		return e
	}
	if len(ids) >= 10000 {
		return failure(503, "ARCHIVE_COMMAND_CONFIRMATION_LIMIT", "拒绝回执达到上限")
	}
	if _, e := s.findCommandConfirmation(in.RequestID); e == nil {
		return nil
	} else if dto.StatusCode(e) != 404 {
		return e
	}
	r := CommandDrainReceipt{RequestID: in.RequestID, RecordID: id, JobID: id, ExecutionID: in.ExecutionID, CommandID: in.CommandID, State: "rejected", InputDigest: digest, ExpectedVersion: in.ExpectedVersion, Action: in.Action, Reason: in.Reason, Operator: reqctx.GetUserID(ctx), At: time.Now().UTC(), Problem: problem}
	if in.TargetID != "" {
		r.JobID = ""
		r.TargetID = in.TargetID
	}
	return dispositionWrite(s.path("command-confirmation-rejections", in.RequestID), r)
}

func validCommandReceiptSubject(r CommandDrainReceipt) bool {
	if r.TargetID == "" {
		return r.JobID == r.RecordID
	}
	return r.JobID == "" && validID(r.TargetID) && r.RecordID == commandProbeID(r.TargetID)
}
