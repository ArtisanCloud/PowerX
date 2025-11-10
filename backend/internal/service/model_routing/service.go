package model_routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	amhrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	routingCacheKey   = "agent:modelhub:routing:%s:%s"
	safeModeCacheKey  = "agent:modelhub:safe_mode:%s:%s"
	statusDraft       = "draft"
	statusStaged      = "staged"
	statusActive      = "active"
	statusRolledBack  = "rolled_back"
	defaultSafeModeTT = 30 * time.Minute
)

var defaultSafeModeThresholds = datatypes.JSONMap{
	"minHitRate":             0.92,
	"maxLatencyMs":           float64(900),
	"maxFallbackFailureRate": 0.05,
}

type routingPolicyStore interface {
	NextVersion(ctx context.Context, env, tenantScope string) (uint32, error)
	CreateVersion(ctx context.Context, policy *model.RoutingPolicy) (*model.RoutingPolicy, error)
	Latest(ctx context.Context, env, tenantScope, status string) (*model.RoutingPolicy, error)
	UpdateStatus(ctx context.Context, env, tenantScope string, version uint32, status string, payload map[string]any) error
	FindVersion(ctx context.Context, env, tenantScope string, version uint32) (*model.RoutingPolicy, error)
}

// Service manages routing policy versions, caching, and telemetry.
type Service struct {
	db      *gorm.DB
	cache   cache.ICache
	audit   audit.Service
	repo    routingPolicyStore
	inst    *instrumentation.Instrumentation
	clock   func() time.Time
	signals ProviderSignalSource
	engine  *DecisionEngine
}

type Options struct {
	shared.Options
	Policies        routingPolicyStore
	ProviderSignals ProviderSignalSource
}

