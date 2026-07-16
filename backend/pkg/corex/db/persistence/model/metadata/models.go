package metadata

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

type DictionaryNamespace struct {
	coremodel.PowerUUIDModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_metadata_dictionary_namespace,priority:1;index:idx_metadata_dictionary_namespace_module_status,priority:1" json:"tenant_uuid"`
	Namespace       string         `gorm:"column:namespace;type:varchar(160);not null;uniqueIndex:uk_metadata_dictionary_namespace,priority:2" json:"namespace"`
	Module          string         `gorm:"column:module;type:varchar(128);not null;index:idx_metadata_dictionary_namespace_module_status,priority:2" json:"module"`
	NameI18n        datatypes.JSON `gorm:"column:name_i18n;type:jsonb;not null" json:"name_i18n"`
	DescriptionI18n datatypes.JSON `gorm:"column:description_i18n;type:jsonb" json:"description_i18n"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'enabled';index:idx_metadata_dictionary_namespace_module_status,priority:3" json:"status"`
}

func (DictionaryNamespace) TableName() string {
	return tableName(TableDictionaryNamespaces)
}

type DictionaryItem struct {
	coremodel.PowerUUIDModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_metadata_dictionary_item,priority:1;index:idx_metadata_dictionary_item_namespace_status,priority:1" json:"tenant_uuid"`
	NamespaceUUID   string         `gorm:"column:namespace_uuid;type:uuid;not null;uniqueIndex:uk_metadata_dictionary_item,priority:2;index:idx_metadata_dictionary_item_namespace_status,priority:2" json:"namespace_uuid"`
	Code            string         `gorm:"column:code;type:varchar(128);not null;uniqueIndex:uk_metadata_dictionary_item,priority:3" json:"code"`
	LabelI18n       datatypes.JSON `gorm:"column:label_i18n;type:jsonb;not null" json:"label_i18n"`
	DescriptionI18n datatypes.JSON `gorm:"column:description_i18n;type:jsonb" json:"description_i18n"`
	SortOrder       int            `gorm:"column:sort_order;not null;default:0;index:idx_metadata_dictionary_item_namespace_status,priority:4" json:"sort_order"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'enabled';index:idx_metadata_dictionary_item_namespace_status,priority:3" json:"status"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb;not null" json:"metadata"`
	ReferenceCount  int64          `gorm:"column:reference_count;not null;default:0" json:"reference_count"`
}

func (DictionaryItem) TableName() string {
	return tableName(TableDictionaryItems)
}

type Taxonomy struct {
	coremodel.PowerUUIDModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_metadata_taxonomy,priority:1;index:idx_metadata_taxonomy_module_status,priority:1" json:"tenant_uuid"`
	Namespace       string         `gorm:"column:namespace;type:varchar(160);not null;uniqueIndex:uk_metadata_taxonomy,priority:2" json:"namespace"`
	Module          string         `gorm:"column:module;type:varchar(128);not null;index:idx_metadata_taxonomy_module_status,priority:2" json:"module"`
	NameI18n        datatypes.JSON `gorm:"column:name_i18n;type:jsonb;not null" json:"name_i18n"`
	DescriptionI18n datatypes.JSON `gorm:"column:description_i18n;type:jsonb" json:"description_i18n"`
	MaxDepth        int            `gorm:"column:max_depth;not null;default:1" json:"max_depth"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'enabled';index:idx_metadata_taxonomy_module_status,priority:3" json:"status"`
}

func (Taxonomy) TableName() string {
	return tableName(TableTaxonomies)
}

type TaxonomyNode struct {
	coremodel.PowerUUIDModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_metadata_taxonomy_node,priority:1;index:idx_metadata_taxonomy_node_parent_sort,priority:1" json:"tenant_uuid"`
	TaxonomyUUID    string         `gorm:"column:taxonomy_uuid;type:uuid;not null;uniqueIndex:uk_metadata_taxonomy_node,priority:2;index:idx_metadata_taxonomy_node_parent_sort,priority:2" json:"taxonomy_uuid"`
	ParentUUID      *string        `gorm:"column:parent_uuid;type:uuid;index:idx_metadata_taxonomy_node_parent_sort,priority:3" json:"parent_uuid"`
	Code            string         `gorm:"column:code;type:varchar(128);not null;uniqueIndex:uk_metadata_taxonomy_node,priority:3" json:"code"`
	LabelI18n       datatypes.JSON `gorm:"column:label_i18n;type:jsonb;not null" json:"label_i18n"`
	DescriptionI18n datatypes.JSON `gorm:"column:description_i18n;type:jsonb" json:"description_i18n"`
	Path            string         `gorm:"column:path;type:text;not null;index" json:"path"`
	Depth           int            `gorm:"column:depth;not null;default:1" json:"depth"`
	SortOrder       int            `gorm:"column:sort_order;not null;default:0;index:idx_metadata_taxonomy_node_parent_sort,priority:4" json:"sort_order"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'enabled';index" json:"status"`
	ReferenceCount  int64          `gorm:"column:reference_count;not null;default:0" json:"reference_count"`
	Version         int64          `gorm:"column:version;not null;default:1" json:"version"`
}

