package skills

import (
	"context"
	"errors"
	"strings"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

// InvokeRequest describes resolved invoke input for service layer.
type InvokeRequest struct {
	TenantUUID string
	SkillID    string
	Version    string
	Entrypoint string
	InvokePath string
	TraceID    string
}

// InvokeResolution returns selected version and normalized routing metadata.
type InvokeResolution struct {
	SkillID    string
	Version    string
	Entrypoint string
	InvokePath string
	TraceID    string
}

// InvokeService resolves versions and validates invocation prerequisites.
type InvokeService struct {
	registryRepo *skillrepo.SkillRegistryRepository
	auditService *AuditTraceService
}

func NewInvokeService(
	registryRepo *skillrepo.SkillRegistryRepository,
	auditService *AuditTraceService,
) *InvokeService {
	if registryRepo == nil {
		panic("invoke service requires registry repository")
	}
	return &InvokeService{registryRepo: registryRepo, auditService: auditService}
}

func (s *InvokeService) Resolve(ctx context.Context, req InvokeRequest) (*InvokeResolution, error) {
	req.TenantUUID = strings.ToLower(strings.TrimSpace(req.TenantUUID))
	req.SkillID = strings.ToLower(strings.TrimSpace(req.SkillID))
	req.Version = strings.TrimSpace(req.Version)
	req.Entrypoint = strings.TrimSpace(req.Entrypoint)
	req.InvokePath = strings.TrimSpace(strings.ToLower(req.InvokePath))

	if req.TenantUUID == "" {
		return nil, errors.New("tenant_uuid is required")
	}
	if req.SkillID == "" {
		return nil, errors.New("skill_id is required")
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "runbook.default"
	}
	if req.InvokePath == "" {
		req.InvokePath = "tenant.skills.invoke"
	}

	selectedVersion := req.Version
	if selectedVersion == "" {
		latest, err := s.registryRepo.GetLatestPublished(ctx, req.SkillID)
		if err != nil {
			return nil, err
		}
		selectedVersion = latest.Version
	}

	rec, err := s.registryRepo.GetBySkillVersion(ctx, req.SkillID, selectedVersion)
	if err != nil {
		return nil, err
	}
	if rec.Status != skillmodel.SkillStatusPublished {
		return nil, errors.New("skill version is not published")
	}

	resolution := &InvokeResolution{
		SkillID:    req.SkillID,
		Version:    selectedVersion,
		Entrypoint: req.Entrypoint,
		InvokePath: req.InvokePath,
		TraceID:    req.TraceID,
	}

	if s.auditService != nil {
		_ = s.auditService.RecordExecutionTrace(ctx, ExecutionTraceInput{
			TraceID:      req.TraceID,
			TenantUUID:   req.TenantUUID,
			SkillID:      resolution.SkillID,
			Version:      resolution.Version,
			Entrypoint:   resolution.Entrypoint,
			InvokePath:   resolution.InvokePath,
			ProtocolUsed: "skill",
			Status:       "resolved",
		})
	}

	return resolution, nil
}
