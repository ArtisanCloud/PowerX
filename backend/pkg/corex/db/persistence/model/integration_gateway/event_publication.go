package integration_gateway

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IntegrationEventPublication 描述待发布或已发布的事件消息。
type IntegrationEventPublication struct {
	coremodel.PowerUUIDModel

	RouteUUID   uuid.UUID      `gorm:"column:route_uuid;type:uuid;not null;index:idx_integration_event_route" json:"route_uuid"`
	TenantUUID  string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_integration_event_tenant" json:"tenant_uuid"`
	Topic       string         `gorm:"column:topic;type:varchar(256);not null;index:idx_integration_event_topic" json:"topic"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb;not null" json:"payload"`
	Status      string         `gorm:"column:status;type:varchar(32);not null;index:idx_integration_event_status" json:"status"`
	Attempts    int            `gorm:"column:attempts;not null;default:0" json:"attempts"`
	LastError   string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	TraceID     string         `gorm:"column:trace_id;type:varchar(128);not null;index:idx_integration_event_trace" json:"trace_id"`
	PublishTime *time.Time     `gorm:"column:publish_time" json:"publish_time,omitempty"`
}

func (IntegrationEventPublication) TableName() string {
	// SQLite 在内存模式下不支持 schema 前缀
	// 在测试中，如果 PowerXSchema 为 "main"，则返回不带前缀的表名
	if coremodel.PowerXSchema == "main" {
		return coremodel.TableIntegrationGatewayEventPublication
	}
	return coremodel.PowerXSchema + "." + coremodel.TableIntegrationGatewayEventPublication
}
