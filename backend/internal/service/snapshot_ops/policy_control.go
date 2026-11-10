package snapshot_ops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type PolicySettings struct {
	Name            string `json:"name"`
	IntervalMinutes int    `json:"interval_minutes"`
	RetentionDays   int    `json:"retention_days"`
	RetentionCount  int    `json:"retention_count"`
}
type PolicyEntry struct {
	ID          string         `json:"id"`
	Revision    int64          `json:"revision"`
	Settings    PolicySettings `json:"settings"`
	Enabled     bool           `json:"enabled"`
	Current     bool           `json:"current"`
	Anchor      time.Time      `json:"anchor"`
	Fingerprint string         `json:"fingerprint"`
}
type LegacyPolicies struct {
	DefaultID string        `json:"default_id"`
	Policies  []PolicyEntry `json:"policies"`
}
type PolicyConfiguration struct {
	Authority        string          `json:"authority"`
	State            string          `json:"state"`
	Ready            bool            `json:"ready"`
	DesiredRevision  int64           `json:"desired_revision"`
	AppliedRevision  int64           `json:"applied_revision"`
	SnapshotRevision *int64          `json:"snapshot_revision"`
	LegacyDigest     string          `json:"legacy_digest"`
	Policies         []PolicyEntry   `json:"policies"`
	ExecutionReady   map[string]bool `json:"execution_ready"`
	Problem          string          `json:"problem,omitempty"`
}
type PolicyAdoption struct {
	RequestID    string `json:"request_id"`
	LegacyDigest string `json:"legacy_digest"`
	Confirm      bool   `json:"confirm"`
}
type PolicyChange struct {
	RequestID              string          `json:"request_id"`
	ExpectedRevision       int64           `json:"expected_revision"`
	ExpectedPolicyRevision int64           `json:"expected_policy_revision"`
	PolicyID               string          `json:"policy_id"`
	Action                 string          `json:"action"`
	Settings               *PolicySettings `json:"settings,omitempty"`
	Enabled                *bool           `json:"enabled,omitempty"`
	Confirm                bool            `json:"confirm"`
}
type PolicyMutation struct {
	RequestID         string              `json:"request_id"`
	OperationRevision int64               `json:"operation_revision"`
	PolicyID          string              `json:"policy_id"`
	Configuration     PolicyConfiguration `json:"configuration"`
}

// 固定控制面由宿主装配；Guard 仅包围有界准入元数据或原子隔离，不能执行采集、完整遍历或物理递归删除。
type PolicyAuthority interface {
	Configuration(context.Context) (PolicyConfiguration, error)
	Adopt(context.Context, PolicyAdoption) (PolicyMutation, error)
	Change(context.Context, PolicyChange) (PolicyMutation, error)
	Request(context.Context, string) (PolicyMutation, error)
	Runtime(context.Context) ([]PolicyEntry, error)
	Guard(context.Context, string, int64, string, func(PolicyEntry) error) error
}
type PolicyBinding struct {
	Version      int    `json:"version"`
	ScopeID      string `json:"scope_id"`
	Revision     int64  `json:"revision"`
	OriginDigest string `json:"origin_digest"`
}

