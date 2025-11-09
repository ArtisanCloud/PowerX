package agent_model_hub

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// ConnectorInstance represents an external automation platform instance (Coze/n8n/others) scoped per tenant.
type ConnectorInstance struct {
	coremodel.PowerUUIDModel

	Env         string `gorm:"column:env;type:varchar(32);not null;index:idx_connector_instances_scope,priority:1" json:"env"`
	TenantScope string `gorm:"column:tenant_scope;type:varchar(128);not null;index:idx_connector_instances_scope,priority:2" json:"tenant_scope"`
	Platform    string `gorm:"column:platform;type:varchar(32);not null;index:idx_connector_instances_platform,priority:1" json:"platform"`
	Region      string `gorm:"column:region;type:varchar(64)" json:"region"`

	OAuthRef             string `gorm:"column:oauth_ref;type:varchar(256);not null" json:"oauth_ref"`
	WebhookSigningKeyRef string `gorm:"column:webhook_signing_key_ref;type:varchar(256);not null" json:"webhook_signing_key_ref"`

	MappingTemplate datatypes.JSON `gorm:"column:mapping_template;type:jsonb;default:'{}'::jsonb" json:"mapping_template"`

	Status             string            `gorm:"column:status;type:varchar(32);not null;default:'active';index" json:"status"`
	ErrorRate          float64           `gorm:"column:error_rate;type:numeric(5,4);default:0" json:"error_rate"`
	LastPauseReason    string            `gorm:"column:last_pause_reason;type:text" json:"last_pause_reason"`
	RateLimitPerMinute uint32            `gorm:"column:rate_limit_per_minute;type:int" json:"rate_limit_per_minute"`
	SealedSecrets      datatypes.JSONMap `gorm:"column:sealed_secrets;type:jsonb;default:'{}'::jsonb" json:"-"`
}

func (ConnectorInstance) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TableAgentConnectorInstances
	}
	return schema + "." + coremodel.TableAgentConnectorInstances
}
