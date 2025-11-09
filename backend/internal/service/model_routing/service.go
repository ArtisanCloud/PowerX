package model_routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const routingCacheKey = "agent:modelhub:routing:%s:%s"

// Service manages routing policy versions, caching, and telemetry.
type Service struct {
	db    *gorm.DB
	cache cache.ICache
	audit audit.Service
	repo  *repo.RoutingPolicyRepository
	inst  *instrumentation.Instrumentation
	clock func() time.Time
}

type Options struct {
	shared.Options
	Policies *repo.RoutingPolicyRepository
}

func NewService(opts Options) *Service {
	opts.Options.Normalize()
	r := opts.Policies
	if r == nil && opts.DB != nil {
		r = repo.NewRoutingPolicyRepository(opts.DB)
	}
	return &Service{
		db:    opts.DB,
		cache: opts.Cache,
		audit: opts.AuditSvc,
		repo:  r,
		inst:  opts.Instrumentation,
		clock: opts.Clock,
	}
}

// PolicyInput is an internal helper for building routing policy versions.
type PolicyInput struct {
	TenantScope        string
	Rules              datatypes.JSON
	FallbackChain      datatypes.JSON
	SafeModeThresholds datatypes.JSONMap
	ApprovalRecord     datatypes.JSONMap
	Status             string
}

// UpsertPolicyVersion creates a new routing policy version and caches it for fast lookups.
func (s *Service) UpsertPolicyVersion(ctx context.Context, env string, input PolicyInput) (*model.RoutingPolicy, error) {
	if s.repo == nil {
		return nil, errors.New("routing repository is not configured")
	}
	scope := strings.TrimSpace(input.TenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	version, err := s.repo.NextVersion(ctx, env, scope)
	if err != nil {
		return nil, err
	}
	record := &model.RoutingPolicy{
		Env:                env,
		TenantScope:        scope,
		Version:            version,
		Status:             sanitizeStatus(input.Status),
		Rules:              input.Rules,
		FallbackChain:      input.FallbackChain,
		SafeModeThresholds: defaultMap(input.SafeModeThresholds),
		ApprovalRecord:     defaultMap(input.ApprovalRecord),
	}
	result, err := s.repo.CreateVersion(ctx, record)
	if err != nil {
		return nil, err
	}
	s.cachePolicy(ctx, result)
	s.emitAudit(ctx, "routing.policy.upsert", result, nil)
	s.inst.RecordMetric(ctx, "agent.routing.policy_versions", 1, map[string]string{
		"tenant_scope": scope,
	})
	return result, nil
}

// LatestPolicy fetches the latest routing policy for a tenant scope with redis caching.
func (s *Service) LatestPolicy(ctx context.Context, env, tenantScope string) (*model.RoutingPolicy, error) {
	if s.repo == nil {
		return nil, errors.New("routing repository is not configured")
	}
	if cached := s.fetchFromCache(ctx, env, tenantScope); cached != nil {
		return cached, nil
	}
	policy, err := s.repo.Latest(ctx, env, tenantScope, "active")
	if err != nil {
		return nil, err
	}
	if policy != nil {
		s.cachePolicy(ctx, policy)
	}
	return policy, nil
}

func (s *Service) cachePolicy(ctx context.Context, policy *model.RoutingPolicy) {
	if s.cache == nil || policy == nil {
		return
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		return
	}
	key := fmt.Sprintf(routingCacheKey, policy.Env, policy.TenantScope)
	if err := s.cache.Set(ctx, key, payload, 5*time.Minute); err != nil {
		logger.WarnF(ctx, "[model_routing] cache set failed: %v", err)
	}
}

func (s *Service) fetchFromCache(ctx context.Context, env, tenantScope string) *model.RoutingPolicy {
	if s.cache == nil {
		return nil
	}
	key := fmt.Sprintf(routingCacheKey, env, tenantScope)
	raw, err := s.cache.Get(ctx, key)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var policy model.RoutingPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		_ = s.cache.Delete(ctx, key)
		return nil
	}
	return &policy
}

func (s *Service) emitAudit(ctx context.Context, op string, policy *model.RoutingPolicy, meta map[string]any) {
	if s.audit == nil || policy == nil {
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta["tenant_scope"] = policy.TenantScope
	meta["version"] = policy.Version
	payload, _ := json.Marshal(meta)
	_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
		Source:       "model_routing.service",
		Operation:    op,
		ResourceType: "agent.routing_policy",
		ResourceID:   fmt.Sprintf("%s:%d", policy.TenantScope, policy.Version),
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         datatypes.JSON(payload),
		OccurredAt:   s.clock(),
	})
}

func defaultMap(v datatypes.JSONMap) datatypes.JSONMap {
	if v == nil {
		return datatypes.JSONMap{}
	}
	return v
}

func sanitizeStatus(s string) string {
	val := strings.TrimSpace(strings.ToLower(s))
	if val == "" {
		return "draft"
	}
	return val
}
