package agentlifecycle

import (
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/google/uuid"
)

type registerAgentRequest struct {
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
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type pauseAgentRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type resumeAgentRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type retireAgentRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type scaleAgentRequest struct {
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

type autoRegisterManifestRequest struct {
	PluginID                 string            `json:"plugin_id" binding:"required"`
	PluginVersion            string            `json:"plugin_version" binding:"required"`
	ManifestVersion          string            `json:"manifest_version" binding:"required"`
	Alias                    string            `json:"alias" binding:"required"`
	DisplayName              string            `json:"display_name"`
	TelemetryContractVersion string            `json:"telemetry_contract_version" binding:"required"`
	ToolGrants               []agentGrantDTO   `json:"tool_grants"`
	DefaultCapacityInstances *int32            `json:"default_capacity_instances"`
	MaxCapacityInstances     *int32            `json:"max_capacity_instances"`
	NotificationChannel      string            `json:"notification_channel"`
	Metadata                 map[string]string `json:"metadata"`
	Capabilities             []string          `json:"capabilities"`
	Permissions              []string          `json:"permissions"`
	RateLimits               map[string]int32  `json:"rate_limits"`
	SandboxProfile           string            `json:"sandbox_profile"`
	Signature                string            `json:"signature" binding:"required"`
	DryRun                   bool              `json:"dry_run"`
	RequestedBy              string            `json:"requested_by"`
	TraceID                  string            `json:"trace_id"`
}

type sandboxRunRequest struct {
	Profile     string `json:"profile"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type agentResponse struct {
	ID                       string            `json:"id"`
	TenantUUID               string            `json:"tenant_uuid"`
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

type autoRegisterResponse struct {
	Agent   agentResponse    `json:"agent"`
	Sandbox *sandboxResponse `json:"sandbox,omitempty"`
	DryRun  bool             `json:"dry_run"`
}

type sandboxResponse struct {
	Status     string             `json:"status"`
	ReportURL  string             `json:"report_url,omitempty"`
	Profile    string             `json:"profile,omitempty"`
	ExecutedAt string             `json:"executed_at,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
}

func toRegisterInput(req registerAgentRequest, tenantUUID string) agent_lifecycle.RegisterInput {
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
		TenantUUID:               tenantUUID,
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
		TenantUUID:               agent.TenantUUID,
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

func toManifestInput(req autoRegisterManifestRequest, tenantUUID string) agent_lifecycle.ManifestRegistrationInput {
	var grants []agent_lifecycle.ToolGrant
	for _, item := range req.ToolGrants {
		grants = append(grants, agent_lifecycle.ToolGrant{
			Name:      item.Name,
			Version:   item.Version,
			ExpiresAt: item.ExpiresAt,
		})
	}
	var defaultCapacity int32
	if req.DefaultCapacityInstances != nil {
		defaultCapacity = *req.DefaultCapacityInstances
	}
	return agent_lifecycle.ManifestRegistrationInput{
		PluginID:                 req.PluginID,
		PluginVersion:            req.PluginVersion,
		ManifestVersion:          req.ManifestVersion,
		TenantUUID:               tenantUUID,
		Alias:                    req.Alias,
		DisplayName:              req.DisplayName,
		ToolGrants:               grants,
		TelemetryContractVersion: req.TelemetryContractVersion,
		DefaultCapacityInstances: defaultCapacity,
		MaxCapacityInstances:     req.MaxCapacityInstances,
		NotificationChannel:      req.NotificationChannel,
		Metadata:                 req.Metadata,
		Capabilities:             req.Capabilities,
		Permissions:              req.Permissions,
		RateLimits:               req.RateLimits,
		SandboxProfile:           req.SandboxProfile,
		Signature:                req.Signature,
		RequestedBy:              req.RequestedBy,
		TraceID:                  req.TraceID,
		DryRun:                   req.DryRun,
	}
}

func fromManifestResult(res *agent_lifecycle.ManifestRegistrationResult) autoRegisterResponse {
	if res == nil {
		return autoRegisterResponse{}
	}
	response := autoRegisterResponse{
		DryRun: res.DryRun,
	}
	if res.Agent != nil {
		response.Agent = fromAgent(res.Agent)
	}
	if res.Sandbox != nil {
		resp := fromSandboxResult(res.Sandbox)
		response.Sandbox = &resp
	}
	return response
}

func fromSandboxResult(result *agent_lifecycle.SandboxRunResult) sandboxResponse {
	if result == nil {
		return sandboxResponse{}
	}
	executedAt := ""
	if !result.ExecutedAt.IsZero() {
		executedAt = result.ExecutedAt.UTC().Format(time.RFC3339)
	}
	return sandboxResponse{
		Status:     result.Status,
		ReportURL:  result.ReportURL,
		Profile:    result.Profile,
		ExecutedAt: executedAt,
		Metrics:    result.Metrics,
	}
}

type healthSummaryResponse struct {
	Status          string                             `json:"status"`
	HealthScore     int32                              `json:"health_score"`
	UpdatedAt       string                             `json:"updated_at"`
	Metrics         agent_lifecycle.HealthMetricsInput `json:"metrics"`
	Recommendations []string                           `json:"recommendations"`
}

type healthHistoryResponse struct {
	Snapshots []healthSummaryResponse `json:"snapshots"`
}

type subscriptionRequest struct {
	MetricsFilter  []string `json:"metrics_filter"`
	HealthStatuses []string `json:"health_statuses"`
	RequestedBy    string   `json:"requested_by"`
	TraceID        string   `json:"trace_id"`
}

type subscriptionResponse struct {
	MetricsFilter  []string `json:"metrics_filter"`
	HealthStatuses []string `json:"health_statuses"`
	UpdatedAt      string   `json:"updated_at"`
}

func fromHealthSummary(summary *agent_lifecycle.HealthSummary) healthSummaryResponse {
	if summary == nil {
		return healthSummaryResponse{}
	}
	return healthSummaryResponse{
		Status:          summary.Status,
		HealthScore:     summary.HealthScore,
		UpdatedAt:       summary.UpdatedAt.UTC().Format(time.RFC3339),
		Metrics:         summary.Metrics,
		Recommendations: summary.Recommendations,
	}
}

func toSubscriptionInput(agentID uuid.UUID, tenantUUID string, req subscriptionRequest) agent_lifecycle.SubscriptionUpdateInput {
	return agent_lifecycle.SubscriptionUpdateInput{
		AgentID:     agentID,
		TenantUUID:  tenantUUID,
		RequestedBy: req.RequestedBy,
		TraceID:     req.TraceID,
		Config: agent_lifecycle.SubscriptionConfig{
			MetricsFilter:  req.MetricsFilter,
			HealthStatuses: req.HealthStatuses,
		},
	}
}

func fromSubscription(cfg *agent_lifecycle.SubscriptionConfig) subscriptionResponse {
	if cfg == nil {
		return subscriptionResponse{}
	}
	return subscriptionResponse{
		MetricsFilter:  cfg.MetricsFilter,
		HealthStatuses: cfg.HealthStatuses,
		UpdatedAt:      cfg.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
