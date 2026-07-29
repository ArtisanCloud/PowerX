package workflow

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowPackSeedRecord 记录内置或插件 Workflow Pack seed 与生成定义版本的对应关系。
type WorkflowPackSeedRecord struct {
	coremodel.PowerUUIDModel

	TenantUUID        string    `gorm:"column:tenant_uuid;type:varchar(128);index:idx_workflow_pack_seed_tenant_key_version,priority:1" json:"tenant_uuid,omitempty"`
	WorkflowKey       string    `gorm:"column:workflow_key;type:varchar(128);not null;index:idx_workflow_pack_seed_tenant_key_version,priority:2" json:"workflow_key"`
	Version           int32     `gorm:"column:version;type:int;not null;default:1;index:idx_workflow_pack_seed_tenant_key_version,priority:3" json:"version"`
	DefinitionUUID    uuid.UUID `gorm:"column:definition_uuid;type:uuid;not null;index:idx_workflow_pack_seed_definition" json:"definition_uuid"`
	DefinitionVersion int32     `gorm:"column:definition_version;type:int;not null;default:1" json:"definition_version"`
	Checksum          string    `gorm:"column:checksum;type:varchar(128);not null" json:"checksum"`
	Source            string    `gorm:"column:source;type:varchar(32);not null;default:'builtin';index:idx_workflow_pack_seed_source" json:"source"`
	SeededAt          time.Time `gorm:"column:seeded_at;type:timestamp with time zone;not null;index:idx_workflow_pack_seed_seeded_at" json:"seeded_at"`
}

func (WorkflowPackSeedRecord) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowPackSeedRecords
}

func (m *WorkflowPackSeedRecord) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowPackSeedRecords
}

func (m *WorkflowPackSeedRecord) BeforeCreate(tx *gorm.DB) error {
	if err := m.PowerUUIDModel.BeforeCreate(tx); err != nil {
		return err
	}
	if strings.TrimSpace(m.WorkflowKey) == "" {
		return errors.New("workflow_key is required")
	}
	if m.Version <= 0 {
		m.Version = 1
	}
	if m.DefinitionUUID == uuid.Nil {
		return errors.New("definition_uuid is required")
	}
	if m.DefinitionVersion <= 0 {
		m.DefinitionVersion = 1
	}
	if strings.TrimSpace(m.Checksum) == "" {
		return errors.New("checksum is required")
	}
	if strings.TrimSpace(m.Source) == "" {
		m.Source = "builtin"
	}
	if m.SeededAt.IsZero() {
		m.SeededAt = time.Now().UTC()
	}
	return nil
}
