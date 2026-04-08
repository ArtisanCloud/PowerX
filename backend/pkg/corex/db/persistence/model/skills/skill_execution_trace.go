package skills

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// SkillExecutionTrace captures one execution lifecycle for auditing/observability.
type SkillExecutionTrace struct {
	coremodel.PowerUUIDModel

	TraceID                string `gorm:"column:trace_id;type:varchar(128);not null;uniqueIndex:uk_skill_execution_trace" json:"trace_id"`
	TenantUUID             string `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_skill_execution_tenant" json:"tenant_uuid"`
	SkillID                string `gorm:"column:skill_id;type:varchar(128);not null;index:idx_skill_execution_skill_created" json:"skill_id"`
	Version                string `gorm:"column:version;type:varchar(64);not null" json:"version"`
	Entrypoint             string `gorm:"column:entrypoint;type:varchar(128);not null" json:"entrypoint"`
	ProtocolUsed           string `gorm:"column:protocol_used;type:varchar(32);not null;default:'skill'" json:"protocol_used"`
	InvokePath             string `gorm:"column:invoke_path;type:varchar(64);not null" json:"invoke_path"`
	Status                 string `gorm:"column:status;type:varchar(32);not null;index:idx_skill_execution_status" json:"status"`
	LatencyMS              int    `gorm:"column:latency_ms;not null;default:0" json:"latency_ms"`
	ErrorCode              string `gorm:"column:error_code;type:varchar(128)" json:"error_code,omitempty"`
	ErrorSummary           string `gorm:"column:error_summary;type:text" json:"error_summary,omitempty"`
	RequestPayloadDigest   string `gorm:"column:request_payload_digest;type:varchar(128)" json:"request_payload_digest,omitempty"`
	ResponsePayloadDigest  string `gorm:"column:response_payload_digest;type:varchar(128)" json:"response_payload_digest,omitempty"`
	CapabilityID           string `gorm:"column:capability_id;type:varchar(128);index:idx_skill_execution_capability" json:"capability_id,omitempty"`
	PlanID                 string `gorm:"column:plan_id;type:varchar(128);index:idx_skill_execution_plan" json:"plan_id,omitempty"`
	NodeID                 string `gorm:"column:node_id;type:varchar(128);index:idx_skill_execution_node" json:"node_id,omitempty"`
	NodeStatus             string `gorm:"column:node_status;type:varchar(32);index:idx_skill_execution_node_status" json:"node_status,omitempty"`
	RetryTrace             string `gorm:"column:retry_trace;type:text" json:"retry_trace,omitempty"`
	FallbackUsed           bool   `gorm:"column:fallback_used;not null;default:false" json:"fallback_used"`
	AuthorizationCheckPass bool   `gorm:"column:authorization_check_pass;not null;default:false" json:"authorization_check_pass"`
}

func (SkillExecutionTrace) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsExecutionTraces
}

func (t *SkillExecutionTrace) Normalize() {
	t.TraceID = strings.TrimSpace(t.TraceID)
	t.TenantUUID = strings.TrimSpace(strings.ToLower(t.TenantUUID))
	t.SkillID = strings.TrimSpace(strings.ToLower(t.SkillID))
	t.Version = strings.TrimSpace(t.Version)
	t.Entrypoint = strings.TrimSpace(t.Entrypoint)
	t.ProtocolUsed = strings.TrimSpace(strings.ToLower(t.ProtocolUsed))
	t.InvokePath = strings.TrimSpace(strings.ToLower(t.InvokePath))
	t.Status = strings.TrimSpace(strings.ToLower(t.Status))
	t.PlanID = strings.TrimSpace(t.PlanID)
	t.NodeID = strings.TrimSpace(t.NodeID)
	t.NodeStatus = strings.TrimSpace(strings.ToLower(t.NodeStatus))
	t.RetryTrace = strings.TrimSpace(t.RetryTrace)
}