func ValidatePolicySettings(p PolicySettings) error {
	if strings.TrimSpace(p.Name) != p.Name || p.Name == "" || utf8.RuneCountInString(p.Name) > 100 || strings.ContainsAny(p.Name, "\r\n\x00") {
		return failure(400, "SNAPSHOT_POLICY_NAME_INVALID", "策略名称无效")
	}
	return validatePolicy(Policy{IntervalMinutes: p.IntervalMinutes, RetentionDays: p.RetentionDays, RetentionCount: p.RetentionCount})
}
func entryRecord(p PolicyEntry) policyRecord {
	return policyRecord{Revision: p.Revision, ID: p.ID, Name: p.Settings.Name, Anchor: p.Anchor, Policy: Policy{Enabled: p.Enabled, IntervalMinutes: p.Settings.IntervalMinutes, RetentionDays: p.Settings.RetentionDays, RetentionCount: p.Settings.RetentionCount}}
}
func (s *Service) controlRequired() bool {
	if s == nil {
		return true
	}
	if s.opts.RequirePolicyControl || s.cfg.PolicyControlRequired || s.policyAuthority != nil {
		return true
	}
	_, e := os.Lstat(filepath.Join(s.cfg.Directory, "policy-control-binding.json"))
	return !errors.Is(e, os.ErrNotExist)
}
func (s *Service) PolicyFingerprint() string {
	return digest([]byte(s.cfg.Instance + "\x00" + s.cfg.Directory + "\x00" + s.cfg.DatabaseDSN + "\x00" + s.cfg.MediaRoot))
}
func (s *Service) LegacyPolicySnapshot(ctx context.Context) (LegacyPolicies, string, error) {
	if e := ctx.Err(); e != nil {
		return LegacyPolicies{}, "", e
	}
	state, e := s.policyState()
	if e != nil {
		return LegacyPolicies{}, "", e
	}
	out := LegacyPolicies{DefaultID: state.DefaultID, Policies: []PolicyEntry{}}
	for _, p := range state.Policies {
		out.Policies = append(out.Policies, PolicyEntry{ID: p.ID, Settings: PolicySettings{p.Name, p.IntervalMinutes, p.RetentionDays, p.RetentionCount}, Enabled: p.Enabled, Current: p.ID == state.DefaultID, Anchor: p.Anchor})
	}
	sort.Slice(out.Policies, func(i, j int) bool { return out.Policies[i].ID < out.Policies[j].ID })
	raw, _ := json.Marshal(out)
	return out, digest(raw), nil
}
func (s *Service) ReadPolicyBinding() (PolicyBinding, error) {
	var b PolicyBinding
	err := readJSON(filepath.Join(s.cfg.Directory, "policy-control-binding.json"), &b)
	if err != nil {
		return b, err
	}
	if b.Version != 1 || len(b.ScopeID) != 64 || b.Revision < 0 || b.Revision > 0 && len(b.OriginDigest) != 64 {
		return b, failure(503, "SNAPSHOT_POLICY_BINDING_INVALID", "策略归属不可确认")
	}
	return b, nil
}
func (s *Service) BindPolicyAuthority(scope string, a PolicyAuthority) error {
	if s.initErr != nil || a == nil || s.policyAuthority != nil || len(scope) != 64 {
		return failure(409, "SNAPSHOT_POLICY_BINDING_INVALID", "策略控制面不可绑定")
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "policy.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	b, e := s.ReadPolicyBinding()
	if dto.StatusCode(e) == 404 {
		b = PolicyBinding{Version: 1, ScopeID: scope}
		e = atomicJSON(filepath.Join(s.cfg.Directory, "policy-control-binding.json"), b)
	}
	if e != nil {
		return e
	}
	if b.ScopeID != scope {
		return failure(409, "SNAPSHOT_POLICY_OWNER_MISMATCH", "快照目录由其他安装管理")
	}
	s.policyAuthority = a
	return nil
}

// 发布时核对原始来源并先落归属水位。投影失败时保留水位，禁止回退旧 JSON。
func (s *Service) PublishPolicyBinding(ctx context.Context, scope, origin string, revision int64, publish func() error) error {
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "policy.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	b, e := s.ReadPolicyBinding()
	if e != nil {
		return e
	}
	if b.ScopeID != scope || b.Revision > revision || b.Revision > 0 && b.OriginDigest != origin {
		return failure(409, "SNAPSHOT_POLICY_BINDING_INVALID", "发布范围或版本不符")
	}
	if b.Revision == 0 {
		_, hash, err := s.LegacyPolicySnapshot(ctx)
		if err != nil {
			return err
		}
		if hash != origin {
			return failure(409, "SNAPSHOT_POLICY_LEGACY_CHANGED", "旧快照策略已变化")
		}
	}
	b.Revision = revision
	b.OriginDigest = origin
	if e = atomicJSON(filepath.Join(s.cfg.Directory, "policy-control-binding.json"), b); e != nil {
		return e
	}
	return publish()
}
func (s *Service) PolicyConfiguration(ctx context.Context) (PolicyConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return PolicyConfiguration{}, failure(403, "SNAPSHOT_ROOT_REQUIRED", "需要平台 Root 会话")
	}
	if s.policyAuthority == nil {
		return PolicyConfiguration{}, failure(503, "SNAPSHOT_POLICY_CONTROL_UNAVAILABLE", "策略控制面不可用")
	}
	return s.policyAuthority.Configuration(ctx)
}
func (s *Service) AdoptPolicyConfiguration(ctx context.Context, in PolicyAdoption) (PolicyMutation, error) {
	if _, e := s.PolicyConfiguration(ctx); e != nil {
		return PolicyMutation{}, e
	}
	return s.policyAuthority.Adopt(ctx, in)
}
func (s *Service) ChangePolicyConfiguration(ctx context.Context, in PolicyChange) (PolicyMutation, error) {
	if _, e := s.PolicyConfiguration(ctx); e != nil {
		return PolicyMutation{}, e
	}
	return s.policyAuthority.Change(ctx, in)
}
func (s *Service) PolicyConfigurationRequest(ctx context.Context, id string) (PolicyMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return PolicyMutation{}, failure(403, "SNAPSHOT_ROOT_REQUIRED", "需要平台 Root 会话")
	}
	if s.policyAuthority == nil {
		return PolicyMutation{}, failure(503, "SNAPSHOT_POLICY_CONTROL_UNAVAILABLE", "策略控制面不可用")
	}
	return s.policyAuthority.Request(ctx, id)
}
func (s *Service) runtimePolicyState(ctx context.Context) (policyState, error) {
	if !s.controlRequired() {
		return s.policyState()
	}
	if s.policyAuthority == nil {
		return policyState{}, failure(503, "SNAPSHOT_POLICY_CONTROL_UNAVAILABLE", "策略控制面不可用")
	}
	entries, e := s.policyAuthority.Runtime(ctx)
	if e != nil {
		return policyState{}, e
	}
	state := policyState{Version: 1, Policies: []policyRecord{}}
	for _, p := range entries {
		state.Policies = append(state.Policies, entryRecord(p))
		if p.Current {
			state.DefaultID = p.ID
		}
	}
	return state, nil
}
func (s *Service) runtimePolicyRecord(ctx context.Context) (policyRecord, error) {
	state, e := s.runtimePolicyState(ctx)
	return state.current(), e
}

func (s *Service) CheckPolicySources(ctx context.Context) error {
	if s.opts.DispatchCapture != nil {
		return failure(409, "SNAPSHOT_POLICY_EXECUTOR_UNSUPPORTED", "旧独立协调器尚未接入数据库清理守卫，请使用同进程执行模式；手动采集仍可用")
	}
	if p := s.problems(ctx); len(p) > 0 {
		return failure(422, "SNAPSHOT_SOURCE_UNAVAILABLE", strings.Join(p, "；"))
	}
	return nil
}
