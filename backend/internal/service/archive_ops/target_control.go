package archive_ops

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	vault "github.com/ArtisanCloud/PowerX/internal/infra/archive_credentials"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
)

// TargetSettings 不接收版本、身份或验证结果；这些值只由服务端产生。
type TargetSettings struct {
	Name           string `json:"name"`
	Driver         string `json:"driver"`
	Endpoint       string `json:"endpoint"`
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix"`
	Region         string `json:"region"`
	ForcePathStyle bool   `json:"force_path_style"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	User           string `json:"user"`
	Directory      string `json:"directory"`
	CredentialRef  string `json:"credential_ref"`
}

func SettingsOfTarget(t Target) TargetSettings {
	return TargetSettings{t.Name, t.Driver, t.Endpoint, t.Bucket, t.Prefix, t.Region, t.ForcePathStyle, t.Host, t.Port, t.User, t.Directory, t.CredentialRef}
}
func (p TargetSettings) Target(id string, revision int64) Target {
	return Target{ID: id, Revision: revision, Name: p.Name, Driver: p.Driver, Endpoint: p.Endpoint, Bucket: p.Bucket, Prefix: p.Prefix, Region: p.Region, ForcePathStyle: p.ForcePathStyle, Host: p.Host, Port: p.Port, User: p.User, Directory: p.Directory, CredentialRef: p.CredentialRef}
}

// ValidateTargetSettings 校验固定配置合同，不发起网络请求，也不读取凭据值。
func ValidateTargetSettings(p TargetSettings) error {
	t := p.Target("", 0)
	valid := strings.TrimSpace(t.Name) == t.Name && t.Name != "" && len(t.Name) <= 128
	if t.Driver == "s3" {
		valid = valid && validS3Target(t) && t.Host == "" && t.Port == 0 && t.User == "" && t.Directory == ""
	} else {
		hostOK := labelPattern.MatchString(t.Host) || net.ParseIP(t.Host) != nil || regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`).MatchString(t.Host)
		valid = valid && (t.Driver == "ssh" || t.Driver == "") && hostOK && userPattern.MatchString(t.User) && t.Port >= 1 && t.Port <= 65535 && remotePathPattern.MatchString(t.Directory) && filepath.Clean(t.Directory) == t.Directory && t.Directory != "/" && labelPattern.MatchString(t.CredentialRef) && t.Endpoint == "" && t.Bucket == "" && t.Prefix == ""
	}
	if !valid {
		return failure(400, "ARCHIVE_TARGET_INVALID", "归档目标配置无效")
	}
	return nil
}

type TargetConfiguration struct {
	Authority        string   `json:"authority"`
	Ready            bool     `json:"ready"`
	State            string   `json:"state"`
	DesiredRevision  int64    `json:"desired_revision"`
	AppliedRevision  int64    `json:"applied_revision"`
	SnapshotRevision *int64   `json:"snapshot_revision"`
	LegacyDigest     string   `json:"legacy_digest"`
	LegacyCount      int      `json:"legacy_count"`
	Problem          string   `json:"problem,omitempty"`
	Targets          []Target `json:"targets"`
}
type TargetChange struct {
	RequestID              string         `json:"request_id"`
	ExpectedRevision       int64          `json:"expected_revision"`
	ExpectedTargetRevision int64          `json:"expected_target_revision"`
	TargetID               string         `json:"target_id"`
	Settings               TargetSettings `json:"settings"`
}
type TargetAdoption struct {
	RequestID    string `json:"request_id"`
	LegacyDigest string `json:"legacy_digest"`
}
type TargetMutation struct {
	RequestID         string              `json:"request_id"`
	OperationRevision int64               `json:"operation_revision"`
	TargetID          string              `json:"target_id,omitempty"`
	Configuration     TargetConfiguration `json:"configuration"`
}

// TargetAuthority 由平台数据库控制面绑定；归档执行器不自行访问业务数据库。
type TargetAuthority interface {
	ChangeDirectories() []string
	Configuration(context.Context) (TargetConfiguration, error)
	Resolve(context.Context, string) (Target, error)
	Save(context.Context, TargetChange) (TargetMutation, error)
	Adopt(context.Context, TargetAdoption) (TargetMutation, error)
	Request(context.Context, string) (TargetMutation, error)
	Confirm(context.Context, Target, func(Target) error) error
}

// BindTargetAuthority 必须在启动调度器之前调用一次。
func (s *Service) BindTargetAuthority(a TargetAuthority) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil || a == nil || s.targetAuthority != nil || s.schedulerRunning {
		return failure(409, "ARCHIVE_TARGET_CONTROL_BINDING_INVALID", "归档目标控制面不可重复绑定")
	}
	if e := s.writeTargetControlBinding(); e != nil {
		return e
	}
	s.targetAuthority = a
	return nil
}
func (s *Service) targetControlRequired() bool {
	return s.opts.RequireTargetControl || s.cfg.TargetControlRequired || s.targetAuthority != nil
}
func (s *Service) targetControlCheck(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return failure(403, "ARCHIVE_USER_REQUIRED", "需要 Root 管理员用户会话")
	}
	if e := s.check(ctx); e != nil {
		return e
	}
	if s.targetAuthority == nil {
		return failure(503, "ARCHIVE_TARGET_CONTROL_UNAVAILABLE", "目标数据库控制面尚未就绪")
	}
	return nil
}
func (s *Service) TargetConfiguration(ctx context.Context) (TargetConfiguration, error) {
	if e := s.targetControlCheck(ctx); e != nil {
		return TargetConfiguration{}, e
	}
	return s.targetAuthority.Configuration(ctx)
}
func (s *Service) SaveTargetConfiguration(ctx context.Context, in TargetChange) (TargetMutation, error) {
	if e := s.targetControlCheck(ctx); e != nil {
		return TargetMutation{}, e
	}
	return s.targetAuthority.Save(ctx, in)
}
func (s *Service) AdoptTargetConfiguration(ctx context.Context, in TargetAdoption) (TargetMutation, error) {
	if e := s.targetControlCheck(ctx); e != nil {
		return TargetMutation{}, e
	}
	return s.targetAuthority.Adopt(ctx, in)
}
func (s *Service) TargetConfigurationRequest(ctx context.Context, key string) (TargetMutation, error) {
	if e := s.targetControlCheck(ctx); e != nil {
		return TargetMutation{}, e
	}
	return s.targetAuthority.Request(ctx, key)
}

