package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	TableAgentProfileLifecycle     = "agent_profiles"
	TableAgentLifecycleEventRecord = "agent_lifecycle_events"
	TableAgentHealthSnapshotRecord = "agent_health_snapshots"
)

// AgentProfileLifecycle 代表可在 Agent 服务运行的代理档案。
type AgentProfileLifecycle struct {
	coremodel.PowerUUIDModel

	TenantUUID               string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_agent_profile_tenant_alias,priority:1;index:idx_agent_profile_tenant_status,priority:1" json:"tenant_uuid"`
	Alias                    string         `gorm:"column:alias;type:varchar(128);not null;index:idx_agent_profile_tenant_alias,priority:2" json:"alias"`
	DisplayName              string         `gorm:"column:display_name;type:varchar(128);not null" json:"display_name"`
	Status                   string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_agent_profile_tenant_status,priority:2" json:"status"`
	ToolGrants               datatypes.JSON `gorm:"column:tool_grants;type:jsonb;default:'[]'" json:"tool_grants,omitempty"`
	TelemetryContractVersion string         `gorm:"column:telemetry_contract_version;type:varchar(64);not null" json:"telemetry_contract_version"`
	DefaultCapacityInstances int32          `gorm:"column:default_capacity_instances;not null;default:1" json:"default_capacity_instances"`
	MaxCapacityInstances     *int32         `gorm:"column:max_capacity_instances" json:"max_capacity_instances,omitempty"`
	CurrentCapacityInstances int32          `gorm:"column:current_capacity_instances;not null;default:0" json:"current_capacity_instances"`
	EventTopicPrefix         string         `gorm:"column:event_topic_prefix;type:varchar(128);not null" json:"event_topic_prefix"`
	NotificationChannel      string         `gorm:"column:notification_channel;type:text" json:"notification_channel,omitempty"`
	Metadata                 datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`
	RetiredAt                *time.Time     `gorm:"column:retired_at" json:"retired_at,omitempty"`
	CreatedBy                string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy                string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (AgentProfileLifecycle) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentProfileLifecycle
}

// AgentLifecycleEventRecord 捕捉生命周期操作。
type AgentLifecycleEventRecord struct {
	coremodel.PowerUUIDModel

	AgentUUID           uuid.UUID      `gorm:"column:agent_uuid;type:uuid;not null;index:idx_agent_lifecycle_agent" json:"agent_uuid"`
	TenantUUID          string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_agent_lifecycle_tenant,priority:1" json:"tenant_uuid"`
	EventType           string         `gorm:"column:event_type;type:varchar(64);not null;index:idx_agent_lifecycle_type" json:"event_type"`
	FromStatus          string         `gorm:"column:from_status;type:varchar(32)" json:"from_status,omitempty"`
	ToStatus            string         `gorm:"column:to_status;type:varchar(32);not null;index:idx_agent_lifecycle_tenant,priority:2" json:"to_status"`
	RequestedCapacity   *int32         `gorm:"column:requested_capacity_instances" json:"requested_capacity_instances,omitempty"`
	Reason              string         `gorm:"column:reason;type:text" json:"reason,omitempty"`
	TriggeredBy         string         `gorm:"column:triggered_by;type:varchar(128)" json:"triggered_by,omitempty"`
	TraceID             string         `gorm:"column:trace_id;type:varchar(128);index:idx_agent_lifecycle_trace" json:"trace_id,omitempty"`
	EventID             string         `gorm:"column:event_id;type:varchar(128)" json:"event_id,omitempty"`
	Metadata            datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`
	OccurredAt          time.Time      `gorm:"column:occurred_at;autoCreateTime" json:"occurred_at"`
	ProcessingLatencyMS int64          `gorm:"column:processing_latency_ms;default:0" json:"processing_latency_ms"`
}

func (AgentLifecycleEventRecord) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentLifecycleEventRecord
}

// AgentHealthSnapshotRecord 表示健康评分快照。
type AgentHealthSnapshotRecord struct {
	coremodel.PowerUUIDModel

	AgentUUID         uuid.UUID      `gorm:"column:agent_uuid;type:uuid;not null;index:idx_agent_health_agent_window,priority:1" json:"agent_uuid"`
	TenantUUID        string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_agent_health_tenant" json:"tenant_uuid"`
	WindowStartedAt   time.Time      `gorm:"column:window_started_at;not null;index:idx_agent_health_agent_window,priority:2" json:"window_started_at"`
	WindowDurationSec int32          `gorm:"column:window_duration_sec;not null" json:"window_duration_sec"`
	ThroughputPerMin  float64        `gorm:"column:throughput_per_min;type:double precision;not null;default:0" json:"throughput_per_min"`
	SuccessRate       float64        `gorm:"column:success_rate;type:double precision;not null;default:0" json:"success_rate"`
	P95LatencyMs      int32          `gorm:"column:p95_latency_ms;not null;default:0" json:"p95_latency_ms"`
	ResourceUtilPct   float64        `gorm:"column:resource_utilization_pct;type:double precision;not null;default:0" json:"resource_utilization_pct"`
	ErrorRate         float64        `gorm:"column:error_rate;type:double precision;not null;default:0" json:"error_rate"`
	HealthScore       int32          `gorm:"column:health_score;not null;index:idx_agent_health_status,priority:2" json:"health_score"`
	Status            string         `gorm:"column:status;type:varchar(32);not null;index:idx_agent_health_status,priority:1" json:"status"`
	AnomalyTraceIDs   datatypes.JSON `gorm:"column:anomaly_trace_ids;type:jsonb;default:'[]'" json:"anomaly_trace_ids,omitempty"`
	Recommendations   datatypes.JSON `gorm:"column:recommendations;type:jsonb;default:'[]'" json:"recommendations,omitempty"`
	LastLifecycleUUID uuid.UUID      `gorm:"column:last_lifecycle_event_uuid;type:uuid" json:"last_lifecycle_event_uuid"`
	TraceID           string         `gorm:"column:trace_id;type:varchar(128)" json:"trace_id,omitempty"`
}

func (AgentHealthSnapshotRecord) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentHealthSnapshotRecord
}
