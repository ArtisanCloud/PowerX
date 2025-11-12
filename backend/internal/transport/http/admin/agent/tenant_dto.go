package agent

import (
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/google/uuid"
)

const timeLayout = time.RFC3339

type tenantGrantDTO struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	ExpiresAt string `json:"expires_at"`
}

type tenantFormRequest struct {
	TenantID                 string            `json:"tenant_id" binding:"required"`
	Alias                    string            `json:"alias" binding:"required"`
	DisplayName              string            `json:"display_name"`
	Purpose                  string            `json:"purpose"`
	PromptTemplate           string            `json:"prompt_template"`
	TelemetryContractVersion string            `json:"telemetry_contract_version" binding:"required"`
	ToolGrants               []tenantGrantDTO  `json:"tool_grants"`
	Permissions              []string          `json:"permissions"`
	RateLimit                int32             `json:"rate_limit"`
	SandboxProfile           string            `json:"sandbox_profile"`
	Metadata                 map[string]string `json:"metadata"`
	RequestedBy              string            `json:"requested_by"`
	TraceID                  string            `json:"trace_id"`
}

type tenantFormResponse struct {
	ID                       string                           `json:"id"`
	TenantID                 string                           `json:"tenant_id"`
	Alias                    string                           `json:"alias"`
	DisplayName              string                           `json:"display_name"`
	Purpose                  string                           `json:"purpose"`
	PromptTemplate           string                           `json:"prompt_template"`
	TelemetryContractVersion string                           `json:"telemetry_contract_version"`
	ToolGrants               []tenantGrantDTO                 `json:"tool_grants"`
	Permissions              []string                         `json:"permissions"`
	RateLimit                int32                            `json:"rate_limit"`
	SandboxProfile           string                           `json:"sandbox_profile"`
	Status                   string                           `json:"status"`
	WorkflowTicketID         string                           `json:"workflow_ticket_id"`
	ConflictReasons          []agent_lifecycle.PolicyConflict `json:"conflict_reasons"`
	RequestedBy              string                           `json:"requested_by"`
	ApprovedBy               string                           `json:"approved_by,omitempty"`
	ApprovedAt               *string                          `json:"approved_at,omitempty"`
	ActivatedAgentID         *string                          `json:"activated_agent_id,omitempty"`
	Metadata                 map[string]string                `json:"metadata,omitempty"`
	CreatedAt                string                           `json:"created_at"`
	UpdatedAt                string                           `json:"updated_at"`
}

type tenantFormActionRequest struct {
	Operator string `json:"operator" binding:"required"`
	Reason   string `json:"reason"`
	TraceID  string `json:"trace_id"`
}

func toTenantFormInput(req tenantFormRequest) agent_lifecycle.TenantFormInput {
	grants := make([]agent_lifecycle.ToolGrant, 0, len(req.ToolGrants))
	for _, item := range req.ToolGrants {
		grants = append(grants, agent_lifecycle.ToolGrant{
			Name:      item.Name,
			Version:   item.Version,
			ExpiresAt: item.ExpiresAt,
		})
	}
	return agent_lifecycle.TenantFormInput{
		TenantID:                 req.TenantID,
		Alias:                    req.Alias,
		DisplayName:              req.DisplayName,
		Purpose:                  req.Purpose,
		PromptTemplate:           req.PromptTemplate,
		TelemetryContractVersion: req.TelemetryContractVersion,
		ToolGrants:               grants,
		Permissions:              req.Permissions,
		RateLimit:                req.RateLimit,
		SandboxProfile:           req.SandboxProfile,
		RequestedBy:              req.RequestedBy,
		TraceID:                  req.TraceID,
		Metadata:                 req.Metadata,
	}
}

func fromTenantFormView(form *agent_lifecycle.TenantForm) tenantFormResponse {
	if form == nil {
		return tenantFormResponse{}
	}
	grants := make([]tenantGrantDTO, 0, len(form.ToolGrants))
	for _, grant := range form.ToolGrants {
		grants = append(grants, tenantGrantDTO{
			Name:      grant.Name,
			Version:   grant.Version,
			ExpiresAt: grant.ExpiresAt,
		})
	}
	resp := tenantFormResponse{
		ID:                       form.ID.String(),
		TenantID:                 form.TenantID,
		Alias:                    form.Alias,
		DisplayName:              form.DisplayName,
		Purpose:                  form.Purpose,
		PromptTemplate:           form.PromptTemplate,
		TelemetryContractVersion: form.TelemetryContractVersion,
		ToolGrants:               grants,
		Permissions:              form.Permissions,
		RateLimit:                form.RateLimit,
		SandboxProfile:           form.SandboxProfile,
		Status:                   form.Status,
		WorkflowTicketID:         form.WorkflowTicketID,
		ConflictReasons:          form.ConflictReasons,
		RequestedBy:              form.RequestedBy,
		ApprovedBy:               form.ApprovedBy,
		Metadata:                 form.Metadata,
		CreatedAt:                form.CreatedAt.UTC().Format(timeLayout),
		UpdatedAt:                form.UpdatedAt.UTC().Format(timeLayout),
	}
	if form.ApprovedAt != nil {
		formatted := form.ApprovedAt.UTC().Format(timeLayout)
		resp.ApprovedAt = &formatted
	}
	if form.ActivatedAgentID != nil {
		id := form.ActivatedAgentID.String()
		resp.ActivatedAgentID = &id
	}
	return resp
}

func parseTenantFormID(idParam string) (uuid.UUID, error) {
	return uuid.Parse(idParam)
}
