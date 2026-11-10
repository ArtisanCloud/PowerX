package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	vault "github.com/ArtisanCloud/PowerX/internal/infra/archive_credentials"
	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Service struct {
	recoveryAnchor   *storagerecovery.Store
	policyAuthority  PolicyAuthority
	targetAuthority  TargetAuthority
	vault            *vault.Store
	vaultErr         error
	cfg              Config
	opts             Options
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	workers          sync.WaitGroup
	commandWorkers   sync.WaitGroup
	commandMu        sync.Mutex
	commandClosed    bool
	executions       sync.Map
	schedulerRunning bool
	hostIdentity     string
	initErr          error
}

func NewService(ctx context.Context, c Config, opts Options) *Service {
	run, cancel := context.WithCancel(ctx)
	s := &Service{cfg: c, opts: opts, ctx: run, cancel: cancel}
	if opts.ConfigError != nil {
		s.initErr = opts.ConfigError
		return s
	}
	hostname, hostErr := os.Hostname()
	if hostErr != nil || strings.TrimSpace(hostname) == "" {
		s.initErr = fmt.Errorf("archive execution host unavailable")
		return s
	}
	machine, _ := os.ReadFile("/etc/machine-id")
	hostDigest := sha256.Sum256(append([]byte(runtime.GOOS+"|"+hostname+"|"), machine...))
	s.hostIdentity = hex.EncodeToString(hostDigest[:])
	if s.initErr = validateConfig(&s.cfg); s.initErr != nil {
		return s
	}
	for _, name := range []string{"policies", "targets", "target-verifications", "jobs", "locks", "objects", "restores", "staging", "secrets", "audit", "remote-gc", "interruption-requests", "command-executions", "command-confirmations", "command-confirmation-rejections"} {
		if e := os.MkdirAll(filepath.Join(s.cfg.Directory, name), 0700); e != nil {
			s.initErr = e
			return s
		}
	}
	path, e := filepath.EvalSymlinks(s.cfg.Directory)
	if e != nil {
		s.initErr = e
		return s
	}
	s.cfg.Directory = path
	for i, source := range s.cfg.Sources {
		if path, e := filepath.EvalSymlinks(source.Directory); e == nil {
			s.cfg.Sources[i].Directory = path
		}
	}
	if e := s.readTargetControlBinding(); e != nil {
		s.initErr = e
		return s
	}
	if e := s.readPolicyControlBinding(); e != nil {
		s.initErr = e
		return s
	}
	s.initCredentialVault()
	if err := s.publishCustodyControl(); err != nil {
		s.initErr = err
	}
	if s.initErr == nil {
		s.initErr = s.reconcileInterrupted(run)
	}
	return s
}

