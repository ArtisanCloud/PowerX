package service

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
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
}

func NewAgentSettingService(db *gorm.DB) *AgentSettingService {
	return &AgentSettingService{
		db:        db,
		credRepo:  repoai.NewAIProviderCredentialRepository(db),
		profRepo:  repoai.NewAIModelProfileRepository(db),
		routeRepo: repoai.NewAIRoutePolicyRepository(db),
		usageRepo: repoai.NewAIUsageLogRepository(db),
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
	env string, tenantID *uint64, // <<< 关键：显式传入作用域
	cred *dbmodel.AIProviderCredential,
	prof *dbmodel.AIModelProfile,
) error {
	// 保险起见，即便 handler 已经填过，这里也强制回写一次作用域字段
	cred.Env, cred.TenantID = env, tenantID
	prof.Env, prof.TenantID = env, tenantID

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		credRepo := repoai.NewAIProviderCredentialRepository(tx)
		// 注意：这里调用的是你**新的** Upsert 签名（包含 env/tenantID）
		if err := credRepo.UpsertByScopeNameProvider(ctx, env, tenantID, cred); err != nil {
			return err
		}

		profRepo := repoai.NewAIModelProfileRepository(tx)
		if err := profRepo.UpsertByScopeModalityProviderModel(ctx, env, tenantID, prof); err != nil {
			return err
		}
		return nil
	})
}

// ---------------- Tests（LLM） ----------------

// PingLLM：健康检查
func (s *AgentSettingService) PingLLM(ctx context.Context, provider, model, baseURL, apiKey string) error {
	if provider == "" || model == "" {
		return fmt.Errorf("provider/model 不能为空")
	}
	mc := agentllm.ModelConfig{
		Provider:     provider,
		Endpoint:     baseURL,
		APIKey:       apiKey,
		Model:        model,
		SystemPrompt: "You are a health check probe.",
		Temperature:  0.0,
		MaxTokens:    8,
		AccessToken:  apiKey, // 兼容部分厂商
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return err
	}
	_, err = cli.ChatOnce(ctx, mc, "ping")
	return err
}

// QuickCallLLM：最小可用试跑
func (s *AgentSettingService) QuickCallLLM(
	ctx context.Context,
	provider, model, baseURL, apiKey string,
	temperature float64, maxTokens int,
	prompt string,
) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Say hello in one short sentence."
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
	// 防止卡住：30s 兜底
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
