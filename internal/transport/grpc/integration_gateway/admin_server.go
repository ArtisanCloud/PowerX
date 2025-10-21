package integration_gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	pbintegration "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration/gateway/v1"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AdminServer 实现 IntegrationGatewayAdminService。
type AdminServer struct {
	pbintegration.UnimplementedIntegrationGatewayAdminServiceServer
	svc *manager.Service
}

// NewAdminServer 构造 AdminServer。
func NewAdminServer(svc *manager.Service) *AdminServer {
	return &AdminServer{svc: svc}
}

func (s *AdminServer) CreateRoute(ctx context.Context, req *pbintegration.CreateRouteRequest) (*pbintegration.CreateRouteResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	route, err := s.svc.CreateRoute(ctx, manager.CreateRouteInput{
		TenantID:     strings.TrimSpace(req.GetTenantId()),
		Actor:        "grpc-admin",
		RouteSlug:    req.GetRouteSlug(),
		CapabilityID: req.GetCapabilityId(),
		ToolGrantIDs: req.GetToolGrantIds(),
		Channels:     req.GetChannels(),
		RateLimit:    protoToRateLimit(req.GetRateLimit()),
		EventTopics:  protoToEventTopics(req.GetEventTopics()),
		Description:  req.GetDescription(),
	})
	if err != nil {
		return nil, translateError(err)
	}

	return &pbintegration.CreateRouteResponse{
		Route: routeToProto(route),
	}, nil
}

func (s *AdminServer) GetRoute(ctx context.Context, req *pbintegration.GetRouteRequest) (*pbintegration.GetRouteResponse, error) {
	if req == nil || req.GetRouteId() == "" {
		return nil, status.Error(codes.InvalidArgument, "route_id is required")
	}
	routeID, err := uuidFromString(req.GetRouteId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid route_id")
	}

	route, err := s.svc.GetRoute(ctx, routeID)
	if err != nil {
		return nil, translateError(err)
	}

	return &pbintegration.GetRouteResponse{
		Route:   routeToProto(route),
		Version: route.CurrentVersion,
	}, nil
}

func (s *AdminServer) UpdateRoute(ctx context.Context, req *pbintegration.UpdateRouteRequest) (*pbintegration.UpdateRouteResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	routeID, err := uuidFromString(req.GetRouteId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid route_id")
	}
	if req.GetExpectVersion() == 0 {
		return nil, status.Error(codes.FailedPrecondition, "expect_version is required")
	}

	route, err := s.svc.UpdateRoute(ctx, manager.UpdateRouteInput{
		RouteID:      routeID,
		TenantID:     "",
		Actor:        "grpc-admin",
		Version:      req.GetExpectVersion(),
		CapabilityID: req.GetCapabilityId(),
		ToolGrantIDs: req.GetToolGrantIds(),
		Channels:     req.GetChannels(),
		RateLimit:    protoToRateLimit(req.GetRateLimit()),
		EventTopics:  protoToEventTopics(req.GetEventTopics()),
		Description:  optionalString(req.GetDescription()),
	})
	if err != nil {
		return nil, translateError(err)
	}

	return &pbintegration.UpdateRouteResponse{
		Route:   routeToProto(route),
		Version: route.CurrentVersion,
	}, nil
}

