package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	capabilityRegistryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RegistryServer 实现 CapabilityRegistryService。
type RegistryServer struct {
	capabilityRegistryPB.UnimplementedCapabilityRegistryServiceServer
	svc *capabilityRegistryService.Service
}

// RegisterCapabilityRegistryServer 注册服务到 gRPC server。
func RegisterCapabilityRegistryServer(server *grpc.Server, svc *capabilityRegistryService.Service) {
	capabilityRegistryPB.RegisterCapabilityRegistryServiceServer(server, &RegistryServer{svc: svc})
}

func (s *RegistryServer) CreateCapability(ctx context.Context, req *capabilityRegistryPB.CreateCapabilityRequest) (*capabilityRegistryPB.CreateCapabilityResponse, error) {
	if req.GetRegistration() == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("registration required"), nil)
	}
	payload, err := pbToPayload(req.GetRegistration())
	if err != nil {
		return nil, err
	}
	reg, err := s.svc.CreateRegistration(ctx, capabilityRegistryService.CreateRegistrationInput{
		Registration: payload,
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &capabilityRegistryPB.CreateCapabilityResponse{Registration: registrationToPB(reg)}, nil
}

func (s *RegistryServer) UpdateCapability(ctx context.Context, req *capabilityRegistryPB.UpdateCapabilityRequest) (*capabilityRegistryPB.UpdateCapabilityResponse, error) {
	if req.GetRegistration() == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("registration required"), nil)
	}
	payload, err := pbToPayload(req.GetRegistration())
	if err != nil {
		return nil, err
	}
	payload.Version = req.GetRegistration().GetVersion()
	reg, err := s.svc.UpdateRegistration(ctx, capabilityRegistryService.UpdateRegistrationInput{Registration: payload})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &capabilityRegistryPB.UpdateCapabilityResponse{Registration: registrationToPB(reg)}, nil
}

func (s *RegistryServer) GetCapability(ctx context.Context, req *capabilityRegistryPB.GetCapabilityRequest) (*capabilityRegistryPB.GetCapabilityResponse, error) {
	if req.GetId() == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("id required"), nil)
	}
	tenantUUID, err := tenantUUIDFromScopedID(req.GetId())
	if err != nil {
		return nil, err
	}
	options := capabilityRegistryService.GetRegistrationOptions{VersionSelector: req.GetVersion()}
	if v, convErr := parseUint(req.GetVersion()); convErr == nil {
		options.Version = v
	}
	reg, err := s.svc.GetRegistration(ctx, req.GetId().GetCapabilityId(), tenantUUID, options)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &capabilityRegistryPB.GetCapabilityResponse{Registration: registrationToPB(reg)}, nil
}

func (s *RegistryServer) DisableCapability(ctx context.Context, req *capabilityRegistryPB.DisableCapabilityRequest) (*capabilityRegistryPB.DisableCapabilityResponse, error) {
	if req.GetId() == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("id required"), nil)
	}
	tenantUUID, err := tenantUUIDFromScopedID(req.GetId())
	if err != nil {
		return nil, err
	}
	reg, err := s.svc.DisableRegistration(ctx, capabilityRegistryService.DisableRegistrationInput{
		CapabilityID: req.GetId().GetCapabilityId(),
		TenantUUID:   tenantUUID,
		Reason:       req.GetReason(),
	})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &capabilityRegistryPB.DisableCapabilityResponse{Registration: registrationToPB(reg)}, nil
}

func (s *RegistryServer) StreamUpdates(*capabilityRegistryPB.StreamUpdatesRequest, capabilityRegistryPB.CapabilityRegistryService_StreamUpdatesServer) error {
	return status.Error(codes.Unimplemented, "stream updates not implemented")
}

