package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Service struct {
	cfg     Config
	opts    Options
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	workers sync.WaitGroup
	initErr error
}

func NewService(ctx context.Context, c Config, opts Options) *Service {
	run, cancel := context.WithCancel(ctx)
	s := &Service{cfg: c, opts: opts, ctx: run, cancel: cancel}
	if opts.ConfigError != nil {
		s.initErr = opts.ConfigError
		return s
	}
	if s.initErr = validateConfig(&s.cfg); s.initErr != nil {
		return s
	}
	for _, name := range []string{"policies", "targets", "jobs", "locks", "objects", "restores", "staging"} {
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
	return s
}
func (s *Service) Close() { s.mu.Lock(); s.cancel(); s.mu.Unlock(); s.workers.Wait() }
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
	if source.Kind == "postgresql_logs" && source.CurrentLogfiles == "" {
		source.Problem = "必须配置 PostgreSQL current_logfiles 文件，以识别正在写入的日志"
		return source
	}
	if source.Kind == "postgresql_logs" {
		if _, e := os.ReadFile(source.CurrentLogfiles); e != nil {
			source.Problem = "无法读取 PostgreSQL current_logfiles"
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
	out = Overview{Instance: s.cfg.Instance, Directory: s.cfg.Directory, Policies: []Policy{}, Targets: []Target{}, Jobs: []Job{}, Restores: []Restore{}, Credentials: []string{}, Sources: []Source{}}
	for _, source := range s.cfg.Sources {
		out.Sources = append(out.Sources, sourceStatus(source))
	}
	for ref := range s.cfg.Credentials {
		out.Credentials = append(out.Credentials, ref)
	}
	sort.Strings(out.Credentials)
	for _, kind := range []string{"policies", "targets", "jobs", "restores"} {
		ids, e := s.records(kind)
		if e != nil {
			return out, e
		}
		for _, id := range ids {
			switch kind {
			case "policies":
				var p Policy
				if e = readJSON(s.path(kind, id), &p); e == nil {
					out.Policies = append(out.Policies, p)
				}
			case "targets":
				var t Target
				if e = readJSON(s.path(kind, id), &t); e == nil {
					out.Targets = append(out.Targets, t)
				}
			case "jobs":
				var j jobRecord
				j, e = s.readJob(id)
				if e == nil {
					copy := j.Job
					copy.Files = nil
					out.Jobs = append(out.Jobs, copy)
					if j.RemoteState == "pending" || j.RemoteState == "failed" {
						out.PendingTransfers++
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
	return out, nil
}
func (s *Service) SavePolicy(ctx context.Context, p Policy) (Policy, error) {
	if e := s.check(ctx); e != nil {
		return p, e
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
	if source.Kind == "postgresql_logs" && p.TargetID == "" {
		return p, failure(400, "ARCHIVE_TARGET_REQUIRED", "数据库日志归档需要配置异地目标")
	}
	if p.Enabled {
		status := sourceStatus(source)
		if !status.Ready {
			return p, failure(422, "ARCHIVE_SOURCE_UNAVAILABLE", status.Problem)
		}
	}
	if p.TargetID != "" {
		if _, e = s.target(p.TargetID); e != nil {
			return p, e
		}
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = time.Now().UTC()
	} else {
		old, e := s.policy(ctx, p.ID)
		if e != nil {
			return p, e
		}
		p.CreatedAt = old.CreatedAt
	}
	unlock, e := s.lock(ctx, p.ID)
	if e != nil {
		return p, failure(409, "ARCHIVE_BUSY", "该策略正在执行")
	}
	defer unlock()
	e = writeJSON(s.path("policies", p.ID), p)
	return p, e
}

var userPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,63}$`)
var remotePathPattern = regexp.MustCompile(`^/(?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+$`)

func (s *Service) SaveTarget(ctx context.Context, t Target) (Target, error) {
	if e := s.check(ctx); e != nil {
		return t, e
	}
	t.Name = strings.TrimSpace(t.Name)
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
		if _, e = s.target(t.ID); e != nil {
			return t, e
		}
	}
	e = writeJSON(s.path("targets", t.ID), t)
	return t, e
}
func (s *Service) target(id string) (Target, error) {
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
			if j.PolicyID != p.ID || (p.ID == "" && (!j.OneTime || j.SourceID != p.SourceID)) {
				return Job{}, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "同一标识不能用于另一策略")
			}
			return j.Job, nil
		}
	}
	unlock, e := s.lock(ctx, p.ID)
	if e != nil {
		return Job{}, failure(409, "ARCHIVE_BUSY", "该策略正在执行")
	}
	source, e := s.source(p.SourceID)
	if e != nil {
		unlock()
		return Job{}, e
	}
	status := sourceStatus(source)
	if !status.Ready {
		unlock()
		return Job{}, failure(422, "ARCHIVE_SOURCE_UNAVAILABLE", status.Problem)
	}
	// 同一策略的执行锁已接管，旧 running 清单对应进程已经退出。
	for _, id := range ids {
		j, e := s.readJob(id)
		if e == nil && j.PolicyID == p.ID && j.Status == "running" {
			j.Status = "failed"
			j.Error = "上次归档进程中断"
			_ = s.saveJob(&j)
		}
	}
	now := time.Now().UTC()
	j := jobRecord{Job: Job{ID: uuid.NewString(), PolicyID: p.ID, SourceID: p.SourceID, Kind: source.Kind, Status: "running", Phase: "capturing", CreatedAt: now, Warnings: []string{}, RemoteState: "not_requested", Operator: operator, TraceID: trace, RemoteRetentionDays: p.RemoteRetentionDays, RemoteRetentionCount: p.RemoteRetentionCount}, RequestKey: hash}
	j.OneTime = p.ID == ""
	if p.TargetID != "" {
		target, e := s.target(p.TargetID)
		if e != nil {
			unlock()
			return Job{}, e
		}
		j.Target = &target
		j.RemoteState = "pending"
	}
	if e = s.saveJob(&j); e != nil {
		unlock()
		return Job{}, e
	}
	response := j.Job
	run := func() { func() { defer unlock(); s.execute(&j, p, source) }(); _, _ = s.cleanup(ctx, p.ID) }
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
	ids, e := s.records("policies")
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
		if !p.Enabled {
			continue
		}
		latest := time.Time{}
		jobs, _ := s.records("jobs")
		for _, jid := range jobs {
			j, e := s.readJob(jid)
			if e == nil && j.PolicyID == p.ID && j.CreatedAt.After(latest) {
				latest = j.CreatedAt
			}
		}
		if time.Since(latest) < time.Duration(p.IntervalMinutes)*time.Minute {
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
		if e == nil && j.Status == "success" && j.Target != nil && (j.RemoteState == "pending" || j.RemoteState == "failed") && j.Attempts < 5 && !time.Now().Before(j.RetryAt) {
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
	if e := s.capture(ctx, j, p, source); e != nil {
		j.Status = "failed"
		j.Phase = "failed"
		j.Error = e.Error()
		_ = s.saveJob(j)
		return
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
	if j.Status != "success" || j.Target == nil {
		return Job{}, failure(409, "ARCHIVE_TRANSFER_NOT_READY", "只能重传已有完整快照")
	}
	unlock, e := s.lock(ctx, j.PolicyID)
	if e != nil {
		return Job{}, failure(409, "ARCHIVE_BUSY", "策略正在执行")
	}
	j.RemoteState = "pending"
	j.RemoteError = ""
	if e = s.saveJob(&j); e != nil {
		unlock()
		return Job{}, e
	}
	out := j.Job
	run := func() {
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