// LegacyTargets 只读保留原文件；接管不改变旧凭据、策略或其启用值。
func (s *Service) LegacyTargets(ctx context.Context) ([]Target, error) {
	ids, e := s.records("targets")
	if e != nil {
		return nil, e
	}
	if len(ids) > 128 {
		return nil, failure(503, "ARCHIVE_TARGET_CATALOG_LIMIT", "目标数量超出接管上限")
	}
	out := []Target{}
	for _, id := range ids {
		var t Target
		if e = readJSON(s.path("targets", id), &t); e != nil {
			return nil, e
		}
		if t.ID != id || t.Revision < 0 {
			return nil, failure(503, "ARCHIVE_TARGET_INVALID", "旧目标身份或版本无效")
		}
		out = append(out, t)
	}
	return out, nil
}
func (s *Service) targetWithProof(t Target) Target {
	raw, e := vault.ReadPrivateFile(s.path("target-verifications", t.ID), 1<<20)
	if e != nil {
		return t
	}
	var proof Target
	if json.Unmarshal(raw, &proof) != nil || proof.ID != t.ID || proof.Revision != t.Revision || proof.ControlDigest != t.ControlDigest || SettingsOfTarget(proof) != SettingsOfTarget(t) {
		return t
	}
	t.VerifiedAt, t.VerifiedFingerprint, t.Executor = proof.VerifiedAt, proof.VerifiedFingerprint, proof.Executor
	t.VerificationValid = s.targetVerified(t)
	return t
}
func (s *Service) confirmTargetVerification(ctx context.Context, t Target, before string) error {
	return s.targetAuthority.Confirm(ctx, t, func(current Target) error {
		after, e := s.targetFingerprint(current)
		if e != nil || before != after {
			return failure(409, "ARCHIVE_REVISION_CONFLICT", "测试期间目标或凭据变化，请重新验证")
		}
		unlock, e := s.lock(ctx, "target-proof")
		if e != nil {
			return ErrBusy
		}
		defer unlock()
		dir := filepath.Join(s.cfg.Directory, "target-verifications")
		st, e := os.Lstat(dir)
		if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
			return failure(503, "ARCHIVE_TARGET_PROOF_UNAVAILABLE", "验证账本目录不可用")
		}
		stat, ok := st.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Geteuid() {
			return failure(503, "ARCHIVE_TARGET_PROOF_UNAVAILABLE", "验证账本目录属主不符")
		}
		now := time.Now().UTC()
		current.Executor = "powerx:" + s.cfg.ExecutionNodeID
		current.VerifiedAt = &now
		current.VerifiedFingerprint = after
		if e = writeJSON(s.path("target-verifications", t.ID), current); e != nil {
			return e
		}
		f, e := os.Open(filepath.Join(s.cfg.Directory, "target-verifications"))
		if e != nil {
			return e
		}
		defer f.Close()
		if e = f.Sync(); e != nil {
			return e
		}
		return s.auditAction(ctx, "target_verified", current.ID, current.Revision)
	})
}

// 私有绑定只禁止新独立 worker 回退旧 JSON，不包含配置或凭据值。
type targetControlBinding struct {
	Version  int    `json:"version"`
	Node     string `json:"node"`
	Instance string `json:"instance"`
}

func (s *Service) readTargetControlBinding() error {
	raw, e := vault.ReadPrivateFile(filepath.Join(s.cfg.Directory, "target-control-binding.json"), 4096)
	if errors.Is(e, vault.ErrNotFound) {
		return nil
	}
	if e != nil {
		return failure(503, "ARCHIVE_TARGET_BINDING_UNAVAILABLE", "目标控制面绑定不可读取")
	}
	var binding targetControlBinding
	if json.Unmarshal(raw, &binding) != nil || binding != (targetControlBinding{1, s.cfg.ExecutionNodeID, s.cfg.Instance}) {
		return failure(503, "ARCHIVE_TARGET_BINDING_UNAVAILABLE", "目标控制面绑定身份不符")
	}
	s.cfg.TargetControlRequired = true
	return nil
}
func (s *Service) writeTargetControlBinding() error {
	if e := writeJSON(filepath.Join(s.cfg.Directory, "target-control-binding.json"), targetControlBinding{1, s.cfg.ExecutionNodeID, s.cfg.Instance}); e != nil {
		return e
	}
	f, e := os.Open(s.cfg.Directory)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
