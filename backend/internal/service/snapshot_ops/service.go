package snapshot_ops

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

// Service 协调组件和独立账本；完整性、业务验证与正式切换分别记录。
type Service struct {
	cfg                Config
	opts               Options
	key                []byte
	ctx                context.Context
	cancel             context.CancelFunc
	mu                 sync.Mutex
	workers            sync.WaitGroup
	initErr            error
	captureRetryDelays []time.Duration
}

// DefaultStorage 返回默认根目录及来源；显式配置不会改变其他安装路径。
func DefaultStorage() (string, string) {
	if r := os.Getenv("POWERX_LINKS_ROOT"); filepath.IsAbs(r) {
		return filepath.Join(r, "storage", "system-snapshots"), "install_root"
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".powerx", "storage", "system-snapshots"), "default"
}
func DefaultDirectory() string { directory, _ := DefaultStorage(); return directory }
func NewService(ctx context.Context, cfg Config, opts Options) *Service {
	if ctx == nil {
		ctx = context.Background()
	}
	run, cancel := context.WithCancel(ctx)
	s := &Service{cfg: cfg, opts: opts, ctx: run, cancel: cancel}
	if s.opts.Now == nil {
		s.opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if s.cfg.Directory == "" {
		s.cfg.Directory, s.cfg.DirectorySource = DefaultStorage()
	}
	if s.cfg.DirectorySource == "" {
		s.cfg.DirectorySource = "explicit"
	}
	if s.cfg.Platform == "" {
		s.cfg.Platform = runtime.GOOS + "/" + runtime.GOARCH
	}
	if !filepath.IsAbs(s.cfg.Directory) {
		s.initErr = errors.New("快照目录必须是绝对路径")
		return s
	}
	for _, kind := range []string{"snapshots", "restores", "contents", "staging", "locks", "requests", "audit"} {
		if e := os.MkdirAll(filepath.Join(s.cfg.Directory, kind), 0700); e != nil {
			s.initErr = e
			return s
		}
	}
	dir, e := filepath.EvalSymlinks(s.cfg.Directory)
	if e != nil {
		s.initErr = e
		return s
	}
	s.cfg.Directory = dir
	unlock, e := lockFile(filepath.Join(dir, "locks", "initialization.lock"))
	if e != nil {
		s.initErr = e
		return s
	}
	defer unlock()
	keyFile := filepath.Join(dir, "master.key")
	s.key, e = os.ReadFile(keyFile)
	if errors.Is(e, os.ErrNotExist) {
		existing, readErr := os.ReadDir(filepath.Join(dir, "contents"))
		if readErr != nil || len(existing) != 0 {
			s.initErr = errors.New("已有快照但主密钥丢失，拒绝生成替代密钥；请使用原密钥或在新目录导入完整包")
			return s
		}
		s.key = make([]byte, 32)
		if _, e = rand.Read(s.key); e == nil {
			e = atomicFile(keyFile, s.key)
		}
	}
	if e != nil || len(s.key) != 32 {
		s.initErr = errors.New("快照加密密钥不可读取")
		return s
	}
	info, e := os.Lstat(keyFile)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		s.initErr = errors.New("快照密钥必须为私有普通文件")
	}
	if s.initErr == nil && !s.opts.Coordinator && s.opts.DispatchCapture == nil {
		s.initErr = s.recoverInterrupted()
	}
	return s
}
func (s *Service) Close() { s.mu.Lock(); s.cancel(); s.mu.Unlock(); s.workers.Wait() }
func root(ctx context.Context) error {
	c := reqctx.GetClaims(ctx)
	if !reqctx.IsRoot(ctx) || c == nil || c.Issuer == "powerx-sts" {
		return failure(403, "SNAPSHOT_ROOT_REQUIRED", "系统快照仅允许平台 Root 用户会话")
	}
	for _, p := range c.Platforms {
		if p == "api_key" {
			return failure(403, "SNAPSHOT_ROOT_REQUIRED", "API Key 不能访问系统快照")
		}
	}
	return nil
}
func (s *Service) check(ctx context.Context) error {
	if e := root(ctx); e != nil {
		return e
	}
	if s == nil || s.initErr != nil {
		return failure(503, "SNAPSHOT_UNAVAILABLE", "快照存储或加密密钥不可用")
	}
	if s.ctx.Err() != nil {
		return failure(503, "SNAPSHOT_STOPPED", "快照服务已停止")
	}
	return nil
}
func (s *Service) problems(ctx context.Context) []string {
	p := s.infrastructureProblems()
	cfg := s.cfg
	if s.opts.Sources != nil {
		// 安装、卸载和发布会改变来源清单。预检查与采集使用同一解析器，
		// 不能因启动时的旧插件目录已删除而永久阻断后续快照。
		current, e := s.opts.Sources(ctx)
		if e != nil {
			// 不回退检查过期的程序清单，也不把解析失败当作就绪。
			return append(p, e.Error())
		}
		cfg = current
	}
	if cfg.Version == "" {
		p = append(p, "缺少 Core 版本")
	}
	if len(cfg.EffectiveConfig) == 0 {
		p = append(p, "缺少实际生效配置")
	}
	if len(cfg.Artifacts) == 0 {
		p = append(p, "缺少可携带的程序产物")
	}
	names := make([]string, 0, len(cfg.Artifacts))
	for name := range cfg.Artifacts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := cfg.Artifacts[name]
		info, e := os.Stat(path)
		if e == nil && (info.IsDir() || info.Mode().IsRegular()) {
			var f *os.File
			f, e = os.Open(path)
			if e == nil {
				_ = f.Close()
				continue
			}
		}
		switch name {
		case "core/powerx":
			p = append(p, "Core 程序不可读取（core/powerx），请检查当前运行程序文件及读取权限")
		case "core/web-admin":
			p = append(p, "Web Admin 发布产物不可读取（core/web-admin），请检查构建目录及读取权限；缺少构建时在 web-admin 执行 npm run build")
		default:
			p = append(p, fmt.Sprintf("程序产物不可读取（%s），请检查当前插件安装目录及读取权限", name))
		}
	}
	return p
}

