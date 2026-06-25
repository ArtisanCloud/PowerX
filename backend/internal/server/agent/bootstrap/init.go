package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	appcfg "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract/embed"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/factory"
	intent2 "github.com/ArtisanCloud/PowerX/internal/server/agent/factory/intent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/intent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	cachepkg "github.com/ArtisanCloud/PowerX/pkg/cache"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	skillmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func InitAgentTools(ctx context.Context, cfg *config.AgentConfig, logCfg *logcfg.LogConfig, db *gorm.DB) error {
	normalizeAgentRuntimePaths(cfg)

	// 新建Agent Manager
	gAgentManager := agent.GetAgentManager()

	// 新建一个Agent实例
	a, err := factory.NewAgentClient(ctx, cfg)
	if err != nil {
		return err
	}
	// AgentMeta信息
	m := &schemas.AgentMeta{}

	// 注册System Agent
	err = gAgentManager.Register(config.AgentSysKey, a, m)
	if err != nil {
		return err
	}

	// 注册的CRM Agent，兜底走 base_flow（你已有）
	if err = gAgentManager.SetDefaultAgent(config.AgentSysKey, config.BaseFlowKey); err != nil {
		return fmt.Errorf("SetDefaultAgent failed: %w", err)
	}
	gAgentManager.SetDebugTraceConfig(resolveAgentDebugTraceConfig(cfg, logCfg))
	gAgentManager.SetContextOptimizerConfig(resolveContextOptimizerConfig(cfg))
	gAgentManager.SetPlannerOptimizerConfig(resolvePlannerOptimizerConfig(cfg))

	// 2) ✨ 先注册内置 handlers（core.debug.* / core.response.format）
	if err := RegisterBuiltinHandlers(); err != nil {
		return fmt.Errorf("RegisterBuiltinHandlers failed: %w", err)
	}

	// 注册一个意图识别的Agent，加载blueprint的意图识别配置
	// * 注意，一定要先注册agent，然后在注册agent的意图识别
	err = RegisterIntentsForAgent(config.AgentSysKey, cfg.FlowSpec.BusinessDir)
	if err != nil {
		return err
	}

	// 从数据库加载已发布 skill/tooling 候选，并在内存中缓存用于 LLM tool-calling。
	// 这样每次请求都直接读 Manager.unifiedCandidates，不会频繁查询 DB。
	if db != nil {
		lastSkills, lastToolings := 0, 0
		if loadedSkills, loadedToolings, loadErr := warmUnifiedCandidatesFromDB(ctx, db, gAgentManager, true); loadErr != nil {
			logger.InfoF(ctx, "[agent] warm unified candidates failed: %v", loadErr)
		} else {
			lastSkills, lastToolings = loadedSkills, loadedToolings
			logger.InfoF(ctx, "[agent] warm unified candidates ok: skills=%d toolings=%d", loadedSkills, loadedToolings)
		}
		// 周期刷新，避免“仅重启时更新”导致候选陈旧。
		go func() {
			ticker := time.NewTicker(2 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s, t, err := warmUnifiedCandidatesFromDB(ctx, db, gAgentManager, false)
					if err != nil {
						logger.InfoF(ctx, "[agent] refresh unified candidates failed: %v", err)
						continue
					}
					if s != lastSkills || t != lastToolings {
						logger.InfoF(ctx, "[agent] refresh unified candidates changed: skills=%d toolings=%d (prev skills=%d toolings=%d)", s, t, lastSkills, lastToolings)
						lastSkills, lastToolings = s, t
					}
				}
			}
		}()
	}

	// 统一节点执行器（skill/tooling）接线到真实服务链路
	if db != nil {
		ctxRefSvc := agentsvc.NewContextRefService(db)
		gAgentManager.SetContextRefAuthorizer(func(ctx context.Context, tenantUUID string, childAgentID uint64, contextRefID string) error {
			return ctxRefSvc.CanAccess(ctx, tenantUUID, childAgentID, contextRefID)
		})

		// skill invoker
		skillRegistryRepo := skillrepo.NewSkillRegistryRepository(db)

		// tooling invoker（capability registry 为 tooling 的数据库权威源）
		catalog := capservice.NewRegistryService(capservice.RegistryServiceOptions{DB: db})
		router := caprouter.NewService(caprouter.ServiceOptions{DB: db})
		capTraceRepo := caprepo.NewInvocationTraceRepository(db)
		capEventRepo := caprepo.NewCapabilityEventPublicationRepository(db)
		capInvoker := capservice.NewInvocationService(capservice.InvocationServiceOptions{
			Catalog:   catalog,
			Router:    router,
			TraceRepo: capTraceRepo,
			EventRepo: capEventRepo,
		})
		gAgentManager.SetSkillInvoker(func(ctx context.Context, in agent.SkillInvokeInput) (*agent.SkillInvokeOutput, error) {
			if in.Payload == nil {
				in.Payload = map[string]any{}
			}
			if in.Context == nil {
				in.Context = map[string]any{}
			}
			enrichedCtx := enrichSkillContext(in.Context, in)
			if capID := strings.TrimSpace(in.CapabilityID); capID != "" {
				result, err := capInvoker.Invoke(ctx, capservice.InvocationInput{
					CapabilityID:      capID,
					TenantUUID:        in.TenantUUID,
					PreferredProtocol: firstNonEmptyString(asStringFromMap(in.Payload, "preferred_protocol"), asStringFromMap(enrichedCtx, "preferred_protocol"), "rest"),
					TraceID:           in.TraceID,
					Payload:           in.Payload,
					Context:           enrichedCtx,
				})
				if err != nil {
					return nil, err
				}
				return &agent.SkillInvokeOutput{
					TraceID:      result.TraceID,
					Status:       result.Status,
					ProtocolUsed: result.ProtocolUsed,
					FallbackUsed: result.FallbackUsed,
					SkillID:      in.SkillID,
					Version:      in.Version,
					Result:       result.Result,
				}, nil
			}
			if capID, resolveErr := resolveSkillActionCapability(ctx, skillRegistryRepo, in.SkillID, in.Version, in.Payload); capID != "" {
				result, err := capInvoker.Invoke(ctx, capservice.InvocationInput{
					CapabilityID:      capID,
					TenantUUID:        in.TenantUUID,
					PreferredProtocol: firstNonEmptyString(asStringFromMap(in.Payload, "preferred_protocol"), asStringFromMap(enrichedCtx, "preferred_protocol"), "rest"),
					TraceID:           in.TraceID,
					Payload:           in.Payload,
					Context:           enrichedCtx,
				})
				if err != nil {
					return nil, err
				}
				return &agent.SkillInvokeOutput{
					TraceID:      result.TraceID,
					Status:       result.Status,
					ProtocolUsed: result.ProtocolUsed,
					FallbackUsed: result.FallbackUsed,
					SkillID:      in.SkillID,
					Version:      in.Version,
					Result:       result.Result,
				}, nil
			} else if resolveErr != nil {
				return nil, resolveErr
			}
			return nil, fmt.Errorf("skill %q cannot execute without action_capabilities mapping or explicit capability_id", strings.TrimSpace(in.SkillID))
		})
		gAgentManager.SetToolingInvoker(func(ctx context.Context, in agent.ToolingInvokeInput) (*agent.ToolingInvokeOutput, error) {
			result, err := capInvoker.Invoke(ctx, capservice.InvocationInput{
				CapabilityID:      in.CapabilityID,
				TenantUUID:        in.TenantUUID,
				PreferredProtocol: in.PreferredProtocol,
				TraceID:           in.TraceID,
				Payload:           in.Payload,
				Context:           in.Context,
			})
			if err != nil {
				return nil, err
			}
			return &agent.ToolingInvokeOutput{
				TraceID:      result.TraceID,
				Status:       result.Status,
				ProtocolUsed: result.ProtocolUsed,
				FallbackUsed: result.FallbackUsed,
				Result:       result.Result,
			}, nil
		})
	}

	// 向量器（OpenAI/Ollama 任选其一）
	vec, err := intent2.NewVectorizerFromConfig(cfg.IntentRecognizer.Embedding)
	if err != nil {
		return fmt.Errorf("init vectorizer: %w", err)
	}
	//diagEmbedding(ctx, vec)

	// 构建意图识别策略
	var strategies []contract.IntentStrategy
	// LLM 优先（仅在 classifier 配置启用时生效）
	if cfg.IntentRecognizer.Classifier.Enabled {
		if cls, clsErr := intent2.NewClassifierFromConfig(cfg.IntentRecognizer.Classifier); clsErr == nil && cls != nil {
			threshold := cfg.IntentRecognizer.Classifier.LLMMinConfidence
			if threshold <= 0 {
				threshold = 0.60
			}
			strategies = append(strategies, &intent.LLMStrategy{
				M:         gAgentManager,
				AgentID:   config.AgentSysKey,
				LLM:       cls,
				Threshold: threshold,
			})
		} else if clsErr != nil {
			logger.InfoF(ctx, "[intent] classifier init skipped: %v", clsErr)
		}
	}

	// 只有当向量器不为空时才添加 embedding 策略
	if vec != nil {
		strategies = append(strategies, &intent.EmbeddingStrategy{
			M:         gAgentManager,
			Vec:       vec,
			AgentID:   config.AgentSysKey,
			Threshold: 0.90, // ✨
			Alpha:     0.6,  // ✨ 负例惩罚
			Margin:    0.06, // ✨ 边际
		})
	}
	// rule 仅用于 /command 类快捷命令（策略内部会强制判定 slash 前缀）
	if cfg.IntentRecognizer.Rule.Enabled {
		strategies = append(strategies, &intent.RuleStrategy{M: gAgentManager})
	}

	gAgentManager.SetIntentStrategies(strategies, 0.6, 0.95)

	// 接线 DB RunLogger → Manager
	WireAgentRunLogger(db)

	return err
}

