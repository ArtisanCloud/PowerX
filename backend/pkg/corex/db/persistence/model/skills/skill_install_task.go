package skills

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	SkillInstallStatusPending = "pending"
	SkillInstallStatusRunning = "running"
	SkillInstallStatusSuccess = "success"
	SkillInstallStatusFailed  = "failed"
)

// SkillInstallTask stores one async third-party skill installation run.
type SkillInstallTask struct {
	coremodel.PowerUUIDModel

	TaskID       string     `gorm:"column:task_id;type:varchar(128);not null;uniqueIndex:uk_skill_install_task_id" json:"task_id"`
	TenantUUID   string     `gorm:"column:tenant_uuid;type:char(36);not null;default:'';index:idx_skill_install_tenant" json:"tenant_uuid"`
	Provider     string     `gorm:"column:provider;type:varchar(32);not null;default:'github';index:idx_skill_install_provider" json:"provider"`
	Repo         string     `gorm:"column:repo;type:varchar(256);not null;index:idx_skill_install_repo_path" json:"repo"`
	RepoURL      string     `gorm:"column:repo_url;type:text" json:"repo_url,omitempty"`
	SourcePath   string     `gorm:"column:source_path;type:varchar(512);not null;index:idx_skill_install_repo_path" json:"source_path"`
	Ref          string     `gorm:"column:source_ref;type:varchar(128);not null;default:'main'" json:"source_ref"`
	Method       string     `gorm:"column:method;type:varchar(32);not null;default:'auto'" json:"method"`
	Source       string     `gorm:"column:source;type:varchar(32);not null;default:'third_party'" json:"source"`
	SkillID      string     `gorm:"column:skill_id;type:varchar(128);index:idx_skill_install_skill" json:"skill_id,omitempty"`
	Version      string     `gorm:"column:version;type:varchar(64);not null;default:'1.0.0'" json:"version"`
	InstallPath  string     `gorm:"column:install_path;type:text" json:"install_path,omitempty"`
	Status       string     `gorm:"column:status;type:varchar(32);not null;index:idx_skill_install_status" json:"status"`
	StdoutLog    string     `gorm:"column:stdout_log;type:text" json:"stdout_log,omitempty"`
	StderrLog    string     `gorm:"column:stderr_log;type:text" json:"stderr_log,omitempty"`
	ErrorSummary string     `gorm:"column:error_summary;type:text" json:"error_summary,omitempty"`
	RequestedBy  string     `gorm:"column:requested_by;type:varchar(128)" json:"requested_by,omitempty"`
	StartedAt    *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt   *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
}

func (SkillInstallTask) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsInstallTasks
}

func (t *SkillInstallTask) Normalize() {
	t.TaskID = strings.TrimSpace(t.TaskID)
	t.TenantUUID = strings.TrimSpace(strings.ToLower(t.TenantUUID))
	t.Provider = strings.TrimSpace(strings.ToLower(t.Provider))
	t.Repo = strings.TrimSpace(strings.ToLower(t.Repo))
	t.RepoURL = strings.TrimSpace(t.RepoURL)
	t.SourcePath = strings.TrimSpace(t.SourcePath)
	t.Ref = strings.TrimSpace(t.Ref)
	t.Method = strings.TrimSpace(strings.ToLower(t.Method))
	t.Source = strings.TrimSpace(strings.ToLower(t.Source))
	t.SkillID = strings.TrimSpace(strings.ToLower(t.SkillID))
	t.Version = strings.TrimSpace(t.Version)
	t.InstallPath = strings.TrimSpace(t.InstallPath)
	t.Status = strings.TrimSpace(strings.ToLower(t.Status))
	t.RequestedBy = strings.TrimSpace(t.RequestedBy)
}
