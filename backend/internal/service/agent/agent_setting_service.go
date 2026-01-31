package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentconf "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	agentllm "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/llm"
	intentfactory "github.com/ArtisanCloud/PowerX/internal/server/agent/factory/intent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ModelRule struct {
	RequireAPIKey    bool
	RequireSecretID  bool
	RequireSecretKey bool
	RequireBaseURL   bool
	DefaultBaseURL   string
}

var sensitiveCredentialKeys = []string{"api_key", "secret_id", "secret_key", "secret", "client_secret", "access_token"}

const TenantSettingKeyAICurrentEnv = "ai.current_env"
const tenantSettingKeyAIProviderHealthPrefix = "ai.provider_health"

type ProviderHealthRecord struct {
	Status    string `json:"status"`    // healthy|unhealthy|unknown
	CheckedAt int64  `json:"checkedAt"` // unix seconds
	Message   string `json:"message"`   // error or "ok"
}

type AgentSettingService struct {
	db          *gorm.DB
	credRepo    *repoai.AIProviderCredentialRepository
	profRepo    *repoai.AIModelProfileRepository
	routeRepo   *repoai.AIRoutePolicyRepository
	usageRepo   *repoai.AIUsageLogRepository
	tks         *tenantkeys.TenantKeyService
	tenantRepo  *tenantrepo.TenantRepository
	settingRepo *settingrepo.TenantSettingRepository
}

func NewAgentSettingService(db *gorm.DB) *AgentSettingService {
	return &AgentSettingService{
		db:          db,
		credRepo:    repoai.NewAIProviderCredentialRepository(db),
		profRepo:    repoai.NewAIModelProfileRepository(db),
		routeRepo:   repoai.NewAIRoutePolicyRepository(db),
		usageRepo:   repoai.NewAIUsageLogRepository(db),
		tks:         tenantkeys.NewTenantKeyService(db),
		tenantRepo:  tenantrepo.NewTenantRepository(db),
		settingRepo: settingrepo.NewTenantSettingRepository(db),
	}
}

// ---------------- Tenant Current AI Env ----------------

func (s *AgentSettingService) GetTenantCurrentAIEnv(
	ctx context.Context, tenantUUID string,
) (env string, configured bool, err error) {
	out, err := s.settingRepo.GetByTenantAndKey(ctx, tenantUUID, TenantSettingKeyAICurrentEnv)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if out == nil || len(out.ValueJSON) == 0 {
		return "", false, nil
	}
	var v string
	if e := json.Unmarshal(out.ValueJSON, &v); e != nil {
		return "", false, nil
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false, nil
	}
	return v, true, nil
}

func (s *AgentSettingService) SetTenantCurrentAIEnv(
	ctx context.Context, tenantUUID string, env string,
) error {
	e := strings.TrimSpace(env)
	if e == "" {
		return fmt.Errorf("env 不能为空")
	}
	raw, _ := json.Marshal(e)
	return s.settingRepo.Upsert(ctx, &dbsetting.TenantSetting{
		TenantUUID: tenantUUID,
		Key:        TenantSettingKeyAICurrentEnv,
		ValueJSON:  datatypes.JSON(raw),
		Group:      "ai",
		Editable:   true,
	})
}

// ---------------- Tenant Provider Health (per env+modality) ----------------

func tenantProviderHealthKey(env string, modality string) string {
	e := strings.TrimSpace(env)
	m := strings.TrimSpace(strings.ToLower(modality))
	if e == "" {
		e = "default"
	}
	if m == "" {
		m = "llm"
	}
	return fmt.Sprintf("%s.%s.%s", tenantSettingKeyAIProviderHealthPrefix, e, m)
}

func (s *AgentSettingService) GetTenantProviderHealthMap(
	ctx context.Context, tenantUUID string, env string, modality string,
) (map[string]ProviderHealthRecord, bool, error) {
	key := tenantProviderHealthKey(env, modality)
	out, err := s.settingRepo.GetByTenantAndKey(ctx, tenantUUID, key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]ProviderHealthRecord{}, false, nil
		}
		return nil, false, err
	}
	if out == nil || len(out.ValueJSON) == 0 {
		return map[string]ProviderHealthRecord{}, false, nil
	}
	var m map[string]ProviderHealthRecord
	if e := json.Unmarshal(out.ValueJSON, &m); e != nil {
		return map[string]ProviderHealthRecord{}, false, nil
	}
	if m == nil {
		return map[string]ProviderHealthRecord{}, false, nil
	}
	// normalize keys
	norm := map[string]ProviderHealthRecord{}
	for k, v := range m {
		p := strings.ToLower(strings.TrimSpace(k))
		if p == "" {
			continue
		}
		norm[p] = v
	}
	return norm, true, nil
}

func (s *AgentSettingService) UpsertTenantProviderHealth(
	ctx context.Context, tenantUUID string, env string, modality string,
	provider string, status string, message string,
) error {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return fmt.Errorf("provider 不能为空")
	}
	key := tenantProviderHealthKey(env, modality)

	m, _, err := s.GetTenantProviderHealthMap(ctx, tenantUUID, env, modality)
	if err != nil {
		return err
	}
	if m == nil {
		m = map[string]ProviderHealthRecord{}
	}
	st := strings.ToLower(strings.TrimSpace(status))
	switch st {
	case "healthy", "unhealthy", "unknown":
	default:
		st = "unknown"
	}
	m[p] = ProviderHealthRecord{
		Status:    st,
		CheckedAt: time.Now().Unix(),
		Message:   strings.TrimSpace(message),
	}
	raw, _ := json.Marshal(m)
	return s.settingRepo.Upsert(ctx, &dbsetting.TenantSetting{
		TenantUUID: tenantUUID,
		Key:        key,
		ValueJSON:  datatypes.JSON(raw),
		Group:      "ai",
		Editable:   true,
	})
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

