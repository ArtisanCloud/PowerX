package workflow

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// WorkflowDefinition 持久化工作流定义的版本化蓝图。
type WorkflowDefinition struct {
	coremodel.PowerUUIDModel

	TenantID             uint64         `gorm:"column:tenant_id;type:bigint;not null;index:idx_workflow_definitions_tenant;index:idx_workflow_definitions_alias_tenant,priority:1" json:"tenant_id"`
	Name                 string         `gorm:"column:name;type:varchar(128);not null;index:idx_workflow_definitions_tenant_name_version,priority:2" json:"name"`
	Description          string         `gorm:"column:description;type:text" json:"description,omitempty"`
	Version              int32          `gorm:"column:version;type:int;not null;default:1;index:idx_workflow_definitions_tenant_name_version,priority:3" json:"version"`
	Status               string         `gorm:"column:status;type:varchar(32);not null;default:'draft';index:idx_workflow_definitions_status" json:"status"`
	StepGraph            datatypes.JSON `gorm:"column:step_graph;type:jsonb;not null" json:"step_graph"`
	DefaultRetryPolicy   datatypes.JSON `gorm:"column:default_retry_policy;type:jsonb;not null;default:'{}'::jsonb" json:"default_retry_policy,omitempty"`
	CompensationPolicy   datatypes.JSON `gorm:"column:compensation_policy;type:jsonb;not null;default:'{}'::jsonb" json:"compensation_policy,omitempty"`
	SlaPolicy            datatypes.JSON `gorm:"column:sla_policy;type:jsonb;not null;default:'{}'::jsonb" json:"sla_policy,omitempty"`
	Metadata             datatypes.JSON `gorm:"column:metadata;type:jsonb;not null;default:'{}'::jsonb" json:"metadata,omitempty"`
	CreatedBy            uuid.UUID      `gorm:"column:created_by;type:uuid;not null" json:"created_by"`
	PublishedAt          *time.Time     `gorm:"column:published_at;type:timestamp with time zone" json:"published_at,omitempty"`
	ArchivedAt           *time.Time     `gorm:"column:archived_at;type:timestamp with time zone" json:"archived_at,omitempty"`
	LastPublishedBy      uuid.UUID      `gorm:"column:last_published_by;type:uuid" json:"last_published_by,omitempty"`
	LastChangeNote       string         `gorm:"column:last_change_note;type:varchar(256)" json:"last_change_note,omitempty"`
	VersionAlias         string         `gorm:"column:version_alias;type:varchar(64);index:idx_workflow_definitions_alias_tenant,priority:2" json:"version_alias,omitempty"`
	InitialContextSchema datatypes.JSON `gorm:"column:initial_context_schema;type:jsonb;not null;default:'{}'::jsonb" json:"initial_context_schema,omitempty"`
}

func (WorkflowDefinition) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableWorkflowDefinitions
}

func (m *WorkflowDefinition) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableWorkflowDefinitions
}

func (m *WorkflowDefinition) BeforeCreate(tx *gorm.DB) error {
	if err := m.PowerUUIDModel.BeforeCreate(tx); err != nil {
		return err
	}
	if m.Version <= 0 {
		m.Version = 1
	}
	if m.Status == "" {
		m.Status = "draft"
	}
	if m.CreatedBy == uuid.Nil {
		return errors.New("created_by is required")
	}
	return nil
}
