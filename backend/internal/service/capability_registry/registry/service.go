package registry

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registryRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
)

// Service 提供能力注册的领域操作。
type Service struct {
	repo              Repository
	bus               eventPublisher
	instrumentation   *domain.Instrumentation
	contracts         ContractVerifier
	toolGrants        ToolGrantVerifier
	auditor           auditAdapter
	now               func() time.Time
	versionGenerator  VersionGenerator
	systemActorLookup SystemActorResolver
}

type eventPublisher interface {
	Publish(eventType string, payload interface{}, ctx context.Context)
}

type auditAdapter interface {
	LogAPI(ctx context.Context, methodPath string, status int, latency time.Duration)
	LogBusPublish(ctx context.Context, topic string, subCount int)
	LogBusDeliver(ctx context.Context, topic, pluginID string, status int, err string)
	LogRBAC(ctx context.Context, subject string, resource string, action string, allow bool)
}

// NewService 创建能力注册服务。
func NewService(opts ServiceOptions) *Service {
	repository := opts.Repository
	if repository == nil {
		if opts.DB == nil {
			panic("registry.Service requires DB when Repository is nil")
		}
		repository = registryRepo.NewCapabilityRegistryRepository(opts.DB)
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	inst := opts.Instrumentation
	if inst == nil {
		inst = domain.NewInstrumentation(nil)
	}

	versionGen := opts.VersionGenerator
	if versionGen == nil {
		versionGen = SequenceVersionGenerator()
	}

	actorResolver := opts.SystemActorResolver
	if actorResolver == nil {
		actorResolver = func(context.Context) string { return "system" }
	}

	auditor := opts.Auditor
	if auditor == nil {
		auditor = auditNoop{}
	}

	return &Service{
		repo:              repository,
		bus:               opts.EventBus,
		instrumentation:   inst,
		contracts:         opts.ContractVerifier,
		toolGrants:        opts.ToolGrantVerifier,
		auditor:           auditor,
		now:               clock,
		versionGenerator:  versionGen,
		systemActorLookup: actorResolver,
	}
}

// SequenceVersionGenerator 返回一个简单的自增版本生成器。
func SequenceVersionGenerator() VersionGenerator {
	return func(_ context.Context, _ string, _ string, current uint64) (uint64, error) {
		return current + 1, nil
	}
}

// CreateRegistration 创建新的能力注册快照。
func (s *Service) CreateRegistration(ctx context.Context, in CreateRegistrationInput) (Registration, error) {
	start := s.now()
	actor := in.Actor
	if actor == "" {
		actor = s.systemActorLookup(ctx)
	}

	if err := s.validatePayload(ctx, in.Registration, false, actor); err != nil {
		return Registration{}, err
	}

	// 确保不存在旧快照
	current, err := s.repo.GetLatest(ctx, nil, in.Registration.CapabilityID, in.Registration.TenantID)
	if err != nil && err != ErrRegistrationNotFound {
		return Registration{}, err
	}
	var latestVersion uint64
	if err == nil {
		latestVersion = current.Version
		if current.Status != string(domain.RegistrationStatusDisabled) {
			return Registration{}, fmt.Errorf("registry exists: %w", ErrVersionConflict)
		}
	}

	nextVersion, err := s.versionGenerator(ctx, in.Registration.CapabilityID, in.Registration.TenantID, latestVersion)
	if err != nil {
		return Registration{}, err
	}

	reg := s.toDomainRegistration(in.Registration)
	reg.Version = nextVersion
	reg.UpdatedBy = actor
	reg.PublishedAt = timePtr(s.now())

	result, err := s.repo.Create(ctx, nil, reg)
	if err != nil {
		return Registration{}, err
	}

	s.publishUpdateEvent(ctx, "created", result)
	s.auditor.LogAPI(ctx, "registry.create", 201, s.now().Sub(start))
	return result, nil
}

// UpdateRegistration 生成新的注册版本。
func (s *Service) UpdateRegistration(ctx context.Context, in UpdateRegistrationInput) (Registration, error) {
	start := s.now()
	actor := in.Actor
	if actor == "" {
		actor = s.systemActorLookup(ctx)
	}

	if in.Registration.Version == 0 {
		return Registration{}, fmt.Errorf("missing version: %w", ErrVersionConflict)
	}

	if err := s.validatePayload(ctx, in.Registration, true, actor); err != nil {
		return Registration{}, err
	}

	latest, err := s.repo.GetLatest(ctx, nil, in.Registration.CapabilityID, in.Registration.TenantID)
	if err != nil {
		return Registration{}, err
	}
	if latest.Version != in.Registration.Version {
		return Registration{}, ErrVersionConflict
	}

	nextVersion, err := s.versionGenerator(ctx, in.Registration.CapabilityID, in.Registration.TenantID, latest.Version)
	if err != nil {
		return Registration{}, err
	}

	reg := s.toDomainRegistration(in.Registration)
	reg.Version = nextVersion
	reg.UpdatedBy = actor
	reg.PublishedAt = latest.PublishedAt

	result, err := s.repo.Update(ctx, nil, reg, latest.Version)
	if err != nil {
		return Registration{}, err
	}

	s.publishUpdateEvent(ctx, "updated", result)
	s.auditor.LogAPI(ctx, "registry.update", 200, s.now().Sub(start))
	return result, nil
}

// DisableRegistration 禁用能力。
func (s *Service) DisableRegistration(ctx context.Context, in DisableRegistrationInput) (Registration, error) {
	start := s.now()
	actor := in.Actor
	if actor == "" {
		actor = s.systemActorLookup(ctx)
	}

	latest, err := s.repo.GetLatest(ctx, nil, in.CapabilityID, in.TenantID)
	if err != nil {
		return Registration{}, err
	}

	expectedVersion := latest.Version
	if in.Version > 0 && in.Version != latest.Version {
		return Registration{}, ErrVersionConflict
	}

	nextVersion, err := s.versionGenerator(ctx, in.CapabilityID, in.TenantID, latest.Version)
	if err != nil {
		return Registration{}, err
	}

	payload := RegistrationPayload{
		CapabilityID:        latest.CapabilityID,
		TenantID:            latest.TenantID,
		ContractRef:         latest.ContractRef,
		Status:              string(domain.RegistrationStatusDisabled),
		EnvironmentPolicies: latest.EnvironmentPolicies,
		Adapters:            latest.Adapters,
		RoutingPolicy:       latest.RoutingPolicy,
		FallbackPlan:        latest.FallbackPlan,
		ToolGrantIDs:        latest.ToolGrantIDs,
		DisableReason:       in.Reason,
	}
	reg := s.toDomainRegistration(payload)
	reg.Version = nextVersion
	reg.UpdatedBy = actor
	reg.PublishedAt = latest.PublishedAt
	reg.DisableReason = in.Reason

	result, err := s.repo.Disable(ctx, nil, in.CapabilityID, in.TenantID, in.Reason, actor, expectedVersion, reg)
	if err != nil {
		return Registration{}, err
	}

	s.publishUpdateEvent(ctx, "disabled", result)
	s.auditor.LogAPI(ctx, "registry.disable", 202, s.now().Sub(start))
	return result, nil
}

// GetRegistration 查询能力注册。
func (s *Service) GetRegistration(ctx context.Context, capabilityID, tenantID string, opts GetRegistrationOptions) (Registration, error) {
	var (
		reg Registration
		err error
	)
	selector := strings.ToLower(opts.VersionSelector)
	switch {
	case selector == "" || selector == "latest" || selector == "draft":
		reg, err = s.repo.GetLatest(ctx, nil, capabilityID, tenantID)
	case opts.Version > 0:
		reg, err = s.repo.GetVersion(ctx, nil, capabilityID, tenantID, opts.Version)
	default:
		if selector != "" {
			if v, parseErr := strconv.ParseUint(selector, 10, 64); parseErr == nil {
				reg, err = s.repo.GetVersion(ctx, nil, capabilityID, tenantID, v)
			} else {
				err = ErrRegistrationNotFound
			}
		} else {
			err = ErrRegistrationNotFound
		}
	}
	if err != nil {
		return Registration{}, err
	}

	if !opts.IncludeDisabled && strings.EqualFold(reg.Status, string(domain.RegistrationStatusDisabled)) {
		return Registration{}, ErrRegistrationNotFound
	}
	return reg, nil
}

// ListLatest 列出租户最新快照。
func (s *Service) ListLatest(ctx context.Context, tenantID string, limit, offset int) ([]Registration, int64, error) {
	return s.repo.ListLatest(ctx, nil, tenantID, limit, offset)
}

func (s *Service) validatePayload(ctx context.Context, payload RegistrationPayload, allowVersion bool, actor string) error {
	if payload.CapabilityID == "" || payload.TenantID == "" {
		return fmt.Errorf("missing identifiers: %w", ErrInvalidPayload)
	}
	if payload.ContractRef == "" {
		return fmt.Errorf("missing contract: %w", ErrInvalidPayload)
	}
	status := payload.Status
	if status == "" {
		status = string(domain.RegistrationStatusPublished)
	}
	if !domain.IsValidRegistrationStatus(domain.RegistrationStatus(status)) {
		return fmt.Errorf("invalid status %s: %w", status, ErrInvalidPayload)
	}
	if len(payload.Adapters) == 0 {
		return fmt.Errorf("missing adapters: %w", ErrInvalidPayload)
	}

	seen := make(map[string]struct{}, len(payload.Adapters))
	for _, adapter := range payload.Adapters {
		if adapter.AdapterID == "" {
			return fmt.Errorf("adapter id required: %w", ErrInvalidPayload)
		}
		if _, ok := seen[adapter.AdapterID]; ok {
			return fmt.Errorf("duplicate adapter id %s: %w", adapter.AdapterID, ErrInvalidPayload)
		}
		seen[adapter.AdapterID] = struct{}{}
		if adapter.Weight < 0 {
			return fmt.Errorf("adapter weight negative: %w", ErrInvalidPayload)
		}
		if adapter.TransportType == "" {
			return fmt.Errorf("adapter transport missing: %w", ErrInvalidPayload)
		}
		if adapter.TimeoutMS <= 0 {
			return fmt.Errorf("adapter timeout invalid: %w", ErrInvalidPayload)
		}
	}

	cooldown := payload.RoutingPolicy.CooldownSeconds
	if cooldown == 0 {
		cooldown = 60
	}
	if cooldown < 30 {
		return fmt.Errorf("cooldown must be >=30: %w", ErrInvalidPayload)
	}
	if !domain.IsValidRoutingStrategy(domain.RoutingStrategy(payload.RoutingPolicy.Strategy)) {
		return fmt.Errorf("invalid routing strategy %s: %w", payload.RoutingPolicy.Strategy, ErrInvalidPayload)
	}

	if s.contracts != nil {
		if err := s.contracts.VerifyContract(ctx, payload.TenantID, payload.ContractRef); err != nil {
			return err
		}
	}
	if s.toolGrants != nil && len(payload.ToolGrantIDs) > 0 {
		err := s.toolGrants.VerifyToolGrants(ctx, payload.TenantID, payload.ToolGrantIDs)
		s.auditToolGrantCheck(ctx, actor, payload.TenantID, payload.ToolGrantIDs, err)
		if err != nil {
			return err
		}
	}

	if !allowVersion && payload.Version != 0 {
		return fmt.Errorf("version should not be provided for create")
	}
	return nil
}

func (s *Service) toDomainRegistration(payload RegistrationPayload) Registration {
	status := payload.Status
	if status == "" {
		status = string(domain.RegistrationStatusPublished)
	}
	envPolicies := payload.EnvironmentPolicies
	if envPolicies == nil {
		envPolicies = map[string]EnvironmentPolicy{}
	}
	policy := payload.RoutingPolicy
	if policy.CooldownSeconds == 0 {
		policy.CooldownSeconds = 60
	}
	return Registration{
		CapabilityID:        payload.CapabilityID,
		TenantID:            payload.TenantID,
		ContractRef:         payload.ContractRef,
		Status:              status,
		EnvironmentPolicies: envPolicies,
		Adapters:            payload.Adapters,
		RoutingPolicy:       policy,
		FallbackPlan:        payload.FallbackPlan,
		ToolGrantIDs:        payload.ToolGrantIDs,
		DisableReason:       payload.DisableReason,
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

type auditNoop struct{}

func (auditNoop) LogAPI(context.Context, string, int, time.Duration)         {}
func (auditNoop) LogBusPublish(context.Context, string, int)                 {}
func (auditNoop) LogBusDeliver(context.Context, string, string, int, string) {}
func (auditNoop) LogRBAC(context.Context, string, string, string, bool)      {}
