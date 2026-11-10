package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

// PolicySettings 不含版本、身份、启停或数据库连接凭据。
type PolicySettings struct {
	Name              string `json:"name"`
	Schedule          string `json:"schedule"`
	RetentionCount    int    `json:"retention_count"`
	RetentionDays     int    `json:"retention_days"`
	RetentionMode     string `json:"retention_mode"`
	Timezone          string `json:"timezone"`
	DrillEnabled      bool   `json:"drill_enabled"`
	DrillIntervalDays int    `json:"drill_interval_days"`
	TargetRef         string `json:"target_ref"`
}
type PolicyEntry struct {
	ExecutionFingerprint string         `json:"execution_fingerprint"`
	UUID                 string         `json:"uuid"`
	Revision             int64          `json:"revision"`
	Settings             PolicySettings `json:"settings"`
	Enabled              bool           `json:"enabled"`
	Current              bool           `json:"current"`
}
type PolicyConfiguration struct {
	ExecutionReady   map[string]bool `json:"execution_ready"`
	Authority        string          `json:"authority"`
	State            string          `json:"state"`
	Ready            bool            `json:"ready"`
	DesiredRevision  int64           `json:"desired_revision"`
	AppliedRevision  int64           `json:"applied_revision"`
	SnapshotRevision *int64          `json:"snapshot_revision"`
	LegacyDigest     string          `json:"legacy_digest"`
	Policies         []PolicyEntry   `json:"policies"`
	Problem          string          `json:"problem,omitempty"`
}
type PolicyChange struct {
	RequestID              string          `json:"request_id"`
	ExpectedRevision       int64           `json:"expected_revision"`
	ExpectedPolicyRevision int64           `json:"expected_policy_revision"`
	PolicyUUID             string          `json:"policy_uuid"`
	Action                 string          `json:"action"`
	Settings               *PolicySettings `json:"settings,omitempty"`
	Enabled                *bool           `json:"enabled,omitempty"`
	Confirm                bool            `json:"confirm"`
}
type PolicyAdoption struct {
	RequestID    string `json:"request_id"`
	LegacyDigest string `json:"legacy_digest"`
	Confirm      bool   `json:"confirm"`
}
type PolicyMutation struct {
	RequestID         string              `json:"request_id"`
	OperationRevision int64               `json:"operation_revision"`
	PolicyUUID        string              `json:"policy_uuid"`
	Configuration     PolicyConfiguration `json:"configuration"`
}

// PolicyAuthority 由统一配置域注入；动作只允许有界元数据或受保护文件操作，禁止在 Guard 内运行 pg_dump。
type PolicyAuthority interface {
	Configuration(context.Context) (PolicyConfiguration, error)
	Change(context.Context, PolicyChange) (PolicyMutation, error)
	Adopt(context.Context, PolicyAdoption) (PolicyMutation, error)
	Request(context.Context, string) (PolicyMutation, error)
	Guard(context.Context, uint64, int64, string, func(context.Context, *gorm.DB, *model.BackupPolicy) error) error
}

