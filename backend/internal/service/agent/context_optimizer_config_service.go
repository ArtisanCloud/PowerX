package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/ArtisanCloud/PowerX/internal/server/agent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	runtimeConfigTypeContextOptimizer = "context_optimizer"
	cacheTTLContextOptimizerActive    = 60 * time.Second
)

var (
	ErrContextOptimizerInvalid    = errors.New("invalid context optimizer config")
	ErrContextOptimizerNotFound   = errors.New("context optimizer config not found")
	ErrContextOptimizerBadVersion = errors.New("invalid config version")
)

type ContextOptimizerConfigPayload struct {
	Enabled                     bool   `json:"enabled"`
	MaxPromptTokens             int    `json:"max_prompt_tokens"`
	ReservedCompletionTokens    int    `json:"reserved_completion_tokens"`
	RecentMessages              int    `json:"recent_messages"`
	RetrievalTopK               int    `json:"retrieval_top_k"`
	CacheMode                   string `json:"cache_mode"`
	SummaryRefreshIntervalSec   int    `json:"summary_refresh_interval_sec"`
	DebugTraceEnabled           bool   `json:"debug_trace_enabled"`
	PlannerEnabled              bool   `json:"planner_enabled"`
	PlannerCandidateTopK        int    `json:"planner_candidate_top_k"`
	PlannerPromptSlimMode       string `json:"planner_prompt_slim_mode"`
	PlannerDecisionCacheEnabled bool   `json:"planner_decision_cache_enabled"`
	PlannerDecisionCacheTTLSec  int    `json:"planner_decision_cache_ttl_sec"`
	PlannerQuotaWorkflow        int    `json:"planner_quota_workflow"`
	PlannerQuotaSkill           int    `json:"planner_quota_skill"`
	PlannerQuotaTooling         int    `json:"planner_quota_tooling"`
	PlannerQuotaLLM             int    `json:"planner_quota_llm"`
}

type ContextOptimizerRuntimeView struct {
	ID           uint64                        `json:"id"`
	UUID         string                        `json:"uuid"`
	Env          string                        `json:"env"`
	Scope        string                        `json:"scope"`
	TenantUUID   *string                       `json:"tenant_uuid,omitempty"`
	Version      int                           `json:"version"`
	Status       string                        `json:"status"`
	Config       ContextOptimizerConfigPayload `json:"config"`
	ChangeReason string                        `json:"change_reason,omitempty"`
	UpdatedAt    time.Time                     `json:"updated_at"`
	PublishedAt  *time.Time                    `json:"published_at,omitempty"`
}

type ContextOptimizerActiveView struct {
	Source    string                        `json:"source"` // tenant|system|yaml_default
	Version   int                           `json:"version"`
	Scope     string                        `json:"scope"`
	Config    ContextOptimizerConfigPayload `json:"config"`
	UpdatedAt *time.Time                    `json:"updated_at,omitempty"`
}

type ContextOptimizerConfigService struct {
	db    *gorm.DB
	cache cache.ICache
}

func NewContextOptimizerConfigService(db *gorm.DB) *ContextOptimizerConfigService {
	return &ContextOptimizerConfigService{
		db:    db,
		cache: cache.GetCache(),
	}
}

func (s *ContextOptimizerConfigService) cacheKey(env string, tenantUUID *string) string {
	scope := "system"
	if tenantUUID != nil && strings.TrimSpace(*tenantUUID) != "" {
		scope = strings.TrimSpace(*tenantUUID)
	}
	return fmt.Sprintf("agent:ctxopt:active:%s:%s", strings.TrimSpace(env), scope)
}

func (s *ContextOptimizerConfigService) normalizeScopeTenant(scope string, tenantUUID *string) (string, *string) {
	if strings.EqualFold(strings.TrimSpace(scope), "system") {
		return "system", nil
	}
	if tenantUUID == nil {
		return "tenant", nil
	}
	val := strings.TrimSpace(*tenantUUID)
	if val == "" {
		return "tenant", nil
	}
	return "tenant", &val
}

func normalizeCacheMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "auto", "force_on", "force_off":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "auto"
	}
}