func pbToPayload(pbReg *capabilityRegistryPB.CapabilityRegistration) (capabilityRegistryService.RegistrationPayload, error) {
	if pbReg == nil || pbReg.GetId() == nil {
		return capabilityRegistryService.RegistrationPayload{}, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("registration id required"), nil)
	}
	tenantUUID, err := tenantUUIDFromScopedID(pbReg.GetId())
	if err != nil {
		return capabilityRegistryService.RegistrationPayload{}, err
	}
	environmentPolicies := make(map[string]capabilityRegistryService.EnvironmentPolicy, len(pbReg.GetEnvironmentPolicies()))
	for key, value := range pbReg.GetEnvironmentPolicies() {
		environmentPolicies[key] = capabilityRegistryService.EnvironmentPolicy{
			IsEnabled: value.GetIsEnabled(),
			Overrides: value.GetOverrides(),
		}
	}

	adapters := make([]capabilityRegistryService.AdapterEndpoint, 0, len(pbReg.GetAdapters()))
	for _, adapter := range pbReg.GetAdapters() {
		visibility := capabilityRegistryService.VisibilityPolicy{}
		if visMsg := adapter.GetVisibility(); visMsg != nil {
			visibility = capabilityRegistryService.VisibilityPolicy{
				Environments: capabilityRegistryService.VisibilityRule{Allow: visMsg.GetAllowEnvironments(), Deny: visMsg.GetDenyEnvironments()},
				Tenants:      capabilityRegistryService.VisibilityRule{Allow: visMsg.GetAllowTenants(), Deny: visMsg.GetDenyTenants()},
			}
		}
		adapters = append(adapters, capabilityRegistryService.AdapterEndpoint{
			AdapterID:     adapter.GetAdapterId(),
			TransportType: adapter.GetTransportType(),
			Endpoint:      adapter.GetEndpoint(),
			Weight:        int(adapter.GetWeight()),
			TimeoutMS:     int(adapter.GetTimeoutMs()),
			MaxConcurrency: func(v uint32) *int {
				if v == 0 {
					return nil
				}
				i := int(v)
				return &i
			}(adapter.GetMaxConcurrency()),
			Labels:     mapCopy(adapter.GetLabels()),
			Visibility: visibility,
		})
	}

	routingPolicy := capabilityRegistryService.RoutingPolicy{}
	if routing := pbReg.GetRoutingPolicy(); routing != nil {
		routingPolicy = capabilityRegistryService.RoutingPolicy{
			Strategy:         routing.GetStrategy(),
			TenantStrategies: mapCopy(routing.GetTenantStrategies()),
			FallbackSequence: routing.GetFallbackSequence(),
			CooldownSeconds:  int(routing.GetCooldownSeconds()),
			StickyKeys:       routing.GetStickyKeys(),
		}
		if routing.GetRateLimit() != nil {
			routingPolicy.RateLimit = &capabilityRegistryService.RateLimit{
				Limit:         routing.GetRateLimit().GetLimit(),
				WindowSeconds: routing.GetRateLimit().GetWindowSeconds(),
			}
		}
	}

	var fallbackPlan *capabilityRegistryService.FallbackPlan
	if fallback := pbReg.GetFallbackPlan(); fallback != nil {
		fallbackPlan = &capabilityRegistryService.FallbackPlan{FallbackTargets: fallback.GetFallbackTargets()}
		if fallback.GetStaticResponse() != nil {
			var payload map[string]interface{}
			if raw := fallback.GetStaticResponse().GetPayloadJson(); raw != "" {
				if err := json.Unmarshal([]byte(raw), &payload); err == nil {
					fallbackPlan.StaticResponse = &capabilityRegistryService.StaticResponse{Payload: payload, TTLSeconds: fallback.GetStaticResponse().GetTtlSeconds()}
				}
			}
		}
	}

	return capabilityRegistryService.RegistrationPayload{
		CapabilityID:        pbReg.GetId().GetCapabilityId(),
		TenantUUID:          tenantUUID,
		ContractRef:         pbReg.GetContractRef(),
		Status:              pbReg.GetStatus(),
		EnvironmentPolicies: environmentPolicies,
		Adapters:            adapters,
		RoutingPolicy:       routingPolicy,
		FallbackPlan:        fallbackPlan,
		ToolGrantIDs:        pbReg.GetToolGrantIds(),
		Version:             pbReg.GetVersion(),
	}, nil
}

