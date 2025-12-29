package agent

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	agentllm "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/llm"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ModelRule struct {
	RequireAPIKey  bool
	RequireBaseURL bool
	DefaultBaseURL string
}

var sensitiveCredentialKeys = []string{"api_key", "secret", "client_secret", "access_token"}

type AgentSettingService struct {
	db         *gorm.DB
	credRepo   *repoai.AIProviderCredentialRepository
	profRepo   *repoai.AIModelProfileRepository
	routeRepo  *repoai.AIRoutePolicyRepository
	usageRepo  *repoai.AIUsageLogRepository
	tks        *tenantkeys.TenantKeyService
	tenantRepo *tenantrepo.TenantRepository
}

func NewAgentSettingService(db *gorm.DB) *AgentSettingService {
	return &AgentSettingService{
		db:         db,
		credRepo:   repoai.NewAIProviderCredentialRepository(db),
		profRepo:   repoai.NewAIModelProfileRepository(db),
		routeRepo:  repoai.NewAIRoutePolicyRepository(db),
		usageRepo:  repoai.NewAIUsageLogRepository(db),
		tks:        tenantkeys.NewTenantKeyService(db),
		tenantRepo: tenantrepo.NewTenantRepository(db),
	}
}

// ---------------- Providers / Models ----------------

func (s *AgentSettingService) Providers(modality string) []aiProviderItem {
	items := catalogGetProviders(strings.TrimSpace(modality))
	out := make([]aiProviderItem, 0, len(items))
	for _, it := range items {
		out = append(out, aiProviderItem{ID: it.ID, Name: it.Name})
	}
	return out
}
func (s *AgentSettingService) Models(modality, provider string) ([]string, error) {
	models, err := catalogGetModels(strings.TrimSpace(modality), strings.TrimSpace(provider))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.ID)
	}
	return out, nil
}

func (s *AgentSettingService) tenantScopeKey(tenantUUID *string) string {
	if tenantUUID == nil {
		return ""
	}
	return strings.TrimSpace(*tenantUUID)
}

// ---------------- Settings 持久化 ----------------
// internal/service/agent/agent_setting_service.go
func (s *AgentSettingService) SaveCredentialAndProfile(
	ctx context.Context,
	env string, tenantUUID *string,
	cred *dbmodel.AIProviderCredential,
	prof *dbmodel.AIModelProfile,
	setActive bool, // ← 新增开关，通常传 true
) error {
	cred.Env, cred.TenantUUID = env, tenantUUID
	prof.Env, prof.TenantUUID = env, tenantUUID
	s.applyManifestToProfile(prof)

	// 是否提交了新密钥？
	sensKeys := []string{"api_key", "access_token", "client_secret", "secret"}
	hasNewSecret := false
	for _, k := range sensKeys {
		if v, _ := cred.Data[k].(string); strings.TrimSpace(v) != "" {
			hasNewSecret = true
			break
		}
	}

	baseURL := ""
	if v, _ := cred.Data["base_url"].(string); v != "" {
		baseURL = v
	}

	// 有新密钥则严格直连校验（只用这次入参，不读库）
	if hasNewSecret {
		if err := s.PingStrict(ctx, prof.Modality, prof.Provider, prof.Model,
			baseURL,
			utils.FirstJSONNonEmpty(cred.Data, sensKeys...),
		); err != nil {
			return fmt.Errorf("连通性校验失败: %w", err)
		}
	} else {
		// 没提新密钥：保留旧的 __sealed 和 base_url（如有）
		if old, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantUUID, cred.Name, cred.Provider); err == nil && old != nil {
			if cred.Data != nil {
				if cred.Data["__sealed"] == nil && old.Data != nil && old.Data["__sealed"] != nil {
					cred.Data["__sealed"] = old.Data["__sealed"]
				}
				if baseURL == "" && old.Data != nil {
					if bu, _ := old.Data["base_url"].(string); strings.TrimSpace(bu) != "" {
						cred.Data["base_url"] = bu
					}
				}
			}
		}
	}

	// 加密敏感键
	scopeKey := s.tenantScopeKey(tenantUUID)
	enc, err := s.tks.SealSensitive(ctx, env, scopeKey, cred.Data, sensKeys...)
	if err != nil {
		return err
	}
	cred.Data = enc

	// 事务：凭据 + 画像 +（可选）激活项
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repoai.NewAIProviderCredentialRepository(tx).
			UpsertByScopeNameProvider(ctx, env, tenantUUID, cred); err != nil {
			return err
		}
		if err := repoai.NewAIModelProfileRepository(tx).
			UpsertByScopeModalityProviderModel(ctx, env, tenantUUID, prof); err != nil {
			return err
		}
		if setActive {
			if err := repoai.NewAIRoutePolicyRepository(tx).
				UpsertDefaultByScopeModality(ctx, env, tenantUUID, prof.Modality, prof.Provider, prof.Model); err != nil {
				return err
			}
		}
		return nil
	})
}

