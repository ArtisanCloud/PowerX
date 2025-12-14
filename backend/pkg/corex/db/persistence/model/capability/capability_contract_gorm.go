package capability

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CapabilityContract 定义能力契约主体信息。
type CapabilityContract struct {
	coremodel.PowerModel

	TenantUUID            string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_capability_contract_tenant_key_version,priority:1;index:idx_capability_contract_tenant_uuid;uniqueIndex:uk_capability_contract_tenant_key_version,priority:1" json:"tenant_uuid"`
	CapabilityKey         string         `gorm:"column:capability_key;type:varchar(128);not null;index:idx_capability_contract_tenant_key_version,priority:2;index:idx_capability_contract_key;uniqueIndex:uk_capability_contract_tenant_key_version,priority:2" json:"capability_key"`
	Version               string         `gorm:"column:version;type:varchar(32);not null;index:idx_capability_contract_tenant_key_version,priority:3;uniqueIndex:uk_capability_contract_tenant_key_version,priority:3" json:"version"`
	ProviderID            string         `gorm:"column:provider_id;type:varchar(128);not null;index:idx_capability_contract_provider" json:"provider_id"`
	DisplayName           string         `gorm:"column:display_name;type:varchar(128);not null" json:"display_name"`
	Description           string         `gorm:"column:description;type:text" json:"description,omitempty"`
	SecurityScope         string         `gorm:"column:security_scope;type:varchar(128);not null" json:"security_scope"`
	ToolGrantRequired     bool           `gorm:"column:tool_grant_required;default:false" json:"tool_grant_required"`
	ObservabilityConfig   datatypes.JSON `gorm:"column:observability_config;type:jsonb;default:'{}'" json:"observability_config,omitempty"`
	LifecycleState        string         `gorm:"column:lifecycle_state;type:varchar(32);not null;index:idx_capability_contract_lifecycle" json:"lifecycle_state"`
	EffectiveAt           *time.Time     `gorm:"column:effective_at" json:"effective_at,omitempty"`
	DeprecatedAt          *time.Time     `gorm:"column:deprecated_at" json:"deprecated_at,omitempty"`
	ReplacementCapability string         `gorm:"column:replacement_capability;type:varchar(128)" json:"replacement_capability,omitempty"`
	TransportPreferences  datatypes.JSON `gorm:"column:transport_preferences;type:jsonb;default:'{}'" json:"transport_preferences,omitempty"`
	CreatedBy             string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy             string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
	Status                int16          `gorm:"column:status;default:1;index" json:"status"`
	ContractUUID          uuid.UUID      `gorm:"column:contract_uuid;type:uuid;not null;uniqueIndex" json:"contract_uuid"`

	IOSchemas         []*CapabilityIOSchema              `gorm:"foreignKey:ContractID" json:"io_schemas,omitempty"`
	ErrorBindings     []*CapabilityContractErrorTaxonomy `gorm:"foreignKey:ContractID" json:"error_bindings,omitempty"`
	TransportProfiles []*CapabilityTransportProfile      `gorm:"foreignKey:ContractID" json:"transport_profiles,omitempty"`
}

func (m *CapabilityContract) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityContract
}

func (m *CapabilityContract) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableCapabilityContract
}

func (m *CapabilityContract) BeforeCreate(tx *gorm.DB) error {
	if m.ContractUUID == uuid.Nil {
		m.ContractUUID = uuid.New()
	}
	return nil
}