func enrichSkillContext(ctxMap map[string]any, in agent.SkillInvokeInput) map[string]any {
	out := make(map[string]any, len(ctxMap)+8)
	for k, v := range ctxMap {
		out[k] = v
	}
	if in.TenantUUID != "" {
		out["tenant_uuid"] = in.TenantUUID
	}
	if in.UserUUID != "" {
		out["user_uuid"] = in.UserUUID
	}
	if in.AgentID > 0 {
		out["agent_id"] = fmt.Sprintf("%d", in.AgentID)
	}
	if in.SessionID != "" {
		out["session_id"] = in.SessionID
	}
	if in.MessageID != "" {
		out["message_id"] = in.MessageID
	}
	if in.SkillID != "" {
		out["skill_id"] = in.SkillID
	}
	if in.TraceID != "" {
		out["trace_id"] = in.TraceID
	}
	if in.CapabilityID != "" {
		out["capability_id"] = in.CapabilityID
	}
	return out
}

func resolveSkillActionCapability(ctx context.Context, repo *skillrepo.SkillRegistryRepository, skillID, version string, payload map[string]any) (string, error) {
	if repo == nil {
		return "", errors.New("skill registry repository is not configured")
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", errors.New("skill_id is required")
	}
	var rec *skillmodels.SkillRegistryRecord
	var err error
	if strings.TrimSpace(version) != "" {
		rec, err = repo.GetBySkillVersion(ctx, skillID, version)
	} else {
		rec, err = repo.GetLatestPublished(ctx, skillID)
	}
	if err != nil || rec == nil || len(rec.ManifestJSON) == 0 {
		return "", fmt.Errorf("skill %q manifest is not published or empty", skillID)
	}
	var manifest map[string]any
	if err := json.Unmarshal(rec.ManifestJSON, &manifest); err != nil {
		return "", fmt.Errorf("skill %q manifest json is invalid: %w", skillID, err)
	}
	actions := actionCapabilityMap(manifest)
	if len(actions) == 0 {
		return "", fmt.Errorf("skill %q missing action_capabilities or executor.action_map", skillID)
	}
	action := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		asStringFromMap(payload, "action"),
		asStringFromMap(payload, "operation"),
		asStringFromNestedMap(payload, "input", "action"),
		asStringFromNestedMap(payload, "template", "action"),
	)))
	if action == "" {
		return "", fmt.Errorf("skill %q requires action before capability invocation", skillID)
	}
	capabilityID := strings.TrimSpace(fmt.Sprint(actions[action]))
	if capabilityID == "" || capabilityID == "<nil>" {
		return "", fmt.Errorf("skill %q action %q has no capability mapping", skillID, action)
	}
	return capabilityID, nil
}