func registrationToPB(reg capabilityRegistryService.Registration) *capabilityRegistryPB.CapabilityRegistration {
	response := &capabilityRegistryPB.CapabilityRegistration{
		Id: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: reg.CapabilityID,
			TenantUuid:   reg.TenantUUID,
		},
		ContractRef: reg.ContractRef,
		Status:      reg.Status,
		EnvironmentPolicies: func() map[string]*capabilityRegistryPB.EnvironmentPolicy {
			policies := make(map[string]*capabilityRegistryPB.EnvironmentPolicy, len(reg.EnvironmentPolicies))
			for key, value := range reg.EnvironmentPolicies {
				policies[key] = &capabilityRegistryPB.EnvironmentPolicy{
					IsEnabled: value.IsEnabled,
					Overrides: value.Overrides,
				}
			}
			return policies
		}(),
		Adapters: make([]*capabilityRegistryPB.AdapterEndpoint, 0, len(reg.Adapters)),
		RoutingPolicy: &capabilityRegistryPB.RoutingPolicy{
			Strategy:         reg.RoutingPolicy.Strategy,
			FallbackSequence: reg.RoutingPolicy.FallbackSequence,
			CooldownSeconds:  int32(reg.RoutingPolicy.CooldownSeconds),
			TenantStrategies: reg.RoutingPolicy.TenantStrategies,
			StickyKeys:       reg.RoutingPolicy.StickyKeys,
		},
		ToolGrantIds: reg.ToolGrantIDs,
		Version:      reg.Version,
		UpdatedAt:    reg.UpdatedAt.Unix(),
		UpdatedBy:    reg.UpdatedBy,
	}

	for _, adapter := range reg.Adapters {
		var maxConcurrency uint32
		if adapter.MaxConcurrency != nil {
			maxConcurrency = uint32(*adapter.MaxConcurrency)
		}
		response.Adapters = append(response.Adapters, &capabilityRegistryPB.AdapterEndpoint{
			AdapterId:      adapter.AdapterID,
			TransportType:  adapter.TransportType,
			Endpoint:       adapter.Endpoint,
			Weight:         int32(adapter.Weight),
			TimeoutMs:      int32(adapter.TimeoutMS),
			MaxConcurrency: maxConcurrency,
			Labels:         adapter.Labels,
			Visibility: &capabilityRegistryPB.VisibilityPolicy{
				AllowEnvironments: adapter.Visibility.Environments.Allow,
				DenyEnvironments:  adapter.Visibility.Environments.Deny,
				AllowTenants:      adapter.Visibility.Tenants.Allow,
				DenyTenants:       adapter.Visibility.Tenants.Deny,
			},
		})
	}

	if reg.RoutingPolicy.RateLimit != nil {
		response.RoutingPolicy.RateLimit = &capabilityRegistryPB.RateLimit{
			Limit:         reg.RoutingPolicy.RateLimit.Limit,
			WindowSeconds: reg.RoutingPolicy.RateLimit.WindowSeconds,
		}
	}

	if reg.FallbackPlan != nil {
		staticResponse := &capabilityRegistryPB.StaticResponse{}
		if reg.FallbackPlan.StaticResponse != nil {
			if payloadBytes, err := json.Marshal(reg.FallbackPlan.StaticResponse.Payload); err == nil {
				staticResponse.PayloadJson = string(payloadBytes)
			}
			staticResponse.TtlSeconds = reg.FallbackPlan.StaticResponse.TTLSeconds
		}
		response.FallbackPlan = &capabilityRegistryPB.FallbackPlan{
			FallbackTargets: reg.FallbackPlan.FallbackTargets,
			StaticResponse:  staticResponse,
		}
	}

	if !reg.UpdatedAt.IsZero() {
		response.UpdatedAt = reg.UpdatedAt.Unix()
	}
	return response
}

func toStatusError(err error) error {
	switch {
	case errors.Is(err, capabilityRegistryService.ErrRegistrationNotFound):
		return capability_registrydto.ToGRPCError(capability_registrydto.ErrNotFound, err)
	case errors.Is(err, capabilityRegistryService.ErrVersionConflict):
		return capability_registrydto.ToGRPCError(capability_registrydto.ErrVersionConflict, err)
	case errors.Is(err, capabilityRegistryService.ErrInvalidPayload):
		return capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidPayload, err)
	default:
		return capability_registrydto.ToGRPCError(capability_registrydto.ErrInternal, err)
	}
}

func mapCopy[K comparable, V any](source map[K]V) map[K]V {
	if len(source) == 0 {
		return nil
	}
	result := make(map[K]V, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func parseUint(val string) (uint64, error) {
	return strconv.ParseUint(val, 10, 64)
}
