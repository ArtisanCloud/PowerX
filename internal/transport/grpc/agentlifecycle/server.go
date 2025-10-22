package agentlifecycle

import (
	"context"
	"errors"
	"strings"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
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
	return &agentv1.RegisterAgentResponse{Agent: toProtoAgent(res)}, nil
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
	agent, err := s.service.Activate(ctx, agent_lifecycle.ActivateInput{
		AgentID:     id,
		TenantID:    "",
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
		TraceID:     req.GetTraceId(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &agentv1.LifecycleEventResponse{Event: &agentv1.LifecycleEvent{
		Id:        agent.ID.String(),
		AgentId:   agent.ID.String(),
		Type:      agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_ACTIVATE,
		ToStatus:  agentStatusToProto(agent.Status),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}}, nil
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

func toStatusError(err error) error {
	switch {
	case errors.Is(err, agent_lifecycle.ErrAliasConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, agent_lifecycle.ErrAgentNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, agent_lifecycle.ErrInvalidStatusTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