func actionCapabilityMap(manifest map[string]any) map[string]any {
	if manifest == nil {
		return nil
	}
	if actions, ok := manifest["action_capabilities"].(map[string]any); ok && len(actions) > 0 {
		return actions
	}
	executor, ok := manifest["executor"].(map[string]any)
	if !ok {
		return nil
	}
	if actions, ok := executor["action_map"].(map[string]any); ok && len(actions) > 0 {
		return actions
	}
	return nil
}

func asStringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(values[key]))
}

func asStringFromNestedMap(values map[string]any, objectKey, key string) string {
	if values == nil {
		return ""
	}
	nested, ok := values[objectKey].(map[string]any)
	if !ok {
		return ""
	}
	return asStringFromMap(nested, key)
}

func normalizeAgentRuntimePaths(cfg *config.AgentConfig) {
	if cfg == nil {
		return
	}
	cfg.FlowSpec.BusinessDir = resolveRuntimeDir(cfg.FlowSpec.BusinessDir)
	cfg.FlowSpec.BaseDir = resolveRuntimeDir(cfg.FlowSpec.BaseDir)
	if strings.TrimSpace(cfg.TemplateDir) != "" {
		cfg.TemplateDir = resolveRuntimeDir(cfg.TemplateDir)
	}
}

func resolveRuntimeDir(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if filepath.IsAbs(raw) {
		return raw
	}
	candidates := make([]string, 0, 8)
	candidates = append(candidates, raw)
	trimmed := strings.TrimPrefix(raw, "./")
	if trimmed != raw {
		candidates = append(candidates, trimmed)
	}
	if trimmed != "" {
		candidates = append(candidates, filepath.Join("backend", trimmed))
	}
	if exe, err := os.Executable(); err == nil && strings.TrimSpace(exe) != "" {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, raw))
		if trimmed != "" {
			candidates = append(candidates, filepath.Join(exeDir, trimmed))
		}
	}
	if cfgPath := strings.TrimSpace(appcfg.GetGlobalConfigPath()); cfgPath != "" {
		cfgDir := filepath.Dir(cfgPath)
		candidates = append(candidates, filepath.Join(cfgDir, raw))
		parent := filepath.Dir(cfgDir)
		candidates = append(candidates, filepath.Join(parent, raw))
		if trimmed != "" {
			candidates = append(candidates, filepath.Join(parent, trimmed))
		}
	}

	seen := map[string]struct{}{}
	for _, c := range candidates {
		if strings.TrimSpace(c) == "" {
			continue
		}
		p := filepath.Clean(c)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return raw
}