func validateContextOptimizerPayload(in ContextOptimizerConfigPayload) error {
	if in.MaxPromptTokens < 1024 || in.MaxPromptTokens > 200000 {
		return fmt.Errorf("%w: max_prompt_tokens out of range", ErrContextOptimizerInvalid)
	}
	if in.ReservedCompletionTokens < 256 || in.ReservedCompletionTokens > 32000 {
		return fmt.Errorf("%w: reserved_completion_tokens out of range", ErrContextOptimizerInvalid)
	}
	if in.ReservedCompletionTokens >= in.MaxPromptTokens {
		return fmt.Errorf("%w: reserved_completion_tokens must be smaller than max_prompt_tokens", ErrContextOptimizerInvalid)
	}
	if in.RecentMessages < 1 || in.RecentMessages > 100 {
		return fmt.Errorf("%w: recent_messages out of range", ErrContextOptimizerInvalid)
	}
	if in.RetrievalTopK < 0 || in.RetrievalTopK > 50 {
		return fmt.Errorf("%w: retrieval_top_k out of range", ErrContextOptimizerInvalid)
	}
	if in.SummaryRefreshIntervalSec < 30 || in.SummaryRefreshIntervalSec > 86400 {
		return fmt.Errorf("%w: summary_refresh_interval_sec out of range", ErrContextOptimizerInvalid)
	}
	if in.PlannerCandidateTopK != 0 && (in.PlannerCandidateTopK < 4 || in.PlannerCandidateTopK > 200) {
		return fmt.Errorf("%w: planner_candidate_top_k out of range", ErrContextOptimizerInvalid)
	}
	if in.PlannerDecisionCacheTTLSec != 0 && (in.PlannerDecisionCacheTTLSec < 1 || in.PlannerDecisionCacheTTLSec > 3600) {
		return fmt.Errorf("%w: planner_decision_cache_ttl_sec out of range", ErrContextOptimizerInvalid)
	}
	if mode := strings.ToLower(strings.TrimSpace(in.PlannerPromptSlimMode)); mode != "" && mode != "compact" && mode != "verbose" {
		return fmt.Errorf("%w: planner_prompt_slim_mode invalid", ErrContextOptimizerInvalid)
	}
	return nil
}

func fromAgentDefaults(opt agentruntime.ContextOptimizerConfig, debugEnabled bool) ContextOptimizerConfigPayload {
	planner := agentruntime.GetAgentManager().GetPlannerOptimizerConfig()
	return ContextOptimizerConfigPayload{
		Enabled:                     opt.Enabled,
		MaxPromptTokens:             opt.MaxPromptTokens,
		ReservedCompletionTokens:    opt.ReservedCompletionTokens,
		RecentMessages:              opt.RecentMessages,
		RetrievalTopK:               opt.RetrievalTopK,
		CacheMode:                   normalizeCacheMode(opt.CacheMode),
		SummaryRefreshIntervalSec:   opt.SummaryRefreshIntervalSec,
		DebugTraceEnabled:           debugEnabled,
		PlannerEnabled:              planner.Enabled,
		PlannerCandidateTopK:        planner.CandidateTopK,
		PlannerPromptSlimMode:       planner.PromptSlimMode,
		PlannerDecisionCacheEnabled: planner.DecisionCacheEnabled,
		PlannerDecisionCacheTTLSec:  planner.DecisionCacheTTLSec,
		PlannerQuotaWorkflow:        planner.PerKindQuota.Workflow,
		PlannerQuotaSkill:           planner.PerKindQuota.Skill,
		PlannerQuotaTooling:         planner.PerKindQuota.Tooling,
		PlannerQuotaLLM:             planner.PerKindQuota.LLM,
	}
}

