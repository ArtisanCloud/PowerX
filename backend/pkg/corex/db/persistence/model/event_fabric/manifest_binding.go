package eventfabric

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// TopicManifestBinding 记录 manifest 播种过的 Topic。
type TopicManifestBinding struct {
	coremodel.PowerUUIDModel

	TenantKey     string    `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_topic_bindings_tpk,priority:1;uniqueIndex:uk_event_topic_manifest_binding,priority:1"`
	PluginID      string    `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_event_topic_bindings_tpk,priority:2;uniqueIndex:uk_event_topic_manifest_binding,priority:2"`
	TopicKey      string    `gorm:"column:topic_key;type:varchar(128);not null;index:idx_event_topic_bindings_tpk,priority:3;uniqueIndex:uk_event_topic_manifest_binding,priority:3"`
	Namespace     string    `gorm:"column:namespace;type:varchar(128);not null"`
	Name          string    `gorm:"column:name;type:varchar(128);not null"`
	FullTopic     string    `gorm:"column:full_topic;type:varchar(256);not null"`
	TopicUUID     string    `gorm:"column:topic_uuid;type:varchar(64)"`
	LastAppliedAt time.Time `gorm:"column:last_applied_at"`
}

func (TopicManifestBinding) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventTopicBindings
}

// AclManifestBinding 记录 manifest 播种过的 ACL。
type AclManifestBinding struct {
	coremodel.PowerUUIDModel

	TenantKey     string         `gorm:"column:tenant_key;type:varchar(128);not null;index:idx_event_acl_manifest_binding,priority:1;uniqueIndex:uk_event_acl_manifest_binding,priority:1"`
	PluginID      string         `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_event_acl_manifest_binding,priority:2;uniqueIndex:uk_event_acl_manifest_binding,priority:2"`
	TopicKey      string         `gorm:"column:topic_key;type:varchar(128);not null;index:idx_event_acl_manifest_binding,priority:3;uniqueIndex:uk_event_acl_manifest_binding,priority:3"`
	PrincipalType string         `gorm:"column:principal_type;type:varchar(64);not null;index:idx_event_acl_manifest_binding,priority:4;uniqueIndex:uk_event_acl_manifest_binding,priority:4"`
	PrincipalID   string         `gorm:"column:principal_id;type:varchar(256);not null;index:idx_event_acl_manifest_binding,priority:5;uniqueIndex:uk_event_acl_manifest_binding,priority:5"`
	Actions       datatypes.JSON `gorm:"column:actions;type:jsonb"`
	ActionsHash   string         `gorm:"column:actions_hash;type:varchar(128);not null"`
	LastAppliedAt time.Time      `gorm:"column:last_applied_at"`
}

func (AclManifestBinding) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventAclManifestBindings
}

func NormalizeTenantKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
