package capability_registry

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CapabilityRecord 记录插件上报后的能力目录条目。
type CapabilityRecord struct {
	coremodel.PowerModel

	UUID uuid.UUID `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"uuid"`

	CapabilityID  string         `gorm:"column:capability_id;type:varchar(128);not null;uniqueIndex:uk_capability_record_capability" json:"capability_id"`
	PluginID      string         `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_capability_record_plugin" json:"plugin_id"`
	PluginVersion string         `gorm:"column:plugin_version;type:varchar(64);not null" json:"plugin_version"`
	Title         string         `gorm:"column:title;type:varchar(128);not null" json:"title"`
	Description   string         `gorm:"column:description;type:text" json:"description,omitempty"`
	Categories    datatypes.JSON `gorm:"column:categories;type:jsonb;default:'[]'" json:"categories,omitempty"`
	Intents       datatypes.JSON `gorm:"column:intents;type:jsonb;default:'[]'" json:"intents,omitempty"`
	ToolScope     datatypes.JSON `gorm:"column:tool_scope;type:jsonb;default:'[]'" json:"tool_scope,omitempty"`
	Protocols     datatypes.JSON `gorm:"column:protocols;type:jsonb;default:'[]'" json:"protocols,omitempty"`
	Policy        datatypes.JSON `gorm:"column:policy;type:jsonb;default:'{}'" json:"policy,omitempty"`

	WorkflowTemplateRefs datatypes.JSON `gorm:"column:workflow_template_refs;type:jsonb;default:'[]'" json:"workflow_template_refs,omitempty"`
	CompositeGraphs      datatypes.JSON `gorm:"column:composite_graphs;type:jsonb;default:'[]'" json:"composite_graphs,omitempty"`
	Annotations          datatypes.JSON `gorm:"column:annotations;type:jsonb;default:'{}'" json:"annotations,omitempty"`

	CapabilitiesHash string     `gorm:"column:capabilities_hash;type:varchar(128);not null;index:idx_capability_record_hash" json:"capabilities_hash"`
	ProtocolHash     string     `gorm:"column:protocol_hash;type:varchar(128);not null" json:"protocol_hash"`
	Status           string     `gorm:"column:status;type:varchar(32);not null;index:idx_capability_record_status" json:"status"`
	PublishedAt      *time.Time `gorm:"column:published_at" json:"published_at,omitempty"`
	CreatedBy        string     `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy        string     `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (CapabilityRecord) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryCapabilityRecord
}

func (r *CapabilityRecord) BeforeCreate(tx *gorm.DB) error {
	if r.UUID == uuid.Nil {
		r.UUID = uuid.New()
	}
	return nil
}

// ProtocolBinding 描述能力在各协议下的接入详情。
type ProtocolBinding struct {
	Channel      string `json:"channel"`
	Endpoint     string `json:"endpoint,omitempty"`
	SchemaRef    string `json:"schema_ref,omitempty"`
	Method       string `json:"method,omitempty"`
	RPC          string `json:"rpc,omitempty"`
	ToolRef      string `json:"tool_ref,omitempty"`
	ToolScope    string `json:"tool_scope,omitempty"`
	AuthType     string `json:"auth_type,omitempty"`
	HealthState  string `json:"health_state,omitempty"`
	HealthReason string `json:"health_reason,omitempty"`
	LatencyP95MS int    `json:"latency_p95_ms,omitempty"`
	ErrorRate    int    `json:"error_rate,omitempty"`
	LastChecked  string `json:"last_checked_at,omitempty"`
}