// Close 停止新接纳并等待所属 worker 和本机外部命令结束。
func (s *Service) Close() {
	s.mu.Lock()
	s.commandMu.Lock()
	s.commandClosed = true
	s.cancel()
	s.commandMu.Unlock()
	s.mu.Unlock()
	s.workers.Wait()
	s.commandWorkers.Wait()
}
func (s *Service) check(ctx context.Context) error {
	if e := requireRoot(ctx); e != nil {
		return e
	}
	if s == nil || s.initErr != nil {
		return failure(503, "ARCHIVE_SERVICE_UNAVAILABLE", "归档服务不可用，请检查运维配置和目录权限")
	}
	if s.ctx.Err() != nil {
		return failure(503, "ARCHIVE_SERVICE_STOPPED", "归档服务已停止")
	}
	return nil
}
func (s *Service) source(id string) (Source, error) {
	for _, source := range s.cfg.Sources {
		if source.ID == id {
			return source, nil
		}
	}
	return Source{}, failure(400, "ARCHIVE_SOURCE_INVALID", "请从服务器已配置的归档来源中选择")
}
func sourceStatus(source Source) Source {
	info, e := os.Stat(source.Directory)
	if e != nil || !info.IsDir() {
		source.Problem = "来源目录不存在或没有访问权限"
		return source
	}
	if source.Kind == "postgresql_logs" && source.RotationFormat == "" && source.CurrentLogfiles == "" {
		source.Problem = "必须配置 PostgreSQL current_logfiles 文件，以识别正在写入的日志"
		return source
	}
	if source.Kind == "postgresql_logs" && source.RotationFormat == "" {
		if _, e := os.ReadFile(source.CurrentLogfiles); e != nil {
			source.Problem = "无法读取 PostgreSQL current_logfiles"
			return source
		}
	}
	if source.RotationFormat != "" {
		if plan, err := sealedLogPlan(context.Background(), source); err != nil || !plan.Complete {
			source.Problem = "无法完整识别稳定封存日志"
			return source
		}
	}
	source.Ready = true
	return source
}
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	var out Overview
	if e := s.check(ctx); e != nil {
		return out, e
	}
	s.mu.Lock()
	out.SchedulerStarted = s.schedulerRunning
	s.mu.Unlock()
	out = Overview{RemoteExecution: RemoteExecutionAvailability{Enabled: !s.cfg.RemoteExecutionBlocked}, ProbeCommandConfirmationVersion: 1, CommandConfirmationVersion: 1, CommandExecution: s.commandOverview(), InterruptionRequestClosureVersion: 1, InterruptionResolutionVersion: 1, ExecutionVersion: 1, RunRequestVersion: 1, SchedulerStarted: out.SchedulerStarted, Instance: s.cfg.Instance, Directory: s.cfg.Directory, Policies: []Policy{}, Targets: []Target{}, Jobs: []Job{}, Restores: []Restore{}, Credentials: []string{}, Sources: []Source{}}
	if s.cfg.RemoteExecutionBlocked {
		out.RemoteExecution.Problem = "ARCHIVE_RELEASE_NOT_ACCEPTED"
	}
	for _, source := range s.cfg.Sources {
		out.Sources = append(out.Sources, sourceStatus(source))
	}
	for ref := range s.cfg.Credentials {
		out.Credentials = append(out.Credentials, ref)
	}
	refs, err := s.s3CredentialRefs()
	if err != nil {
		return out, err
	}
	out.S3Credentials = refs
	out.CredentialOverview = s.credentialOverview(ctx)
	out.Custody = s.custodyStatus()
	out.RemoteRuns, err = s.remoteGCRuns()
	if err != nil {
		return out, err
	}
	sort.Strings(out.Credentials)
	for _, kind := range []string{"policies", "targets", "jobs", "restores"} {
		if kind == "policies" && s.policyControlRequired() {
			out.PolicyConfiguration = PolicyConfiguration{Authority: "unavailable", State: "unavailable", Problem: "ARCHIVE_POLICY_CONTROL_UNAVAILABLE", Policies: []Policy{}}
			if s.policyAuthority != nil {
				configuration, _ := s.policyAuthority.Configuration(ctx)
				out.PolicyConfiguration = configuration
			}
			for _, p := range out.PolicyConfiguration.Policies {
				if active, e := s.policy(ctx, p.ID); e == nil {
					p.ExecutionReady = active.Enabled || active.RemoteCleanupEnabled
				} else {
					p.ExecutionProblem = "ARCHIVE_POLICY_NOT_CONFIRMED"
				}
				out.Policies = append(out.Policies, p)
			}
			continue
		}
		if kind == "targets" && s.targetControlRequired() {
			out.TargetConfiguration = TargetConfiguration{Authority: "unavailable", State: "unavailable", Problem: "ARCHIVE_TARGET_CONTROL_UNAVAILABLE", Targets: []Target{}}
			if s.targetAuthority != nil {
				var e error
				out.TargetConfiguration, e = s.targetAuthority.Configuration(ctx)
				if e != nil {
					out.TargetConfiguration.Problem = "ARCHIVE_TARGET_CONTROL_UNAVAILABLE"
				}
			}
			for _, t := range out.TargetConfiguration.Targets {
				if resolved, e := s.target(t.ID); e == nil {
					t = resolved
				}
				t.Executor = "powerx:" + s.cfg.ExecutionNodeID
				out.Targets = append(out.Targets, t)
			}
			continue
		}
		ids, e := s.records(kind)
		if e != nil {
			return out, e
		}
		for _, id := range ids {
			switch kind {
			case "policies":
				var p Policy
				if p, e = s.policy(ctx, id); e == nil {
					out.Policies = append(out.Policies, p)
				}
			case "targets":
				var t Target
				if e = readJSON(s.path(kind, id), &t); e == nil {
					t.VerificationValid = s.targetVerified(t)
					t.Executor = "powerx:" + s.cfg.ExecutionNodeID
					out.Targets = append(out.Targets, t)
				}
			case "jobs":
				var j jobRecord
				j, e = s.readJob(id)
				if e == nil {
					copy := s.observedJob(j)
					if copy.ExecutionState == "interrupted" || copy.ExecutionState == "unconfirmed" {
						out.ExecutionAttention = append(out.ExecutionAttention, ExecutionAttention{ID: j.ID, PolicyID: j.PolicyID, ExecutionState: copy.ExecutionState})
					}
					copy.Files = nil
					out.Jobs = append(out.Jobs, copy)
					if j.RemoteState == "pending" || j.RemoteState == "failed" || j.RemoteState == "unconfirmed" {
						out.PendingTransfers++
						if isLogSource(j.Kind) {
							out.LogPendingTransfers++
						}
					}
				}
			case "restores":
				var r Restore
				if e = readJSON(s.path(kind, id), &r); e == nil {
					out.Restores = append(out.Restores, r)
				}
			}
			if e != nil {
				return out, e
			}
		}
	}
	sort.Slice(out.Jobs, func(i, j int) bool { return out.Jobs[i].CreatedAt.After(out.Jobs[j].CreatedAt) })
	if len(out.Jobs) > 100 {
		out.Jobs = out.Jobs[:100]
	}
	if s.cfg.RemoteExecutionBlocked {
		for i := range out.Policies {
			if out.Policies[i].TargetID != "" || out.Policies[i].RemoteCleanupEnabled {
				out.Policies[i].ExecutionReady = false
				out.Policies[i].ExecutionProblem = "ARCHIVE_RELEASE_NOT_ACCEPTED"
			}
		}
	}
	return out, nil
}
func (s *Service) SavePolicy(ctx context.Context, p Policy) (Policy, error) {
	if s.policyControlRequired() {
		return p, failure(428, "ARCHIVE_POLICY_CONTROL_REQUIRED", "请使用带版本和动作的策略配置接口")
	}
	if e := s.check(ctx); e != nil {
		return p, e
	}
	if (p.Enabled && p.TargetID != "") || p.RemoteCleanupEnabled {
		if e := s.remoteExecutionAllowed(); e != nil {
			return p, e
		}
	}
	source, e := s.source(p.SourceID)
	if e != nil {
		return p, e
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 128 || p.IntervalMinutes < 1 || p.IntervalMinutes > 525600 || p.RetentionDays < 1 || p.RetentionDays > 3650 || p.RetentionCount < 1 || p.RetentionCount > 10000 || p.RemoteRetentionDays < 1 || p.RemoteRetentionDays > 3650 || p.RemoteRetentionCount < 1 || p.RemoteRetentionCount > 10000 {
		return p, failure(400, "ARCHIVE_POLICY_INVALID", "策略名称、频率或保留规则无效")
	}
	if p.IncludeDatabase && (source.Kind != "media" || source.DatabaseDSN == "") {
		return p, failure(400, "ARCHIVE_DATABASE_UNAVAILABLE", "此媒体来源没有可用于配套快照的数据库连接")
	}
	if p.Enabled && isLogSource(source.Kind) && p.TargetID == "" {
		return p, failure(400, "ARCHIVE_TARGET_REQUIRED", "数据库日志归档需要配置异地目标")
	}
	if p.RemoteCleanupEnabled && p.TargetID == "" {
		return p, failure(400, "ARCHIVE_TARGET_REQUIRED", "归档到期治理需要独立目标")
	}
	if p.Enabled {
		status := sourceStatus(source)
		if !status.Ready {
			return p, failure(422, "ARCHIVE_SOURCE_UNAVAILABLE", status.Problem)
		}
		if isLogSource(source.Kind) && s.custodyStatus().LegacyDetected && !s.ManagedLegacySource(source.Directory) {
			return p, failure(409, "ARCHIVE_LEGACY_HANDOFF_REQUIRED", "此来源的旧临时任务尚未接管，请先完成部署绑定")
		}
	}
	if p.TargetID != "" {
		var target Target
		if target, e = s.target(p.TargetID); e != nil {
			return p, e
		}
		if (p.Enabled || p.RemoteCleanupEnabled) && (isLogSource(source.Kind) || target.Driver == "s3") && !s.targetVerified(target) {
			return p, failure(422, "ARCHIVE_TARGET_UNVERIFIED", "请先从实际执行节点验证当前归档目标，再明确开启归档")
		}
		if p.Enabled || p.RemoteCleanupEnabled {
			p.TargetRevision = target.Revision
			p.TargetFingerprint = target.VerifiedFingerprint
		}
		if p.RemoteCleanupEnabled && target.Driver != "s3" {
			return p, failure(400, "ARCHIVE_REMOTE_CLEANUP_UNSUPPORTED", "此远端到期任务仅支持已验证的 S3 目标")
		}
		if p.Enabled && target.Driver == "s3" && p.RemoteMaxBytes < 1 {
			return p, failure(400, "ARCHIVE_BUDGET_REQUIRED", "请明确配置远端归档容量预算")
		}
	}
	if p.RemoteMaxBytes < 0 || p.RemoteMaxBytes > 1<<50 {
		return p, failure(400, "ARCHIVE_POLICY_INVALID", "归档预算无效")
	}
	unlock, e := s.lock(ctx, "settings")
	if e != nil {
		return p, failure(409, "ARCHIVE_BUSY", "归档配置正在修改")
	}
	defer unlock()
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = time.Now().UTC()
	} else {
		old, e := s.policy(ctx, p.ID)
		if e != nil {
			return p, e
		}
		p.CreatedAt = old.CreatedAt
		if old.Revision != p.Revision {
			return p, failure(409, "ARCHIVE_REVISION_CONFLICT", "归档策略已被修改，请刷新后重试")
		}
	}
	p.Revision++
	if e = s.auditAction(ctx, "policy_change_intent", p.ID, p.Revision); e != nil {
		return p, e
	}
	e = writeJSON(s.path("policies", p.ID), p)
	if e == nil {
		e = s.publishCustodyControl()
	}
	if e == nil {
		s.auditAction(ctx, "policy_saved", p.ID, p.Revision)
	}
	return p, e
}

var userPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,63}$`)
var remotePathPattern = regexp.MustCompile(`^/(?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+$`)

func (s *Service) SaveTarget(ctx context.Context, t Target) (Target, error) {
	if e := s.check(ctx); e != nil {
		return t, e
	}
	if s.targetControlRequired() {
		return t, failure(428, "ARCHIVE_TARGET_CONTROL_REQUIRED", "请使用带请求标识和版本的目标配置接口")
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.Driver == "s3" {
		return s.saveS3Target(ctx, t)
	}
	if t.Driver != "" && t.Driver != "ssh" {
		return t, failure(400, "ARCHIVE_TARGET_INVALID", "归档目标类型无效")
	}
	hostOK := labelPattern.MatchString(t.Host) || net.ParseIP(t.Host) != nil || regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`).MatchString(t.Host)
	if t.Name == "" || len(t.Name) > 128 || !hostOK || !userPattern.MatchString(t.User) || t.Port < 1 || t.Port > 65535 || !remotePathPattern.MatchString(t.Directory) || filepath.Clean(t.Directory) != t.Directory || t.Directory == "/" {
		return t, failure(400, "ARCHIVE_TARGET_INVALID", "SSH 主机、端口、用户名或绝对保存目录无效")
	}
	if _, ok := s.cfg.Credentials[t.CredentialRef]; !ok {
		return t, failure(400, "ARCHIVE_CREDENTIAL_INVALID", "请选择已部署的 SSH 凭据引用")
	}
	unlock, e := s.lock(ctx, "settings")
	if e != nil {
		return t, failure(409, "ARCHIVE_BUSY", "配置正在修改")
	}
	defer unlock()
	if t.ID == "" {
		t.ID = uuid.NewString()
	} else {
		var old Target
		if old, e = s.target(t.ID); e != nil {
			return t, e
		}
		if old.Revision != t.Revision {
			return t, failure(409, "ARCHIVE_REVISION_CONFLICT", "目标已被修改，请刷新后重试")
		}
	}
	t.Revision++
	t.VerifiedAt, t.VerifiedFingerprint = nil, ""
	t.Executor = "powerx:" + s.cfg.ExecutionNodeID
	e = writeJSON(s.path("targets", t.ID), t)
	return t, e
}
func (s *Service) target(id string) (Target, error) { return s.targetContext(s.ctx, id) }
func (s *Service) targetContext(ctx context.Context, id string) (Target, error) {
	if target, ok := ctx.Value(guardedTargetContext{}).(*Target); ok && target != nil && target.ID == id {
		return *target, nil
	}
	if s.targetControlRequired() {
		if s.targetAuthority == nil {
			return Target{}, failure(503, "ARCHIVE_TARGET_CONTROL_UNAVAILABLE", "目标控制面不可用")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		t, e := s.targetAuthority.Resolve(ctx, id)
		if e != nil {
			return t, e
		}
		return s.targetWithProof(t), nil
	}
	var t Target
	if !validID(id) {
		return t, failure(400, "ARCHIVE_TARGET_INVALID", "异地目标标识无效")
	}
	e := readJSON(s.path("targets", id), &t)
	return t, e
}
func (s *Service) Start(ctx context.Context, policyID, key, operator, trace string) (Job, error) {
	if e := s.check(ctx); e != nil {
		return Job{}, e
	}
	return s.start(ctx, policyID, key, operator, trace, false)
}
func (s *Service) start(ctx context.Context, policyID, key, operator, trace string, synchronous bool) (Job, error) {
	p, e := s.policy(ctx, policyID)
	if e != nil {
		return Job{}, e
	}
	if s.policyControlRequired() && !p.Enabled {
		return Job{}, failure(409, "ARCHIVE_POLICY_DISABLED", "策略已暂停，请先明确启用或使用按需媒体导出")
	}
	return s.startPolicy(ctx, p, key, operator, trace, synchronous)
}

// StartOneTime 只读取预先登记的本地媒体来源，不创建策略或配套数据库任务。
func (s *Service) StartOneTime(ctx context.Context, sourceID, key, operator, trace string) (Job, error) {
	if e := s.check(ctx); e != nil {
		return Job{}, e
	}
	source, e := s.source(sourceID)
	if e != nil {
		return Job{}, e
	}
	if source.Kind != "media" || source.S3 != nil {
		return Job{}, failure(400, "ARCHIVE_ONE_TIME_UNSUPPORTED", "按需媒体导出仅支持已配置的本地媒体来源")
	}
	p := oneTimeMediaPolicy()
	p.SourceID = sourceID
	return s.startPolicy(ctx, p, key, operator, trace, false)
}

func oneTimeMediaPolicy() Policy {
	return Policy{Name: "按需媒体导出", RetentionDays: 7, RetentionCount: 168}
}

func (s *Service) startPolicy(ctx context.Context, p Policy, key, operator, trace string, synchronous bool) (Job, error) {
	return s.startPolicyAtRevision(ctx, p, key, operator, trace, synchronous, 0)
}
func (s *Service) startPolicyAtRevision(ctx context.Context, p Policy, key, operator, trace string, synchronous bool, expectedRevision int64) (Job, error) {
	if !validID(key) {
		return Job{}, failure(400, "ARCHIVE_IDEMPOTENCY_REQUIRED", "归档请求需要 UUID Idempotency-Key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return Job{}, failure(503, "ARCHIVE_STOPPED", "归档服务已停止")
	}
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])
	ids, e := s.records("jobs")
	if e != nil {
		return Job{}, e
	}
	for _, id := range ids {
		j, e := s.readJob(id)
		if e != nil {
			return Job{}, e
		}
		if j.RequestKey == hash {
			if j.PolicyID != p.ID || (p.ID == "" && (!j.OneTime || j.SourceID != p.SourceID)) || expectedRevision > 0 && j.PolicyRevision != expectedRevision {
				return Job{}, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "同一标识不能用于另一策略或策略版本")
			}
			return s.observedJob(j), nil
		}
	}
	if p.TargetID != "" || p.RemoteCleanupEnabled {
		if e := s.remoteExecutionAllowed(); e != nil {
			return Job{}, e
		}
	}
	if e = s.catalogCapacity(); e != nil {
		return Job{}, e
	}
	var unlock func()
	var source Source
	var j jobRecord
	var finishExecution func()
	admit := func(ctx context.Context) error {
		unlock, e = s.lock(ctx, p.ID)
		if e != nil {
			return failure(409, "ARCHIVE_BUSY", "该策略正在执行")
		}
		source, e = s.source(p.SourceID)
		if e != nil {
			return e
		}
		status := sourceStatus(source)
		if !status.Ready {
			return failure(422, "ARCHIVE_SOURCE_UNAVAILABLE", status.Problem)
		}
		if isLogSource(source.Kind) && !p.Enabled {
			return failure(409, "ARCHIVE_DISABLED", "请先在页面明确开启日志归档")
		}
		blocked, err := s.policyHasInterrupted(ctx, p.ID)
		if err != nil {
			return err
		}
		if blocked {
			return failure(409, "ARCHIVE_INTERRUPTED_JOB_PENDING", "原策略有执行中或中断待核对任务，不能创建替代任务")
		}

		now := time.Now().UTC()
		j = jobRecord{Job: Job{PolicyRevision: p.Revision, ID: uuid.NewString(), PolicyID: p.ID, SourceID: p.SourceID, Kind: source.Kind, Status: "running", Phase: "capturing", CreatedAt: now, Warnings: []string{}, RemoteState: "not_requested", Operator: operator, TraceID: trace, RemoteRetentionDays: p.RemoteRetentionDays, RemoteRetentionCount: p.RemoteRetentionCount}, RequestKey: hash}
		j.OneTime = p.ID == ""
		if p.TargetID != "" {
			target, e := s.targetContext(ctx, p.TargetID)
			if e != nil {
				return e
			}
			if (isLogSource(source.Kind) || target.Driver == "s3") && (!p.Enabled || !s.targetVerified(target) || p.TargetRevision != target.Revision || p.TargetFingerprint != target.VerifiedFingerprint) {
				return failure(409, "ARCHIVE_DISABLED", "归档尚未开启或目标配置验证已失效")
			}
			j.Target = &target
			j.RemoteState = "pending"
		}
		finishExecution, e = s.beginExecution(&j)
		if e != nil {
			return e
		}
		if e = s.saveJob(&j); e != nil {
			return e
		}
		return nil
	}
	var admissionErr error
	if s.policyControlRequired() && p.ID != "" {
		admissionErr = s.guardPolicyAction(ctx, p, "start", admit)
	} else {
		admissionErr = admit(ctx)
	}
	if admissionErr != nil {
		if finishExecution != nil {
			finishExecution()
		}
		if j.ID != "" && (j.CommandProtocol == 1 || j.CommandProtocol == 2) {
			j.Status = "failed"
			j.Phase = "failed"
			j.Error = "执行授权未确认，未启动归档"
			_ = s.saveJob(&j)
		}
		if unlock != nil {
			unlock()
		}
		return Job{}, admissionErr
	}
	response := s.observedJob(j)
	run := func() {
		defer finishExecution()
		func() { defer unlock(); s.execute(&j, p, source) }()
		tidy, cancel := context.WithTimeout(s.ctx, 15*time.Second)
		defer cancel()
		_, _ = s.cleanup(tidy, p.ID)
	}
	if synchronous {
		s.mu.Unlock()
		run()
		s.mu.Lock()
		return j.Job, nil
	}
	s.workers.Add(1)
	go func() { defer s.workers.Done(); run() }()
	return response, nil
}
func (s *Service) Tick(ctx context.Context) error {
	if s.initErr != nil {
		return s.initErr
	}
	if err := s.publishCustodyControl(); err != nil {
		return err
	}
	if err := s.reconcileInterrupted(ctx); err != nil {
		return err
	}
	s.pruneExpiredCatalog(ctx)
	ids, e := s.policyIDs(ctx)
	if e != nil {
		return e
	}
	var failures []string
	for _, id := range ids {
		p, e := s.policy(ctx, id)
		if e != nil {
			failures = append(failures, e.Error())
			continue
		}
		if s.cfg.RemoteExecutionBlocked && (p.TargetID != "" || p.RemoteCleanupEnabled) {
			continue
		}
		blocked, err := s.policyHasInterrupted(ctx, p.ID)
		if err != nil {
			return err
		}
		if blocked {
			continue
		}
		if !p.Enabled {
			if p.RemoteCleanupEnabled {
				if err := s.refreshPolicyTarget(ctx, p); err != nil {
					failures = append(failures, err.Error())
				} else if err = s.tickRemoteGC(ctx, p); err != nil {
					failures = append(failures, err.Error())
				}
			}
			continue
		}
		if err := s.refreshPolicyTarget(ctx, p); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if p.RemoteCleanupEnabled {
			if err := s.tickRemoteGC(ctx, p); err != nil {
				failures = append(failures, err.Error())
			}
		}
		latest := time.Time{}
		var last jobRecord
		jobs, _ := s.records("jobs")
		for _, jid := range jobs {
			j, e := s.readJob(jid)
			if e == nil && j.PolicyID == p.ID && j.CreatedAt.After(latest) {
				latest = j.CreatedAt
				last = j
			}
		}
		if time.Since(latest) < time.Duration(p.IntervalMinutes)*time.Minute {
			continue
		}
		source, e := s.source(p.SourceID)
		if e != nil {
			failures = append(failures, e.Error())
			continue
		}
		unchanged, e := s.sealedUnchanged(ctx, source, last, p)
		if e != nil {
			failures = append(failures, e.Error())
			continue
		}
		if unchanged {
			continue
		}
		job, runErr := s.start(ctx, p.ID, uuid.NewString(), "system.archive_timer", "", true)
		if runErr != nil && dto.StatusCode(runErr) == 409 {
			continue
		}
		if runErr != nil {
			failures = append(failures, runErr.Error())
		} else if job.Status == "failed" {
			failures = append(failures, job.ID+": "+job.Error)
		} else if job.RemoteState == "failed" {
			failures = append(failures, job.ID+": "+job.RemoteError)
		}
	}
	jobs, _ := s.records("jobs")
	for _, id := range jobs {
		j, e := s.readJob(id)
		if e == nil && !interruptionPending(j) && j.Status == "success" && j.Target != nil && (j.RemoteState == "pending" || j.RemoteState == "failed") && j.Attempts < 5 && !time.Now().Before(j.RetryAt) {
			retried, retryErr := s.retry(ctx, id, true)
			if retryErr != nil && dto.StatusCode(retryErr) == 409 {
				continue
			}
			if retryErr != nil {
				failures = append(failures, retryErr.Error())
			} else if retried.RemoteState == "failed" {
				failures = append(failures, retried.ID+": "+retried.RemoteError)
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("archive tick: %s", strings.Join(failures, "; "))
	}
	return nil
}
func (s *Service) execute(j *jobRecord, p Policy, source Source) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
	defer cancel()
	if isLogSource(source.Kind) {
		if err := s.transferAllowed(ctx, *j); err != nil {
			j.Status = "failed"
			j.Error = "归档已关闭或配置变化"
			_ = s.saveJob(j)
			return
		}
	}
	if e := s.capture(ctx, j, p, source); e != nil {
		j.Status = "failed"
		j.Phase = "failed"
		j.Error = e.Error()
		if ctx.Err() != nil || s.jobCommandState(*j) != "drained" {
			now := time.Now().UTC()
			j.Status = "interrupted"
			j.Phase = "interrupted"
			j.InterruptionPending = true
			j.InterruptedAt = &now
			j.InterruptedPhase = "capturing"
		}
		_ = s.saveJob(j)
		return
	}
	if isLogSource(source.Kind) {
		j.ProvenanceVerified = true
	}
	j.Status = "success"
	j.Phase = "completed"
	_ = s.saveJob(j)
	if j.Target != nil {
		s.upload(ctx, j)
	}
}
func (s *Service) Retry(ctx context.Context, id string) (Job, error) {
	if e := s.check(ctx); e != nil {
		return Job{}, e
	}
	return s.retry(ctx, id, false)
}
func (s *Service) retry(ctx context.Context, id string, synchronous bool) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, e := s.readJob(id)
	if e != nil {
		return Job{}, e
	}
	if j.Target != nil {
		if e := s.remoteExecutionAllowed(); e != nil {
			return Job{}, e
		}
	}
	if err := s.transferAllowed(ctx, j); err != nil {
		return Job{}, failure(409, "ARCHIVE_DISABLED", "归档已关闭或配置已改变，不能继续重传旧任务")
	}
	if j.Status != "success" || j.Target == nil {
		return Job{}, failure(409, "ARCHIVE_TRANSFER_NOT_READY", "只能重传已有完整快照")
	}
	unlock, e := s.lock(ctx, j.PolicyID)
	if e != nil {
		return Job{}, failure(409, "ARCHIVE_BUSY", "策略正在执行")
	}
	// 锁之前的快照可能已经由另一执行更新；只在原锁内重读后登记本次重传。
	j, e = s.readJob(id)
	if e != nil || j.Status != "success" || j.Target == nil {
		unlock()
		if e != nil {
			return Job{}, e
		}
		return Job{}, failure(409, "ARCHIVE_TRANSFER_NOT_READY", "只能重传已有完整快照")
	}
	finishExecution, e := s.beginExecution(&j)
	if e != nil {
		unlock()
		return Job{}, e
	}
	j.RemoteState = "pending"
	j.RemoteError = ""
	if e = s.saveJob(&j); e != nil {
		finishExecution()
		unlock()
		return Job{}, e
	}
	out := s.observedJob(j)
	run := func() {
		defer finishExecution()
		defer unlock()
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		defer cancel()
		s.upload(ctx, &j)
	}
	if synchronous {
		s.mu.Unlock()
		run()
		s.mu.Lock()
		return j.Job, nil
	}
	s.workers.Add(1)
	go func() { defer s.workers.Done(); run() }()
	return out, nil
}