// 从库里解密出 api_key/base_url 作为回退
func (s *AgentSettingService) resolveConnFromStore(
	ctx context.Context, env string, tenantUUID *string, provider string,
	baseURLIn, apiKeyIn string,
) (baseURL, apiKey string, err error) {
	baseURL, apiKey = baseURLIn, apiKeyIn
	// 名称规则与你 handler 构造时一致
	name := utils.Slug(env + "-" + provider)

	cred, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantUUID, name, provider)
	if err != nil {
		return baseURL, apiKey, err
	}
	// 先用存量 base_url
	if baseURL == "" {
		if v, ok := cred.Data["base_url"].(string); ok {
			baseURL = v
		}
	}
	// 再补 api_key（仅后端内部使用，不回前端）
	if apiKey == "" {
		var sec struct {
			APIKey string `json:"api_key"`
			Secret string `json:"secret"`
		}
		if e := s.tks.UnsealSensitive(ctx, env, s.tenantScopeKey(tenantUUID), cred.Data, &sec); e == nil {
			apiKey = sec.APIKey
		}
	}
	return baseURL, apiKey, nil
}

// 只用“这次提交的表单值”直连一次（不读库、不回退、不解封）
func (s *AgentSettingService) PingStrict(ctx context.Context, modality, provider, model, baseURL, apiKey string) error {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("provider/model 不能为空")
	}
	rule := s.resolveModelRule(modality, provider, model)

	// 应用规则
	if rule.RequireAPIKey && strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("%s/%s 需要 apiKey", provider, model)
	}
	if strings.TrimSpace(baseURL) == "" {
		if rule.DefaultBaseURL != "" {
			baseURL = rule.DefaultBaseURL
		} else if rule.RequireBaseURL {
			return fmt.Errorf("%s/%s 需要 baseURL", provider, model)
		}
	}

	mc := agentcfg.ModelConfig{
		Provider: provider, Endpoint: baseURL, APIKey: apiKey,
		Model: model, SystemPrompt: "You are a health check probe.",
		Temperature: 0, MaxTokens: 8, AccessToken: apiKey,
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return err
	}
	c2, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = cli.Invoke(c2, &mc, "ping")
	return err
}

// TestConnectionPreferInput：测试接口用
// 逻辑：如果提交里有 baseURL 或 apiKey（任一非空），就用提交参数“严格直连”；
// 否则从库里读取已保存配置（含密钥解封）后再 ping。
func (s *AgentSettingService) TestConnectionPreferInput(
	ctx context.Context, env string, tenantUUID *string,
	modality, provider, model, baseURL, apiKey string,
) error {

	mod := strings.ToLower(strings.TrimSpace(modality))
	prov := strings.TrimSpace(provider)

	switch contract.Modality(mod) {
	case contract.ModLLM:
		req := catalog.AuthReqFromCatalog(prov)
		bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, prov, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
		if err != nil {
			return err
		}
		return s.PingStrict(ctx, mod, prov, model, bu, ak)

	default:
		return s.PingGeneric(ctx, env, tenantUUID, contract.Modality(mod), provider, model, baseURL, apiKey)
	}
}

