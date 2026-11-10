package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	vault "github.com/ArtisanCloud/PowerX/internal/infra/archive_credentials"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
)

// PolicySettings 仅接收编辑字段，启停与执行证明由独立操作维护。
type PolicySettings struct {
	Name                 string `json:"name"`
	SourceID             string `json:"source_id"`
	TargetID             string `json:"target_id"`
	IntervalMinutes      int    `json:"interval_minutes"`
	RetentionDays        int    `json:"retention_days"`
	RetentionCount       int    `json:"retention_count"`
	RemoteRetentionDays  int    `json:"remote_retention_days"`
	RemoteRetentionCount int    `json:"remote_retention_count"`
	RemoteMaxBytes       int64  `json:"remote_max_bytes"`
	IncludeDatabase      bool   `json:"include_database"`
}

func SettingsOfPolicy(p Policy) PolicySettings {
	return PolicySettings{p.Name, p.SourceID, p.TargetID, p.IntervalMinutes, p.RetentionDays, p.RetentionCount, p.RemoteRetentionDays, p.RemoteRetentionCount, p.RemoteMaxBytes, p.IncludeDatabase}
}
func (p PolicySettings) Policy(id string, revision int64) Policy {
	return Policy{ID: id, Revision: revision, Name: p.Name, SourceID: p.SourceID, TargetID: p.TargetID, IntervalMinutes: p.IntervalMinutes, RetentionDays: p.RetentionDays, RetentionCount: p.RetentionCount, RemoteRetentionDays: p.RemoteRetentionDays, RemoteRetentionCount: p.RemoteRetentionCount, RemoteMaxBytes: p.RemoteMaxBytes, IncludeDatabase: p.IncludeDatabase}
}
func ValidatePolicySettings(p PolicySettings) error {
	if strings.TrimSpace(p.Name) != p.Name || p.Name == "" || len(p.Name) > 128 || !labelPattern.MatchString(p.SourceID) || p.TargetID != "" && !validID(p.TargetID) || p.IntervalMinutes < 1 || p.IntervalMinutes > 525600 || p.RetentionDays < 1 || p.RetentionDays > 3650 || p.RetentionCount < 1 || p.RetentionCount > 10000 || p.RemoteRetentionDays < 1 || p.RemoteRetentionDays > 3650 || p.RemoteRetentionCount < 1 || p.RemoteRetentionCount > 10000 || p.RemoteMaxBytes < 0 || p.RemoteMaxBytes > 1<<50 {
		return failure(400, "ARCHIVE_POLICY_INVALID", "策略频率、保留规则或预算无效")
	}
	return nil
}

type PolicyConfiguration struct {
	Authority        string   `json:"authority"`
	Ready            bool     `json:"ready"`
	State            string   `json:"state"`
	DesiredRevision  int64    `json:"desired_revision"`
	AppliedRevision  int64    `json:"applied_revision"`
	SnapshotRevision *int64   `json:"snapshot_revision"`
	LegacyDigest     string   `json:"legacy_digest"`
	LegacyCount      int      `json:"legacy_count"`
	Problem          string   `json:"problem,omitempty"`
	Policies         []Policy `json:"policies"`
}
type PolicyAdoption struct {
	RequestID    string `json:"request_id"`
	LegacyDigest string `json:"legacy_digest"`
}
type PolicyChange struct {
	RequestID                 string          `json:"request_id"`
	ExpectedRevision          int64           `json:"expected_revision"`
	ExpectedPolicyRevision    int64           `json:"expected_policy_revision"`
	PolicyID                  string          `json:"policy_id"`
	Action                    string          `json:"action"`
	Settings                  *PolicySettings `json:"settings,omitempty"`
	Enabled                   *bool           `json:"enabled,omitempty"`
	RemoteCleanupEnabled      *bool           `json:"remote_cleanup_enabled,omitempty"`
	Confirm                   bool            `json:"confirm"`
	ExpectedTargetRevision    int64           `json:"expected_target_revision"`
	ExpectedTargetFingerprint string          `json:"expected_target_fingerprint"`
}
type PolicyMutation struct {
	RequestID         string              `json:"request_id"`
	OperationRevision int64               `json:"operation_revision"`
	PolicyID          string              `json:"policy_id,omitempty"`
	Configuration     PolicyConfiguration `json:"configuration"`
}

