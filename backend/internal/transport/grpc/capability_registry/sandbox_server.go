package capability_registry

import (
	"context"
	"encoding/json"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	sandbox "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
)

// RouterSandboxServer 实现 CapabilityRouterSandboxService。
type RouterSandboxServer struct {
	capabilityRegistryPB.UnimplementedCapabilityRouterSandboxServiceServer
	service *sandbox.Service
}

// NewRouterSandboxServer 构造 gRPC Sandbox Server。
func NewRouterSandboxServer(service *sandbox.Service) *RouterSandboxServer {
	return &RouterSandboxServer{service: service}
}

// RegisterCapabilityRouterSandboxServer 注册 Sandbox 服务。
func RegisterCapabilityRouterSandboxServer(server *grpc.Server, service *sandbox.Service) {
	if server == nil || service == nil {
		return
	}
	capabilityRegistryPB.RegisterCapabilityRouterSandboxServiceServer(server, &RouterSandboxServer{service: service})
}

// Simulate 执行路由模拟。
func (s *RouterSandboxServer) Simulate(ctx context.Context, req *capabilityRegistryPB.SandboxInvokeRequest) (*capabilityRegistryPB.SandboxInvokeResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "sandbox service unavailable")
	}
	if req == nil || req.GetRequest() == nil || req.GetRequest().GetCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "request capability required")
	}
	invoke := req.GetRequest()
	capability := invoke.GetCapability()
	var override *router.Registration
	if req.GetRegistrationOverride() != nil {
		regOverride := req.GetRegistrationOverride()
		override = &router.Registration{
			CapabilityID:        regOverride.GetId().GetCapabilityId(),
			TenantID:            regOverride.GetId().GetTenantId(),
			ContractRef:         regOverride.GetContractRef(),
			Status:              regOverride.GetStatus(),
			EnvironmentPolicies: convertEnvironmentPolicies(regOverride.GetEnvironmentPolicies()),
			Adapters:            convertAdapters(regOverride.GetAdapters()),
			RoutingPolicy:       convertRoutingPolicy(regOverride.GetRoutingPolicy()),
			FallbackPlan:        convertFallbackPlan(regOverride.GetFallbackPlan()),
			ToolGrantIDs:        regOverride.GetToolGrantIds(),
			Version:             regOverride.GetVersion(),
			UpdatedAt:           time.Unix(regOverride.GetUpdatedAt(), 0),
			UpdatedBy:           regOverride.GetUpdatedBy(),
		}
	}
	result, err := s.service.SimulateInvoke(ctx, capability.GetCapabilityId(), capability.GetTenantId(), router.InvokeRequest{
		Payload:   invoke.GetPayload(),
		Timeout:   timeDurationFromProto(invoke.GetTimeoutMs()),
		StickyKey: invoke.GetStickyKey(),
	}, override)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp := &capabilityRegistryPB.SandboxInvokeResponse{
		Response: &capabilityRegistryPB.InvokeResponse{
			AdapterId:    result.AdapterID,
			Payload:      result.Payload,
			FallbackUsed: result.FallbackUsed,
			LatencyMs:    uint32(result.Latency / time.Millisecond),
		},
	}
	return resp, nil
}

func convertAdapters(adapters []*capabilityRegistryPB.AdapterEndpoint) []router.AdapterEndpoint {
	result := make([]router.AdapterEndpoint, 0, len(adapters))
	for _, ep := range adapters {
		result = append(result, router.AdapterEndpoint{
			AdapterID:     ep.GetAdapterId(),
			TransportType: ep.GetTransportType(),
			Endpoint:      ep.GetEndpoint(),
			Weight:        int(ep.GetWeight()),
			TimeoutMS:     int(ep.GetTimeoutMs()),
			MaxConcurrency: func(v uint32) *int {
				if v == 0 {
					return nil
				}
				i := int(v)
				return &i
			}(ep.GetMaxConcurrency()),
			Labels:     ep.GetLabels(),
			Visibility: convertVisibility(ep.GetVisibility()),
			IsActive:   true,
		})
	}
	return result
}

func convertRoutingPolicy(policy *capabilityRegistryPB.RoutingPolicy) router.RoutingPolicy {
	if policy == nil {
		return router.RoutingPolicy{}
	}
	result := router.RoutingPolicy{
		Strategy:         policy.GetStrategy(),
		TenantStrategies: policy.GetTenantStrategies(),
		FallbackSequence: policy.GetFallbackSequence(),
		CooldownSeconds:  int(policy.GetCooldownSeconds()),
		StickyKeys:       policy.GetStickyKeys(),
	}
	if policy.GetRateLimit() != nil {
		result.RateLimit = &router.RateLimit{
			Limit:         policy.GetRateLimit().GetLimit(),
			WindowSeconds: policy.GetRateLimit().GetWindowSeconds(),
		}
	}
	return result
}

func convertFallbackPlan(plan *capabilityRegistryPB.FallbackPlan) *router.FallbackPlan {
	if plan == nil {
		return nil
	}
	result := &router.FallbackPlan{
		FallbackTargets: plan.GetFallbackTargets(),
	}
	if plan.GetStaticResponse() != nil {
		payload := map[string]interface{}{}
		if raw := plan.GetStaticResponse().GetPayloadJson(); raw != "" {
			_ = json.Unmarshal([]byte(raw), &payload)
		}
		result.StaticResponse = &router.StaticResponse{
			Payload:    payload,
			TTLSeconds: plan.GetStaticResponse().GetTtlSeconds(),
		}
	}
	return result
}

func convertEnvironmentPolicies(policies map[string]*capabilityRegistryPB.EnvironmentPolicy) map[string]router.EnvironmentPolicy {
	if len(policies) == 0 {
		return nil
	}
	result := make(map[string]router.EnvironmentPolicy, len(policies))
	for env, policy := range policies {
		if policy == nil {
			continue
		}
		result[env] = router.EnvironmentPolicy{
			IsEnabled: policy.GetIsEnabled(),
			Overrides: policy.GetOverrides(),
		}
	}
	return result
}

func convertVisibility(visibility *capabilityRegistryPB.VisibilityPolicy) router.VisibilityPolicy {
	if visibility == nil {
		return router.VisibilityPolicy{}
	}
	return router.VisibilityPolicy{
		Environments: router.VisibilityRule{
			Allow: visibility.GetAllowEnvironments(),
			Deny:  visibility.GetDenyEnvironments(),
		},
		Tenants: router.VisibilityRule{
			Allow: visibility.GetAllowTenants(),
			Deny:  visibility.GetDenyTenants(),
		},
	}
}

func timeDurationFromProto(timeout uint32) time.Duration {
	if timeout == 0 {
		return 0
	}
	return time.Duration(timeout) * time.Millisecond
}