func (TaxonomyNode) TableName() string {
	return tableName(TableTaxonomyNodes)
}

type Tag struct {
	coremodel.PowerUUIDModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_metadata_tag,priority:1;index:idx_metadata_tag_resource_status,priority:1" json:"tenant_uuid"`
	Namespace       string         `gorm:"column:namespace;type:varchar(160);not null;uniqueIndex:uk_metadata_tag,priority:2" json:"namespace"`
	ResourceType    string         `gorm:"column:resource_type;type:varchar(160);not null;uniqueIndex:uk_metadata_tag,priority:3;index:idx_metadata_tag_resource_status,priority:2" json:"resource_type"`
	Code            string         `gorm:"column:code;type:varchar(128);not null;uniqueIndex:uk_metadata_tag,priority:4" json:"code"`
	LabelI18n       datatypes.JSON `gorm:"column:label_i18n;type:jsonb;not null" json:"label_i18n"`
	DescriptionI18n datatypes.JSON `gorm:"column:description_i18n;type:jsonb" json:"description_i18n"`
	Color           string         `gorm:"column:color;type:varchar(32)" json:"color"`
	Source          string         `gorm:"column:source;type:varchar(32);not null;default:'admin';index" json:"source"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'enabled';index:idx_metadata_tag_resource_status,priority:3" json:"status"`
	UsageCount      int64          `gorm:"column:usage_count;not null;default:0" json:"usage_count"`
}

func (Tag) TableName() string {
	return tableName(TableTags)
}

type TagBinding struct {
	TenantUUID    string    `gorm:"column:tenant_uuid;type:uuid;not null;primaryKey;index:idx_metadata_tag_binding_resource,priority:1" json:"tenant_uuid"`
	TagUUID       string    `gorm:"column:tag_uuid;type:uuid;not null;primaryKey;index" json:"tag_uuid"`
	ResourceType  string    `gorm:"column:resource_type;type:varchar(160);not null;primaryKey;index:idx_metadata_tag_binding_resource,priority:2" json:"resource_type"`
	ResourceUUID  string    `gorm:"column:resource_uuid;type:uuid;not null;primaryKey;index:idx_metadata_tag_binding_resource,priority:3" json:"resource_uuid"`
	CreatedByUUID string    `gorm:"column:created_by_uuid;type:uuid;index" json:"created_by_uuid"`
	CreatedAt     time.Time `gorm:"column:created_at; ->;<-:create" json:"created_at"`
}

func (TagBinding) TableName() string {
	return tableName(TableTagBindings)
}

type ResourceType struct {
	coremodel.PowerUUIDModel

	TenantUUID      string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_metadata_resource_type,priority:1;index:idx_metadata_resource_type_module_status,priority:1" json:"tenant_uuid"`
	ResourceType    string         `gorm:"column:resource_type;type:varchar(160);not null;uniqueIndex:uk_metadata_resource_type,priority:2" json:"resource_type"`
	Module          string         `gorm:"column:module;type:varchar(128);not null;index:idx_metadata_resource_type_module_status,priority:2" json:"module"`
	NameI18n        datatypes.JSON `gorm:"column:name_i18n;type:jsonb;not null" json:"name_i18n"`
	DescriptionI18n datatypes.JSON `gorm:"column:description_i18n;type:jsonb" json:"description_i18n"`
	ValidatorKey    string         `gorm:"column:validator_key;type:varchar(160)" json:"validator_key"`
	BindingEnabled  bool           `gorm:"column:binding_enabled;not null;default:false" json:"binding_enabled"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'enabled';index:idx_metadata_resource_type_module_status,priority:3" json:"status"`
}

func (ResourceType) TableName() string {
	return tableName(TableResourceTypes)
}

type Reference struct {
	TenantUUID   string    `gorm:"column:tenant_uuid;type:uuid;not null;primaryKey;index:idx_metadata_reference_metadata,priority:1;index:idx_metadata_reference_resource,priority:1" json:"tenant_uuid"`
	MetadataType string    `gorm:"column:metadata_type;type:varchar(64);not null;primaryKey;index:idx_metadata_reference_metadata,priority:2" json:"metadata_type"`
	MetadataUUID string    `gorm:"column:metadata_uuid;type:uuid;not null;primaryKey;index:idx_metadata_reference_metadata,priority:3" json:"metadata_uuid"`
	ResourceType string    `gorm:"column:resource_type;type:varchar(160);not null;primaryKey;index:idx_metadata_reference_resource,priority:2" json:"resource_type"`
	ResourceUUID string    `gorm:"column:resource_uuid;type:uuid;not null;primaryKey;index:idx_metadata_reference_resource,priority:3" json:"resource_uuid"`
	FieldName    string    `gorm:"column:field_name;type:varchar(128);not null;primaryKey" json:"field_name"`
	CreatedAt    time.Time `gorm:"column:created_at; ->;<-:create" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Reference) TableName() string {
	return tableName(TableReferences)
}

func tableName(name string) string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" || schema == "main" {
		return name
	}
	return schema + "." + name
}