func (s *AdminServer) ListRoutes(ctx context.Context, req *pbintegration.ListRoutesRequest) (*pbintegration.ListRoutesResponse, error) {
	if req == nil || strings.TrimSpace(req.GetTenantId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	lifecycle := ""
	switch req.GetLifecycleState() {
	case pbintegration.IntegrationRoute_ACTIVE:
		lifecycle = manager.LifecycleActive
	case pbintegration.IntegrationRoute_SUSPENDED:
		lifecycle = manager.LifecycleSuspended
	case pbintegration.IntegrationRoute_RETIRED:
		lifecycle = manager.LifecycleRetired
	}

	routes, total, err := s.svc.ListRoutes(ctx, manager.ListRoutesInput{
		TenantID:       strings.TrimSpace(req.GetTenantId()),
		CapabilityID:   strings.TrimSpace(req.GetCapabilityId()),
		LifecycleState: lifecycle,
		Page:           int(req.GetPage()),
		PageSize:       int(req.GetPageSize()),
	})
	if err != nil {
		return nil, translateError(err)
	}

	summaries := make([]*pbintegration.IntegrationRouteSummary, 0, len(routes))
	for _, route := range routes {
		summaries = append(summaries, routeToSummaryProto(route))
	}

	return &pbintegration.ListRoutesResponse{
		Items:    summaries,
		Page:     req.GetPage(),
		PageSize: req.GetPageSize(),
		Total:    uint64(total),
	}, nil
}

func (s *AdminServer) ChangeLifecycle(ctx context.Context, req *pbintegration.ChangeLifecycleRequest) (*pbintegration.ChangeLifecycleResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	routeID, err := uuidFromString(req.GetRouteId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid route_id")
	}

	action, err := lifecycleActionFromProto(req.GetTargetState())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	route, err := s.svc.ChangeLifecycle(ctx, manager.ChangeLifecycleInput{
		RouteID:  routeID,
		TenantID: "",
		Actor:    "grpc-admin",
		Action:   action,
		Reason:   req.GetReason(),
	})
	if err != nil {
		return nil, translateError(err)
	}

	return &pbintegration.ChangeLifecycleResponse{Route: routeToProto(route)}, nil
}

func (s *AdminServer) ListRouteVersions(ctx context.Context, req *pbintegration.ListRouteVersionsRequest) (*pbintegration.ListRouteVersionsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	routeID, err := uuidFromString(req.GetRouteId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid route_id")
	}

	versions, err := s.svc.ListVersions(ctx, routeID)
	if err != nil {
		return nil, translateError(err)
	}

	items := make([]*pbintegration.IntegrationRouteVersion, 0, len(versions))
	for _, version := range versions {
		items = append(items, routeVersionToProto(version))
	}

	return &pbintegration.ListRouteVersionsResponse{Versions: items}, nil
}

func protoToRateLimit(rate *pbintegration.RateLimitPolicy) *manager.RateLimitPolicy {
	if rate == nil {
		return nil
	}
	policy := &manager.RateLimitPolicy{
		Limit:         rate.GetLimit(),
		Burst:         rate.GetBurst(),
		WindowSeconds: int(rate.GetWindowSeconds()),
	}
	switch rate.GetScope() {
	case pbintegration.RateLimitPolicy_PER_ROUTE:
		policy.Scope = "per_route"
	case pbintegration.RateLimitPolicy_PER_TENANT:
		policy.Scope = "per_tenant"
	case pbintegration.RateLimitPolicy_PER_ROUTE_PER_TENANT:
		policy.Scope = "per_route_per_tenant"
	default:
		policy.Scope = "per_route_per_tenant"
	}
	return policy
}

func protoToEventTopics(cfg *pbintegration.EventTopicConfig) *manager.EventTopics {
	if cfg == nil {
		return nil
	}
	return &manager.EventTopics{
		Created:             cfg.GetCreated(),
		Updated:             cfg.GetUpdated(),
		InvocationSucceeded: cfg.GetInvocationSucceeded(),
		InvocationFailed:    cfg.GetInvocationFailed(),
	}
}

func routeToProto(route manager.Route) *pbintegration.IntegrationRoute {
	resp := &pbintegration.IntegrationRoute{
		RouteId:        route.RouteID.String(),
		TenantId:       route.TenantID,
		RouteSlug:      route.RouteSlug,
		CapabilityId:   route.CapabilityID,
		ToolGrantIds:   route.ToolGrantIDs,
		Channels:       route.Channels,
		RateLimit:      rateLimitToProto(route.RateLimit),
		EventTopics:    eventTopicsToProto(route.EventTopics),
		CurrentVersion: route.CurrentVersion,
		Description:    route.Description,
	}
	switch route.LifecycleState {
	case manager.LifecyclePending:
		resp.LifecycleState = pbintegration.IntegrationRoute_PENDING
	case manager.LifecycleActive:
		resp.LifecycleState = pbintegration.IntegrationRoute_ACTIVE
	case manager.LifecycleSuspended:
		resp.LifecycleState = pbintegration.IntegrationRoute_SUSPENDED
	case manager.LifecycleRetired:
		resp.LifecycleState = pbintegration.IntegrationRoute_RETIRED
	default:
		resp.LifecycleState = pbintegration.IntegrationRoute_LIFECYCLE_STATE_UNSPECIFIED
	}
	if route.Status == manager.StatusEnabled {
		resp.Enabled = true
	}
	resp.CreatedAt = timestamppb.New(route.CreatedAt)
	resp.UpdatedAt = timestamppb.New(route.UpdatedAt)
	return resp
}

func routeToSummaryProto(route manager.Route) *pbintegration.IntegrationRouteSummary {
	summary := &pbintegration.IntegrationRouteSummary{
		RouteId:      route.RouteID.String(),
		TenantId:     route.TenantID,
		RouteSlug:    route.RouteSlug,
		CapabilityId: route.CapabilityID,
		Channels:     route.Channels,
		UpdatedAt:    timestamppb.New(route.UpdatedAt),
	}
	switch route.LifecycleState {
	case manager.LifecycleActive:
		summary.LifecycleState = pbintegration.IntegrationRoute_ACTIVE
	case manager.LifecycleSuspended:
		summary.LifecycleState = pbintegration.IntegrationRoute_SUSPENDED
	case manager.LifecycleRetired:
		summary.LifecycleState = pbintegration.IntegrationRoute_RETIRED
	default:
		summary.LifecycleState = pbintegration.IntegrationRoute_PENDING
	}
	return summary
}

func routeVersionToProto(version manager.RouteVersion) *pbintegration.IntegrationRouteVersion {
	return &pbintegration.IntegrationRouteVersion{
		Version:       version.Version,
		ChangeType:    version.ChangeType,
		ChangeSummary: version.ChangeSummary,
		ChangedBy:     version.ChangedBy,
		ChangedAt:     timestamppb.New(version.ChangedAt),
		TraceId:       version.TraceID,
		Snapshot:      routeToProto(version.Snapshot),
	}
}

func rateLimitToProto(policy manager.RateLimitPolicy) *pbintegration.RateLimitPolicy {
	resp := &pbintegration.RateLimitPolicy{
		Limit:         policy.Limit,
		Burst:         policy.Burst,
		WindowSeconds: uint32(policy.WindowSeconds),
	}
	switch strings.ToLower(policy.Scope) {
	case "per_route":
		resp.Scope = pbintegration.RateLimitPolicy_PER_ROUTE
	case "per_tenant":
		resp.Scope = pbintegration.RateLimitPolicy_PER_TENANT
	default:
		resp.Scope = pbintegration.RateLimitPolicy_PER_ROUTE_PER_TENANT
	}
	return resp
}

func eventTopicsToProto(topics manager.EventTopics) *pbintegration.EventTopicConfig {
	return &pbintegration.EventTopicConfig{
		Created:             topics.Created,
		Updated:             topics.Updated,
		InvocationSucceeded: topics.InvocationSucceeded,
		InvocationFailed:    topics.InvocationFailed,
	}
}

func optionalString(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	value := strings.TrimSpace(v)
	return &value
}

func lifecycleActionFromProto(state pbintegration.IntegrationRoute_LifecycleState) (string, error) {
	switch state {
	case pbintegration.IntegrationRoute_SUSPENDED:
		return "suspend", nil
	case pbintegration.IntegrationRoute_ACTIVE:
		return "resume", nil
	case pbintegration.IntegrationRoute_RETIRED:
		return "retire", nil
	default:
		return "", fmt.Errorf("unsupported lifecycle state: %s", state.String())
	}
}

func translateError(err error) error {
	switch {
	case errors.Is(err, manager.ErrRouteNotFound):
		return status.Error(codes.NotFound, "route not found")
	case errors.Is(err, manager.ErrVersionConflict):
		return status.Error(codes.FailedPrecondition, "version conflict")
	case errors.Is(err, manager.ErrSlugConflict):
		return status.Error(codes.AlreadyExists, "route slug already exists")
	default:
		if appErr, ok := err.(*dto.AppError); ok {
			return status.Error(codeFromHTTP(appErr.HTTPCode), appErr.Message)
		}
		return status.Error(codes.Internal, "integration gateway operation failed")
	}
}

func codeFromHTTP(code int) codes.Code {
	switch code {
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusPreconditionFailed:
		return codes.FailedPrecondition
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	default:
		return codes.Internal
	}
}

func uuidFromString(value string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(value))
}
