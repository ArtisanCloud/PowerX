package integration_gateway

import (
	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IntegrationInvocationLog 记录每次调用的执行轨迹与审计摘要。
type IntegrationInvocationLog struct {
	coremodel.PowerUUIDModel

	RouteUUID          uuid.UUID      `gorm:"column:route_uuid;type:uuid;not null;index:idx_integration_invocation_route" json:"route_uuid"`
	TenantID           string         `gorm:"column:tenant_id;type:varchar(128);not null;index:idx_integration_invocation_tenant" json:"tenant_id"`
	TraceID            string         `gorm:"column:trace_id;type:varchar(128);not null;index:idx_integration_invocation_trace" json:"trace_id"`
	Status             string         `gorm:"column:status;type:varchar(32);not null;index:idx_integration_invocation_status" json:"status"`
	DurationMS         int            `gorm:"column:duration_ms;not null" json:"duration_ms"`
	RequestPayload     datatypes.JSON `gorm:"column:request_payload;type:jsonb;default:'{}'" json:"request_payload,omitempty"`
	ResponsePayload    datatypes.JSON `gorm:"column:response_payload;type:jsonb;default:'{}'" json:"response_payload,omitempty"`
	RoutedCapabilityID string         `gorm:"column:routed_capability_id;type:varchar(128)" json:"routed_capability_id,omitempty"`
	RoutedAdapter      string         `gorm:"column:routed_adapter;type:varchar(128)" json:"routed_adapter,omitempty"`
	EventPublished     bool           `gorm:"column:event_published;not null;default:false" json:"event_published"`
	ErrorCode          string         `gorm:"column:error_code;type:varchar(64)" json:"error_code,omitempty"`
	ErrorMessage       string         `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
}

func (IntegrationInvocationLog) TableName() string {
	// SQLite 在内存模式下不支持 schema 前缀
	// 在测试中，如果 PowerXSchema 为 "main"，则返回不带前缀的表名
	if coremodel.PowerXSchema == "main" {
		return coremodel.TableIntegrationGatewayInvocationLog
	}
	return coremodel.PowerXSchema + "." + coremodel.TableIntegrationGatewayInvocationLog
}
