package integration_gateway

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IntegrationRouteVersion 保存路由配置的不可变快照。
type IntegrationRouteVersion struct {
	coremodel.PowerUUIDModel

	RouteUUID     uuid.UUID      `gorm:"column:route_uuid;type:uuid;not null;index:idx_integration_route_version,priority:1" json:"route_uuid"`
	Version       uint32         `gorm:"column:version;not null;index:idx_integration_route_version,priority:2" json:"version"`
	Snapshot      datatypes.JSON `gorm:"column:snapshot;type:jsonb;not null" json:"snapshot"`
	ChangeType    string         `gorm:"column:change_type;type:varchar(32);not null" json:"change_type"`
	ChangeSummary string         `gorm:"column:change_summary;type:text" json:"change_summary,omitempty"`
	ChangedBy     string         `gorm:"column:changed_by;type:varchar(128)" json:"changed_by,omitempty"`
	TraceID       string         `gorm:"column:trace_id;type:varchar(128)" json:"trace_id,omitempty"`
	ChangedAt     time.Time      `gorm:"column:changed_at;not null" json:"changed_at"`
}

func (IntegrationRouteVersion) TableName() string {
	// SQLite 在内存模式下不支持 schema 前缀
	// 在测试中，如果 PowerXSchema 为 "main"，则返回不带前缀的表名
	if coremodel.PowerXSchema == "main" {
		return coremodel.TableIntegrationGatewayRouteVersion
	}
	return coremodel.PowerXSchema + "." + coremodel.TableIntegrationGatewayRouteVersion
}