func diagEmbedding(ctx context.Context, vec embed.Vectorizer) {
	q := "创建线索"
	qs, err := vec.Embed(ctx, []string{q})
	if err != nil || len(qs) == 0 {
		logger.InfoF(ctx, "[EMB] query embed failed: %v", err)
		return
	}
	logger.InfoF(ctx, "[EMB] query dim=%d first3=%v", len(qs[0]), qs[0][:min(3, len(qs[0]))])
}

const (
	agentUnifiedCandidatesCacheKey = "agent:unified_candidates:v1"
	agentUnifiedCandidatesCacheTTL = 2 * time.Minute
)

func warmUnifiedCandidatesFromDB(ctx context.Context, db *gorm.DB, mgr *agent.Manager, preferCache bool) (int, int, error) {
	if db == nil || mgr == nil {
		return 0, 0, nil
	}

	// 先尝试走 PowerX 统一缓存封装（默认 redis）。
	if preferCache {
		if cached, ok, err := loadUnifiedCandidatesFromCache(ctx); err == nil && ok {
			skills, toolings := 0, 0
			for _, c := range cached {
				mgr.UpsertUnifiedCandidate(c)
				switch strings.ToLower(strings.TrimSpace(c.NodeKind)) {
				case "skill":
					skills++
				case "tooling":
					toolings++
				}
			}
			return skills, toolings, nil
		} else if err != nil {
			logger.InfoF(ctx, "[agent] read unified candidates cache failed: %v", err)
		}
	}

	loadedSkills := 0
	loadedToolings := 0
	candidates := make([]agent.ToolCallCandidate, 0, 1024)

	regRepo := skillrepo.NewSkillRegistryRepository(db)
	page := 1
	const pageSize = 200
	for {
		rows, total, err := regRepo.List(ctx, skillrepo.SkillRegistryFilter{
			Status:   []string{skillmodels.SkillStatusPublished},
			Page:     page,
			PageSize: pageSize,
			OrderBy:  "updated_at DESC",
		})
		if err != nil {
			return loadedSkills, loadedToolings, err
		}
		for _, row := range rows {
			c, ok := skillCandidateFromRegistry(row)
			if !ok {
				continue
			}
			mgr.UpsertUnifiedCandidate(c)
			candidates = append(candidates, c)
			loadedSkills++
		}
		if len(rows) == 0 || int64(page*pageSize) >= total {
			break
		}
		page++
	}

	capRecordRepo := caprepo.NewCapabilityRecordRepository(db, nil)
	capRows, err := capRecordRepo.List(ctx, caprepo.CapabilityRecordFilter{
		Limit:   2000,
		OrderBy: "updated_at DESC",
	})
	if err != nil {
		return loadedSkills, loadedToolings, err
	}
	for _, row := range capRows {
		c, ok := toolingCandidateFromCapability(row)
		if !ok {
			continue
		}
		mgr.UpsertUnifiedCandidate(c)
		candidates = append(candidates, c)
		loadedToolings++
	}
	if err := saveUnifiedCandidatesToCache(ctx, candidates); err != nil {
		logger.InfoF(ctx, "[agent] write unified candidates cache failed: %v", err)
	}
	return loadedSkills, loadedToolings, nil
}

