package integration_gateway

import (
	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IntegrationGatewayAPIKeyAuditLog 记录 API Key 请求审计。
type IntegrationGatewayAPIKeyAuditLog struct {
	coremodel.PowerUUIDModel

	APIKeyUUID   uuid.UUID      `gorm:"column:api_key_uuid;type:uuid;not null;index:idx_igw_api_key_audit_key" json:"api_key_uuid"`
	TenantUUID   string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_igw_api_key_audit_tenant" json:"tenant_uuid"`
	Path         string         `gorm:"column:path;type:varchar(255);not null" json:"path"`
	Method       string         `gorm:"column:method;type:varchar(16);not null" json:"method"`
	StatusCode   int            `gorm:"column:status_code;not null;default:0" json:"status_code"`
	Result       string         `gorm:"column:result;type:varchar(32);not null;index:idx_igw_api_key_audit_result" json:"result"`
	Reason       string         `gorm:"column:reason;type:text" json:"reason,omitempty"`
	TraceID      string         `gorm:"column:trace_id;type:varchar(128);index:idx_igw_api_key_audit_trace" json:"trace_id,omitempty"`
	LatencyMS    int            `gorm:"column:latency_ms;not null;default:0" json:"latency_ms"`
	RequestExtra datatypes.JSON `gorm:"column:request_extra;type:jsonb;default:'{}'" json:"request_extra,omitempty"`
}

func (IntegrationGatewayAPIKeyAuditLog) TableName() string {
	if coremodel.PowerXSchema == "main" {
		return coremodel.TableIntegrationGatewayAPIKeyAuditLog
	}
	return coremodel.PowerXSchema + "." + coremodel.TableIntegrationGatewayAPIKeyAuditLog
}
