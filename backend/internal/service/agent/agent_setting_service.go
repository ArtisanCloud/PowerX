package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	agentllm "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/llm"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
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
	db        *gorm.DB
	credRepo  *repoai.AIProviderCredentialRepository
	profRepo  *repoai.AIModelProfileRepository
	routeRepo *repoai.AIRoutePolicyRepository
	usageRepo *repoai.AIUsageLogRepository
	tks       *tenantkeys.TenantKeyService
}

func NewAgentSettingService(db *gorm.DB) *AgentSettingService {
	return &AgentSettingService{
		db:        db,
		credRepo:  repoai.NewAIProviderCredentialRepository(db),
		profRepo:  repoai.NewAIModelProfileRepository(db),
		routeRepo: repoai.NewAIRoutePolicyRepository(db),
		usageRepo: repoai.NewAIUsageLogRepository(db),
		tks:       tenantkeys.NewTenantKeyService(db),
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

// ---------------- Settings 持久化 ----------------
// internal/service/agent/agent_setting_service.go
func (s *AgentSettingService) SaveCredentialAndProfile(
	ctx context.Context,
	env string, tenantID *uint64,
	cred *dbmodel.AIProviderCredential,
	prof *dbmodel.AIModelProfile,
	setActive bool, // ← 新增开关，通常传 true
) error {
	cred.Env, cred.TenantID = env, tenantID
	prof.Env, prof.TenantID = env, tenantID

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
		if old, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantID, cred.Name, cred.Provider); err == nil && old != nil {
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
	enc, err := s.tks.SealSensitive(ctx, env, tenantID, cred.Data, sensKeys...)
	if err != nil {
		return err
	}
	cred.Data = enc

	// 事务：凭据 + 画像 +（可选）激活项
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repoai.NewAIProviderCredentialRepository(tx).
			UpsertByScopeNameProvider(ctx, env, tenantID, cred); err != nil {
			return err
		}
		if err := repoai.NewAIModelProfileRepository(tx).
			UpsertByScopeModalityProviderModel(ctx, env, tenantID, prof); err != nil {
			return err
		}
		if setActive {
			if err := repoai.NewAIRoutePolicyRepository(tx).
				UpsertDefaultByScopeModality(ctx, env, tenantID, prof.Modality, prof.Provider, prof.Model); err != nil {
				return err
			}
		}
		return nil
	})
}

// 从库里解密出 api_key/base_url 作为回退
func (s *AgentSettingService) resolveConnFromStore(
	ctx context.Context, env string, tenantID *uint64, provider string,
	baseURLIn, apiKeyIn string,
) (baseURL, apiKey string, err error) {
	baseURL, apiKey = baseURLIn, apiKeyIn
	// 名称规则与你 handler 构造时一致
	name := utils.Slug(env + "-" + provider)

	cred, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantID, name, provider)
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
		if e := s.tks.UnsealSensitive(ctx, env, tenantID, cred.Data, &sec); e == nil {
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
	ctx context.Context, env string, tenantID *uint64,
	modality, provider, model, baseURL, apiKey string,
) error {

	mod := strings.ToLower(strings.TrimSpace(modality))
	prov := strings.TrimSpace(provider)

	switch contract.Modality(mod) {
	case contract.ModLLM:
		// 1) 从 catalog 读取鉴权要求与默认值
		req := catalog.AuthReqFromCatalog(prov) // ← 见下方

		// 2) 先用这次提交的值
		bu := strings.TrimSpace(baseURL)
		ak := strings.TrimSpace(apiKey)

		// 3) baseURL：若该 provider 声明需要 base_url 且未传，先用 defaults.base_url 再用已保存
		if req.NeedBaseURL && bu == "" {
			if req.DefaultBaseURL != "" {
				bu = req.DefaultBaseURL
			}
			if bu == "" {
				// 再从已保存补 base_url（不解密）
				if cred, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantID, utils.Slug(env+"-"+prov), prov); err == nil && cred != nil {
					if v, ok := cred.Data["base_url"].(string); ok && strings.TrimSpace(v) != "" {
						bu = strings.TrimSpace(v)
					}
				}
			}
			if bu == "" {
				return fmt.Errorf("缺少 BaseURL（%s 要求 base_url）", provider)
			}
		}

		// 4) api_key：若该 provider 需要 key 且未传，尝试从已保存配置解封；仍然没有则报错
		if req.NeedKey && ak == "" {
			_, ak2, err := s.resolveConnFromStore(ctx, env, tenantID, prov, bu, ak)
			if err == nil && strings.TrimSpace(ak2) != "" {
				ak = strings.TrimSpace(ak2)
			}
			if ak == "" {
				return fmt.Errorf("缺少 API Key（%s 要求 api_key）", provider)
			}
		}

		// 5) 严格直连：只用我们最终算出的 bu/ak，不再做其他回退
		return s.PingStrict(ctx, mod, prov, model, bu, ak)

	default:
		return s.TestConnectionBasic(ctx, env, tenantID, contract.Modality(mod), provider, model)
	}
}