func loadUnifiedCandidatesFromCache(ctx context.Context) ([]agent.ToolCallCandidate, bool, error) {
	store := cachepkg.GetCache()
	if store == nil {
		return nil, false, nil
	}
	raw, err := store.Get(ctx, agentUnifiedCandidatesCacheKey)
	if err != nil {
		return nil, false, err
	}
	if len(raw) == 0 {
		return nil, false, nil
	}
	var out []agent.ToolCallCandidate
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func saveUnifiedCandidatesToCache(ctx context.Context, candidates []agent.ToolCallCandidate) error {
	store := cachepkg.GetCache()
	if store == nil {
		return nil
	}
	payload, err := json.Marshal(candidates)
	if err != nil {
		return err
	}
	return store.Set(ctx, agentUnifiedCandidatesCacheKey, payload, agentUnifiedCandidatesCacheTTL)
}

func skillCandidateFromRegistry(rec skillmodels.SkillRegistryRecord) (agent.ToolCallCandidate, bool) {
	if !strings.EqualFold(strings.TrimSpace(rec.Status), skillmodels.SkillStatusPublished) {
		return agent.ToolCallCandidate{}, false
	}
	skillID := strings.TrimSpace(rec.SkillID)
	if skillID == "" {
		return agent.ToolCallCandidate{}, false
	}
	var manifest map[string]interface{}
	if len(rec.ManifestJSON) > 0 {
		_ = json.Unmarshal(rec.ManifestJSON, &manifest)
	}
	desc := firstNonEmptyString(
		readManifestString(manifest, "description"),
		strings.TrimSpace(rec.ApprovalNote),
		skillID,
	)
	displayName := firstNonEmptyString(
		readManifestString(manifest, "title"),
		readManifestString(manifest, "name"),
		skillID,
	)
	requiredArgs, optionalArgs := extractManifestInputArgs(manifest)
	examples := extractManifestStringArray(manifest, "intent_examples")
	actions := extractManifestStringEnum(manifest, "input_schema", "action")
	responseGuidance := extractManifestResponseGuidance(manifest)
	actionRequiredArgs := extractManifestActionRequiredArgs(manifest)
	return agent.ToolCallCandidate{
		Name:               skillID,
		DisplayName:        displayName,
		NodeKind:           "skill",
		NodeRef:            skillID,
		FlowID:             skillID,
		SourceScope:        "system",
		Source:             strings.TrimSpace(rec.Source),
		Visibility:         "public",
		BindingStatus:      "active",
		Description:        desc,
		IntentHints:        append(extractManifestStringArray(manifest, "entrypoints"), examples...),
		Examples:           examples,
		Actions:            actions,
		ResponseGuidance:   responseGuidance,
		ActionRequiredArgs: actionRequiredArgs,
		Tags:               []string{"registry", strings.TrimSpace(rec.Source)},
		SemanticText:       desc,
		RequiredArgs:       requiredArgs,
		OptionalArgs:       optionalArgs,
	}, true
}

func extractManifestActionRequiredArgs(m map[string]interface{}) map[string][]string {
	if m == nil {
		return nil
	}
	out := actionRequiredArgsFromValue(m["action_required_args"])
	if len(out) == 0 {
		if promptSpec, ok := m["prompt_spec"].(map[string]interface{}); ok {
			out = actionRequiredArgsFromValue(promptSpec["action_required_args"])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func actionRequiredArgsFromValue(raw interface{}) map[string][]string {
	obj, ok := raw.(map[string]interface{})
	if !ok || len(obj) == 0 {
		return nil
	}
	out := make(map[string][]string, len(obj))
	for action, fields := range obj {
		action = strings.ToLower(strings.TrimSpace(action))
		if action == "" {
			continue
		}
		values := normalizeBootstrapStringList(anyToStringSlice(fields))
		if values == nil {
			values = []string{}
		}
		out[action] = values
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toolingCandidateFromCapability(rec capmodels.CapabilityRecord) (agent.ToolCallCandidate, bool) {
	status := strings.ToLower(strings.TrimSpace(rec.Status))
	if status == "disabled" || status == "deprecated" || status == "draft" {
		return agent.ToolCallCandidate{}, false
	}
	capabilityID := strings.TrimSpace(rec.CapabilityID)
	if capabilityID == "" {
		return agent.ToolCallCandidate{}, false
	}
	source := "plugin"
	if strings.Contains(strings.ToLower(strings.TrimSpace(rec.PluginID)), "corex") {
		source = "builtin"
	}
	desc := firstNonEmptyString(strings.TrimSpace(rec.Description), strings.TrimSpace(rec.Title), capabilityID)
	displayName := firstNonEmptyString(strings.TrimSpace(rec.Title), capabilityID)
	return agent.ToolCallCandidate{
		Name:          capabilityID,
		DisplayName:   displayName,
		NodeKind:      "tooling",
		NodeRef:       capabilityID,
		FlowID:        capabilityID,
		SourceScope:   "system",
		Source:        source,
		Visibility:    "public",
		BindingStatus: "active",
		Description:   desc,
		IntentHints:   jsonStringArray(rec.Intents),
		Tags:          []string{"registry", source},
		SemanticText:  desc,
	}, true
}

func readManifestString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	raw, ok := m[key]
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", raw))
}

func extractManifestStringArray(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key]
	if !ok {
		return nil
	}
	return anyToStringSlice(raw)
}

func extractManifestResponseGuidance(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, 8)
	out = append(out, responseGuidanceFromValue(m["response_guidance"])...)
	if promptSpec, ok := m["prompt_spec"].(map[string]interface{}); ok {
		out = append(out, responseGuidanceFromValue(promptSpec["response_guidance"])...)
	}
	return normalizeBootstrapStringList(out)
}

func responseGuidanceFromValue(raw interface{}) []string {
	switch x := raw.(type) {
	case nil:
		return nil
	case string:
		if trimmed := strings.TrimSpace(x); trimmed != "" {
			return []string{trimmed}
		}
	case []string, []interface{}:
		return anyToStringSlice(x)
	case map[string]interface{}:
		out := make([]string, 0, len(x))
		for _, key := range []string{"general", "capability_intro", "capability_howto", "clarify_params", "skill_execution", "error_explain"} {
			values := responseGuidanceFromValue(x[key])
			if len(values) == 0 {
				continue
			}
			for _, value := range values {
				if key == "general" {
					out = append(out, value)
				} else {
					out = append(out, key+": "+value)
				}
			}
		}
		return out
	}
	return nil
}

func normalizeBootstrapStringList(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func extractManifestInputArgs(m map[string]interface{}) ([]string, []string) {
	if m == nil {
		return nil, nil
	}
	inputRaw, ok := m["input_schema"]
	if !ok {
		return nil, nil
	}
	inputMap, ok := inputRaw.(map[string]interface{})
	if !ok {
		return nil, nil
	}
	required := map[string]struct{}{}
	for _, name := range anyToStringSlice(inputMap["required"]) {
		required[name] = struct{}{}
	}
	propsRaw, ok := inputMap["properties"]
	if !ok {
		return nil, nil
	}
	props, ok := propsRaw.(map[string]interface{})
	if !ok {
		return nil, nil
	}
	requiredArgs := make([]string, 0, len(props))
	optionalArgs := make([]string, 0, len(props))
	for name := range props {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := required[name]; ok {
			requiredArgs = append(requiredArgs, name)
		} else {
			optionalArgs = append(optionalArgs, name)
		}
	}
	return requiredArgs, optionalArgs
}

func extractManifestStringEnum(m map[string]interface{}, objectKey string, propertyKey string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[objectKey]
	if !ok {
		return nil
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	props, ok := obj["properties"].(map[string]interface{})
	if !ok {
		return nil
	}
	prop, ok := props[propertyKey].(map[string]interface{})
	if !ok {
		return nil
	}
	return anyToStringSlice(prop["enum"])
}

func anyToStringSlice(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		out := make([]string, 0, len(x))
		for _, item := range x {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func jsonStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for i := range arr {
			arr[i] = strings.TrimSpace(arr[i])
		}
		return arr
	}
	var arrAny []interface{}
	if err := json.Unmarshal(raw, &arrAny); err != nil {
		return nil
	}
	return anyToStringSlice(arrAny)
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func resolveAgentDebugTraceConfig(agentCfg *config.AgentConfig, logCfg *logcfg.LogConfig) agent.DebugTraceConfig {
	// 兼容旧配置：agent.debug_trace
	out := agent.DebugTraceConfig{
		Enabled:      agentCfg != nil && agentCfg.DebugTrace.Enabled,
		Dir:          "",
		MaxBodyBytes: 0,
	}
	if agentCfg != nil {
		out.Dir = strings.TrimSpace(agentCfg.DebugTrace.Dir)
		out.MaxBodyBytes = agentCfg.DebugTrace.MaxBodyBytes
	}

	// 新主配置：log.agent_debug（若配置过则优先）。
	if logCfg != nil {
		ad := logCfg.AgentDebug
		configured := ad.Enable || strings.TrimSpace(ad.Dir) != "" || ad.MaxBodyBytes > 0
		if configured {
			out.Enabled = ad.Enable
			out.Dir = strings.TrimSpace(ad.Dir)
			out.MaxBodyBytes = ad.MaxBodyBytes
		}
	}
	return out
}

func resolveContextOptimizerConfig(agentCfg *config.AgentConfig) agent.ContextOptimizerConfig {
	out := agent.ContextOptimizerConfig{
		Enabled:                   true,
		MaxPromptTokens:           12000,
		ReservedCompletionTokens:  1200,
		RecentMessages:            8,
		RetrievalTopK:             6,
		CacheMode:                 "auto",
		SummaryRefreshIntervalSec: 900,
	}
	if agentCfg == nil {
		return out
	}
	c := agentCfg.ContextOptimizer
	out.Enabled = c.Enabled
	if c.MaxPromptTokens > 0 {
		out.MaxPromptTokens = c.MaxPromptTokens
	}
	if c.ReservedCompletionTokens > 0 {
		out.ReservedCompletionTokens = c.ReservedCompletionTokens
	}
	if c.RecentMessages > 0 {
		out.RecentMessages = c.RecentMessages
	}
	if c.RetrievalTopK > 0 {
		out.RetrievalTopK = c.RetrievalTopK
	}
	if c.SummaryRefreshIntervalSec > 0 {
		out.SummaryRefreshIntervalSec = c.SummaryRefreshIntervalSec
	}
	mode := strings.ToLower(strings.TrimSpace(c.CacheMode))
	switch mode {
	case "auto", "force_off", "force_on":
		out.CacheMode = mode
	}
	return out
}

func resolvePlannerOptimizerConfig(agentCfg *config.AgentConfig) agent.PlannerOptimizerConfig {
	out := agent.PlannerOptimizerConfig{
		Enabled:       true,
		CandidateTopK: 24,
		PerKindQuota: agent.PlannerKindQuota{
			Workflow: 4,
			Skill:    10,
			Tooling:  8,
			LLM:      2,
		},
		PromptSlimMode:       "compact",
		DecisionCacheEnabled: true,
		DecisionCacheTTLSec:  60,
	}
	if agentCfg == nil {
		return out
	}
	c := agentCfg.PlannerOptimizer
	// 兼容旧配置：若 planner_optimizer 整块未配置（全零值），保持默认开启；
	// 只有检测到显式配置迹象时才采用 c.Enabled。
	explicit := c.CandidateTopK > 0 ||
		c.PerKindQuota.Workflow > 0 ||
		c.PerKindQuota.Skill > 0 ||
		c.PerKindQuota.Tooling > 0 ||
		c.PerKindQuota.LLM > 0 ||
		strings.TrimSpace(c.PromptSlimMode) != "" ||
		c.DecisionCacheEnabled ||
		c.DecisionCacheTTLSec > 0 ||
		c.Enabled
	if explicit {
		out.Enabled = c.Enabled
	}
	if c.CandidateTopK > 0 {
		out.CandidateTopK = c.CandidateTopK
	}
	if c.PerKindQuota.Workflow > 0 {
		out.PerKindQuota.Workflow = c.PerKindQuota.Workflow
	}
	if c.PerKindQuota.Skill > 0 {
		out.PerKindQuota.Skill = c.PerKindQuota.Skill
	}
	if c.PerKindQuota.Tooling > 0 {
		out.PerKindQuota.Tooling = c.PerKindQuota.Tooling
	}
	if c.PerKindQuota.LLM > 0 {
		out.PerKindQuota.LLM = c.PerKindQuota.LLM
	}
	switch mode := strings.ToLower(strings.TrimSpace(c.PromptSlimMode)); mode {
	case "compact", "verbose":
		out.PromptSlimMode = mode
	}
	out.DecisionCacheEnabled = c.DecisionCacheEnabled
	if c.DecisionCacheTTLSec > 0 {
		out.DecisionCacheTTLSec = c.DecisionCacheTTLSec
	}
	return out
}
