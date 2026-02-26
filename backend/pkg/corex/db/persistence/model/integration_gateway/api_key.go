package integration_gateway

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IntegrationGatewayAPIKey 租户级网关 API Key（仅保存哈希和前缀）。
type IntegrationGatewayAPIKey struct {
	coremodel.PowerUUIDModel

	TenantUUID  string     `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_igw_api_key_tenant" json:"tenant_uuid"`
	ProfileID   uint64     `gorm:"column:profile_id;not null;index:idx_igw_api_key_profile" json:"profile_id"`
	Name        string     `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Description string     `gorm:"column:description;type:text" json:"description,omitempty"`
	KeyPrefix   string     `gorm:"column:key_prefix;type:varchar(32);not null;index:idx_igw_api_key_prefix" json:"key_prefix"`
	KeyHash     string     `gorm:"column:key_hash;type:varchar(128);not null;uniqueIndex:uk_igw_api_key_hash" json:"-"`
	Status      string     `gorm:"column:status;type:varchar(32);not null;default:'active';index:idx_igw_api_key_status" json:"status"`
	ExpiresAt   *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `gorm:"column:last_used_at;index:idx_igw_api_key_last_used_at" json:"last_used_at,omitempty"`
	CreatedBy   string     `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy   string     `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (IntegrationGatewayAPIKey) TableName() string {
	if coremodel.PowerXSchema == "main" {
		return coremodel.TableIntegrationGatewayAPIKey
	}
	return coremodel.PowerXSchema + "." + coremodel.TableIntegrationGatewayAPIKey
}