func NewService(opts Options) *Service {
	opts.Options.Normalize()
	r := opts.Policies
	if r == nil && opts.DB != nil {
		r = amhrepo.NewRoutingPolicyRepository(opts.DB)
	}
	signals := opts.ProviderSignals
	if signals == nil {
		signals = newRepoSignalSource(opts.DB)
	}
	engine := NewDecisionEngine(DecisionEngineOptions{
		Signals: signals,
		Clock:   opts.Clock,
	})
	return &Service{
		db:      opts.DB,
		cache:   opts.Cache,
		audit:   opts.AuditSvc,
		repo:    r,
		inst:    opts.Instrumentation,
		clock:   opts.Clock,
		signals: signals,
		engine:  engine,
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

// ApprovalUpdate captures approval workflow metadata for status transitions.
type ApprovalUpdate struct {
	WorkflowID        string
	RequestedBy       string
	Approvers         []string
	Outcome           string
	Notes             string
	RequiredApprovers uint32
	DecidedAt         time.Time
}

// StatusUpdateInput controls policy status transitions and auditing metadata.
type StatusUpdateInput struct {
	TargetStatus string
	Reason       string
	Actor        string
	Approval     *ApprovalUpdate
	Force        bool
}

// SafeModeState reflects the current toggle info for a tenant scope.
type SafeModeState struct {
	TenantScope string
	Env         string
	Enabled     bool
	Reason      string
	Actor       string
	UpdatedAt   time.Time
	ExpiresAt   *time.Time
}

// UpsertPolicyVersion creates a new routing policy version and caches it for fast lookups.
func (s *Service) UpsertPolicyVersion(ctx context.Context, env string, input PolicyInput) (*model.RoutingPolicy, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	if s.repo == nil {
		return nil, errors.New("routing repository is not configured")
	}
	scope := strings.TrimSpace(input.TenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	if len(input.Rules) == 0 {
		return nil, errors.New("rules required")
	}
	if err := validatePolicyRules(input.Rules); err != nil {
		return nil, err
	}
	env = strings.TrimSpace(env)
	if env == "" {
		env = "default"
	}
	ctx = instrumentation.WithTenant(ctx, scope)

	var err error
	spanCtx, span := s.inst.Tracer().StartSpan(ctx, "model_routing.upsert_policy", instrumentation.SpanAttributes(ctx, map[string]string{
		"tenant_scope": scope,
		"env":          env,
	}))
	ctx = spanCtx
	defer func() {
		span.End(err)
	}()

	version, err := s.repo.NextVersion(ctx, env, scope)
	if err != nil {
		return nil, err
	}
	status := sanitizeStatus(input.Status)
	if status == "" {
		status = statusDraft
	}
	record := &model.RoutingPolicy{
		Env:                env,
		TenantScope:        scope,
		Version:            version,
		Status:             status,
		Rules:              input.Rules,
		FallbackChain:      normalizeFallbackChain(input.FallbackChain),
		SafeModeThresholds: s.normalizeSafeModeThresholds(input.SafeModeThresholds),
		ApprovalRecord:     s.normalizeApprovalRecord(input.ApprovalRecord),
	}
	result, err := s.repo.CreateVersion(ctx, record)
	if err != nil {
		return nil, err
	}
	s.cachePolicy(ctx, result)
	s.emitAudit(ctx, "routing.policy.upsert", result, nil)
	s.inst.RecordMetric(ctx, "agent.routing.policy_versions", 1, map[string]string{
		"tenant_scope": scope,
		"env":          env,
	})
	return result, nil
}

// LatestPolicy fetches the latest routing policy for a tenant scope with redis caching.
func (s *Service) LatestPolicy(ctx context.Context, env, tenantScope string) (*model.RoutingPolicy, error) {
	if s.repo == nil {
		return nil, errors.New("routing repository is not configured")
	}
	scope := strings.TrimSpace(tenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	env = strings.TrimSpace(env)
	if env == "" {
		env = "default"
	}
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	ctx = instrumentation.WithTenant(ctx, scope)

	if cached := s.fetchFromCache(ctx, env, scope); cached != nil {
		return cached, nil
	}
	policy, err := s.repo.Latest(ctx, env, scope, statusActive)
	if err != nil {
		return nil, err
	}
	if policy != nil {
		s.cachePolicy(ctx, policy)
	}
	return policy, nil
}

// DecideRoute evaluates the active policy and returns a routing decision.
func (s *Service) DecideRoute(ctx context.Context, env, tenantScope string, taskCtx map[string]any) (*DecisionResult, error) {
	if s.engine == nil {
		return nil, errors.New("decision engine unavailable")
	}
	if taskCtx == nil {
		taskCtx = map[string]any{}
	}
	scope := strings.TrimSpace(tenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	env = strings.TrimSpace(env)
	if env == "" {
		env = "default"
	}
	ctx, traceID := instrumentation.EnsureTraceContext(ctx)
	ctx = instrumentation.WithTenant(ctx, scope)

	start := s.clock()
	policy, err := s.LatestPolicy(ctx, env, scope)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, fmt.Errorf("active policy not found for tenant %s", scope)
	}
	safeMode, err := s.SafeModeState(ctx, env, scope)
	if err != nil {
		logger.WarnF(ctx, "[model_routing] safe mode state read failed: %v", err)
	}
	input := DecisionInput{
		TenantScope:     scope,
		TaskContext:     taskCtx,
		SafeModeEnabled: safeMode != nil && safeMode.Enabled,
		Budget:          parseBudget(taskCtx),
	}
	result, err := s.engine.Decide(ctx, policy, input)
	if err != nil {
		return nil, err
	}
	result.TraceID = traceID
	result.PolicyVersion = policy.Version
	result.SafeMode = input.SafeModeEnabled

	duration := s.clock().Sub(start).Seconds()
	labels := map[string]string{
		"tenant_scope": scope,
		"env":          env,
	}
	s.inst.RecordMetric(ctx, "agent.routing.decision_latency", duration, labels)
	if result.FallbackUsed {
		s.inst.RecordMetric(ctx, "agent.routing.fallback_total", 1, labels)
	}
	if input.SafeModeEnabled {
		s.inst.RecordMetric(ctx, "agent.routing.safe_mode_active", 1, labels)
	}
	return result, nil
}

// UpdatePolicyStatus transitions a routing policy version through the approval workflow.
func (s *Service) UpdatePolicyStatus(ctx context.Context, env, tenantScope string, version uint32, input StatusUpdateInput) (*model.RoutingPolicy, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	if s.repo == nil {
		return nil, errors.New("routing repository is not configured")
	}
	rawTarget := strings.TrimSpace(strings.ToLower(input.TargetStatus))
	if rawTarget != "" && !isKnownStatus(rawTarget) {
		return nil, fmt.Errorf("unsupported target status %s", input.TargetStatus)
	}
	target := sanitizeStatus(input.TargetStatus)
	if target == "" {
		return nil, errors.New("target status required")
	}
	scope := strings.TrimSpace(tenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	env = strings.TrimSpace(env)
	if env == "" {
		env = "default"
	}
	ctx = instrumentation.WithTenant(ctx, scope)

	var err error
	spanCtx, span := s.inst.Tracer().StartSpan(ctx, "model_routing.update_policy_status", instrumentation.SpanAttributes(ctx, map[string]string{
		"tenant_scope": scope,
		"env":          env,
		"target":       target,
	}))
	ctx = spanCtx
	defer func() {
		span.End(err)
	}()

	policy, err := s.findPolicyVersion(ctx, env, scope, version)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, fmt.Errorf("policy not found for tenant %s version %d", scope, version)
	}

	current := sanitizeStatus(policy.Status)
	if current == target {
		return policy, nil
	}
	if !input.Force && !isTransitionAllowed(current, target) {
		return nil, fmt.Errorf("invalid status transition from %s to %s", current, target)
	}

	merged := s.mergeApprovalRecord(policy.ApprovalRecord, input.Approval)
	payload := map[string]any{
		"approval_record": merged,
	}

	if err = s.repo.UpdateStatus(ctx, env, scope, policy.Version, target, payload); err != nil {
		return nil, err
	}
	updated, err := s.repo.FindVersion(ctx, env, scope, policy.Version)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("policy version %d missing after status change", policy.Version)
	}

	if isActiveStatus(updated.Status) {
		s.cachePolicy(ctx, updated)
	} else {
		s.invalidateCache(ctx, env, scope)
	}

	meta := map[string]any{
		"previous_status": current,
		"target_status":   target,
	}
	if outcome := strings.TrimSpace(utils.ToStr(merged["outcome"])); outcome != "" {
		meta["approval_outcome"] = outcome
	}
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		meta["reason"] = reason
	}
	if actor := strings.TrimSpace(input.Actor); actor != "" {
		meta["actor"] = actor
	}
	s.emitAudit(ctx, statusChangeAuditOp(target), updated, meta)
	s.recordStatusMetrics(ctx, updated, target)

	return updated, nil
}

// RollbackPolicy marks a policy version as rolled_back (or latest if version is zero).
func (s *Service) RollbackPolicy(ctx context.Context, env, tenantScope string, version uint32) (*model.RoutingPolicy, error) {
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	reason := "manual rollback"
	if version > 0 {
		reason = fmt.Sprintf("manual rollback to version %d", version)
	}
	return s.UpdatePolicyStatus(ctx, env, tenantScope, version, StatusUpdateInput{
		TargetStatus: statusRolledBack,
		Reason:       reason,
		Force:        true,
	})
}

func (s *Service) cachePolicy(ctx context.Context, policy *model.RoutingPolicy) {
	if s.cache == nil || policy == nil {
		return
	}
	key := fmt.Sprintf(routingCacheKey, policy.Env, policy.TenantScope)
	if !isActiveStatus(policy.Status) {
		_ = s.cache.Delete(ctx, key)
		return
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		return
	}
	if err := s.cache.Set(ctx, key, payload, 5*time.Minute); err != nil {
		logger.WarnF(ctx, "[model_routing] cache set failed: %v", err)
	}
}

func (s *Service) invalidateCache(ctx context.Context, env, tenantScope string) {
	if s.cache == nil {
		return
	}
	key := fmt.Sprintf(routingCacheKey, env, tenantScope)
	if err := s.cache.Delete(ctx, key); err != nil {
		logger.WarnF(ctx, "[model_routing] cache delete failed: %v", err)
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
	if !isActiveStatus(policy.Status) {
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

func (s *Service) normalizeSafeModeThresholds(raw datatypes.JSONMap) datatypes.JSONMap {
	out := utils.CloneJSONMap(defaultSafeModeThresholds)
	if out == nil {
		out = datatypes.JSONMap{}
	}
	for k, v := range raw {
		out[k] = v
	}
	// Ensure numeric fields are floats/ints
	if _, ok := out["minHitRate"]; !ok {
		out["minHitRate"] = defaultSafeModeThresholds["minHitRate"]
	}
	if _, ok := out["maxLatencyMs"]; !ok {
		out["maxLatencyMs"] = defaultSafeModeThresholds["maxLatencyMs"]
	}
	if _, ok := out["maxFallbackFailureRate"]; !ok {
		out["maxFallbackFailureRate"] = defaultSafeModeThresholds["maxFallbackFailureRate"]
	}
	return out
}

func (s *Service) normalizeApprovalRecord(raw datatypes.JSONMap) datatypes.JSONMap {
	out := utils.CloneJSONMap(raw)
	if out == nil {
		out = datatypes.JSONMap{}
	}
	if _, ok := out["requestedAt"]; !ok {
		out["requestedAt"] = s.clock().UTC().Format(time.RFC3339)
	}
	if _, ok := out["requiredApprovers"]; !ok {
		out["requiredApprovers"] = 1
	}
	if outcome, ok := out["outcome"]; !ok || utils.ToStr(outcome) == "" {
		out["outcome"] = "pending"
	} else {
		out["outcome"] = strings.ToLower(utils.ToStr(outcome))
	}
	return out
}

func normalizeFallbackChain(raw datatypes.JSON) datatypes.JSON {
	if len(raw) == 0 {
		return datatypes.JSON(utils.MustJSONBytes([]string{}))
	}
	return raw
}

func validatePolicyRules(raw datatypes.JSON) error {
	if len(raw) == 0 {
		return errors.New("rules required")
	}
	var rules []struct {
		Candidates []map[string]any `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &rules); err != nil {
		return fmt.Errorf("invalid rules payload: %w", err)
	}
	if len(rules) == 0 {
		return errors.New("rules required")
	}
	for _, rule := range rules {
		for _, cand := range rule.Candidates {
			if id := strings.TrimSpace(utils.ToStr(cand["providerId"])); id != "" {
				return nil
			}
		}
	}
	return errors.New("rules must include at least one provider candidate")
}

func sanitizeStatus(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case statusStaged:
		return statusStaged
	case statusActive:
		return statusActive
	case statusRolledBack:
		return statusRolledBack
	case statusDraft, "":
		return statusDraft
	default:
		return ""
	}
}

func isKnownStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case statusDraft, statusStaged, statusActive, statusRolledBack:
		return true
	default:
		return false
	}
}

func isActiveStatus(status string) bool {
	return strings.EqualFold(status, statusActive)
}

func isTransitionAllowed(current, target string) bool {
	switch current {
	case statusDraft:
		return target == statusDraft || target == statusStaged || target == statusActive
	case statusStaged:
		return target == statusStaged || target == statusActive || target == statusRolledBack
	case statusActive:
		return target == statusActive || target == statusRolledBack
	case statusRolledBack:
		return target == statusDraft || target == statusStaged
	default:
		return target == statusDraft
	}
}

func statusChangeAuditOp(target string) string {
	switch target {
	case statusStaged:
		return "routing.policy.stage"
	case statusActive:
		return "routing.policy.activate"
	case statusRolledBack:
		return "routing.policy.rollback"
	default:
		return "routing.policy.status_change"
	}
}

func safeModeAuditOp(enabled bool) string {
	if enabled {
		return "routing.safe_mode.enable"
	}
	return "routing.safe_mode.disable"
}

func (s *Service) recordStatusMetrics(ctx context.Context, policy *model.RoutingPolicy, target string) {
	labels := map[string]string{
		"tenant_scope": policy.TenantScope,
		"env":          policy.Env,
	}
	switch target {
	case statusStaged:
		s.inst.RecordMetric(ctx, "agent.routing.policy_stage_total", 1, labels)
	case statusActive:
		s.inst.RecordMetric(ctx, "agent.routing.policy_publish_total", 1, labels)
		latency := s.clock().Sub(policy.CreatedAt).Seconds()
		if latency < 0 {
			latency = 0
		}
		s.inst.RecordMetric(ctx, "agent.routing.policy_publish_latency", latency, labels)
	case statusRolledBack:
		s.inst.RecordMetric(ctx, "agent.routing.policy_rollback_total", 1, labels)
	}
}

func (s *Service) mergeApprovalRecord(existing datatypes.JSONMap, update *ApprovalUpdate) datatypes.JSONMap {
	record := s.normalizeApprovalRecord(existing)
	if update == nil {
		return record
	}
	if wf := strings.TrimSpace(update.WorkflowID); wf != "" {
		record["workflowId"] = wf
	}
	if requestedBy := strings.TrimSpace(update.RequestedBy); requestedBy != "" {
		record["requestedBy"] = requestedBy
	}
	if update.RequiredApprovers > 0 {
		record["requiredApprovers"] = update.RequiredApprovers
	}
	if len(update.Approvers) > 0 {
		cloned := make([]string, len(update.Approvers))
		copy(cloned, update.Approvers)
		record["approvers"] = cloned
	}
	if outcome := strings.TrimSpace(strings.ToLower(update.Outcome)); outcome != "" {
		record["outcome"] = outcome
	}
	if notes := strings.TrimSpace(update.Notes); notes != "" {
		record["notes"] = notes
	}
	decidedAt := update.DecidedAt
	if decidedAt.IsZero() {
		decidedAt = s.clock().UTC()
	}
	record["decidedAt"] = decidedAt.UTC().Format(time.RFC3339)
	return record
}

func (s *Service) findPolicyVersion(ctx context.Context, env, scope string, version uint32) (*model.RoutingPolicy, error) {
	if version == 0 {
		return s.repo.Latest(ctx, env, scope, "")
	}
	return s.repo.FindVersion(ctx, env, scope, version)
}

type safeModeRecord struct {
	TenantScope string `json:"tenant_scope"`
	Env         string `json:"env"`
	Enabled     bool   `json:"enabled"`
	Reason      string `json:"reason"`
	Actor       string `json:"actor"`
	UpdatedAt   string `json:"updated_at"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

// ToggleSafeMode sets or clears manual safe-mode for the tenant scope.
func (s *Service) ToggleSafeMode(ctx context.Context, env, tenantScope string, enabled bool, ttl time.Duration, actor, reason string) (*SafeModeState, error) {
	if s.cache == nil {
		return nil, errors.New("cache not configured for safe-mode")
	}
	scope := strings.TrimSpace(tenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	env = strings.TrimSpace(env)
	if env == "" {
		env = "default"
	}
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	ctx = instrumentation.WithTenant(ctx, scope)

	key := fmt.Sprintf(safeModeCacheKey, env, scope)
	now := s.clock().UTC()
	state := &SafeModeState{
		TenantScope: scope,
		Env:         env,
		Enabled:     enabled,
		Reason:      strings.TrimSpace(reason),
		Actor:       strings.TrimSpace(actor),
		UpdatedAt:   now,
	}

	record := safeModeRecord{
		TenantScope: scope,
		Env:         env,
		Enabled:     enabled,
		Reason:      state.Reason,
		Actor:       state.Actor,
		UpdatedAt:   now.Format(time.RFC3339),
	}

	if enabled {
		if ttl <= 0 {
			ttl = defaultSafeModeTT
		}
		expires := now.Add(ttl)
		state.ExpiresAt = &expires
		record.ExpiresAt = expires.Format(time.RFC3339)
		payload, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		if err := s.cache.Set(ctx, key, payload, ttl); err != nil {
			return nil, err
		}
	} else {
		state.Reason = ""
		state.Actor = ""
		if err := s.cache.Delete(ctx, key); err != nil {
			logger.WarnF(ctx, "[model_routing] safe-mode delete failed: %v", err)
		}
	}

	meta := map[string]any{
		"enabled": enabled,
		"reason":  state.Reason,
	}
	if state.Actor != "" {
		meta["actor"] = state.Actor
	}
	if state.ExpiresAt != nil {
		meta["expires_at"] = state.ExpiresAt.Format(time.RFC3339)
	}
	s.emitAudit(ctx, safeModeAuditOp(enabled), &model.RoutingPolicy{
		Env:         env,
		TenantScope: scope,
		Version:     0,
	}, meta)
	s.inst.RecordMetric(ctx, "agent.routing.safe_mode_toggle_total", 1, map[string]string{
		"tenant_scope": scope,
		"env":          env,
		"enabled":      fmt.Sprintf("%t", enabled),
	})
	return state, nil
}

// SafeModeState reads the cached safe-mode toggle (if any).
func (s *Service) SafeModeState(ctx context.Context, env, tenantScope string) (*SafeModeState, error) {
	if s.cache == nil {
		return nil, errors.New("cache not configured for safe-mode")
	}
	scope := strings.TrimSpace(tenantScope)
	if scope == "" {
		return nil, errors.New("tenant scope required")
	}
	env = strings.TrimSpace(env)
	if env == "" {
		env = "default"
	}
	ctx, _ = instrumentation.EnsureTraceContext(ctx)
	ctx = instrumentation.WithTenant(ctx, scope)

	key := fmt.Sprintf(safeModeCacheKey, env, scope)
	raw, err := s.cache.Get(ctx, key)
	if err != nil || len(raw) == 0 {
		if err != nil {
			return nil, err
		}
		return &SafeModeState{
			TenantScope: scope,
			Env:         env,
			Enabled:     false,
		}, nil
	}
	var rec safeModeRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		_ = s.cache.Delete(ctx, key)
		return nil, err
	}
	state := &SafeModeState{
		TenantScope: scope,
		Env:         env,
		Enabled:     rec.Enabled,
		Reason:      rec.Reason,
		Actor:       rec.Actor,
	}
	if rec.UpdatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, rec.UpdatedAt); err == nil {
			state.UpdatedAt = ts
		}
	}
	if rec.ExpiresAt != "" {
		if ts, err := time.Parse(time.RFC3339, rec.ExpiresAt); err == nil {
			state.ExpiresAt = &ts
		}
	}
	return state, nil
}

func parseBudget(ctx map[string]any) float64 {
	if ctx == nil {
		return 0
	}
	raw := ctx["budget"]
	switch v := raw.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return f
		}
	}
	return 0
}