// PolicyAuthority 在不引入 archive_ops→数据库包循环依赖的前提下复用正式配置控制面。
type PolicyAuthority interface {
	Configuration(context.Context) (PolicyConfiguration, error)
	Resolve(context.Context, string) (Policy, error)
	Adopt(context.Context, PolicyAdoption) (PolicyMutation, error)
	Save(context.Context, PolicyChange) (PolicyMutation, error)
	Request(context.Context, string) (PolicyMutation, error)
	Guard(context.Context, Policy, string, func([]Policy, *Target) error) error
}

func (s *Service) BindPolicyAuthority(a PolicyAuthority) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil || a == nil || s.policyAuthority != nil || s.schedulerRunning {
		return failure(409, "ARCHIVE_POLICY_BINDING_INVALID", "策略控制面不可重复绑定")
	}
	if e := s.writePolicyControlBinding(); e != nil {
		return e
	}
	s.policyAuthority = a
	return nil
}
func (s *Service) policyControlRequired() bool {
	return s.opts.RequirePolicyControl || s.cfg.PolicyControlRequired || s.policyAuthority != nil
}
func (s *Service) policyControlCheck(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return failure(403, "ARCHIVE_USER_REQUIRED", "需要 Root 管理员用户会话")
	}
	if e := s.check(ctx); e != nil {
		return e
	}
	if s.policyAuthority == nil {
		return failure(503, "ARCHIVE_POLICY_CONTROL_UNAVAILABLE", "策略数据库控制面不可用")
	}
	return nil
}
func (s *Service) PolicyConfiguration(ctx context.Context) (PolicyConfiguration, error) {
	if e := s.policyControlCheck(ctx); e != nil {
		return PolicyConfiguration{}, e
	}
	return s.policyAuthority.Configuration(ctx)
}
func (s *Service) AdoptPolicyConfiguration(ctx context.Context, in PolicyAdoption) (PolicyMutation, error) {
	if e := s.policyControlCheck(ctx); e != nil {
		return PolicyMutation{}, e
	}
	return s.policyAuthority.Adopt(ctx, in)
}
func (s *Service) SavePolicyConfiguration(ctx context.Context, in PolicyChange) (PolicyMutation, error) {
	if e := s.policyControlCheck(ctx); e != nil {
		return PolicyMutation{}, e
	}
	return s.policyAuthority.Save(ctx, in)
}
func (s *Service) PolicyConfigurationRequest(ctx context.Context, key string) (PolicyMutation, error) {
	if e := s.policyControlCheck(ctx); e != nil {
		return PolicyMutation{}, e
	}
	return s.policyAuthority.Request(ctx, key)
}
func (s *Service) LegacyPolicies(ctx context.Context) ([]Policy, error) {
	ids, e := s.records("policies")
	if e != nil {
		return nil, e
	}
	if len(ids) > 128 {
		return nil, failure(503, "ARCHIVE_POLICY_CATALOG_LIMIT", "策略数量超出接管上限")
	}
	out := []Policy{}
	for _, id := range ids {
		var p Policy
		if e = readJSON(s.path("policies", id), &p); e != nil {
			return nil, e
		}
		if p.ID != id || p.Revision < 0 {
			return nil, failure(503, "ARCHIVE_POLICY_INVALID", "旧策略身份或版本无效")
		}
		out = append(out, p)
	}
	return out, nil
}