func mergeConfigWithDefaults(cfg ContextOptimizerConfigPayload, defaults ContextOptimizerConfigPayload) ContextOptimizerConfigPayload {
	// 基础字段：零值时使用默认值，避免旧配置缺字段导致运行时退化。
	if cfg.MaxPromptTokens == 0 {
		cfg.MaxPromptTokens = defaults.MaxPromptTokens
	}
	if cfg.ReservedCompletionTokens == 0 {
		cfg.ReservedCompletionTokens = defaults.ReservedCompletionTokens
	}
	if cfg.RecentMessages == 0 {
		cfg.RecentMessages = defaults.RecentMessages
	}
	if cfg.SummaryRefreshIntervalSec == 0 {
		cfg.SummaryRefreshIntervalSec = defaults.SummaryRefreshIntervalSec
	}
	if strings.TrimSpace(cfg.CacheMode) == "" {
		cfg.CacheMode = defaults.CacheMode
	}

	// Planner 字段：若整组均缺失（历史配置），回填当前默认 planner 配置。
	legacyPlannerMissing :=
		cfg.PlannerCandidateTopK == 0 &&
			strings.TrimSpace(cfg.PlannerPromptSlimMode) == "" &&
			cfg.PlannerDecisionCacheTTLSec == 0 &&
			cfg.PlannerQuotaWorkflow == 0 &&
			cfg.PlannerQuotaSkill == 0 &&
			cfg.PlannerQuotaTooling == 0 &&
			cfg.PlannerQuotaLLM == 0
	if legacyPlannerMissing {
		cfg.PlannerEnabled = defaults.PlannerEnabled
		cfg.PlannerCandidateTopK = defaults.PlannerCandidateTopK
		cfg.PlannerPromptSlimMode = defaults.PlannerPromptSlimMode
		cfg.PlannerDecisionCacheEnabled = defaults.PlannerDecisionCacheEnabled
		cfg.PlannerDecisionCacheTTLSec = defaults.PlannerDecisionCacheTTLSec
		cfg.PlannerQuotaWorkflow = defaults.PlannerQuotaWorkflow
		cfg.PlannerQuotaSkill = defaults.PlannerQuotaSkill
		cfg.PlannerQuotaTooling = defaults.PlannerQuotaTooling
		cfg.PlannerQuotaLLM = defaults.PlannerQuotaLLM
	}
	cfg.CacheMode = normalizeCacheMode(cfg.CacheMode)
	if mode := strings.ToLower(strings.TrimSpace(cfg.PlannerPromptSlimMode)); mode == "compact" || mode == "verbose" {
		cfg.PlannerPromptSlimMode = mode
	} else {
		cfg.PlannerPromptSlimMode = defaults.PlannerPromptSlimMode
	}
	return cfg
}

func (s *ContextOptimizerConfigService) toView(rec *dbmodel.AgentRuntimeConfig) (*ContextOptimizerRuntimeView, error) {
	if rec == nil {
		return nil, nil
	}
	out := ContextOptimizerRuntimeView{
		ID:           rec.ID,
		UUID:         rec.UUID.String(),
		Env:          rec.Env,
		Scope:        rec.Scope,
		TenantUUID:   rec.TenantUUID,
		Version:      rec.Version,
		Status:       rec.Status,
		ChangeReason: rec.ChangeReason,
		UpdatedAt:    rec.UpdatedAt,
		PublishedAt:  rec.PublishedAt,
	}
	bs, _ := json.Marshal(rec.ConfigJSON)
	var cfg ContextOptimizerConfigPayload
	if len(bs) > 0 {
		_ = json.Unmarshal(bs, &cfg)
	}
	cfg.CacheMode = normalizeCacheMode(cfg.CacheMode)
	out.Config = cfg
	return &out, nil
}

