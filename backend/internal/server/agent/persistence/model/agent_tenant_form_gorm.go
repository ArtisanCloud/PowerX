package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	TableAgentTenantForm = "agent_tenant_forms"
)

// AgentTenantForm 存储租户自助提交的代理表单。
type AgentTenantForm struct {
	coremodel.PowerUUIDModel

	TenantUUID               string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_agent_tenant_form_tenant,priority:1" json:"tenant_uuid"`
	Alias                    string         `gorm:"column:alias;type:varchar(128);not null;index:idx_agent_tenant_form_tenant,priority:2" json:"alias"`
	DisplayName              string         `gorm:"column:display_name;type:varchar(128);not null" json:"display_name"`
	Purpose                  string         `gorm:"column:purpose;type:text" json:"purpose,omitempty"`
	PromptTemplate           string         `gorm:"column:prompt_template;type:text" json:"prompt_template,omitempty"`
	TelemetryContractVersion string         `gorm:"column:telemetry_contract_version;type:varchar(64)" json:"telemetry_contract_version,omitempty"`
	ToolGrants               datatypes.JSON `gorm:"column:tool_grants;type:jsonb;default:'[]'" json:"tool_grants,omitempty"`
	Permissions              datatypes.JSON `gorm:"column:permissions;type:jsonb;default:'[]'" json:"permissions,omitempty"`
	RateLimit                int32          `gorm:"column:rate_limit" json:"rate_limit"`
	Status                   string         `gorm:"column:status;type:varchar(32);not null;index:idx_agent_tenant_form_status" json:"status"`
	ConflictReasons          datatypes.JSON `gorm:"column:conflict_reasons;type:jsonb;default:'[]'" json:"conflict_reasons,omitempty"`
	WorkflowTicketID         string         `gorm:"column:workflow_ticket_id;type:varchar(128)" json:"workflow_ticket_id,omitempty"`
	RequestedBy              string         `gorm:"column:requested_by;type:varchar(128)" json:"requested_by,omitempty"`
	ApprovedBy               string         `gorm:"column:approved_by;type:varchar(128)" json:"approved_by,omitempty"`
	ApprovedAt               *time.Time     `gorm:"column:approved_at" json:"approved_at,omitempty"`
	SandboxProfile           string         `gorm:"column:sandbox_profile;type:varchar(64)" json:"sandbox_profile,omitempty"`
	Metadata                 datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`
	LastError                string         `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	ActivatedAgentID         *uuid.UUID     `gorm:"column:activated_agent_uuid;type:uuid" json:"activated_agent_uuid,omitempty"`
}

func (AgentTenantForm) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentTenantForm
}