// 对非 LLM 模态的基础校验，暂不做真实连通测试
func (s *AgentSettingService) RotateTenantCredentials(
	ctx context.Context, env string, tenantUUID *string,
) error {
	scopeKey := s.tenantScopeKey(tenantUUID)
	creds, err := s.credRepo.ListByScope(ctx, env, tenantUUID)
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		_, err := s.tks.RotateKeyPair(ctx, env, scopeKey)
		return err
	}

	type rotationItem struct {
		cred    *dbmodel.AIProviderCredential
		secrets map[string]any
	}
	items := make([]rotationItem, 0, len(creds))
	for i := range creds {
		clone := utils.CloneJSONMap(creds[i].Data)
		secret := map[string]any{}
		if err := s.tks.UnsealSensitive(ctx, env, scopeKey, clone, &secret); err != nil {
			return err
		}
		items = append(items, rotationItem{cred: &creds[i], secrets: secret})
	}
	if len(items) == 0 {
		return nil
	}

	if _, err := s.tks.RotateKeyPair(ctx, env, scopeKey); err != nil {
		return err
	}

	for _, item := range items {
		if item.cred.Data == nil {
			item.cred.Data = datatypes.JSONMap{}
		}
		delete(item.cred.Data, "__sealed")
		for k, v := range item.secrets {
			item.cred.Data[k] = v
		}
		sealed, err := s.tks.SealSensitive(ctx, env, scopeKey, item.cred.Data, sensitiveCredentialKeys...)
		if err != nil {
			return err
		}
		item.cred.Data = sealed
		if err := s.credRepo.UpsertByScopeNameProvider(ctx, env, tenantUUID, item.cred); err != nil {
			return err
		}
	}
	return nil
}

// 修改 PingLLM：多两个参数 env/tenantUUID，并支持回退解密
func (s *AgentSettingService) PingLLM(ctx context.Context, env string, tenantUUID *string,
	provider, model, baseURL, apiKey string,
) error {
	if err := ensureModelExists(string(contract.ModLLM), provider, model); err != nil {
		return err
	}
	req := catalog.AuthReqFromCatalog(provider)
	var err error
	baseURL, apiKey, err = s.prepareAuthInputs(ctx, env, tenantUUID, provider, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return err
	}
	if err := validateEndpoint(baseURL); err != nil {
		return err
	}
	mc := agentcfg.ModelConfig{
		Provider:     provider,
		Endpoint:     baseURL,
		APIKey:       apiKey,
		Model:        model,
		SystemPrompt: "You are a health check probe.",
		Temperature:  0.0,
		MaxTokens:    8,
		AccessToken:  apiKey,
		Extra:        s.buildModelExtras(contract.ModLLM, provider, model),
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return err
	}
	_, err = cli.Invoke(ctx, &mc, "ping")
	return err
}

func (s *AgentSettingService) PingGeneric(
	ctx context.Context, env string, tenantUUID *string,
	modality contract.Modality, provider, model, baseURL, apiKey string,
) error {
	if err := ensureModelExists(string(modality), provider, model); err != nil {
		return err
	}
	req := catalog.AuthReqFromCatalog(provider)
	bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, provider, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return err
	}
	if err := validateEndpoint(bu); err != nil {
		return err
	}
	_ = ak
	return nil
}

// 修改 QuickCallLLM：同样带 env/tenantUUID + 回退解密
func (s *AgentSettingService) QuickCallLLM(
	ctx context.Context, env string, tenantUUID *string,
	provider, model, baseURL, apiKey string,
	temperature float64, maxTokens int,
	prompt string,
) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Say hello in one short sentence."
	}
	// 回退解密
	var err error
	baseURL, apiKey, err = s.resolveConnFromStore(ctx, env, tenantUUID, provider, baseURL, apiKey)
	if err != nil { /* 忽略错误，尽量继续 */
	}

	mc := agentcfg.ModelConfig{
		Provider:     provider,
		Endpoint:     baseURL,
		APIKey:       apiKey,
		Model:        model,
		SystemPrompt: "You are a helpful assistant.",
		Temperature:  temperature,
		MaxTokens:    utils.MaxInt(maxTokens, 64),
		AccessToken:  apiKey,
		Extra:        s.buildModelExtras(contract.ModLLM, provider, model),
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.Invoke(ctx, &mc, prompt)
}