func (s *ContextOptimizerConfigService) getPublished(ctx context.Context, env string, tenantUUID *string) (*dbmodel.AgentRuntimeConfig, error) {
	var out dbmodel.AgentRuntimeConfig
	err := s.db.WithContext(ctx).
		Model(&dbmodel.AgentRuntimeConfig{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("config_type = ? AND status = ?", runtimeConfigTypeContextOptimizer, "published").
		Order("version DESC, id DESC").
		First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *ContextOptimizerConfigService) GetActive(ctx context.Context, env string, tenantUUID *string, fallback agentruntime.ContextOptimizerConfig, debugEnabled bool) (*ContextOptimizerActiveView, error) {
	defaultCfg := fromAgentDefaults(fallback, debugEnabled)
	if s == nil || s.db == nil {
		cfg := defaultCfg
		return &ContextOptimizerActiveView{
			Source:  "yaml_default",
			Scope:   "system",
			Version: 0,
			Config:  cfg,
		}, nil
	}
	key := s.cacheKey(env, tenantUUID)
	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, key); err == nil && len(raw) > 0 {
			var hit ContextOptimizerActiveView
			if json.Unmarshal(raw, &hit) == nil {
				hit.Config = mergeConfigWithDefaults(hit.Config, defaultCfg)
				return &hit, nil
			}
		}
	}

	var active *ContextOptimizerActiveView
	if tenantUUID != nil && strings.TrimSpace(*tenantUUID) != "" {
		tenantCfg, err := s.getPublished(ctx, env, tenantUUID)
		if err != nil {
			return nil, err
		}
		if tenantCfg != nil {
			v, _ := s.toView(tenantCfg)
			active = &ContextOptimizerActiveView{
				Source:    "tenant",
				Scope:     "tenant",
				Version:   v.Version,
				Config:    v.Config,
				UpdatedAt: &v.UpdatedAt,
			}
		}
	}
	if active == nil {
		sysCfg, err := s.getPublished(ctx, env, nil)
		if err != nil {
			return nil, err
		}
		if sysCfg != nil {
			v, _ := s.toView(sysCfg)
			active = &ContextOptimizerActiveView{
				Source:    "system",
				Scope:     "system",
				Version:   v.Version,
				Config:    v.Config,
				UpdatedAt: &v.UpdatedAt,
			}
		}
	}
	if active == nil {
		cfg := defaultCfg
		active = &ContextOptimizerActiveView{
			Source:  "yaml_default",
			Scope:   "system",
			Version: 0,
			Config:  cfg,
		}
	}
	active.Config = mergeConfigWithDefaults(active.Config, defaultCfg)
	if s.cache != nil {
		if bs, err := json.Marshal(active); err == nil {
			_ = s.cache.Set(ctx, key, bs, cacheTTLContextOptimizerActive)
		}
	}
	return active, nil
}

