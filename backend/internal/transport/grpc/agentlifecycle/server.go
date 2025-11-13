package agentlifecycle

import (
	"context"
	"errors"
	"strings"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server 实现 AgentLifecycleService。
type Server struct {
	agentv1.UnimplementedAgentLifecycleServiceServer
	service *agent_lifecycle.Service
}

// NewServer 构造 gRPC Server。
func NewServer(service *agent_lifecycle.Service) *Server {
	return &Server{service: service}
}

// Register 将服务注册到 gRPC Server。
func Register(registrar grpc.ServiceRegistrar, server *Server) {
	agentv1.RegisterAgentLifecycleServiceServer(registrar, server)
}

// RegisterAgent gRPC 实现。
func (s *Server) RegisterAgent(ctx context.Context, req *agentv1.RegisterAgentRequest) (*agentv1.RegisterAgentResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	input := agent_lifecycle.RegisterInput{
		TenantID:                 req.GetTenantId(),
		Alias:                    req.GetAlias(),
		DisplayName:              req.GetAlias(),
		TelemetryContractVersion: req.GetTelemetryContractVersion(),
		DefaultCapacityInstances: req.GetDefaultCapacityInstances(),
		MaxCapacityInstances:     req.MaxCapacityInstances,
		EventTopicPrefix:         "",
		NotificationChannel:      req.GetNotificationChannel(),
		Metadata:                 req.GetMetadata(),
		TraceID:                  req.GetTraceId(),
	}
	for _, grant := range req.GetToolGrants() {
		input.ToolGrants = append(input.ToolGrants, agent_lifecycle.ToolGrant{
			Name:      grant.GetName(),
			Version:   grant.GetVersion(),
			ExpiresAt: grant.GetExpiresAt(),
		})
	}
	res, err := s.service.Register(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.RegisterAgentResponse{Agent: toProtoAgent(res.Agent)}, nil
}

// GetAgent gRPC 实现。
func (s *Server) GetAgent(ctx context.Context, req *agentv1.GetAgentRequest) (*agentv1.GetAgentResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	agent, err := s.service.Get(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.GetAgentResponse{Agent: toProtoAgent(agent)}, nil
}

// ActivateAgent gRPC 实现。
func (s *Server) ActivateAgent(ctx context.Context, req *agentv1.LifecycleCommandRequest) (*agentv1.LifecycleEventResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	result, err := s.service.Activate(ctx, agent_lifecycle.ActivateInput{
		AgentID:     id,
		TenantID:    "",
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.LifecycleEventResponse{Event: toProtoLifecycleEvent(result.Event)}, nil
}

func (s *Server) PauseAgent(ctx context.Context, req *agentv1.LifecycleCommandRequest) (*agentv1.LifecycleEventResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	result, err := s.service.Pause(ctx, agent_lifecycle.PauseInput{
		AgentID:     id,
		TenantID:    "",
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.LifecycleEventResponse{Event: toProtoLifecycleEvent(result.Event)}, nil
}

func (s *Server) ResumeAgent(ctx context.Context, req *agentv1.LifecycleCommandRequest) (*agentv1.LifecycleEventResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	result, err := s.service.Resume(ctx, agent_lifecycle.ResumeInput{
		AgentID:     id,
		TenantID:    "",
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.LifecycleEventResponse{Event: toProtoLifecycleEvent(result.Event)}, nil
}

func (s *Server) RetireAgent(ctx context.Context, req *agentv1.LifecycleCommandRequest) (*agentv1.LifecycleEventResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	result, err := s.service.Retire(ctx, agent_lifecycle.RetireInput{
		AgentID:     id,
		TenantID:    "",
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.LifecycleEventResponse{Event: toProtoLifecycleEvent(result.Event)}, nil
}

func (s *Server) ScaleAgent(ctx context.Context, req *agentv1.ScaleAgentRequest) (*agentv1.LifecycleEventResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	result, err := s.service.Scale(ctx, agent_lifecycle.ScaleInput{
		AgentID:     id,
		TenantID:    "",
		Target:      req.GetTargetCapacityInstances(),
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.LifecycleEventResponse{Event: toProtoLifecycleEvent(result.Event)}, nil
}

func (s *Server) GetHealthSummary(ctx context.Context, req *agentv1.GetHealthSummaryRequest) (*agentv1.GetHealthSummaryResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	summary, err := s.service.GetHealthSummary(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.GetHealthSummaryResponse{Summary: toProtoHealthSummary(summary)}, nil
}

func (s *Server) ListHealthSnapshots(ctx context.Context, req *agentv1.ListHealthSnapshotsRequest) (*agentv1.ListHealthSnapshotsResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	snapshots, err := s.service.ListHealthSnapshots(ctx, id, int(req.GetRangeHours()), 0)
	if err != nil {
		return nil, toStatusError(err)
	}
	resp := &agentv1.ListHealthSnapshotsResponse{}
	for _, snap := range snapshots {
		resp.Snapshots = append(resp.Snapshots, toProtoHealthSnapshot(snap))
	}
	return resp, nil
}

func (s *Server) UpdateSubscription(ctx context.Context, req *agentv1.UpdateSubscriptionRequest) (*agentv1.UpdateSubscriptionResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	input := agent_lifecycle.SubscriptionUpdateInput{
		AgentID:     id,
		TenantID:    req.GetTenantId(),
		RequestedBy: req.GetTraceId(),
		TraceID:     req.GetTraceId(),
		Config:      subscriptionConfigFromProto(req.GetConfig()),
	}
	cfg, err := s.service.UpdateSubscription(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.UpdateSubscriptionResponse{Config: toProtoSubscriptionConfig(cfg)}, nil
}

func (s *Server) GetSubscription(ctx context.Context, req *agentv1.GetSubscriptionRequest) (*agentv1.GetSubscriptionResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	id, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	cfg, err := s.service.GetSubscription(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.GetSubscriptionResponse{Config: toProtoSubscriptionConfig(cfg)}, nil
}

func (s *Server) RegisterManifest(ctx context.Context, req *agentv1.RegisterManifestRequest) (*agentv1.RegisterManifestResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	input := agent_lifecycle.ManifestRegistrationInput{
		PluginID:                 req.GetPluginId(),
		PluginVersion:            req.GetPluginVersion(),
		ManifestVersion:          req.GetManifestVersion(),
		TenantID:                 req.GetTenantId(),
		Alias:                    req.GetAlias(),
		DisplayName:              req.GetDisplayName(),
		TelemetryContractVersion: req.GetTelemetryContractVersion(),
		DefaultCapacityInstances: req.GetDefaultCapacityInstances(),
		MaxCapacityInstances:     req.MaxCapacityInstances,
		NotificationChannel:      req.GetNotificationChannel(),
		Metadata:                 req.GetMetadata(),
		Capabilities:             req.GetCapabilities(),
		Permissions:              req.GetPermissions(),
		RateLimits:               req.GetRateLimits(),
		SandboxProfile:           req.GetSandboxProfile(),
		Signature:                req.GetSignature(),
		RequestedBy:              req.GetRequestedBy(),
		TraceID:                  req.GetTraceId(),
		DryRun:                   req.GetDryRun(),
	}
	for _, grant := range req.GetToolGrants() {
		input.ToolGrants = append(input.ToolGrants, agent_lifecycle.ToolGrant{
			Name:      grant.GetName(),
			Version:   grant.GetVersion(),
			ExpiresAt: grant.GetExpiresAt(),
		})
	}
	result, err := s.service.RegisterManifest(ctx, input)
	if err != nil {
		return nil, toStatusError(err)
	}
	resp := &agentv1.RegisterManifestResponse{
		DryRun: result.DryRun,
	}
	if result.Agent != nil {
		resp.Agent = toProtoAgent(result.Agent)
	}
	if result.Sandbox != nil {
		resp.Sandbox = toProtoSandboxResult(result.Sandbox)
	}
	return resp, nil
}

func (s *Server) RunSandbox(ctx context.Context, req *agentv1.RunSandboxRequest) (*agentv1.RunSandboxResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	agentID, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	result, err := s.service.RunSandbox(ctx, agent_lifecycle.SandboxRunInput{
		AgentID:     agentID,
		Profile:     req.GetProfile(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.RunSandboxResponse{Sandbox: toProtoSandboxResult(result)}, nil
}

func (s *Server) ShareAgent(ctx context.Context, req *agentv1.CreateAgentShareRequest) (*agentv1.AgentShareResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	agentID, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	share, err := s.service.ShareAgent(ctx, agent_lifecycle.ShareInput{
		AgentID:     agentID,
		TenantID:    req.GetTenantId(),
		Quotas:      shareQuotasFromProto(req.GetQuotas()),
		Metadata:    req.GetMetadata(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return toProtoShare(share), nil
}

func (s *Server) RevokeAgentShare(ctx context.Context, req *agentv1.RevokeAgentShareRequest) (*agentv1.AgentShareResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "agent lifecycle service unavailable")
	}
	shareID, err := uuid.Parse(req.GetShareId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid share_id: %v", err)
	}
	share, err := s.service.RevokeAgentShare(ctx, agent_lifecycle.RevokeShareInput{
		ShareID:     shareID,
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return toProtoShare(share), nil
}

func toProtoAgent(agent *agent_lifecycle.Agent) *agentv1.Agent {
	if agent == nil {
		return nil
	}
	pb := &agentv1.Agent{
		Id:                       agent.ID.String(),
		Alias:                    agent.Alias,
		TenantId:                 agent.TenantID,
		Status:                   agentStatusToProto(agent.Status),
		TelemetryContractVersion: agent.TelemetryContractVersion,
		DefaultCapacityInstances: agent.DefaultCapacityInstances,
		MaxCapacityInstances:     getOrZero(agent.MaxCapacityInstances),
		CurrentCapacityInstances: agent.CurrentCapacityInstances,
		EventTopic:               agent.EventTopicPrefix,
		NotificationChannel:      agent.NotificationChannel,
		CreatedAt:                agent.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:                agent.UpdatedAt.UTC().Format(time.RFC3339),
	}
	for _, grant := range agent.ToolGrants {
		pb.ToolGrants = append(pb.ToolGrants, &agentv1.ToolGrant{
			Name:      grant.Name,
			Version:   grant.Version,
			ExpiresAt: grant.ExpiresAt,
		})
	}
	return pb
}

func agentStatusToProto(status string) agentv1.AgentStatus {
	switch strings.ToLower(status) {
	case "pending":
		return agentv1.AgentStatus_AGENT_STATUS_PENDING
	case "active":
		return agentv1.AgentStatus_AGENT_STATUS_ACTIVE
	case "paused":
		return agentv1.AgentStatus_AGENT_STATUS_PAUSED
	case "retired":
		return agentv1.AgentStatus_AGENT_STATUS_RETIRED
	default:
		return agentv1.AgentStatus_AGENT_STATUS_UNSPECIFIED
	}
}

func getOrZero(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func toProtoLifecycleEvent(event *agentmodel.AgentLifecycleEventRecord) *agentv1.LifecycleEvent {
	if event == nil {
		return nil
	}
	pb := &agentv1.LifecycleEvent{
		Id:          event.UUID.String(),
		AgentId:     event.AgentUUID.String(),
		Type:        eventTypeToProto(event.EventType),
		FromStatus:  agentStatusToProto(event.FromStatus),
		ToStatus:    agentStatusToProto(event.ToStatus),
		Reason:      event.Reason,
		TriggeredBy: event.TriggeredBy,
		TraceId:     event.TraceID,
		EventId:     event.EventID,
		CreatedAt:   event.OccurredAt.UTC().Format(time.RFC3339),
	}
	if event.RequestedCapacity != nil {
		pb.RequestedCapacity = *event.RequestedCapacity
	}
	return pb
}

func eventTypeToProto(eventType string) agentv1.LifecycleEventType {
	switch strings.ToLower(eventType) {
	case "register":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_REGISTER
	case "activate":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_ACTIVATE
	case "pause":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_PAUSE
	case "resume":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_RESUME
	case "scale":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_SCALE_UP
	case "retire":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_RETIRE
	case "auto_degrade":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_AUTO_DEGRADE
	case "auto_recover":
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_AUTO_RECOVER
	default:
		return agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_UNSPECIFIED
	}
}

func healthStatusToProto(status string) agentv1.HealthStatus {
	switch strings.ToLower(status) {
	case "healthy":
		return agentv1.HealthStatus_HEALTH_STATUS_HEALTHY
	case "degraded":
		return agentv1.HealthStatus_HEALTH_STATUS_DEGRADED
	case "unavailable":
		return agentv1.HealthStatus_HEALTH_STATUS_UNAVAILABLE
	case "unknown":
		return agentv1.HealthStatus_HEALTH_STATUS_UNKNOWN
	default:
		return agentv1.HealthStatus_HEALTH_STATUS_UNSPECIFIED
	}
}

func toProtoHealthSummary(summary *agent_lifecycle.HealthSummary) *agentv1.HealthSummary {
	if summary == nil {
		return nil
	}
	return &agentv1.HealthSummary{
		AgentId:     summary.AgentID.String(),
		Status:      healthStatusToProto(summary.Status),
		HealthScore: summary.HealthScore,
		UpdatedAt:   summary.UpdatedAt.UTC().Format(time.RFC3339),
		Metrics: &agentv1.HealthMetrics{
			ThroughputPerMin:       summary.Metrics.ThroughputPerMin,
			SuccessRate:            summary.Metrics.SuccessRate,
			P95LatencyMs:           summary.Metrics.P95LatencyMs,
			ResourceUtilizationPct: summary.Metrics.ResourceUtilPct,
			ErrorRate:              summary.Metrics.ErrorRate,
		},
		Recommendations: summary.Recommendations,
	}
}

func toProtoHealthSnapshot(summary *agent_lifecycle.HealthSummary) *agentv1.HealthSnapshot {
	if summary == nil {
		return nil
	}
	return &agentv1.HealthSnapshot{
		WindowStartedAt:   summary.UpdatedAt.UTC().Format(time.RFC3339),
		WindowDurationSec: summary.WindowDurationSec,
		HealthScore:       summary.HealthScore,
		Status:            healthStatusToProto(summary.Status),
		Metrics: &agentv1.HealthMetrics{
			ThroughputPerMin:       summary.Metrics.ThroughputPerMin,
			SuccessRate:            summary.Metrics.SuccessRate,
			P95LatencyMs:           summary.Metrics.P95LatencyMs,
			ResourceUtilizationPct: summary.Metrics.ResourceUtilPct,
			ErrorRate:              summary.Metrics.ErrorRate,
		},
		AnomalyTraceIds: summary.Metrics.AnomalyTraceIDs,
	}
}

func toProtoSubscriptionConfig(cfg *agent_lifecycle.SubscriptionConfig) *agentv1.SubscriptionConfig {
	if cfg == nil {
		return &agentv1.SubscriptionConfig{}
	}
	updatedAt := ""
	if !cfg.UpdatedAt.IsZero() {
		updatedAt = cfg.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return &agentv1.SubscriptionConfig{
		MetricsFilter:  cfg.MetricsFilter,
		HealthStatuses: cfg.HealthStatuses,
		UpdatedAt:      updatedAt,
	}
}

func subscriptionConfigFromProto(cfg *agentv1.SubscriptionConfig) agent_lifecycle.SubscriptionConfig {
	if cfg == nil {
		return agent_lifecycle.SubscriptionConfig{}
	}
	var updatedAt time.Time
	if ts := cfg.GetUpdatedAt(); ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			updatedAt = parsed
		}
	}
	return agent_lifecycle.SubscriptionConfig{
		MetricsFilter:  cfg.GetMetricsFilter(),
		HealthStatuses: cfg.GetHealthStatuses(),
		UpdatedAt:      updatedAt,
	}
}

func toProtoSandboxResult(res *agent_lifecycle.SandboxRunResult) *agentv1.SandboxResult {
	if res == nil {
		return nil
	}
	executedAt := ""
	if !res.ExecutedAt.IsZero() {
		executedAt = res.ExecutedAt.UTC().Format(time.RFC3339)
	}
	return &agentv1.SandboxResult{
		Status:     res.Status,
		ReportUrl:  res.ReportURL,
		Profile:    res.Profile,
		ExecutedAt: executedAt,
		Metrics:    res.Metrics,
	}
}

func toProtoShare(share *agent_lifecycle.AgentShare) *agentv1.AgentShareResponse {
	if share == nil {
		return nil
	}
	resp := &agentv1.AgentShareResponse{
		ShareId:  share.ID.String(),
		Status:   share.Status,
		TenantId: share.TenantID,
		IssuedAt: share.CreatedAt,
	}
	if share.RevokedAt != nil {
		resp.RevokedAt = *share.RevokedAt
	}
	return resp
}

func shareQuotasFromProto(items []*agentv1.ShareQuota) []agent_lifecycle.ShareQuota {
	if len(items) == 0 {
		return nil
	}
	quotas := make([]agent_lifecycle.ShareQuota, 0, len(items))
	for _, item := range items {
		quotas = append(quotas, agent_lifecycle.ShareQuota{
			Type:  item.GetType(),
			Limit: item.GetLimit(),
		})
	}
	return quotas
}

func toStatusError(err error) error {
	switch {
	case errors.Is(err, agent_lifecycle.ErrAliasConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, agent_lifecycle.ErrAgentNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, agent_lifecycle.ErrInvalidStatusTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, agent_lifecycle.ErrInvalidCapacity), errors.Is(err, agent_lifecycle.ErrCapacityExceeded):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, agent_lifecycle.ErrInvalidSubscription):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, agent_lifecycle.ErrInvalidManifestPayload):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, agent_lifecycle.ErrInvalidManifestSignature):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, agent_lifecycle.ErrSandboxExecutionFailed):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, agent_lifecycle.ErrAgentShareExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, agent_lifecycle.ErrAgentShareNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, agent_lifecycle.ErrShareValidationFailed):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