// CapabilityIOSchema 记录输入/输出的 Schema 描述。
type CapabilityIOSchema struct {
	coremodel.PowerModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_capability_io_schema_tenant_contract,priority:1" json:"tenant_uuid"`
	ContractID      uint64         `gorm:"column:contract_id;not null;index:idx_capability_io_schema_tenant_contract,priority:2" json:"contract_id"`
	Direction       string         `gorm:"column:direction;type:varchar(16);not null;index:idx_capability_io_schema_contract_direction,priority:3" json:"direction"`
	Format          string         `gorm:"column:format;type:varchar(32);not null" json:"format"`
	SchemaURI       string         `gorm:"column:schema_uri;type:text" json:"schema_uri,omitempty"`
	SchemaHash      string         `gorm:"column:schema_hash;type:varchar(128)" json:"schema_hash,omitempty"`
	ValidationRules datatypes.JSON `gorm:"column:validation_rules;type:jsonb;default:'{}'" json:"validation_rules,omitempty"`
	Status          int16          `gorm:"column:status;default:1;index" json:"status"`
}

func (m *CapabilityIOSchema) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityIOSchema
}

func (m *CapabilityIOSchema) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableCapabilityIOSchema
}

// CapabilityErrorTaxonomy 定义标准化错误条目，可跨契约复用。
type CapabilityErrorTaxonomy struct {
	coremodel.PowerModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_capability_error_taxonomy_ns_code,priority:1" json:"tenant_uuid"`
	Namespace       string         `gorm:"column:namespace;type:varchar(64);not null;index:idx_capability_error_taxonomy_ns_code,priority:2" json:"namespace"`
	Category        string         `gorm:"column:category;type:varchar(64);not null;index:idx_capability_error_taxonomy_ns_code,priority:3" json:"category"`
	Code            string         `gorm:"column:code;type:varchar(128);not null;index:idx_capability_error_taxonomy_ns_code,priority:4" json:"code"`
	Severity        string         `gorm:"column:severity;type:varchar(32);not null" json:"severity"`
	Stage           string         `gorm:"column:stage;type:varchar(32);not null" json:"stage"`
	HTTPStatus      int            `gorm:"column:http_status" json:"http_status"`
	GRPCStatus      int            `gorm:"column:grpc_status" json:"grpc_status"`
	SuggestedAction string         `gorm:"column:suggested_action;type:text" json:"suggested_action,omitempty"`
	TelemetryTags   datatypes.JSON `gorm:"column:telemetry_tags;type:jsonb;default:'{}'" json:"telemetry_tags,omitempty"`
	Status          int16          `gorm:"column:status;default:1;index" json:"status"`
	ErrorCodeUUID   uuid.UUID      `gorm:"column:error_uuid;type:uuid;not null;uniqueIndex:uk_capability_error_uuid" json:"error_uuid"`
}

func (m *CapabilityErrorTaxonomy) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityErrorTaxonomy
}

func (m *CapabilityErrorTaxonomy) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableCapabilityErrorTaxonomy
}

func (m *CapabilityErrorTaxonomy) BeforeCreate(tx *gorm.DB) error {
	if m.ErrorCodeUUID == uuid.Nil {
		m.ErrorCodeUUID = uuid.New()
	}
	return nil
}

// CapabilityContractErrorTaxonomy 记录契约与错误条目的关联。
type CapabilityContractErrorTaxonomy struct {
	coremodel.PowerModel

	TenantUUID      string `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_capability_contract_error_rel,priority:1;uniqueIndex:uk_capability_contract_error,priority:1" json:"tenant_uuid"`
	ContractID      uint64 `gorm:"column:contract_id;not null;index:idx_capability_contract_error_rel,priority:2;uniqueIndex:uk_capability_contract_error,priority:2" json:"contract_id"`
	ErrorTaxonomyID uint64 `gorm:"column:error_taxonomy_id;not null;index:idx_capability_contract_error_rel,priority:3;uniqueIndex:uk_capability_contract_error,priority:3" json:"error_taxonomy_id"`

	ErrorTaxonomy *CapabilityErrorTaxonomy `gorm:"foreignKey:ErrorTaxonomyID;references:ID" json:"error_taxonomy,omitempty"`
}

func (m *CapabilityContractErrorTaxonomy) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityContractErrorTaxonomy
}

func (m *CapabilityContractErrorTaxonomy) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableCapabilityContractErrorTaxonomy
}
