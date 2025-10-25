package capability_registry

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	discovery "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
)

// DiscoveryServer 提供 CapabilityDiscoveryService 的 gRPC 实现。
type DiscoveryServer struct {
	capabilityRegistryPB.UnimplementedCapabilityDiscoveryServiceServer
	service *discovery.Service
}

// NewDiscoveryServer 创建 Discovery gRPC Server。
func NewDiscoveryServer(service *discovery.Service) *DiscoveryServer {
	return &DiscoveryServer{service: service}
}

// RegisterCapabilityDiscoveryServer 将服务注册到 gRPC Server。
func RegisterCapabilityDiscoveryServer(server *grpc.Server, service *discovery.Service) {
	if server == nil || service == nil {
		return
	}
	capabilityRegistryPB.RegisterCapabilityDiscoveryServiceServer(server, &DiscoveryServer{service: service})
}

// GetSnapshot 返回单个能力快照。
func (s *DiscoveryServer) GetSnapshot(ctx context.Context, req *capabilityRegistryPB.GetSnapshotRequest) (*capabilityRegistryPB.GetSnapshotResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "discovery service unavailable")
	}
	if req.GetId() == nil {
		return nil, status.Error(codes.InvalidArgument, "tenant scoped id required")
	}
	id := req.GetId()
	snapshot, err := s.service.GetSnapshot(ctx, id.GetTenantId(), id.GetCapabilityId(), "")
	if err != nil {
		switch err {
		case discovery.ErrInvalidRequest:
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case discovery.ErrSnapshotNotFound:
			return nil, status.Error(codes.NotFound, err.Error())
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &capabilityRegistryPB.GetSnapshotResponse{
		Registration: toProtoRegistration(snapshot),
		ExpiresAt:    snapshot.ExpiresAt.Unix(),
	}, nil
}

// ListSnapshots 根据租户批量返回能力快照。
func (s *DiscoveryServer) ListSnapshots(ctx context.Context, req *capabilityRegistryPB.ListSnapshotsRequest) (*capabilityRegistryPB.ListSnapshotsResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "discovery service unavailable")
	}
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant id required")
	}
	snapshots, err := s.service.Sync(ctx, discovery.SyncRequest{
		TenantID:     req.GetTenantId(),
		Capabilities: req.GetCapabilityIds(),
		ClientID:     "",
		Force:        true,
	})
	if err != nil {
		if err == discovery.ErrInvalidRequest {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp := &capabilityRegistryPB.ListSnapshotsResponse{
		Snapshots: make([]*capabilityRegistryPB.GetSnapshotResponse, 0, len(snapshots)),
	}
	for _, snapshot := range snapshots {
		resp.Snapshots = append(resp.Snapshots, &capabilityRegistryPB.GetSnapshotResponse{
			Registration: toProtoRegistration(snapshot),
			ExpiresAt:    snapshot.ExpiresAt.Unix(),
		})
	}
	return resp, nil
}

func toProtoRegistration(snapshot discovery.Snapshot) *capabilityRegistryPB.CapabilityRegistration {
	return &capabilityRegistryPB.CapabilityRegistration{
		Id: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: snapshot.CapabilityID,
			TenantId:     snapshot.TenantID,
		},
		Status:              "published",
		Version:             snapshot.Version,
		RoutingPolicy:       toProtoRoutingPolicy(snapshot.RoutingPolicy),
		Adapters:            toProtoAdapters(snapshot.Adapters),
		FallbackPlan:        toProtoFallback(snapshot.FallbackPlan),
		UpdatedAt:           snapshot.IssuedAt.Unix(),
		ToolGrantIds:        nil,
		EnvironmentPolicies: map[string]*capabilityRegistryPB.EnvironmentPolicy{},
	}
}

func toProtoRoutingPolicy(policy registry.RoutingPolicy) *capabilityRegistryPB.RoutingPolicy {
	return &capabilityRegistryPB.RoutingPolicy{
		Strategy:         policy.Strategy,
		FallbackSequence: policy.FallbackSequence,
		CooldownSeconds:  int32(policy.CooldownSeconds),
		RateLimit:        toProtoRateLimit(policy.RateLimit),
		TenantStrategies: policy.TenantStrategies,
		StickyKeys:       policy.StickyKeys,
	}
}

func toProtoRateLimit(limit *registry.RateLimit) *capabilityRegistryPB.RateLimit {
	if limit == nil {
		return nil
	}
	return &capabilityRegistryPB.RateLimit{
		Limit:         limit.Limit,
		WindowSeconds: limit.WindowSeconds,
	}
}

func toProtoAdapters(adapters []registry.AdapterEndpoint) []*capabilityRegistryPB.AdapterEndpoint {
	result := make([]*capabilityRegistryPB.AdapterEndpoint, 0, len(adapters))
	for _, adapter := range adapters {
		result = append(result, &capabilityRegistryPB.AdapterEndpoint{
			AdapterId:     adapter.AdapterID,
			TransportType: adapter.TransportType,
			Endpoint:      adapter.Endpoint,
			Weight:        int32(adapter.Weight),
			TimeoutMs:     int32(adapter.TimeoutMS),
			MaxConcurrency: func(v *int) uint32 {
				if v == nil {
					return 0
				}
				if *v < 0 {
					return 0
				}
				return uint32(*v)
			}(adapter.MaxConcurrency),
			Labels: adapter.Labels,
			Visibility: &capabilityRegistryPB.VisibilityPolicy{
				AllowEnvironments: adapter.Visibility.Environments.Allow,
				DenyEnvironments:  adapter.Visibility.Environments.Deny,
				AllowTenants:      adapter.Visibility.Tenants.Allow,
				DenyTenants:       adapter.Visibility.Tenants.Deny,
			},
		})
	}
	return result
}

func toProtoFallback(plan *registry.FallbackPlan) *capabilityRegistryPB.FallbackPlan {
	if plan == nil {
		return nil
	}
	resp := &capabilityRegistryPB.FallbackPlan{
		FallbackTargets: plan.FallbackTargets,
	}
	if plan.StaticResponse != nil {
		payload, _ := json.Marshal(plan.StaticResponse.Payload)
		resp.StaticResponse = &capabilityRegistryPB.StaticResponse{
			PayloadJson: string(payload),
			TtlSeconds:  plan.StaticResponse.TTLSeconds,
		}
	}
	return resp
}
