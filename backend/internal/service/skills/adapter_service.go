package skills

import (
	"context"
	"strings"

	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

// AdapterService bridges unified invocation requests to skill invocation.
type AdapterService struct {
	invokeSvc   *InvokeService
	bindingRepo *skillrepo.SkillCapabilityBindingRepository
}

type UnifiedInvokeRequest struct {
	TenantUUID        string
	CapabilityID      string
	PreferredProtocol string
	Payload           map[string]interface{}
	TraceID           string
}

type UnifiedInvokeResult struct {
	TraceID      string
	Status       string
	ProtocolUsed string
	FallbackUsed bool
	Result       map[string]interface{}
	SkillID      string
	Version      string
}

func NewAdapterService(invokeSvc *InvokeService, bindingRepo *skillrepo.SkillCapabilityBindingRepository) *AdapterService {
	if invokeSvc == nil || bindingRepo == nil {
		return nil
	}
	return &AdapterService{
		invokeSvc:   invokeSvc,
		bindingRepo: bindingRepo,
	}
}

func (s *AdapterService) InvokeUnified(ctx context.Context, req UnifiedInvokeRequest) (*UnifiedInvokeResult, error) {
	if s == nil {
		return nil, nil
	}
	if strings.TrimSpace(strings.ToLower(req.PreferredProtocol)) != "skill" {
		return nil, nil
	}
	binding, err := s.bindingRepo.GetLatestActiveByCapability(ctx, req.CapabilityID)
	if err != nil {
		return nil, err
	}
	resolved, err := s.invokeSvc.Resolve(ctx, InvokeRequest{
		TenantUUID: req.TenantUUID,
		SkillID:    binding.SkillID,
		Version:    binding.Version,
		InvokePath: "tenant.invocations",
		TraceID:    req.TraceID,
	})
	if err != nil {
		return nil, err
	}
	return &UnifiedInvokeResult{
		TraceID:      resolved.TraceID,
		Status:       "completed",
		ProtocolUsed: "skill",
		FallbackUsed: false,
		Result:       req.Payload,
		SkillID:      resolved.SkillID,
		Version:      resolved.Version,
	}, nil
}
