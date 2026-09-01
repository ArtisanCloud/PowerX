package capability_registry

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// InvocationTrace 记录能力调用链路的审计记录。
type InvocationTrace struct {
	coremodel.PowerUUIDModel

	TraceID            string         `gorm:"column:trace_id;type:varchar(128);not null;index:idx_capability_invocation_trace" json:"trace_id"`
	TenantUUID         string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_capability_invocation_tenant" json:"tenant_uuid"`
	PluginID           string         `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_capability_invocation_plugin" json:"plugin_id"`
	CapabilityID       string         `gorm:"column:capability_id;type:varchar(128);not null;index:idx_capability_invocation_capability" json:"capability_id"`
	RouteID            *uuid.UUID     `gorm:"column:route_id;type:uuid" json:"route_id,omitempty"`
	PreferredProtocol  string         `gorm:"column:preferred_protocol;type:varchar(32)" json:"preferred_protocol,omitempty"`
	ProtocolUsed       string         `gorm:"column:protocol_used;type:varchar(32);not null" json:"protocol_used"`
	FallbackUsed       bool           `gorm:"column:fallback_used;not null;default:false" json:"fallback_used"`
	FallbackReason     string         `gorm:"column:fallback_reason;type:text" json:"fallback_reason,omitempty"`
	Status             string         `gorm:"column:status;type:varchar(32);not null;index:idx_capability_invocation_status" json:"status"`
	IdempotencyKey     string         `gorm:"column:idempotency_key;type:varchar(128)" json:"idempotency_key,omitempty"`
	RequestPayload     datatypes.JSON `gorm:"column:request_payload;type:jsonb;default:'{}'" json:"request_payload,omitempty"`
	ResponsePayload    datatypes.JSON `gorm:"column:response_payload;type:jsonb;default:'{}'" json:"response_payload,omitempty"`
	ErrorSummary       string         `gorm:"column:error_summary;type:text" json:"error_summary,omitempty"`
	LatencyMS          int            `gorm:"column:latency_ms;not null;default:0" json:"latency_ms"`
	EventPublished     bool           `gorm:"column:event_published;not null;default:false" json:"event_published"`
	EventPublicationID *uuid.UUID     `gorm:"column:event_publication_id;type:uuid" json:"event_publication_id,omitempty"`
}

func (InvocationTrace) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryInvocationTrace
}

// CapabilityEventPublication 记录能力调用相关事件的投递信息。
type CapabilityEventPublication struct {
	coremodel.PowerUUIDModel

	TenantUUID  string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_capability_event_tenant" json:"tenant_uuid"`
	Topic       string         `gorm:"column:topic;type:varchar(256);not null;index:idx_capability_event_topic" json:"topic"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb;not null" json:"payload"`
	Status      string         `gorm:"column:status;type:varchar(32);not null;index:idx_capability_event_status" json:"status"`
	Attempts    int            `gorm:"column:attempts;not null;default:0" json:"attempts"`
	LastError   string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	TraceID     string         `gorm:"column:trace_id;type:varchar(128);not null;index:idx_capability_event_trace" json:"trace_id"`
	PublishTime *time.Time     `gorm:"column:publish_time" json:"publish_time,omitempty"`
}

func (CapabilityEventPublication) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityRegistryEventPublication
}
