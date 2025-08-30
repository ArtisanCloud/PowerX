package service

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"

	// 直接复用你已有的一次性聊天能力
	agentllm "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/llm"
	"gorm.io/gorm"
)

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

func (s *AgentSettingService) Providers(modality string) []string {
	// 交给 catalog 实现（由 handler 传参）
	items := catalogGetProviders(strings.TrimSpace(modality))
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Name)
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
// SaveCredentialAndProfile：把当前模态的“连接信息/参数”写入两张表（幂等）
func (s *AgentSettingService) SaveCredentialAndProfile(
	ctx context.Context,
	env string, tenantID *uint64,
	cred *dbmodel.AIProviderCredential,
	prof *dbmodel.AIModelProfile,
) error {
	cred.Env, cred.TenantID = env, tenantID
	prof.Env, prof.TenantID = env, tenantID

	// 关键：把 api_key/secret/access_token/client_secret 等敏感键“封装后存库”
	var err error
	cred.Data, err = s.tks.SealSensitive(ctx, env, tenantID, cred.Data,
		"api_key", "secret", "access_token", "client_secret",
	)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repoai.NewAIProviderCredentialRepository(tx).
			UpsertByScopeNameProvider(ctx, env, tenantID, cred); err != nil {
			return err
		}
		if err := repoai.NewAIModelProfileRepository(tx).
			UpsertByScopeModalityProviderModel(ctx, env, tenantID, prof); err != nil {
			return err
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

	mc := agentllm.ModelConfig{
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
	_, err = cli.ChatOnce(ctx, mc, "ping")
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

	mc := agentllm.ModelConfig{
		Provider:     provider,
		Endpoint:     baseURL,
		APIKey:       apiKey,
		Model:        model,
		SystemPrompt: "You are a helpful assistant.",
		Temperature:  temperature,
		MaxTokens:    maxInt(maxTokens, 64),
		AccessToken:  apiKey,
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.ChatOnce(ctx, mc, prompt)
}

// ---------------- helpers ----------------

func maxInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

// —— catalog 适配：在 service 层做轻薄封装，避免 handler 直依赖 —— //

type aiProviderItem struct{ Name string }
type aiModelItem struct{ ID string }

// 这两个函数直接调用你现有的 catalog，全局注册器：catalog.GetGlobalAIRegister()
func catalogGetProviders(mod string) []aiProviderItem {
	items := catalog.GetGlobalAIRegister().Providers(mod)
	out := make([]aiProviderItem, 0, len(items))
	for _, it := range items {
		out = append(out, aiProviderItem{Name: it.Name})
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
