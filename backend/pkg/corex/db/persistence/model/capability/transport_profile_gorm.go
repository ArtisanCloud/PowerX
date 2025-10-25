package capability

import (
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CapabilityTransportProfile 描述不同协议的运行偏好与 QoS 策略。
type CapabilityTransportProfile struct {
	coremodel.PowerModel

	TenantID         uint64         `gorm:"column:tenant_id;not null;index:idx_capability_transport_profile_tenant;uniqueIndex:uk_capability_transport_contract,priority:1" json:"tenant_id"`
	ContractID       uint64         `gorm:"column:contract_id;not null;index:idx_capability_transport_profile_contract;uniqueIndex:uk_capability_transport_contract,priority:2" json:"contract_id"`
	CapabilityKey    string         `gorm:"column:capability_key;type:varchar(128);not null;index:idx_capability_transport_profile_tenant" json:"capability_key"`
	Transport        string         `gorm:"column:transport;type:varchar(16);not null;uniqueIndex:uk_capability_transport_contract,priority:3" json:"transport"`
	Mode             string         `gorm:"column:mode;type:varchar(16);not null" json:"mode"`
	TimeoutMillis    int            `gorm:"column:timeout_ms;not null" json:"timeout_ms"`
	RetryPolicy      datatypes.JSON `gorm:"column:retry;type:jsonb;default:'{}'" json:"retry,omitempty"`
	Streaming        bool           `gorm:"column:streaming;default:false" json:"streaming"`
	QoS              datatypes.JSON `gorm:"column:qos;type:jsonb;default:'{}'" json:"qos,omitempty"`
	EndpointSelector datatypes.JSON `gorm:"column:endpoint_selector;type:jsonb;default:'{}'" json:"endpoint_selector,omitempty"`
	LastHealthStatus datatypes.JSON `gorm:"column:last_health_status;type:jsonb;default:'{}'" json:"last_health_status,omitempty"`
	Status           int16          `gorm:"column:status;default:1;index" json:"status"`
}

func (m *CapabilityTransportProfile) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityTransportProfile
}

func (m *CapabilityTransportProfile) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableCapabilityTransportProfile
}
