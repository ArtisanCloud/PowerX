package integration_gateway

import (
	"github.com/google/uuid"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IntegrationGatewayAPIKeyPermission 描述 API Key 的 scope/action/resource 授权。
type IntegrationGatewayAPIKeyPermission struct {
	coremodel.PowerUUIDModel

	APIKeyUUID      uuid.UUID `gorm:"column:api_key_uuid;type:uuid;not null;index:idx_igw_api_key_perm_key" json:"api_key_uuid"`
	Scope           string    `gorm:"column:scope;type:varchar(128);not null;index:idx_igw_api_key_perm_scope" json:"scope"`
	Action          string    `gorm:"column:action;type:varchar(64);not null" json:"action"`
	ResourceType    string    `gorm:"column:resource_type;type:varchar(64);not null" json:"resource_type"`
	ResourcePattern string    `gorm:"column:resource_pattern;type:varchar(256);not null" json:"resource_pattern"`
	PluginID        string    `gorm:"column:plugin_id;type:varchar(128)" json:"plugin_id,omitempty"`
	Effect          string    `gorm:"column:effect;type:varchar(16);not null;default:'allow'" json:"effect"`
}

func (IntegrationGatewayAPIKeyPermission) TableName() string {
	if coremodel.PowerXSchema == "main" {
		return coremodel.TableIntegrationGatewayAPIKeyPermission
	}
	return coremodel.PowerXSchema + "." + coremodel.TableIntegrationGatewayAPIKeyPermission
}