func (s *AgentSettingService) applyManifestToProfile(prof *dbmodel.AIModelProfile) {
	if prof == nil {
		return
	}
	if prof.Defaults == nil {
		prof.Defaults = datatypes.JSONMap{}
	}
	manifest := findModelManifest(prof.Modality, prof.Provider, prof.Model)
	if manifest != nil {
		if prof.Label == "" {
			prof.Label = manifest.Label
		}
		prof.Defaults = mergeManifestDefaults(prof.Defaults, manifest.Defaults)
		prof.Tags = mergeTags(prof.Modality, manifest.Tags, prof.Tags)
	} else {
		prof.Tags = mergeTags(prof.Modality, nil, prof.Tags)
	}
}

func (s *AgentSettingService) prepareAuthInputs(
	ctx context.Context, env string, tenantUUID *string,
	provider, baseURL, apiKey string,
	needBase bool, defaultBase string, needKey bool,
) (string, string, error) {
	bu := strings.TrimSpace(baseURL)
	ak := strings.TrimSpace(apiKey)

	if bu == "" || ak == "" {
		if resolvedBase, resolvedKey, err := s.resolveConnFromStore(ctx, env, tenantUUID, provider, bu, ak); err == nil {
			if bu == "" && strings.TrimSpace(resolvedBase) != "" {
				bu = strings.TrimSpace(resolvedBase)
			}
			if ak == "" && strings.TrimSpace(resolvedKey) != "" {
				ak = strings.TrimSpace(resolvedKey)
			}
		}
	}

	if needBase && bu == "" {
		bu = strings.TrimSpace(defaultBase)
		if bu == "" {
			return "", "", fmt.Errorf("缺少 BaseURL（%s 要求 base_url）", provider)
		}
	}
	if needKey && ak == "" {
		return bu, ak, fmt.Errorf("缺少 API Key（%s 要求 api_key）", provider)
	}
	return bu, ak, nil
}

func validateEndpoint(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("base_url 非法: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("base_url 非法: %s", raw)
	}
	return nil
}

func ensureModelExists(modality, provider, model string) error {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("%s provider/model 不能为空", modality)
	}
	if manifest := findModelManifest(modality, provider, model); manifest == nil {
		return fmt.Errorf("provider %s 不包含模型 %s (%s)", provider, model, modality)
	}
	return nil
}

func (s *AgentSettingService) buildModelExtras(modality contract.Modality, provider, model string) map[string]any {
	manifest := findModelManifest(string(modality), provider, model)
	if manifest == nil || manifest.Defaults == nil {
		return nil
	}
	if raw, ok := manifest.Defaults["api_path"]; ok {
		if path, ok2 := raw.(string); ok2 && strings.TrimSpace(path) != "" {
			return map[string]any{"api_path": path}
		}
	}
	return nil
}

func mergeManifestDefaults(user datatypes.JSONMap, manifest map[string]any) datatypes.JSONMap {
	out := datatypes.JSONMap{}
	if manifest != nil {
		for k, v := range manifest {
			out[k] = v
		}
	}
	if user != nil {
		for k, v := range user {
			out[k] = v
		}
	}
	return out
}

func mergeTags(modality string, manifestTags []string, existing datatypes.JSONSlice[string]) datatypes.JSONSlice[string] {
	all := make([]string, 0, len(manifestTags)+len(existing)+1)
	all = append(all, strings.ToLower(strings.TrimSpace(modality)))
	all = append(all, manifestTags...)
	for _, tag := range existing {
		all = append(all, tag)
	}
	dedup := make([]string, 0, len(all))
	seen := map[string]struct{}{}
	for _, tag := range all {
		t := strings.TrimSpace(tag)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dedup = append(dedup, t)
	}
	return datatypes.JSONSlice[string](dedup)
}

func findModelManifest(modality, provider, model string) *catalog.ModelManifest {
	reg := catalog.GetGlobalAIRegister()
	models, err := reg.Models(modality, provider)
	if err != nil {
		return nil
	}
	for _, m := range models {
		if strings.EqualFold(m.ID, model) {
			copy := m
			return &copy
		}
	}
	return nil
}

// —— catalog 适配：在 service 层做轻薄封装，避免 handler 直依赖 —— //

type aiProviderItem struct {
	ID   string
	Name string
}

type aiModelItem struct {
	ID   string
	Name string
}