// infrastructureProblems 与动态枚举分开：部署合同或存储错误不能被新的来源覆盖。
func (s *Service) infrastructureProblems() []string {
	p := []string{}
	if s.cfg.SourceProblem != "" {
		p = append(p, s.cfg.SourceProblem)
	}
	if s.initErr != nil {
		p = append(p, "快照目录或加密密钥不可用")
	}
	if s.opts.Backend == nil {
		p = append(p, "未配置数据库与媒体采集适配器")
	}
	return p
}
func (s *Service) audit(ctx context.Context, action, id, outcome string) error {
	event := AuditEvent{Action: action, ID: id, Outcome: outcome, Operator: operator(ctx), TraceID: reqctx.GetTraceID(ctx)}
	if e := atomicJSON(s.record("audit", uuid.NewString()), struct {
		Event AuditEvent `json:"event"`
		At    time.Time  `json:"at"`
	}{event, s.opts.Now()}); e != nil {
		return e
	}
	if s.opts.Audit == nil {
		return nil
	}
	return s.opts.Audit(ctx, event)
}

func (s *Service) finishAudit(ctx context.Context, action, id, outcome string) {
	// HTTP 返回后请求会取消；后台任务仍应带原操作者和 Trace 写入终态审计。
	run, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = s.audit(run, action, id, outcome)
}
func operator(ctx context.Context) string {
	if c := reqctx.GetClaims(ctx); c != nil {
		if c.Subject != "" {
			return c.Subject
		}
		if c.UserID > 0 {
			return "user:" + strconv.FormatUint(c.UserID, 10)
		}
	}
	return "system.snapshot_scheduler"
}
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	out := Overview{Problems: []string{}, Policies: []NamedPolicy{}, Snapshots: []Snapshot{}, Restores: []Restore{}}
	if e := s.check(ctx); e != nil {
		return out, e
	}
	state, e := s.policyState()
	if e != nil {
		return out, e
	}
	policy := state.current()
	out.Policy = policy.Policy
	out.DefaultPolicyID = state.DefaultID
	out.Instance = s.cfg.Instance
	out.Directory = s.cfg.Directory
	out.DirectorySource = s.cfg.DirectorySource
	out.Problems = s.problems(ctx)
	out.Ready = len(out.Problems) == 0
	ids, e := s.ids("snapshots")
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		v, e := s.snapshot(id)
		if e != nil {
			return out, e
		}
		out.Snapshots = append(out.Snapshots, v)
	}
	sort.Slice(out.Snapshots, func(i, j int) bool { return out.Snapshots[i].CreatedAt.After(out.Snapshots[j].CreatedAt) })
	ids, e = s.ids("restores")
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		var r Restore
		if e = readJSON(s.record("restores", id), &r); e != nil {
			return out, e
		}
		out.Restores = append(out.Restores, r)
	}
	sort.Slice(out.Restores, func(i, j int) bool { return out.Restores[i].CreatedAt.After(out.Restores[j].CreatedAt) })
	out.NextRunAt = nextPolicyRun(policy, out.Snapshots)
	for _, record := range state.Policies {
		named := exposePolicy(record, state.DefaultID)
		named.NextRunAt = nextPolicyRun(record, out.Snapshots)
		out.Policies = append(out.Policies, named)
	}
	var scheduler struct {
		Error string `json:"error"`
	}
	if e = readJSON(filepath.Join(s.cfg.Directory, "scheduler.json"), &scheduler); e == nil {
		out.SchedulerError = scheduler.Error
	} else if _, statErr := os.Stat(filepath.Join(s.cfg.Directory, "scheduler.json")); !errors.Is(statErr, os.ErrNotExist) {
		return out, e
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, id string) (Snapshot, error) {
	if e := s.check(ctx); e != nil {
		return Snapshot{}, e
	}
	return s.snapshot(id)
}
func (s *Service) GetRestore(ctx context.Context, id string) (Restore, error) {
	var r Restore
	if e := s.check(ctx); e != nil {
		return r, e
	}
	if !validID(id) {
		return r, failure(400, "SNAPSHOT_ID_INVALID", "恢复标识无效")
	}
	e := readJSON(s.record("restores", id), &r)
	return r, e
}