func SettingsOf(p model.BackupPolicy) PolicySettings {
	return PolicySettings{p.Name, p.Schedule, int(p.RetentionCount), int(p.RetentionDays), p.RetentionMode, p.Timezone, p.DrillEnabled, int(p.DrillIntervalDays), p.TargetRef}
}
func ValidateSettings(p PolicySettings, env string) error {
	if strings.TrimSpace(p.Name) != p.Name || p.Name == "" || utf8.RuneCountInString(p.Name) > 128 || p.RetentionCount < 1 || p.RetentionCount > 10000 || p.RetentionDays < 1 || p.RetentionDays > 3650 || p.DrillIntervalDays < 1 || p.DrillIntervalDays > 3650 || p.TargetRef != "local_dump" || p.DrillEnabled {
		return controlFailure(400, "BACKUP_POLICY_SETTINGS_INVALID")
	}
	d, normalized, e := parseScheduleDurationStrict(p.Schedule)
	if e != nil || normalized != p.Schedule || validateScheduleDurationByEnv(d, env) != nil {
		return controlFailure(400, "BACKUP_POLICY_SETTINGS_INVALID")
	}
	if _, e := time.LoadLocation(p.Timezone); e != nil || p.Timezone == "" || len(p.Timezone) > 64 {
		return controlFailure(400, "BACKUP_POLICY_SETTINGS_INVALID")
	}
	if p.RetentionMode != "count" && p.RetentionMode != "age_and_count" {
		return controlFailure(400, "BACKUP_POLICY_SETTINGS_INVALID")
	}
	return nil
}
func ApplySettings(row *model.BackupPolicy, p PolicySettings) {
	row.Name = p.Name
	row.BackupType = model.BackupTypeLogical
	row.Schedule = p.Schedule
	row.IntervalHours = int32(parseScheduleHours(p.Schedule))
	row.RetentionCount = int32(p.RetentionCount)
	row.RetentionDays = int32(p.RetentionDays)
	row.RetentionMode = p.RetentionMode
	row.Timezone = p.Timezone
	row.DrillEnabled = p.DrillEnabled
	row.DrillIntervalDays = int32(p.DrillIntervalDays)
	row.TargetRef = p.TargetRef
	row.StorageTarget = p.TargetRef
}
func controlFailure(status int, code string) error {
	return dto.NewErrorWithCode(status, code, code, nil)
}
func (s *PolicyService) RequireControl() { s.requireControl = true }
func (s *JobService) RequireControl()    { s.requireControl = true; s.cleanupSvc.requireControl = true }
func (s *PolicyService) BindAuthority(a PolicyAuthority) error {
	if a == nil || s.authority != nil {
		return controlFailure(409, "BACKUP_POLICY_CONTROL_ALREADY_BOUND")
	}
	s.authority = a
	s.requireControl = true
	return nil
}
func (s *JobService) BindAuthority(a PolicyAuthority) error {
	if a == nil || s.authority != nil {
		return controlFailure(409, "BACKUP_POLICY_CONTROL_ALREADY_BOUND")
	}
	s.authority = a
	s.requireControl = true
	s.cleanupSvc.authority = a
	s.cleanupSvc.requireControl = true
	return nil
}
func (s *PolicyService) legacyWriteAllowed(ctx context.Context) error {
	if s.requireControl || s.authority != nil {
		return controlFailure(428, "BACKUP_POLICY_CONTROL_REQUIRED")
	}
	if s.repo != nil && s.repo.DB.Migrator().HasColumn(&model.BackupPolicy{}, "control_scope_id") {
		var n int64
		if e := s.repo.DB.WithContext(ctx).Model(&model.BackupPolicy{}).Where("control_scope_id <> ?", "").Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return controlFailure(428, "BACKUP_POLICY_CONTROL_REQUIRED")
		}
	}
	return nil
}
func (s *PolicyService) ControlConfiguration(ctx context.Context) (PolicyConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return PolicyConfiguration{}, controlFailure(403, "BACKUP_ROOT_REQUIRED")
	}
	if s.authority == nil {
		return PolicyConfiguration{}, controlFailure(503, "BACKUP_POLICY_CONTROL_UNAVAILABLE")
	}
	return s.authority.Configuration(ctx)
}
func (s *PolicyService) ChangeConfiguration(ctx context.Context, in PolicyChange) (PolicyMutation, error) {
	if _, e := s.ControlConfiguration(ctx); e != nil {
		return PolicyMutation{}, e
	}
	return s.authority.Change(ctx, in)
}
func (s *PolicyService) AdoptConfiguration(ctx context.Context, in PolicyAdoption) (PolicyMutation, error) {
	if _, e := s.ControlConfiguration(ctx); e != nil {
		return PolicyMutation{}, e
	}
	return s.authority.Adopt(ctx, in)
}
func (s *PolicyService) ConfigurationRequest(ctx context.Context, id string) (PolicyMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return PolicyMutation{}, controlFailure(403, "BACKUP_ROOT_REQUIRED")
	}
	if s.authority == nil {
		return PolicyMutation{}, controlFailure(503, "BACKUP_POLICY_CONTROL_UNAVAILABLE")
	}
	return s.authority.Request(ctx, id)
}

// ExecutionFingerprint 只返回摘要，不把源 DSN 或文件系统路径写入配置正文。
func (s *JobService) ExecutionFingerprint() string {
	sum := sha256.Sum256([]byte(sourceDSN(s.db) + "\x00" + s.artifactBaseDir + "\x00" + s.scriptDir))
	return hex.EncodeToString(sum[:])
}