// 对非 LLM 模态的基础校验，暂不做真实连通测试
func (s *AgentSettingService) TestConnectionBasic(
	ctx context.Context, env string, tenantID *uint64,
	modality contract.Modality, provider, model string,
) error {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("%s provider/model 不能为空", modality)
	}
	return nil
}

func (s *AgentSettingService) RotateTenantCredentials(
	ctx context.Context, env string, tenantID *uint64,
) error {
	creds, err := s.credRepo.ListByScope(ctx, env, tenantID)
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		_, err := s.tks.RotateKeyPair(ctx, env, tenantID)
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
		if err := s.tks.UnsealSensitive(ctx, env, tenantID, clone, &secret); err != nil {
			return err
		}
		items = append(items, rotationItem{cred: &creds[i], secrets: secret})
	}
	if len(items) == 0 {
		return nil
	}

	if _, err := s.tks.RotateKeyPair(ctx, env, tenantID); err != nil {
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
		sealed, err := s.tks.SealSensitive(ctx, env, tenantID, item.cred.Data, sensitiveCredentialKeys...)
		if err != nil {
			return err
		}
		item.cred.Data = sealed
		if err := s.credRepo.UpsertByScopeNameProvider(ctx, env, tenantID, item.cred); err != nil {
			return err
		}
	}
	return nil
}

// 修改 PingLLM：多两个参数 env/tenantID，并支持回退解密
func (s *AgentSettingService) PingLLM(ctx context.Context, env string, tenantID *uint64,
	provider, model, baseURL, apiKey string,
) error {
	if provider == "" || model == "" {
		return fmt.Errorf("provider/model 不能为空")
	}
	// 回退解密
	var err error
	baseURL, apiKey, err = s.resolveConnFromStore(ctx, env, tenantID, provider, baseURL, apiKey)
	if err != nil { /* 忽略查不到，走传入的 */
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
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return err
	}
	_, err = cli.Invoke(ctx, &mc, "ping")
	return err
}

// 修改 QuickCallLLM：同样带 env/tenantID + 回退解密
func (s *AgentSettingService) QuickCallLLM(
	ctx context.Context, env string, tenantID *uint64,
	provider, model, baseURL, apiKey string,
	temperature float64, maxTokens int,
	prompt string,
) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Say hello in one short sentence."
	}
	// 回退解密
	var err error
	baseURL, apiKey, err = s.resolveConnFromStore(ctx, env, tenantID, provider, baseURL, apiKey)
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
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.Invoke(ctx, &mc, prompt)
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
	ctx context.Context, env string, tenantID *uint64, modalities ...string,
) ([]dbmodel.AIModelProfile, error) {

	return s.profRepo.ListByScope(ctx, env, tenantID, modalities...)

}

// （可选）拉本租户的凭据列表
func (s *AgentSettingService) ListCredentials(
	ctx context.Context, env string, tenantID *uint64,
) ([]dbmodel.AIProviderCredential, error) {

	return s.credRepo.ListByScope(ctx, env, tenantID)
}

func (s *AgentSettingService) GetActiveProfile(
	ctx context.Context, env string, tenantID *uint64, modality string,
) (*dbmodel.AIModelProfile, error) {
	rp, err := s.routeRepo.FindDefaultByScopeModality(ctx, env, tenantID, modality) // Name="__default"
	if err == nil && rp != nil && rp.Provider != "" && rp.Model != "" {
		return s.profRepo.FindByScopeModalityProviderModel(ctx, env, tenantID, modality, rp.Provider, rp.Model)
	}
	// 没设置默认时可选一个兜底（例如最近更新的）
	list, err := s.profRepo.ListByScope(ctx, env, tenantID, modality)
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
	ctx context.Context, env string, tenantID *uint64, modality, provider, model string,
) error {
	return s.routeRepo.UpsertDefaultByScopeModality(ctx, env, tenantID, modality, provider, model)
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