// ModelsForTenant：给 HTTP handler 用；针对 OpenRouter 支持远端拉取模型列表，失败则回退到本地 catalog。
func (s *AgentSettingService) ModelsForTenant(
	ctx context.Context,
	env string,
	tenantUUID *string,
	modality string,
	provider string,
) ([]string, error) {
	mod := strings.TrimSpace(strings.ToLower(modality))
	prov := strings.TrimSpace(strings.ToLower(provider))

	// OpenRouter：模型目录变化快，优先走远端 /models；失败则回退到本地目录（占位/示例）。
	if prov == "openrouter" && (mod == "llm" || mod == "embedding") {
		if remote, err := s.fetchOpenRouterModels(ctx, env, tenantUUID, mod); err == nil && len(remote) > 0 {
			return remote, nil
		}
	}
	return s.Models(mod, prov)
}

type openRouterModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Models []struct {
		ID string `json:"id"`
	} `json:"models"`
}

func (s *AgentSettingService) fetchOpenRouterModels(
	ctx context.Context,
	env string,
	tenantUUID *string,
	modality string,
) ([]string, error) {
	req := catalog.AuthReqFromCatalog("openrouter")
	// 拉模型列表通常不要求 key，但如果已配置 key，我们也带上，避免账号维度过滤导致列表不全。
	baseURL, apiKey, err := s.prepareAuthInputs(ctx, env, tenantUUID, "openrouter", "", "", true, req.DefaultBaseURL, false)
	if err != nil {
		return nil, err
	}
	if err := validateEndpoint(baseURL); err != nil {
		return nil, err
	}

	modelsURL := strings.TrimRight(baseURL, "/") + "/models"
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	// OpenRouter 建议携带（可选）
	r.Header.Set("X-Title", "PowerX")
	// 部分场景下 OpenRouter 会建议提供来源（可选，但对某些网关/策略更友好）
	r.Header.Set("HTTP-Referer", "https://powerx.local")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter models status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out openRouterModelsResponse
	if e := json.Unmarshal(body, &out); e != nil {
		return nil, fmt.Errorf("openrouter models parse failed: %w (body=%s)", e, strings.TrimSpace(string(body)))
	}

	raw := out.Data
	if len(raw) == 0 && len(out.Models) > 0 {
		raw = out.Models
	}

	ids := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, it := range raw {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		// 轻度过滤：按模态做保守筛选（embedding 更偏向 openai/* embedding）
		if strings.EqualFold(modality, "embedding") {
			if !strings.Contains(strings.ToLower(id), "embedding") && !strings.HasSuffix(strings.ToLower(id), "-embed") {
				// 不强过滤（很多 provider 不按 embedding 命名）；先放过
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
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
	sensKeys := []string{"api_key", "secret_id", "secret_key", "access_token", "client_secret", "secret"}
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
		// LLM：真实连通性校验（会实际调用第三方）
		if strings.EqualFold(prof.Modality, "llm") {
			apiKey := ""
			if v, _ := cred.Data["api_key"].(string); strings.TrimSpace(v) != "" {
				apiKey = strings.TrimSpace(v)
			}
			secretID := ""
			if v, _ := cred.Data["secret_id"].(string); strings.TrimSpace(v) != "" {
				secretID = strings.TrimSpace(v)
			}
			secretKey := ""
			if v, _ := cred.Data["secret_key"].(string); strings.TrimSpace(v) != "" {
				secretKey = strings.TrimSpace(v)
			}
			region := ""
			if v, _ := cred.Data["region"].(string); strings.TrimSpace(v) != "" {
				region = strings.TrimSpace(v)
			}
			// 腾讯混元：支持两种接入方式（OpenAI SDK / 腾讯云 SDK）
			if strings.EqualFold(strings.TrimSpace(prof.Provider), "hunyuan") {
				mode := ""
				if v, _ := cred.Data["auth_mode"].(string); strings.TrimSpace(v) != "" {
					mode = strings.TrimSpace(v)
				}
				if err := s.PingHunyuan(ctx, env, tenantUUID, mode, prof.Model, baseURL, apiKey, secretID, secretKey, region); err != nil {
					return fmt.Errorf("连通性校验失败: %w", err)
				}
			} else {
				if err := s.PingStrict(ctx, prof.Modality, prof.Provider, prof.Model,
					baseURL,
					apiKey,
					secretID,
					secretKey,
					region,
				); err != nil {
					return fmt.Errorf("连通性校验失败: %w", err)
				}
			}
		} else {
			// 非 LLM：目前只做结构校验（模型是否存在、base_url 是否合法、鉴权字段是否齐）
			if err := s.PingGeneric(
				ctx,
				env,
				tenantUUID,
				contract.Modality(strings.ToLower(strings.TrimSpace(prof.Modality))),
				prof.Provider,
				prof.Model,
				baseURL,
				utils.FirstJSONNonEmpty(cred.Data, sensKeys...),
			); err != nil {
				return fmt.Errorf("连通性校验失败: %w", err)
			}
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

// SaveCredentialOnly：用于“连接测试成功后自动保存凭据”（不写画像、不激活默认路由）
// - 会 SealSensitive（api_key/secret/...）
// - 若未提交新密钥，则保留旧的 __sealed / base_url
func (s *AgentSettingService) SaveCredentialOnly(
	ctx context.Context,
	env string, tenantUUID *string,
	cred *dbmodel.AIProviderCredential,
) error {
	if cred == nil {
		return fmt.Errorf("credential 不能为空")
	}
	cred.Env, cred.TenantUUID = env, tenantUUID

	// 是否提交了新密钥？
	sensKeys := []string{"api_key", "access_token", "client_secret", "secret"}
	hasNewSecret := false
	for _, k := range sensKeys {
		if cred.Data != nil {
			if v, _ := cred.Data[k].(string); strings.TrimSpace(v) != "" {
				hasNewSecret = true
				break
			}
		}
	}

	baseURL := ""
	if cred.Data != nil {
		if v, _ := cred.Data["base_url"].(string); v != "" {
			baseURL = v
		}
	}

	// 没提新密钥：保留旧的 __sealed 和 base_url（如有）
	if !hasNewSecret {
		if old, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantUUID, cred.Name, cred.Provider); err == nil && old != nil {
			if cred.Data == nil {
				cred.Data = datatypes.JSONMap{}
			}
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

	// 加密敏感键
	scopeKey := s.tenantScopeKey(tenantUUID)
	enc, err := s.tks.SealSensitive(ctx, env, scopeKey, cred.Data, sensKeys...)
	if err != nil {
		return err
	}
	cred.Data = enc

	return s.credRepo.UpsertByScopeNameProvider(ctx, env, tenantUUID, cred)
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
		logger.WarnF(ctx, "[agent_setting] credential lookup failed env=%s tenant=%s provider=%s name=%s err=%v", env, s.tenantScopeKey(tenantUUID), provider, name, err)
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
		// 兼容：历史记录可能未加密 api_key（明文存放）
		if v, ok := cred.Data["api_key"].(string); ok && strings.TrimSpace(v) != "" {
			apiKey = strings.TrimSpace(v)
		} else if v, ok := cred.Data["apiKey"].(string); ok && strings.TrimSpace(v) != "" {
			apiKey = strings.TrimSpace(v)
		}

		sec := map[string]any{}
		if e := s.tks.UnsealSensitive(ctx, env, s.tenantScopeKey(tenantUUID), cred.Data, &sec); e == nil {
			if apiKey == "" {
				if v, ok := sec["api_key"].(string); ok && strings.TrimSpace(v) != "" {
					apiKey = strings.TrimSpace(v)
				} else if v, ok := sec["access_token"].(string); ok && strings.TrimSpace(v) != "" {
					apiKey = strings.TrimSpace(v)
				} else if v, ok := sec["secret"].(string); ok && strings.TrimSpace(v) != "" {
					apiKey = strings.TrimSpace(v)
				}
			}
			if apiKey == "" {
				keys := make([]string, 0, len(sec))
				for k := range sec {
					keys = append(keys, k)
				}
				logger.WarnF(ctx, "[agent_setting] resolved empty api_key after unseal env=%s tenant=%s provider=%s sealed_keys=%v", env, s.tenantScopeKey(tenantUUID), provider, keys)
			}
		} else if cred.Data != nil && cred.Data["__sealed"] != nil {
			logger.WarnF(ctx, "[agent_setting] unseal api_key failed env=%s tenant=%s provider=%s err=%v", env, s.tenantScopeKey(tenantUUID), provider, e)
		} else {
			logger.WarnF(ctx, "[agent_setting] credential missing __sealed env=%s tenant=%s provider=%s", env, s.tenantScopeKey(tenantUUID), provider)
		}
	}
	return baseURL, apiKey, nil
}

// ResolveConnFromStore 用于运行时读取已保存的 credential（含解密）来补全 base_url/api_key。
// 注意：该方法仅在后端内部使用，严禁把返回的 apiKey 透传给前端。
func (s *AgentSettingService) ResolveConnFromStore(
	ctx context.Context, env string, tenantUUID *string, provider string,
	baseURLIn, apiKeyIn string,
) (baseURL, apiKey string, err error) {
	return s.resolveConnFromStore(ctx, env, tenantUUID, provider, baseURLIn, apiKeyIn)
}

// 只用“这次提交的表单值”直连一次（不读库、不回退、不解封）
func (s *AgentSettingService) PingStrict(ctx context.Context, modality, provider, model, baseURL, apiKey, secretID, secretKey, region string) error {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("provider/model 不能为空")
	}
	rule := s.resolveModelRule(modality, provider, model)

	// 应用规则
	if rule.RequireAPIKey && strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("%s/%s 需要 apiKey", provider, model)
	}
	if rule.RequireSecretID && strings.TrimSpace(secretID) == "" {
		return fmt.Errorf("%s/%s 需要 secretId", provider, model)
	}
	if rule.RequireSecretKey && strings.TrimSpace(secretKey) == "" {
		return fmt.Errorf("%s/%s 需要 secretKey", provider, model)
	}
	if strings.TrimSpace(baseURL) == "" {
		if rule.DefaultBaseURL != "" {
			baseURL = rule.DefaultBaseURL
		} else if rule.RequireBaseURL {
			return fmt.Errorf("%s/%s 需要 baseURL", provider, model)
		}
	}
	if err := validateEndpoint(baseURL); err != nil {
		return err
	}

	mc := agentcfg.ModelConfig{
		Provider: provider, Endpoint: baseURL, APIKey: apiKey,
		SecretID: secretID, SecretKey: secretKey, Region: region,
		Model: model, SystemPrompt: "You are a health check probe.",
		Temperature: 0, MaxTokens: 8, AccessToken: apiKey,
		Extra: s.buildModelExtras(contract.Modality(strings.ToLower(strings.TrimSpace(modality))), provider, model),
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return err
	}
	c2, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = cli.Invoke(c2, &mc, "ping")
	if err == nil {
		return nil
	}
	msg := err.Error()
	// 常见：请求到了 HTML 页面（如 404/反爬/网关错误页），导致 JSON 解析报 '<'
	if strings.Contains(msg, "invalid character '<' looking for beginning of value") {
		return fmt.Errorf("上游返回非 JSON（疑似 HTML 错误页）；请检查 base_url 是否正确，以及是否被网关/反爬拦截：%w", err)
	}
	if strings.EqualFold(strings.TrimSpace(provider), "openrouter") &&
		strings.Contains(msg, "is not a valid model ID") {
		return fmt.Errorf("OpenRouter 模型 ID 无效（可能已更名或你账号无权限）：%s；建议在“模型”下拉刷新后重选，或到 OpenRouter 控制台确认可用模型 ID：%w", model, err)
	}
	return err
}

// TestConnectionPreferInput：测试接口用
// 逻辑：如果提交里有 baseURL 或 apiKey（任一非空），就用提交参数“严格直连”；
// 否则从库里读取已保存配置（含密钥解封）后再 ping。
func (s *AgentSettingService) TestConnectionPreferInput(
	ctx context.Context, env string, tenantUUID *string,
	modality, provider, model, baseURL, apiKey, secretID, secretKey, region, authMode string,
) error {

	mod := strings.ToLower(strings.TrimSpace(modality))
	prov := strings.TrimSpace(provider)

	switch contract.Modality(mod) {
	case contract.ModLLM:
		if strings.EqualFold(strings.TrimSpace(prov), "hunyuan") {
			return s.PingHunyuan(ctx, env, tenantUUID, authMode, model, baseURL, apiKey, secretID, secretKey, region)
		}

		// 其他 provider：要求具备可用 driver
		if _, err := agentllm.NewClient(prov); err != nil {
			return fmt.Errorf("Provider %s 暂未实现 LLM 直连驱动，无法测试连接", prov)
		}
		req := catalog.AuthReqFromCatalog(prov)
		bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, prov, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
		if err != nil {
			return err
		}
		return s.PingStrict(ctx, mod, prov, model, bu, ak, "", "", "")

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
	provider, model, baseURL, apiKey, secretID, secretKey, region, authMode string,
) error {
	if err := ensureModelExists(string(contract.ModLLM), provider, model); err != nil {
		return err
	}
	p := strings.TrimSpace(strings.ToLower(provider))
	if p == "hunyuan" {
		return s.PingHunyuan(ctx, env, tenantUUID, authMode, model, baseURL, apiKey, secretID, secretKey, region)
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

// ProbeEmbeddingDimensionsPreferInput performs a real embedding call to discover vector dimensions, and writes it back to the profile.
// NOTE: This is used by AI Settings "测试连接"，不做任何向量表创建，仅探测 provider/model 的 embedding 输出。
func (s *AgentSettingService) ProbeEmbeddingDimensionsPreferInput(
	ctx context.Context,
	env string,
	tenantUUID *string,
	provider string,
	model string,
	baseURL string,
	apiKey string,
) (int, error) {
	if err := ensureModelExists(string(contract.ModEmbed), provider, model); err != nil {
		return 0, err
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return 0, fmt.Errorf("provider/model 不能为空")
	}

	req := catalog.AuthReqFromCatalog(p)
	if strings.TrimSpace(baseURL) == "" {
		if v := catalog.DefaultBaseURLForModel(p, m); strings.TrimSpace(v) != "" {
			baseURL = v
		}
	}
	bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, p, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return 0, err
	}
	if err := validateEndpoint(bu); err != nil {
		return 0, err
	}

	// driver mapping: provider may declare a different embedding driver (OpenAI-compatible etc.)
	driverKey := p
	if man, ok := catalog.GetGlobalAIRegister().Manifest(p); ok && man != nil {
		if dk := strings.ToLower(strings.TrimSpace(man.Drivers["embedding"])); dk != "" {
			driverKey = dk
		}
	}

	embCfg := agentconf.EmbeddingConfig{
		Enabled:  true,
		Provider: driverKey,
		Endpoint: bu,
		Model:    m,
		APIKey:   ak,
		MaxBatch: 8,
		Dim:      0,
	}
	vec, err := intentfactory.NewVectorizerFromConfig(embCfg)
	if err != nil {
		return 0, err
	}
	if vec == nil {
		return 0, fmt.Errorf("embedding vectorizer unavailable (provider=%s model=%s)", p, m)
	}
	out, err := vec.Embed(ctx, []string{"powerx-dim-probe"})
	if err != nil {
		return 0, err
	}
	if len(out) == 0 || len(out[0]) == 0 {
		return 0, fmt.Errorf("embedding probe returned empty vector")
	}
	dim := len(out[0])
	maxInputTokens := probeEmbeddingMaxInputTokens(ctx, p, bu, m)
	if maxInputTokens > 0 {
		logger.InfoF(ctx, "[agent_setting] embedding max_input_tokens probed provider=%s model=%s tokens=%d", p, m, maxInputTokens)
	} else {
		logger.InfoF(ctx, "[agent_setting] embedding max_input_tokens probe empty provider=%s model=%s base=%s", p, m, bu)
	}

	// write back to model profile (defaults + cap_cache)
	profile := &dbmodel.AIModelProfile{
		Modality: "embedding",
		Provider: p,
		Model:    m,
		Label:    "probe.embedding",
		Defaults: datatypes.JSONMap{
			"dimensions": dim,
		},
		CapCache: datatypes.JSONMap{
			"dimensions": dim,
			"probed_at":  time.Now().UTC().Format(time.RFC3339Nano),
		},
		Tags: []string{"embedding", "probed"},
	}
	if maxInputTokens > 0 {
		profile.Defaults["max_input_tokens"] = maxInputTokens
		profile.CapCache["max_input_tokens"] = maxInputTokens
	}
	// keep existing defaults/cap_cache best-effort (no hard dependency)
	if exist, e := s.profRepo.FindByScopeModalityProviderModel(ctx, env, tenantUUID, "embedding", p, m); e == nil && exist != nil {
		if exist.Defaults != nil {
			for k, v := range exist.Defaults {
				if _, ok := profile.Defaults[k]; ok {
					continue
				}
				profile.Defaults[k] = v
			}
		}
		if exist.CapCache != nil {
			for k, v := range exist.CapCache {
				profile.CapCache[k] = v
			}
			profile.CapCache["dimensions"] = dim
			profile.CapCache["probed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			if maxInputTokens > 0 {
				profile.CapCache["max_input_tokens"] = maxInputTokens
			}
		}
		if strings.TrimSpace(exist.Label) != "" {
			profile.Label = exist.Label
		}
		if len(exist.Tags) > 0 {
			profile.Tags = exist.Tags
		}
	}
	if maxInputTokens <= 0 {
		delete(profile.Defaults, "max_input_tokens")
		delete(profile.CapCache, "max_input_tokens")
	}
	_ = s.profRepo.UpsertByScopeModalityProviderModel(ctx, env, tenantUUID, profile)

	return dim, nil
}

// BuildEmbeddingConfig resolves tenant embedding connection info and returns a ready config.
func (s *AgentSettingService) BuildEmbeddingConfig(
	ctx context.Context,
	env string,
	tenantUUID *string,
	provider string,
	model string,
	baseURL string,
	apiKey string,
) (agentconf.EmbeddingConfig, error) {
	if err := ensureModelExists(string(contract.ModEmbed), provider, model); err != nil {
		return agentconf.EmbeddingConfig{}, err
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return agentconf.EmbeddingConfig{}, fmt.Errorf("provider/model 不能为空")
	}
	req := catalog.AuthReqFromCatalog(p)
	if strings.TrimSpace(baseURL) == "" {
		if v := catalog.DefaultBaseURLForModel(p, m); strings.TrimSpace(v) != "" {
			baseURL = v
		}
	}
	bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, p, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return agentconf.EmbeddingConfig{}, err
	}
	if err := validateEndpoint(bu); err != nil {
		return agentconf.EmbeddingConfig{}, err
	}

	driverKey := p
	if man, ok := catalog.GetGlobalAIRegister().Manifest(p); ok && man != nil {
		if dk := strings.ToLower(strings.TrimSpace(man.Drivers["embedding"])); dk != "" {
			driverKey = dk
		}
	}

	return agentconf.EmbeddingConfig{
		Enabled:  true,
		Provider: driverKey,
		Endpoint: bu,
		Model:    m,
		APIKey:   ak,
		MaxBatch: 8,
		Dim:      0,
	}, nil
}

func probeEmbeddingMaxInputTokens(ctx context.Context, provider, baseURL, model string) int {
	if strings.ToLower(strings.TrimSpace(provider)) != "ollama" {
		return 0
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	if base == "" || strings.TrimSpace(model) == "" {
		return 0
	}
	payload := map[string]string{"name": strings.TrimSpace(model)}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/show", bytes.NewReader(body))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.WarnF(ctx, "[agent_setting] ollama show failed provider=%s model=%s base=%s err=%v", provider, model, base, err)
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.WarnF(ctx, "[agent_setting] ollama show non-2xx provider=%s model=%s base=%s status=%d", provider, model, base, resp.StatusCode)
		return 0
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil || len(raw) == 0 {
		logger.WarnF(ctx, "[agent_setting] ollama show empty body provider=%s model=%s base=%s", provider, model, base)
		return 0
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		logger.WarnF(ctx, "[agent_setting] ollama show invalid json provider=%s model=%s base=%s err=%v", provider, model, base, err)
		return 0
	}
	logger.InfoF(ctx, "[agent_setting] ollama show keys provider=%s model=%s keys=%v", provider, model, mapKeys(data))
	if params, ok := data["parameters"].(map[string]any); ok {
		if v := parseAnyInt(params["num_ctx"]); v > 0 {
			return v
		}
		if v := parseAnyInt(params["context_length"]); v > 0 {
			return v
		}
		if v := parseAnyInt(params["context_len"]); v > 0 {
			return v
		}
		if v := parseAnyInt(params["context"]); v > 0 {
			return v
		}
	}
	if params, ok := data["parameters"].(string); ok && strings.TrimSpace(params) != "" {
		re := regexp.MustCompile(`(?mi)\b(num_ctx|context_length|context_len|context\s+length)\s*[:=]?\s*(\d+)\b`)
		if m := re.FindStringSubmatch(params); len(m) > 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(m[2])); err == nil && n > 0 {
				return n
			}
		}
	}
	if v := parseAnyInt(data["context_length"]); v > 0 {
		return v
	}
	if v := parseAnyInt(data["context_len"]); v > 0 {
		return v
	}
	if v := parseAnyInt(data["context"]); v > 0 {
		return v
	}
	if info, ok := data["model_info"]; ok {
		if v := extractOllamaContext(info); v > 0 {
			return v
		}
	}
	if details, ok := data["details"]; ok {
		if v := extractOllamaContext(details); v > 0 {
			return v
		}
	}
	if modelInfo, ok := data["model"]; ok {
		if v := extractOllamaContext(modelInfo); v > 0 {
			return v
		}
	}
	if v := parseAnyInt(data["num_ctx"]); v > 0 {
		return v
	}
	if mf, ok := data["modelfile"].(string); ok && strings.TrimSpace(mf) != "" {
		re := regexp.MustCompile(`(?mi)^\s*PARAMETER\s+num_ctx\s+(\d+)\s*$`)
		if m := re.FindStringSubmatch(mf); len(m) > 1 {
			if n, err := strconv.Atoi(strings.TrimSpace(m[1])); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

func extractOllamaContext(raw any) int {
	switch info := raw.(type) {
	case map[string]any:
		if v := parseAnyInt(info["num_ctx"]); v > 0 {
			return v
		}
		if v := parseAnyInt(info["context_length"]); v > 0 {
			return v
		}
		if v := parseAnyInt(info["context_len"]); v > 0 {
			return v
		}
		if v := parseAnyInt(info["context"]); v > 0 {
			return v
		}
		// Ollama 的 model_info/details 常见 key: "bert.context_length" / "llama.context_length"
		for k, v := range info {
			lk := strings.ToLower(strings.TrimSpace(k))
			if strings.Contains(lk, "context_length") || strings.HasSuffix(lk, ".context_length") || strings.HasSuffix(lk, "context_length") {
				if n := parseAnyInt(v); n > 0 {
					return n
				}
			}
			if strings.Contains(lk, "num_ctx") || strings.HasSuffix(lk, ".num_ctx") {
				if n := parseAnyInt(v); n > 0 {
					return n
				}
			}
		}
	case string:
		if v := parseContextFromString(info); v > 0 {
			return v
		}
	}
	return 0
}

func parseContextFromString(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	re := regexp.MustCompile(`(?mi)\b(num_ctx|context_length|context_len|context\s+length)\s*[:=]?\s*(\d+)\b`)
	if m := re.FindStringSubmatch(s); len(m) > 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(m[2])); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func parseAnyInt(v any) int {
	switch val := v.(type) {
	case int:
		if val > 0 {
			return val
		}
	case int64:
		if val > 0 {
			return int(val)
		}
	case float64:
		if int(val) > 0 {
			return int(val)
		}
	case float32:
		if int(val) > 0 {
			return int(val)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && parsed > 0 {
			return parsed
		}
	case json.Number:
		if parsed, err := val.Int64(); err == nil && parsed > 0 {
			return int(parsed)
		}
		if parsed, err := strconv.Atoi(strings.TrimSpace(val.String())); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

// 修改 QuickCallLLM：同样带 env/tenantUUID + 回退解密
func (s *AgentSettingService) QuickCallLLM(
	ctx context.Context, env string, tenantUUID *string,
	provider, model, baseURL, apiKey, secretID, secretKey, region, authMode string,
	temperature float64, maxTokens int,
	prompt string,
) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Say hello in one short sentence."
	}
	p := strings.TrimSpace(strings.ToLower(provider))
	if p == "hunyuan" {
		mode := authMode
		if strings.TrimSpace(mode) == "" {
			mode = "openai"
		}
		if strings.EqualFold(strings.TrimSpace(mode), "tc3") {
			bu, sid, sk, rg, err := s.prepareHunyuanTC3Inputs(ctx, env, tenantUUID, baseURL, secretID, secretKey, region)
			if err != nil {
				return "", err
			}
			mc := agentcfg.ModelConfig{
				Provider:     "hunyuan",
				Endpoint:     bu,
				SecretID:     sid,
				SecretKey:    sk,
				Region:       rg,
				Model:        model,
				SystemPrompt: "You are a helpful assistant.",
				Temperature:  temperature,
				MaxTokens:    utils.MaxInt(maxTokens, 64),
				Extra:        s.buildModelExtras(contract.ModLLM, "hunyuan", model),
			}
			cli, err := agentllm.NewClient("hunyuan")
			if err != nil {
				return "", err
			}
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			return cli.Invoke(ctx, &mc, prompt)
		}
		// OpenAI SDK 兼容：走 openai client
		bu, ak, err := s.prepareHunyuanOpenAIInputs(ctx, env, tenantUUID, baseURL, apiKey)
		if err != nil {
			return "", err
		}
		mc := agentcfg.ModelConfig{
			Provider:     "openai",
			Endpoint:     bu,
			APIKey:       ak,
			Model:        model,
			SystemPrompt: "You are a helpful assistant.",
			Temperature:  temperature,
			MaxTokens:    utils.MaxInt(maxTokens, 64),
			AccessToken:  ak,
			Extra:        s.buildModelExtras(contract.ModLLM, "hunyuan", model),
		}
		cli, err := agentllm.NewClient("openai")
		if err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return cli.Invoke(ctx, &mc, prompt)
	}

	// 回退解密（OpenAI-compatible 等）
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

func (s *AgentSettingService) resolveHunyuanFromStore(
	ctx context.Context, env string, tenantUUID *string,
) (baseURL string, secretID string, secretKey string, region string, err error) {
	name := utils.Slug(env + "-hunyuan")
	cred, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantUUID, name, "hunyuan")
	if err != nil {
		return "", "", "", "", err
	}
	if v, ok := cred.Data["base_url"].(string); ok {
		baseURL = strings.TrimSpace(v)
	}
	if v, ok := cred.Data["region"].(string); ok {
		region = strings.TrimSpace(v)
	}

	// 兼容：历史记录可能未加密 secret_id/secret_key（明文存放）
	if v, ok := cred.Data["secret_id"].(string); ok && strings.TrimSpace(v) != "" {
		secretID = strings.TrimSpace(v)
	} else if v, ok := cred.Data["secretId"].(string); ok && strings.TrimSpace(v) != "" {
		secretID = strings.TrimSpace(v)
	}
	if v, ok := cred.Data["secret_key"].(string); ok && strings.TrimSpace(v) != "" {
		secretKey = strings.TrimSpace(v)
	} else if v, ok := cred.Data["secretKey"].(string); ok && strings.TrimSpace(v) != "" {
		secretKey = strings.TrimSpace(v)
	}

	var sec struct {
		SecretID  string `json:"secret_id"`
		SecretKey string `json:"secret_key"`
	}
	if e := s.tks.UnsealSensitive(ctx, env, s.tenantScopeKey(tenantUUID), cred.Data, &sec); e == nil {
		if secretID == "" {
			secretID = strings.TrimSpace(sec.SecretID)
		}
		if secretKey == "" {
			secretKey = strings.TrimSpace(sec.SecretKey)
		}
	}
	return baseURL, secretID, secretKey, region, nil
}

func (s *AgentSettingService) hunyuanModeDefaults(mode string) (defaultBaseURL string, defaultRegion string) {
	reg := catalog.GetGlobalAIRegister()
	m, ok := reg.Manifest("hunyuan")
	if !ok || m == nil {
		return "", ""
	}
	target := strings.ToLower(strings.TrimSpace(mode))
	for _, md := range m.Auth.Modes {
		if strings.ToLower(strings.TrimSpace(md.ID)) == target {
			if md.Defaults != nil {
				return strings.TrimSpace(md.Defaults["base_url"]), strings.TrimSpace(md.Defaults["region"])
			}
			return "", ""
		}
	}
	return "", ""
}

func (s *AgentSettingService) prepareHunyuanTC3Inputs(
	ctx context.Context, env string, tenantUUID *string,
	baseURLIn string, secretIDIn string, secretKeyIn string, regionIn string,
) (baseURL string, secretID string, secretKey string, region string, err error) {
	baseURL = strings.TrimSpace(baseURLIn)
	secretID = strings.TrimSpace(secretIDIn)
	secretKey = strings.TrimSpace(secretKeyIn)
	region = strings.TrimSpace(regionIn)

	defBase, defRegion := s.hunyuanModeDefaults("tc3")
	// 兼容：用户在 UI 从 OpenAI 模式切到 tc3 时，可能仍保留 openai 的 base_url（带 /v1 或 hunyuan.cloud.tencent.com）。
	// tc3 模式必须使用腾讯云 API endpoint（hunyuan.tencentcloudapi.com 风格），否则会出现 404/缺公共参数等问题。
	if strings.TrimSpace(baseURL) == "" {
		baseURL = strings.TrimSpace(defBase)
	} else {
		buIn := strings.TrimSpace(baseURL)
		if u, e := url.Parse(buIn); e == nil {
			host := strings.ToLower(strings.TrimSpace(u.Host))
			// openai 兼容域名 → 强制切回 tc3 默认 endpoint
			if strings.Contains(host, "hunyuan.cloud.tencent.com") {
				baseURL = strings.TrimSpace(defBase)
			} else if u.Path != "" && u.Path != "/" {
				// tc3 endpoint 不需要 path，避免把 /v1 之类带进去
				baseURL = strings.TrimRight(fmt.Sprintf("%s://%s", u.Scheme, u.Host), "/")
			}
		}
	}

	// fallback from store
	if baseURL == "" || secretID == "" || secretKey == "" || region == "" {
		if bu, sid, sk, rg, e := s.resolveHunyuanFromStore(ctx, env, tenantUUID); e == nil {
			if baseURL == "" {
				baseURL = strings.TrimSpace(bu)
			}
			if secretID == "" {
				secretID = strings.TrimSpace(sid)
			}
			if secretKey == "" {
				secretKey = strings.TrimSpace(sk)
			}
			if region == "" {
				region = strings.TrimSpace(rg)
			}
		}
	}

	// apply defaults & validate
	if baseURL == "" {
		baseURL = strings.TrimSpace(defBase)
	}
	if baseURL == "" {
		return "", "", "", "", fmt.Errorf("缺少 BaseURL（hunyuan 要求 base_url）")
	}
	if err := validateEndpoint(baseURL); err != nil {
		return "", "", "", "", err
	}
	if secretID == "" {
		return "", "", "", "", fmt.Errorf("缺少 SecretID（hunyuan 要求 secret_id）")
	}
	if secretKey == "" {
		return "", "", "", "", fmt.Errorf("缺少 SecretKey（hunyuan 要求 secret_key）")
	}
	if region == "" {
		region = defRegion
	}
	if region == "" {
		region = "ap-guangzhou"
	}
	return baseURL, secretID, secretKey, region, nil
}

func (s *AgentSettingService) prepareHunyuanOpenAIInputs(
	ctx context.Context,
	env string,
	tenantUUID *string,
	baseURLIn string,
	apiKeyIn string,
) (baseURL string, apiKey string, err error) {
	defBase, _ := s.hunyuanModeDefaults("openai")
	// OpenAI SDK 兼容：要求 api_key + base_url
	// 兼容：用户可能误把 tc3 endpoint（*.tencentcloudapi.com）填进来；此时强制回退到 openai 默认 base_url
	buIn := strings.TrimSpace(baseURLIn)
	// 关键兜底：如果用户不填 base_url，必须使用 OpenAI 模式默认值；
	// 不能从已保存的 tc3 凭据里回填（否则会打到 hunyuan.tencentcloudapi.com 并要求 X-TC-Version）。
	if buIn == "" {
		buIn = strings.TrimSpace(defBase)
	}
	if buIn != "" {
		if u, e := url.Parse(buIn); e == nil {
			host := strings.ToLower(u.Host)
			if strings.Contains(host, "tencentcloudapi.com") {
				buIn = strings.TrimSpace(defBase)
			} else if u.Path == "" || u.Path == "/" {
				// 常见：只填 host，忘了 /v1
				buIn = strings.TrimRight(buIn, "/") + "/v1"
			}
		} else {
			// 非法 URL：让后续 validateEndpoint 报错更明确
		}
	}

	bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, "hunyuan", buIn, apiKeyIn, true, defBase, true)
	if err != nil && strings.Contains(err.Error(), "缺少 BaseURL") {
		return "", "", fmt.Errorf("缺少 BaseURL（混元 OpenAI SDK/API Key 模式需要 base_url；请到腾讯云大模型 API 控制台的“OpenAI SDK 方式接入”复制 Base URL）")
	}
	return bu, ak, err
}

func (s *AgentSettingService) PingHunyuan(
	ctx context.Context,
	env string,
	tenantUUID *string,
	authMode string,
	model string,
	baseURL string,
	apiKey string,
	secretID string,
	secretKey string,
	region string,
) error {
	mode := strings.ToLower(strings.TrimSpace(authMode))
	if mode == "" {
		mode = "openai" // 默认优先 API Key（OpenAI SDK 兼容）
	}
	switch mode {
	case "tc3":
		bu, sid, sk, rg, err := s.prepareHunyuanTC3Inputs(ctx, env, tenantUUID, baseURL, secretID, secretKey, region)
		if err != nil {
			return err
		}
		mc := agentcfg.ModelConfig{
			Provider:     "hunyuan",
			Endpoint:     bu,
			SecretID:     sid,
			SecretKey:    sk,
			Region:       rg,
			Model:        model,
			SystemPrompt: "You are a health check probe.",
			Temperature:  0,
			MaxTokens:    8,
			Extra:        s.buildModelExtras(contract.ModLLM, "hunyuan", model),
		}
		cli, err := agentllm.NewClient("hunyuan")
		if err != nil {
			return err
		}
		c2, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		_, err = cli.Invoke(c2, &mc, "ping")
		return err
	default:
		bu, ak, err := s.prepareHunyuanOpenAIInputs(ctx, env, tenantUUID, baseURL, apiKey)
		if err != nil {
			return err
		}
		if err := validateEndpoint(bu); err != nil {
			return err
		}
		mc := agentcfg.ModelConfig{
			Provider:     "openai",
			Endpoint:     bu,
			APIKey:       ak,
			Model:        model,
			SystemPrompt: "You are a health check probe.",
			Temperature:  0,
			MaxTokens:    8,
			AccessToken:  ak,
			Extra:        s.buildModelExtras(contract.ModLLM, "hunyuan", model),
		}
		cli, err := agentllm.NewClient("openai")
		if err != nil {
			return err
		}
		c2, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		_, err = cli.Invoke(c2, &mc, "ping")
		return err
	}
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
	m := strings.TrimSpace(strings.ToLower(mod))
	reg := catalog.GetGlobalAIRegister()

	// ✅ 对齐：图像/视频两套 Provider 列表保持一致：image ∪ video
	var items []catalog.ProviderItem
	if m == "video" || m == "image" {
		seen := map[string]struct{}{}
		add := func(list []catalog.ProviderItem) {
			for _, it := range list {
				if it.ID == "" {
					continue
				}
				if _, ok := seen[it.ID]; ok {
					continue
				}
				seen[it.ID] = struct{}{}
				items = append(items, it)
			}
		}
		add(reg.Providers("image"))
		add(reg.Providers("video"))
	} else {
		items = reg.Providers(m)
	}
	out := make([]aiProviderItem, 0, len(items))
	for _, it := range items {
		out = append(out, aiProviderItem{ID: it.ID, Name: it.Name})
	}
	return out
}
func catalogGetModels(mod, prov string) ([]aiModelItem, error) {
	m := strings.TrimSpace(strings.ToLower(mod))
	reg := catalog.GetGlobalAIRegister()

	// ✅ 对齐：图像/视频如果该模态没模型，则回退到另一模态（避免下拉为空）
	ms, err := reg.Models(m, prov)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		if m == "video" {
			if fallback, e := reg.Models("image", prov); e == nil && len(fallback) > 0 {
				ms = fallback
			}
		}
		if m == "image" {
			if fallback, e := reg.Models("video", prov); e == nil && len(fallback) > 0 {
				ms = fallback
			}
		}
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
		// 最后兜底：读取 config.yaml 的 ai.defaults（仅作为“系统默认值”，不代表已完成凭据配置）
		cfg := agentconf.GetGlobalAIConfig()
		if cfg == nil {
			return nil, err
		}
		mod := strings.TrimSpace(strings.ToLower(modality))
		switch mod {
		case "llm":
			p := strings.TrimSpace(cfg.Defaults.LLM.Provider)
			m := strings.TrimSpace(cfg.Defaults.LLM.Model)
			if p == "" || m == "" {
				return nil, err
			}
			return &dbmodel.AIModelProfile{
				Modality: "llm",
				Provider: p,
				Model:    m,
				Label:    "config.default.llm",
				Defaults: datatypes.JSONMap{
					"temperature": cfg.Defaults.LLM.Temperature,
					"maxTokens":   cfg.Defaults.LLM.MaxTokens,
					"topP":        cfg.Defaults.LLM.TopP,
					"stream":      cfg.Defaults.LLM.Stream,
				},
				Tags: []string{"llm", "config_default"},
			}, nil
		case "image":
			p := strings.TrimSpace(cfg.Defaults.Image.Provider)
			m := strings.TrimSpace(cfg.Defaults.Image.Model)
			if p == "" || m == "" {
				return nil, err
			}
			return &dbmodel.AIModelProfile{
				Modality: "image",
				Provider: p,
				Model:    m,
				Label:    "config.default.image",
				Defaults: datatypes.JSONMap{
					"size":       cfg.Defaults.Image.Size,
					"quality":    cfg.Defaults.Image.Quality,
					"format":     cfg.Defaults.Image.Format,
					"promptHint": cfg.Defaults.Image.PromptHint,
				},
				Tags: []string{"image", "config_default"},
			}, nil
		case "embedding":
			p := strings.TrimSpace(cfg.Defaults.Embedding.Provider)
			m := strings.TrimSpace(cfg.Defaults.Embedding.Model)
			if p == "" || m == "" {
				return nil, err
			}
			return &dbmodel.AIModelProfile{
				Modality: "embedding",
				Provider: p,
				Model:    m,
				Label:    "config.default.embedding",
				Defaults: datatypes.JSONMap{
					"dimensions": cfg.Defaults.Embedding.Dimensions,
					"truncate":   cfg.Defaults.Embedding.Truncate,
					"batch":      cfg.Defaults.Embedding.Batch,
				},
				Tags: []string{"embedding", "config_default"},
			}, nil
		case "video":
			p := strings.TrimSpace(cfg.Defaults.Video.Provider)
			m := strings.TrimSpace(cfg.Defaults.Video.Model)
			if p == "" || m == "" {
				return nil, err
			}
			return &dbmodel.AIModelProfile{
				Modality: "video",
				Provider: p,
				Model:    m,
				Label:    "config.default.video",
				Defaults: datatypes.JSONMap{
					"resolution":     cfg.Defaults.Video.Resolution,
					"fps":            cfg.Defaults.Video.FPS,
					"maxDurationSec": cfg.Defaults.Video.MaxDurationSec,
					"promptHint":     cfg.Defaults.Video.PromptHint,
				},
				Tags: []string{"video", "config_default"},
			}, nil
		default:
			return nil, err
		}
	}
	latest := list[0]
	for i := 1; i < len(list); i++ {
		if list[i].UpdatedAt.After(latest.UpdatedAt) {
			latest = list[i]
		}
	}
	return &latest, nil
}

// GetProfile returns the profile row for a specific (env, scope, modality, provider, model).
func (s *AgentSettingService) GetProfile(
	ctx context.Context, env string, tenantUUID *string, modality, provider, model string,
) (*dbmodel.AIModelProfile, error) {
	return s.profRepo.FindByScopeModalityProviderModel(ctx, env, tenantUUID, modality, provider, model)
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
		reqSID := false
		reqSKey := false
		reqBase := false
		for _, f := range m.Auth.Fields {
			lf := strings.ToLower(strings.TrimSpace(f))
			if lf == "api_key" {
				reqAPI = true
			}
			if lf == "secret_id" {
				reqSID = true
			}
			if lf == "secret_key" {
				reqSKey = true
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
		return ModelRule{
			RequireAPIKey:    reqAPI,
			RequireSecretID:  reqSID,
			RequireSecretKey: reqSKey,
			RequireBaseURL:   reqBase,
			DefaultBaseURL:   def,
		}
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
