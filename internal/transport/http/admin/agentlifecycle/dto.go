package agentlifecycle

import (
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
)

type registerAgentRequest struct {
	TenantID                 string            `json:"tenant_id" binding:"required"`
	Alias                    string            `json:"alias" binding:"required"`
	DisplayName              string            `json:"display_name"`
	ToolGrants               []agentGrantDTO   `json:"tool_grants"`
	TelemetryContractVersion string            `json:"telemetry_contract_version" binding:"required"`
	DefaultCapacityInstances *int32            `json:"default_capacity_instances"`
	MaxCapacityInstances     *int32            `json:"max_capacity_instances"`
	EventTopicPrefix         string            `json:"event_topic_prefix"`
	NotificationChannel      string            `json:"notification_channel"`
	Metadata                 map[string]string `json:"metadata"`
	RequestedBy              string            `json:"requested_by"`
}

type activateAgentRequest struct {
	TenantID    string `json:"tenant_id" binding:"required"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type pauseAgentRequest struct {
	TenantID    string `json:"tenant_id" binding:"required"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type resumeAgentRequest struct {
	TenantID    string `json:"tenant_id" binding:"required"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type retireAgentRequest struct {
	TenantID    string `json:"tenant_id" binding:"required"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type scaleAgentRequest struct {
	TenantID                string `json:"tenant_id" binding:"required"`
	TargetCapacityInstances int32  `json:"target_capacity_instances" binding:"required"`
	Reason                  string `json:"reason"`
	RequestedBy             string `json:"requested_by"`
	TraceID                 string `json:"trace_id"`
}

type agentGrantDTO struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	ExpiresAt string `json:"expires_at"`
}

type agentResponse struct {
	ID                       string            `json:"id"`
	TenantID                 string            `json:"tenant_id"`
	Alias                    string            `json:"alias"`
	DisplayName              string            `json:"display_name"`
	Status                   string            `json:"status"`
	ToolGrants               []agentGrantDTO   `json:"tool_grants,omitempty"`
	TelemetryContractVersion string            `json:"telemetry_contract_version"`
	DefaultCapacityInstances int32             `json:"default_capacity_instances"`
	MaxCapacityInstances     *int32            `json:"max_capacity_instances,omitempty"`
	CurrentCapacityInstances int32             `json:"current_capacity_instances"`
	EventTopicPrefix         string            `json:"event_topic_prefix"`
	NotificationChannel      string            `json:"notification_channel,omitempty"`
	Metadata                 map[string]string `json:"metadata,omitempty"`
	CreatedAt                string            `json:"created_at"`
	UpdatedAt                string            `json:"updated_at"`
}

func toRegisterInput(req registerAgentRequest) agent_lifecycle.RegisterInput {
	var grants []agent_lifecycle.ToolGrant
	for _, item := range req.ToolGrants {
		grants = append(grants, agent_lifecycle.ToolGrant{
			Name:      item.Name,
			Version:   item.Version,
			ExpiresAt: item.ExpiresAt,
		})
	}
	var capacity int32
	if req.DefaultCapacityInstances != nil {
		capacity = *req.DefaultCapacityInstances
	}
	return agent_lifecycle.RegisterInput{
		TenantID:                 req.TenantID,
		Alias:                    req.Alias,
		DisplayName:              req.DisplayName,
		ToolGrants:               grants,
		TelemetryContractVersion: req.TelemetryContractVersion,
		DefaultCapacityInstances: capacity,
		MaxCapacityInstances:     req.MaxCapacityInstances,
		EventTopicPrefix:         req.EventTopicPrefix,
		NotificationChannel:      req.NotificationChannel,
		Metadata:                 req.Metadata,
		RequestedBy:              req.RequestedBy,
	}
}

func fromAgent(agent *agent_lifecycle.Agent) agentResponse {
	if agent == nil {
		return agentResponse{}
	}
	dtos := make([]agentGrantDTO, 0, len(agent.ToolGrants))
	for _, grant := range agent.ToolGrants {
		dtos = append(dtos, agentGrantDTO{
			Name:      grant.Name,
			Version:   grant.Version,
			ExpiresAt: grant.ExpiresAt,
		})
	}
	return agentResponse{
		ID:                       agent.ID.String(),
		TenantID:                 agent.TenantID,
		Alias:                    agent.Alias,
		DisplayName:              agent.DisplayName,
		Status:                   agent.Status,
		ToolGrants:               dtos,
		TelemetryContractVersion: agent.TelemetryContractVersion,
		DefaultCapacityInstances: agent.DefaultCapacityInstances,
		MaxCapacityInstances:     agent.MaxCapacityInstances,
		CurrentCapacityInstances: agent.CurrentCapacityInstances,
		EventTopicPrefix:         agent.EventTopicPrefix,
		NotificationChannel:      agent.NotificationChannel,
		Metadata:                 agent.Metadata,
		CreatedAt:                agent.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:                agent.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