func (s *ContextOptimizerConfigService) ListVersions(ctx context.Context, env string, scope string, tenantUUID *string, limit int) ([]ContextOptimizerRuntimeView, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	scope, tenantUUID = s.normalizeScopeTenant(scope, tenantUUID)
	tx := s.db.WithContext(ctx).
		Model(&dbmodel.AgentRuntimeConfig{}).
		Where("env = ? AND tenant_uuid IS NOT DISTINCT FROM ? AND config_type = ?", env, tenantUUID, runtimeConfigTypeContextOptimizer)
	if scope == "system" {
		tx = tx.Where("scope = ?", "system")
	} else {
		tx = tx.Where("scope = ?", "tenant")
	}
	var rows []dbmodel.AgentRuntimeConfig
	if err := tx.Order("version DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ContextOptimizerRuntimeView, 0, len(rows))
	for i := range rows {
		v, err := s.toView(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

func (s *ContextOptimizerConfigService) SaveDraft(ctx context.Context, env string, scope string, tenantUUID *string, cfg ContextOptimizerConfigPayload, reason string, actor uint64) (*ContextOptimizerRuntimeView, error) {
	cfg.CacheMode = normalizeCacheMode(cfg.CacheMode)
	if err := validateContextOptimizerPayload(cfg); err != nil {
		return nil, err
	}
	scope, tenantUUID = s.normalizeScopeTenant(scope, tenantUUID)
	var maxVer int64
	if err := s.db.WithContext(ctx).
		Model(&dbmodel.AgentRuntimeConfig{}).
		Where("env = ? AND tenant_uuid IS NOT DISTINCT FROM ? AND config_type = ?", env, tenantUUID, runtimeConfigTypeContextOptimizer).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVer).Error; err != nil {
		return nil, err
	}
	rec := &dbmodel.AgentRuntimeConfig{
		Env:          env,
		TenantUUID:   tenantUUID,
		Scope:        scope,
		ConfigType:   runtimeConfigTypeContextOptimizer,
		Version:      int(maxVer) + 1,
		Status:       "draft",
		ConfigJSON:   datatypes.JSONMap(utilsJSONMap(cfg)),
		ChangeReason: strings.TrimSpace(reason),
		CreatedBy:    actor,
	}
	if err := s.db.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, err
	}
	return s.toView(rec)
}

func (s *ContextOptimizerConfigService) Publish(ctx context.Context, env string, scope string, tenantUUID *string, version int, reason string, actor uint64) (*ContextOptimizerRuntimeView, error) {
	if version <= 0 {
		return nil, ErrContextOptimizerBadVersion
	}
	scope, tenantUUID = s.normalizeScopeTenant(scope, tenantUUID)
	var out *ContextOptimizerRuntimeView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target dbmodel.AgentRuntimeConfig
		if err := tx.Model(&dbmodel.AgentRuntimeConfig{}).
			Where("env = ? AND tenant_uuid IS NOT DISTINCT FROM ? AND config_type = ? AND version = ?", env, tenantUUID, runtimeConfigTypeContextOptimizer, version).
			First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrContextOptimizerNotFound
			}
			return err
		}
		if err := tx.Model(&dbmodel.AgentRuntimeConfig{}).
			Where("env = ? AND tenant_uuid IS NOT DISTINCT FROM ? AND config_type = ? AND status = ?", env, tenantUUID, runtimeConfigTypeContextOptimizer, "published").
			Updates(map[string]any{"status": "archived"}).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&dbmodel.AgentRuntimeConfig{}).Where("id = ?", target.ID).Updates(map[string]any{
			"status":        "published",
			"published_by":  actor,
			"published_at":  &now,
			"change_reason": strings.TrimSpace(reason),
			"updated_at":    now,
		}).Error; err != nil {
			return err
		}
		target.Status = "published"
		target.PublishedAt = &now
		target.PublishedBy = actor
		target.ChangeReason = strings.TrimSpace(reason)
		v, convErr := s.toView(&target)
		if convErr != nil {
			return convErr
		}
		out = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, s.cacheKey(env, tenantUUID))
		if tenantUUID != nil {
			_ = s.cache.Delete(ctx, s.cacheKey(env, nil))
		}
	}
	return out, nil
}

func (s *ContextOptimizerConfigService) Rollback(ctx context.Context, env string, scope string, tenantUUID *string, targetVersion int, reason string, actor uint64) (*ContextOptimizerRuntimeView, error) {
	return s.Publish(ctx, env, scope, tenantUUID, targetVersion, reason, actor)
}

func utilsJSONMap(cfg ContextOptimizerConfigPayload) map[string]any {
	plannerMode := strings.ToLower(strings.TrimSpace(cfg.PlannerPromptSlimMode))
	if plannerMode != "verbose" {
		plannerMode = "compact"
	}
	return map[string]any{
		"enabled":                        cfg.Enabled,
		"max_prompt_tokens":              cfg.MaxPromptTokens,
		"reserved_completion_tokens":     cfg.ReservedCompletionTokens,
		"recent_messages":                cfg.RecentMessages,
		"retrieval_top_k":                cfg.RetrievalTopK,
		"cache_mode":                     normalizeCacheMode(cfg.CacheMode),
		"summary_refresh_interval_sec":   cfg.SummaryRefreshIntervalSec,
		"debug_trace_enabled":            cfg.DebugTraceEnabled,
		"planner_enabled":                cfg.PlannerEnabled,
		"planner_candidate_top_k":        cfg.PlannerCandidateTopK,
		"planner_prompt_slim_mode":       plannerMode,
		"planner_decision_cache_enabled": cfg.PlannerDecisionCacheEnabled,
		"planner_decision_cache_ttl_sec": cfg.PlannerDecisionCacheTTLSec,
		"planner_quota_workflow":         cfg.PlannerQuotaWorkflow,
		"planner_quota_skill":            cfg.PlannerQuotaSkill,
		"planner_quota_tooling":          cfg.PlannerQuotaTooling,
		"planner_quota_llm":              cfg.PlannerQuotaLLM,
	}
}
