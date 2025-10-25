package integration_gateway

import (
	"gorm.io/datatypes"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IntegrationRoute 表示租户可用的集成入口。
type IntegrationRoute struct {
	coremodel.PowerUUIDModel

	TenantID        string         `gorm:"column:tenant_id;type:varchar(128);not null;index:idx_integration_route_tenant_slug,priority:1" json:"tenant_id"`
	RouteSlug       string         `gorm:"column:route_slug;type:varchar(128);not null;index:idx_integration_route_tenant_slug,priority:2" json:"route_slug"`
	CapabilityID    string         `gorm:"column:capability_id;type:varchar(128);not null;index:idx_integration_route_capability" json:"capability_id"`
	ToolGrantIDs    datatypes.JSON `gorm:"column:tool_grant_ids;type:jsonb;default:'[]'" json:"tool_grant_ids,omitempty"`
	Channels        datatypes.JSON `gorm:"column:channels;type:jsonb;default:'[\"http\"]'" json:"channels,omitempty"`
	RateLimit       datatypes.JSON `gorm:"column:rate_limit;type:jsonb;default:'{}'" json:"rate_limit,omitempty"`
	EventTopics     datatypes.JSON `gorm:"column:event_topics;type:jsonb;default:'{}'" json:"event_topics,omitempty"`
	LifecycleState  string         `gorm:"column:lifecycle_state;type:varchar(32);not null;index:idx_integration_route_lifecycle" json:"lifecycle_state"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;index:idx_integration_route_status" json:"status"`
	CurrentVersion  uint32         `gorm:"column:current_version;not null;default:0" json:"current_version"`
	Description     string         `gorm:"column:description;type:text" json:"description,omitempty"`
	CreatedBy       string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy       string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
	LastActivityAt  *time.Time     `gorm:"column:last_activity_at" json:"last_activity_at,omitempty"`
	LastPublishedAt *time.Time     `gorm:"column:last_published_at" json:"last_published_at,omitempty"`

	Versions []IntegrationRouteVersion `gorm:"foreignKey:RouteUUID;references:UUID" json:"versions,omitempty"`
}

func (IntegrationRoute) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIntegrationGatewayRoute
}

// EnsureUniqueConstraint 返回联合唯一索引字段。
func (IntegrationRoute) EnsureUniqueConstraint() []string {
	return []string{"tenant_id", "route_slug"}
}