// PolicySourceFingerprint 绑定部署登记，摘要不包含可读凭据或 DSN。
func (s *Service) PolicySourceFingerprint(id string) (string, error) {
	source, e := s.source(id)
	if e != nil {
		return "", e
	}
	raw, e := json.Marshal(struct {
		Source   Source
		Active   []string
		Owner    *uint32
		Database string
		S3       *S3Config
	}{source, source.ActiveFiles, source.FileOwnerUID, source.DatabaseDSN, source.S3})
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// PreparePolicy 校验用户编辑及明确启用；暂停不依赖网络或已失效的目标。
func (s *Service) PreparePolicy(ctx context.Context, p Policy) (Policy, error) {
	if e := ValidatePolicySettings(SettingsOfPolicy(p)); e != nil {
		return p, e
	}
	source, e := s.source(p.SourceID)
	if e != nil {
		return p, e
	}
	if p.IncludeDatabase && (source.Kind != "media" || source.DatabaseDSN == "") {
		return p, failure(400, "ARCHIVE_DATABASE_UNAVAILABLE", "此来源未配置配套数据库")
	}
	if !p.Enabled && !p.RemoteCleanupEnabled {
		return p, nil
	}
	if p.TargetID != "" || p.RemoteCleanupEnabled {
		if e := s.remoteExecutionAllowed(); e != nil {
			return p, e
		}
	}
	if !sourceStatus(source).Ready {
		return p, failure(422, "ARCHIVE_SOURCE_UNAVAILABLE", "来源尚未就绪")
	}
	if isLogSource(source.Kind) && s.custodyStatus().LegacyDetected && !s.ManagedLegacySource(source.Directory) {
		return p, failure(409, "ARCHIVE_LEGACY_HANDOFF_REQUIRED", "旧临时任务尚未接管")
	}
	if p.TargetID == "" {
		if source.Kind == "media" && !p.RemoteCleanupEnabled {
			p.TargetRevision = 0
			p.TargetFingerprint = ""
			return p, nil
		}
		return p, failure(400, "ARCHIVE_TARGET_REQUIRED", "日志归档和远端清理必须选择已验证目标")
	}
	t, e := s.targetContext(ctx, p.TargetID)
	if e != nil {
		return p, e
	}
	if t.Driver != "s3" {
		return p, failure(422, "ARCHIVE_POLICY_TARGET_UNSUPPORTED", "受控策略启用仅支持已验证的 S3 目标")
	}
	if !s.targetVerified(t) {
		return p, failure(422, "ARCHIVE_TARGET_UNVERIFIED", "请先测试当前目标")
	}
	if p.RemoteMaxBytes < 1 {
		return p, failure(400, "ARCHIVE_BUDGET_REQUIRED", "启用前必须明确归档预算")
	}
	p.TargetRevision = t.Revision
	p.TargetFingerprint = t.VerifiedFingerprint
	return p, nil
}

type guardedPolicyContext struct{}
type guardedTargetContext struct{}

func (s *Service) policyIDs(ctx context.Context) ([]string, error) {
	if policies, ok := ctx.Value(guardedPolicyContext{}).([]Policy); ok {
		ids := []string{}
		for _, p := range policies {
			ids = append(ids, p.ID)
		}
		return ids, nil
	}
	if !s.policyControlRequired() {
		return s.records("policies")
	}
	if s.policyAuthority == nil {
		return nil, failure(503, "ARCHIVE_POLICY_CONTROL_UNAVAILABLE", "策略控制面不可用")
	}
	out, e := s.policyAuthority.Configuration(ctx)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for _, p := range out.Policies {
		ids = append(ids, p.ID)
	}
	return ids, nil
}
func (s *Service) guardPolicyAction(ctx context.Context, p Policy, operation string, action func(context.Context) error) error {
	if !s.policyControlRequired() {
		return action(ctx)
	}
	if policies, ok := ctx.Value(guardedPolicyContext{}).([]Policy); ok {
		if p.ID != "" {
			found := false
			for _, current := range policies {
				if current.ID == p.ID && current.ControlDigest == p.ControlDigest {
					found = true
					if (operation == "transfer" || operation == "start") && !current.Enabled || operation == "remote-cleanup" && !current.RemoteCleanupEnabled || operation == "local-retention" && !current.Enabled && !current.RemoteCleanupEnabled {
						return failure(409, "ARCHIVE_POLICY_DISABLED", "策略已暂停")
					}
					break
				}
			}
			if !found {
				return failure(409, "ARCHIVE_POLICY_CHANGED", "策略版本变化")
			}
		} else if operation != "local-retention" {
			return failure(409, "ARCHIVE_POLICY_CHANGED", "策略缺失")
		}

		if target, ok := ctx.Value(guardedTargetContext{}).(*Target); ok && target != nil {
			if e := s.CheckCapturedTarget(*target); e != nil {
				return e
			}
		}
		return action(ctx)
	}
	if s.policyAuthority == nil {
		return failure(503, "ARCHIVE_POLICY_CONTROL_UNAVAILABLE", "策略控制面不可用")
	}
	run, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return s.policyAuthority.Guard(run, p, operation, func(policies []Policy, target *Target) error {
		bounded := context.WithValue(run, guardedPolicyContext{}, policies)
		bounded = context.WithValue(bounded, guardedTargetContext{}, target)
		return action(bounded)
	})
}
func (s *Service) guardTransfer(ctx context.Context, j jobRecord, action func(context.Context) error) error {
	if !s.policyControlRequired() {
		return action(ctx)
	}
	p, e := s.policy(ctx, j.PolicyID)
	if e != nil || p.Revision != j.PolicyRevision || j.Target == nil || j.Target.ID != p.TargetID || j.Target.Revision != p.TargetRevision || j.Target.VerifiedFingerprint != p.TargetFingerprint {
		return failure(409, "ARCHIVE_POLICY_CHANGED", "策略或目标版本已变化")
	}
	return s.guardPolicyAction(ctx, p, "transfer", action)
}

// PolicyExecutionTarget 在进入数据库事务前读取执行证明，防止单连接池的嵌套借用。
func (s *Service) PolicyExecutionTarget(ctx context.Context, p Policy) (Target, error) {
	t, e := s.targetContext(ctx, p.TargetID)
	if e != nil {
		return t, e
	}
	if !s.targetVerified(t) || t.Revision != p.TargetRevision || t.VerifiedFingerprint != p.TargetFingerprint {
		return t, failure(409, "ARCHIVE_POLICY_TARGET_CHANGED", "目标执行证明已变化")
	}
	return t, nil
}
func (s *Service) CheckCapturedTarget(t Target) error {
	clean := t
	clean.VerifiedAt = nil
	clean.VerifiedFingerprint = ""
	clean.VerificationValid = false
	clean.Executor = ""
	current := s.targetWithProof(clean)
	if !s.targetVerified(current) || current.VerifiedFingerprint != t.VerifiedFingerprint {
		return failure(409, "ARCHIVE_POLICY_TARGET_CHANGED", "目标凭据或执行证明已变化")
	}
	return nil
}

func (s *Service) readPolicyControlBinding() error {
	raw, e := vault.ReadPrivateFile(filepath.Join(s.cfg.Directory, "policy-control-binding.json"), 4096)
	if errors.Is(e, vault.ErrNotFound) {
		return nil
	}
	if e != nil {
		return failure(503, "ARCHIVE_POLICY_BINDING_UNAVAILABLE", "策略绑定不可读取")
	}
	var b targetControlBinding
	if json.Unmarshal(raw, &b) != nil || b != (targetControlBinding{1, s.cfg.ExecutionNodeID, s.cfg.Instance}) {
		return failure(503, "ARCHIVE_POLICY_BINDING_UNAVAILABLE", "策略绑定身份不符")
	}
	s.cfg.PolicyControlRequired = true
	return nil
}
func (s *Service) writePolicyControlBinding() error {
	if e := writeJSON(filepath.Join(s.cfg.Directory, "policy-control-binding.json"), targetControlBinding{1, s.cfg.ExecutionNodeID, s.cfg.Instance}); e != nil {
		return e
	}
	f, e := os.Open(s.cfg.Directory)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}

func (s *Service) CheckPolicyExecution(ctx context.Context, p Policy, sourceFingerprint string) error {
	fp, e := s.PolicySourceFingerprint(p.SourceID)
	if e != nil || fp != sourceFingerprint {
		return failure(409, "ARCHIVE_POLICY_SOURCE_CHANGED", "来源登记已变化，请重新保存并确认策略")
	}
	if !p.Enabled && !p.RemoteCleanupEnabled {
		return nil
	}
	if p.TargetID == "" {
		source, e := s.source(p.SourceID)
		if e == nil && source.Kind == "media" && !p.RemoteCleanupEnabled {
			return nil
		}
		return failure(400, "ARCHIVE_TARGET_REQUIRED", "此策略必须配置归档目标")
	}
	t, e := s.targetContext(ctx, p.TargetID)
	if e != nil {
		return e
	}
	if !s.targetVerified(t) || t.Revision != p.TargetRevision || t.VerifiedFingerprint != p.TargetFingerprint {
		return failure(409, "ARCHIVE_POLICY_TARGET_CHANGED", "目标版本或验证已变化，请重新确认启用")
	}
	return nil
}
