package workflow

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	WorkflowPackInstallationStatusEnabled  = "enabled"
	WorkflowPackInstallationStatusDisabled = "disabled"
	WorkflowPackInstallationStatusDeleted  = "deleted"
)

// WorkflowPackInstallation 记录租户对内置 Workflow Pack 的显式安装状态。
type WorkflowPackInstallation struct {
	coremodel.PowerUUIDModel

	TenantUUID        string     `gorm:"column:tenant_uuid;type:varchar(128);not null;uniqueIndex:uk_workflow_pack_installation_tenant_key,priority:1;index:idx_workflow_pack_installation_tenant_status,priority:1" json:"tenant_uuid"`
	WorkflowKey       string     `gorm:"column:workflow_key;type:varchar(128);not null;uniqueIndex:uk_workflow_pack_installation_tenant_key,priority:2;index:idx_workflow_pack_installation_workflow_key" json:"workflow_key"`
	Version           int32      `gorm:"column:version;type:int;not null;default:1" json:"version"`
	Checksum          string     `gorm:"column:checksum;type:varchar(128);not null" json:"checksum"`
	Status            string     `gorm:"column:status;type:varchar(32);not null;default:'enabled';index:idx_workflow_pack_installation_tenant_status,priority:2" json:"status"`
	DefinitionUUID    uuid.UUID  `gorm:"column:definition_uuid;type:uuid;index:idx_workflow_pack_installation_definition" json:"definition_uuid,omitempty"`
	DefinitionVersion int32      `gorm:"column:definition_version;type:int;not null;default:1" json:"definition_version"`
	Source            string     `gorm:"column:source;type:varchar(32);not null;default:'builtin';index:idx_workflow_pack_installation_source" json:"source"`
	InstalledAt       *time.Time `gorm:"column:installed_at;type:timestamp with time zone" json:"installed_at,omitempty"`
	RemovedAt         *time.Time `gorm:"column:removed_at;type:timestamp with time zone" json:"removed_at,omitempty"`
	RemovedBy         uuid.UUID  `gorm:"column:removed_by;type:uuid" json:"removed_by,omitempty"`
	LastSeededAt      *time.Time `gorm:"column:last_seeded_at;type:timestamp with time zone" json:"last_seeded_at,omitempty"`
	LastAction        string     `gorm:"column:last_action;type:varchar(32);not null;default:'install'" json:"last_action"`
}

func (WorkflowPackInstallation) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowPackInstallations
}

func (m *WorkflowPackInstallation) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowPackInstallations
}

func (m *WorkflowPackInstallation) BeforeCreate(tx *gorm.DB) error {
	if err := m.PowerUUIDModel.BeforeCreate(tx); err != nil {
		return err
	}
	return m.validate()
}

func (m *WorkflowPackInstallation) BeforeSave(tx *gorm.DB) error {
	return m.validate()
}

func (m *WorkflowPackInstallation) validate() error {
	m.TenantUUID = strings.ToLower(strings.TrimSpace(m.TenantUUID))
	m.WorkflowKey = strings.TrimSpace(m.WorkflowKey)
	m.Status = strings.TrimSpace(m.Status)
	m.Source = strings.TrimSpace(m.Source)
	m.LastAction = strings.TrimSpace(m.LastAction)
	if m.TenantUUID == "" {
		return errors.New("tenant_uuid is required")
	}
	if m.WorkflowKey == "" {
		return errors.New("workflow_key is required")
	}
	if m.Version <= 0 {
		m.Version = 1
	}
	if strings.TrimSpace(m.Checksum) == "" {
		return errors.New("checksum is required")
	}
	if m.Status == "" {
		m.Status = WorkflowPackInstallationStatusEnabled
	}
	switch m.Status {
	case WorkflowPackInstallationStatusEnabled, WorkflowPackInstallationStatusDisabled, WorkflowPackInstallationStatusDeleted:
	default:
		return errors.New("invalid workflow pack installation status")
	}
	if m.Status == WorkflowPackInstallationStatusEnabled && m.DefinitionUUID == uuid.Nil {
		return errors.New("definition_uuid is required for enabled workflow pack installation")
	}
	if m.DefinitionVersion <= 0 {
		m.DefinitionVersion = 1
	}
	if m.Source == "" {
		m.Source = "builtin"
	}
	if m.LastAction == "" {
		m.LastAction = "install"
	}
	return nil
}