// 这两个函数直接调用你现有的 catalog，全局注册器：catalog.GetGlobalAIRegister()
func catalogGetProviders(mod string) []aiProviderItem {
	items := catalog.GetGlobalAIRegister().Providers(mod)
	out := make([]aiProviderItem, 0, len(items))
	for _, it := range items {
		out = append(out, aiProviderItem{ID: it.ID, Name: it.Name})
	}
	return out
}
func catalogGetModels(mod, prov string) ([]aiModelItem, error) {
	ms, err := catalog.GetGlobalAIRegister().Models(mod, prov)
	if err != nil {
		return nil, err
	}
	out := make([]aiModelItem, 0, len(ms))
	for _, m := range ms {
		out = append(out, aiModelItem{ID: m.ID})
	}
	return out, nil
}

func (s *AgentSettingService) ListProfiles(
	ctx context.Context, env string, tenantUUID *string, modalities ...string,
) ([]dbmodel.AIModelProfile, error) {

	return s.profRepo.ListByScope(ctx, env, tenantUUID, modalities...)

}

// （可选）拉本租户的凭据列表
func (s *AgentSettingService) ListCredentials(
	ctx context.Context, env string, tenantUUID *string,
) ([]dbmodel.AIProviderCredential, error) {

	return s.credRepo.ListByScope(ctx, env, tenantUUID)
}

func (s *AgentSettingService) GetActiveProfile(
	ctx context.Context, env string, tenantUUID *string, modality string,
) (*dbmodel.AIModelProfile, error) {
	rp, err := s.routeRepo.FindDefaultByScopeModality(ctx, env, tenantUUID, modality) // Name="__default"
	if err == nil && rp != nil && rp.Provider != "" && rp.Model != "" {
		return s.profRepo.FindByScopeModalityProviderModel(ctx, env, tenantUUID, modality, rp.Provider, rp.Model)
	}
	// 没设置默认时可选一个兜底（例如最近更新的）
	list, err := s.profRepo.ListByScope(ctx, env, tenantUUID, modality)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	latest := list[0]
	for i := 1; i < len(list); i++ {
		if list[i].UpdatedAt.After(latest.UpdatedAt) {
			latest = list[i]
		}
	}
	return &latest, nil
}

// service：设置某模态的“当前激活”
func (s *AgentSettingService) SetActiveProfile(
	ctx context.Context, env string, tenantUUID *string, modality, provider, model string,
) error {
	return s.routeRepo.UpsertDefaultByScopeModality(ctx, env, tenantUUID, modality, provider, model)
}

// 统一解析规则：优先从 catalog 读取；否则按常见 provider 兜底。
func (s *AgentSettingService) resolveModelRule(modality, provider, model string) ModelRule {
	reg := catalog.GetGlobalAIRegister()
	if m, ok := reg.Manifest(provider); ok && m != nil {
		// 1) 必填项来自 auth.fields
		reqAPI := false
		reqBase := false
		for _, f := range m.Auth.Fields {
			lf := strings.ToLower(strings.TrimSpace(f))
			if lf == "api_key" {
				reqAPI = true
			}
			if lf == "base_url" {
				reqBase = true
			}
		}
		// 2) 默认 base_url: model.defaults 覆盖 auth.defaults
		def := ""
		// 先 model 级
		if models, _ := reg.Models(modality, provider); len(models) > 0 {
			for _, mm := range models {
				if strings.EqualFold(mm.ID, model) {
					if v, ok := mm.Defaults["base_url"].(string); ok && strings.TrimSpace(v) != "" {
						def = v
					}
					break
				}
			}
		}
		// 再 provider 级
		if def == "" && m.Auth.Fields != nil && m.Auth.Defaults != nil {
			if v := strings.TrimSpace(m.Auth.Defaults["base_url"]); v != "" {
				def = v
			}
		}
		return ModelRule{RequireAPIKey: reqAPI, RequireBaseURL: reqBase, DefaultBaseURL: def}
	}
	// 兜底（避免 catalog 未配置时不可用）
	//switch strings.ToLower(provider) {
	//case "openai":
	//	return ModelRule{RequireAPIKey: true, DefaultBaseURL: "https://api.openai.com/v1"}
	//case "ollama", "ollama (local)":
	//	return ModelRule{RequireBaseURL: true}
	//default:
	//	return ModelRule{}
	//}

	return ModelRule{}
}