// Start 供内部协调器保留回退点等维护操作使用。管理端手动采集必须调用 StartForPolicy。
func (s *Service) Start(ctx context.Context, key string) (Snapshot, error) {
	if e := s.check(ctx); e != nil {
		return Snapshot{}, e
	}
	return s.start(ctx, key, "manual")
}

// StartForPolicy 将用户确认的系统策略绑定到本次采集，策略切换后不得静默改用新策略。
func (s *Service) StartForPolicy(ctx context.Context, key, policyID string) (Snapshot, error) {
	if e := s.check(ctx); e != nil {
		return Snapshot{}, e
	}
	if !validID(policyID) {
		return Snapshot{}, failure(400, "SNAPSHOT_POLICY_REQUIRED", "请选择系统默认快照策略")
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "policy.lock"))
	if e != nil {
		return Snapshot{}, e
	}
	defer unlock()
	return s.startForPolicy(ctx, key, "manual", policyID)
}
func (s *Service) start(ctx context.Context, key, trigger string) (Snapshot, error) {
	return s.startForPolicy(ctx, key, trigger, "")
}
func capturePolicyReplay(prior Snapshot, policyID string) (Snapshot, error) {
	if policyID != "" && prior.PolicyID != policyID {
		return Snapshot{}, failure(409, "SNAPSHOT_IDEMPOTENCY_CONFLICT", "本次请求已关联其他快照策略，请重新发起")
	}
	return prior, nil
}
func (s *Service) startForPolicy(ctx context.Context, key, trigger, policyID string) (Snapshot, error) {
	var out Snapshot
	if !validID(key) {
		return out, failure(400, "SNAPSHOT_IDEMPOTENCY_REQUIRED", "请提供 UUID Idempotency-Key")
	}
	if prior, found, e := s.findCaptureRequest(key); e != nil {
		return out, e
	} else if found {
		return capturePolicyReplay(prior, policyID)
	}
	if policyID != "" {
		current, e := s.policyRecord()
		if e != nil {
			return out, e
		}
		if current.ID == "" {
			return out, failure(409, "SNAPSHOT_POLICY_REQUIRED", "请先设置系统默认快照策略，再创建快照")
		}
		if current.ID != policyID {
			return out, failure(409, "SNAPSHOT_POLICY_CHANGED", "系统默认快照策略已变化，请核对当前策略后重新创建")
		}
	}
	if problems := s.problems(ctx); len(problems) > 0 {
		return out, failure(422, "SNAPSHOT_SOURCE_UNAVAILABLE", strings.Join(problems, "；"))
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return out, e
	}
	hash := digest([]byte("capture:" + key))
	ids, e := s.ids("snapshots")
	if e != nil {
		unlock()
		return out, e
	}
	for _, id := range ids {
		v, e := s.snapshot(id)
		if e != nil {
			unlock()
			return out, e
		}
		var saved struct {
			Hash string `json:"hash"`
		}
		_ = readJSON(s.record("requests", id), &saved)
		if saved.Hash == hash {
			unlock()
			return capturePolicyReplay(v, policyID)
		}
		if v.Status == "queued" || v.Status == "capturing" || v.Status == "verifying" {
			if s.opts.DispatchCapture != nil {
				unlock()
				return out, failure(409, "SNAPSHOT_BUSY", "独立协调器已有采集任务")
			}
			v.Status = "interrupted"
			v.ErrorCode = "SNAPSHOT_INTERRUPTED"
			v.Error = "上次采集进程已中断"
			v.UpdatedAt = s.opts.Now()
			if e = atomicJSON(s.record("snapshots", id), v); e != nil {
				unlock()
				return out, e
			}
		}
	}
	policy, e := s.policyRecord()
	if e != nil {
		unlock()
		return out, e
	}
	id := uuid.NewString()
	if e = os.Mkdir(s.data(id), 0700); e != nil {
		unlock()
		return out, e
	}
	if e = atomicJSON(s.record("requests", id), map[string]string{"hash": hash}); e != nil {
		unlock()
		return out, e
	}
	out = Snapshot{PolicyID: policy.ID, PolicyName: policy.Name, ID: id, Status: "queued", Phase: "queued", CreatedAt: s.opts.Now(), UpdatedAt: s.opts.Now(), Trigger: trigger, Instance: s.cfg.Instance, Version: s.cfg.Version, Components: []Component{}, TraceID: reqctx.GetTraceID(ctx), Operator: operator(ctx)}
	if e = s.audit(ctx, "capture", id, "queued"); e != nil {
		unlock()
		return Snapshot{}, e
	}
	if e = atomicJSON(s.record("snapshots", id), out); e != nil {
		unlock()
		return Snapshot{}, e
	}
	if s.opts.DispatchCapture != nil {
		e = s.opts.DispatchCapture(ctx, id)
		unlock()
		if e != nil {
			out.Status = "failed"
			out.Phase = "dispatch_failed"
			out.ErrorCode = "SNAPSHOT_COORDINATOR_UNAVAILABLE"
			out.Error = e.Error()
			_ = atomicJSON(s.record("snapshots", id), out)
			return out, e
		}
		return out, nil
	}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		unlock()
		return Snapshot{}, failure(503, "SNAPSHOT_STOPPED", "服务已停止")
	}
	s.workers.Add(1)
	s.mu.Unlock()
	go func(j Snapshot) {
		defer s.workers.Done()
		defer unlock()
		run, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
		defer cancel()
		e := s.captureWithRetry(run, &j)
		if e != nil {
			j.Status = "failed"
			j.Phase = "failed"
			j.ErrorCode = "SNAPSHOT_CAPTURE_FAILED"
			j.Error = e.Error()
			_ = os.RemoveAll(s.data(j.ID))
		} else {
			j.Status = "ready"
			j.Phase = "complete"
		}
		j.UpdatedAt = s.opts.Now()
		_ = atomicJSON(s.record("snapshots", j.ID), j)
		s.finishAudit(ctx, "capture", j.ID, j.Status)
		if e == nil {
			_ = s.cleanup(run)
		}
	}(out)
	return out, nil
}
func (s *Service) Tick(ctx context.Context) error {
	if s.initErr != nil {
		return s.initErr
	}
	scheduleUnlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "scheduler.lock"))
	if e != nil {
		return e
	}
	defer scheduleUnlock()
	record, e := s.policyRecord()
	p := record.Policy
	if e != nil || !p.Enabled {
		return e
	}
	ids, e := s.ids("snapshots")
	if e != nil {
		return e
	}
	latest := record.Anchor
	for _, id := range ids {
		j, e := s.snapshot(id)
		if e != nil {
			return e
		}
		if !j.Imported && (j.PolicyID == record.ID || (j.PolicyID == "" && record.ID == legacyPolicyID)) && j.CreatedAt.After(latest) {
			latest = j.CreatedAt
		}
	}
	if !latest.IsZero() && s.opts.Now().Sub(latest) < time.Duration(p.IntervalMinutes)*time.Minute {
		return nil
	}
	policyUnlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "policy.lock"))
	if e != nil {
		return e
	}
	defer policyUnlock()
	current, e := s.policyRecord()
	if e != nil {
		return e
	}
	if current.ID != record.ID || current.Policy != record.Policy || !current.Anchor.Equal(record.Anchor) {
		return nil
	}
	_, e = s.start(ctx, uuid.NewString(), "scheduled")
	message := ""
	if e != nil {
		message = e.Error()
	}
	if writeErr := atomicJSON(filepath.Join(s.cfg.Directory, "scheduler.json"), map[string]string{"error": message}); writeErr != nil && e == nil {
		e = writeErr
	}
	return e
}
func (s *Service) StartScheduler() {
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.workers.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.workers.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				_ = s.Tick(s.ctx)
			}
		}
	}()
}
func (s *Service) Protect(ctx context.Context, id string, value bool) (Snapshot, error) {
	if e := s.check(ctx); e != nil {
		return Snapshot{}, e
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return Snapshot{}, e
	}
	defer unlock()
	j, e := s.snapshot(id)
	if e != nil {
		return j, e
	}
	j.Protected = value
	j.UpdatedAt = s.opts.Now()
	if e = s.audit(ctx, "protect", id, "success"); e != nil {
		return j, e
	}
	return j, atomicJSON(s.record("snapshots", id), j)
}
func (s *Service) cleanup(ctx context.Context) error {
	policyUnlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "policy.lock"))
	if e != nil {
		return e
	}
	defer policyUnlock()
	state, e := s.policyState()
	if e != nil {
		return e
	}
	rules := map[string]Policy{}
	for _, record := range state.Policies {
		rules[record.ID] = record.Policy
	}
	ids, e := s.ids("snapshots")
	if e != nil {
		return e
	}
	groups := map[string][]Snapshot{}
	for _, id := range ids {
		r, e := s.snapshot(id)
		if e != nil {
			return e
		}
		if r.Status != "ready" {
			continue
		}
		key := r.PolicyID
		if key == "" {
			if _, exists := rules[legacyPolicyID]; exists {
				key = legacyPolicyID
			}
		}
		groups[key] = append(groups[key], r)
	}
	// 分别按所属策略清理，切换默认策略不能使用新规则清理其他策略的历史包。
	for key, rows := range groups {
		p, ok := rules[key]
		if !ok {
			p = defaultPolicy()
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
		for i, j := range rows {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if i == 0 || j.Protected || (i < p.RetentionCount && s.opts.Now().Sub(j.CreatedAt) < time.Duration(p.RetentionDays)*24*time.Hour) {
				continue
			}
			if e = os.RemoveAll(s.data(j.ID)); e != nil {
				return e
			}
			j.Status, j.Phase, j.UpdatedAt = "pruned", "pruned", s.opts.Now()
			if e = atomicJSON(s.record("snapshots", j.ID), j); e != nil {
				return e
			}
		}
	}
	return nil
}
