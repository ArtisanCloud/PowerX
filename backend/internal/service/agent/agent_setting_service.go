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
	intentfactory "github.com/ArtisanCloud/PowerX/internal/server/agent/factory/intent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	imagefactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/image"
	agentllm "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	vlmfactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/vlm"
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

func isSecretPlaceholder(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	runes := []rune(trimmed)
	if len(runes) < 6 {
		return false
	}
	for _, r := range runes {
		if r != '*' && r != '•' {
			return false
		}
	}
	return true
}

func normalizeCredentialSecretInput(data datatypes.JSONMap, keys ...string) (hasNewSecret bool, placeholderKeys []string) {
	if data == nil {
		return false, nil
	}
	for _, key := range keys {
		raw, ok := data[key].(string)
		if !ok {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			data[key] = ""
			continue
		}
		if isSecretPlaceholder(value) {
			delete(data, key)
			placeholderKeys = append(placeholderKeys, key)
			continue
		}
		data[key] = value
		hasNewSecret = true
	}
	return hasNewSecret, placeholderKeys
}

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
		out = append(out, aiProviderItem{
			ID:   it.ID,
			Name: it.Name,
			Apps: it.Apps,
		})
	}
	return out
}
func (s *AgentSettingService) Models(modality, provider, app string) ([]string, error) {
	models, err := catalogGetModels(strings.TrimSpace(modality), strings.TrimSpace(provider), strings.TrimSpace(app))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" || id == "*" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
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
	app string,
) ([]string, error) {
	mod := strings.TrimSpace(strings.ToLower(modality))
	prov := strings.TrimSpace(strings.ToLower(provider))
	app = strings.TrimSpace(strings.ToLower(app))

	// OpenRouter：模型目录变化快，优先走远端 /models；失败则回退到本地目录（占位/示例）。
	if prov == "openrouter" && (mod == "llm" || mod == "embedding") {
		if remote, err := s.fetchOpenRouterModels(ctx, env, tenantUUID, mod); err == nil && len(remote) > 0 {
			return remote, nil
		}
	}
	return s.Models(mod, prov, app)
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
	hasNewSecret, placeholderKeys := normalizeCredentialSecretInput(cred.Data, sensKeys...)

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
		} else if len(placeholderKeys) > 0 {
			return fmt.Errorf("密钥字段为占位符且没有可保留的已保存密钥：%s", strings.Join(placeholderKeys, ","))
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
	hasNewSecret, placeholderKeys := normalizeCredentialSecretInput(cred.Data, sensKeys...)

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
		} else if len(placeholderKeys) > 0 {
			return fmt.Errorf("密钥字段为占位符且没有可保留的已保存密钥：%s", strings.Join(placeholderKeys, ","))
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
	req := catalog.AuthReqFromCatalog(provider)
	logger.InfoF(ctx, "[agent_setting] resolve_conn start env=%s tenant=%s provider=%s name=%s need_key=%t need_base=%t has_base=%t has_api_key=%t", env, s.tenantScopeKey(tenantUUID), provider, name, req.NeedKey, req.NeedBaseURL, strings.TrimSpace(baseURLIn) != "", strings.TrimSpace(apiKeyIn) != "")

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
	// 再补 api_key（仅后端内部使用，不回前端）。只有 provider 明确要求 key 时才解封敏感字段；
	// 例如 Ollama 只需要 base_url，不能因为没有 api_key 阻断运行时。
	if apiKey == "" && req.NeedKey {
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
				return baseURL, apiKey, fmt.Errorf("已保存凭据解封成功但不包含 api_key env=%s tenant=%s provider=%s sealed_keys=%v", env, s.tenantScopeKey(tenantUUID), provider, keys)
			}
		} else if cred.Data != nil && cred.Data["__sealed"] != nil {
			logger.WarnF(ctx, "[agent_setting] unseal api_key failed env=%s tenant=%s provider=%s err=%v", env, s.tenantScopeKey(tenantUUID), provider, e)
			return baseURL, apiKey, fmt.Errorf("解封已保存 api_key 失败 env=%s tenant=%s provider=%s: %w", env, s.tenantScopeKey(tenantUUID), provider, e)
		} else {
			logger.WarnF(ctx, "[agent_setting] credential missing __sealed env=%s tenant=%s provider=%s", env, s.tenantScopeKey(tenantUUID), provider)
			return baseURL, apiKey, fmt.Errorf("已保存凭据缺少 __sealed env=%s tenant=%s provider=%s", env, s.tenantScopeKey(tenantUUID), provider)
		}
	}
	if apiKey == "" && !req.NeedKey {
		logger.InfoF(ctx, "[agent_setting] resolve_conn skip_api_key env=%s tenant=%s provider=%s name=%s reason=provider_does_not_require_key", env, s.tenantScopeKey(tenantUUID), provider, name)
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

	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return err
	}
	_ = cli // 仅做 provider 驱动可用性校验；真实探活统一走 QuickCallLLMResult 链路。

	c2, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = s.QuickCallLLMResult(
		c2, "", nil,
		provider, model, baseURL, apiKey, secretID, secretKey, region, "",
		0, 0,
		"ping",
	)
	return normalizePingLLMError(provider, model, err)
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
		if strings.TrimSpace(prov) == "" {
			return fmt.Errorf("provider 不能为空")
		}
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("model 不能为空")
		}
		// 要求具备可用 driver
		if _, err := agentllm.NewClient(prov); err != nil {
			return fmt.Errorf("Provider %s 暂未实现 LLM 直连驱动，无法测试连接", prov)
		}
		// 与正式调用链对齐：统一走 QuickCallLLMResult。
		prompt := "ping"
		if strings.TrimSpace(baseURL) != "" || strings.TrimSpace(apiKey) != "" || strings.TrimSpace(secretID) != "" || strings.TrimSpace(secretKey) != "" {
			_, err := s.QuickCallLLMResult(
				ctx, env, tenantUUID,
				prov, model, baseURL, apiKey, secretID, secretKey, region, authMode,
				0, 0,
				prompt,
			)
			return normalizePingLLMError(prov, model, err)
		}
		_, err := s.QuickCallLLMResult(
			ctx, env, tenantUUID,
			prov, model, "", "", "", "", "", authMode,
			0, 0,
			prompt,
		)
		return normalizePingLLMError(prov, model, err)

	case contract.ModImage:
		return s.PingImage(ctx, env, tenantUUID, provider, model, baseURL, apiKey, secretID, secretKey, region, "")
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
	_, err := s.QuickCallLLMResult(
		ctx, env, tenantUUID,
		provider, model, baseURL, apiKey, secretID, secretKey, region, authMode,
		0, 0,
		"ping",
	)
	return normalizePingLLMError(provider, model, err)
}

func (s *AgentSettingService) PingImage(
	ctx context.Context, env string, tenantUUID *string,
	provider, model, baseURL, apiKey, secretID, secretKey, region, organization string,
) error {
	logger.InfoF(ctx, "[agent_setting] ping_image start env=%s provider=%s model=%s", env, strings.TrimSpace(provider), strings.TrimSpace(model))
	if err := ensureModelExists(string(contract.ModImage), provider, model); err != nil {
		return err
	}
	p := strings.TrimSpace(strings.ToLower(provider))
	req := catalog.AuthReqFromCatalog(provider)
	var err error
	baseURL, apiKey, err = s.prepareAuthInputs(ctx, env, tenantUUID, p, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return err
	}
	if err := validateEndpoint(baseURL); err != nil {
		return err
	}

	manifest := findModelManifest(string(contract.ModImage), provider, model)
	size := "256x256"
	quality := "auto"
	format := "png"
	promptHint := ""
	if manifest != nil && manifest.Defaults != nil {
		if v, ok := manifest.Defaults["size"].(string); ok && strings.TrimSpace(v) != "" {
			size = strings.TrimSpace(v)
		}
		if v, ok := manifest.Defaults["quality"].(string); ok && strings.TrimSpace(v) != "" {
			quality = strings.TrimSpace(v)
		}
		if v, ok := manifest.Defaults["format"].(string); ok && strings.TrimSpace(v) != "" {
			format = strings.TrimSpace(v)
		}
		if v, ok := manifest.Defaults["promptHint"].(string); ok && strings.TrimSpace(v) != "" {
			promptHint = strings.TrimSpace(v)
		}
	}
	prompt := "A tiny white cube on a blue background."
	if promptHint != "" {
		prompt = strings.TrimSpace(prompt + "\n" + promptHint)
	}

	mc := agentcfg.ModelConfig{
		Provider:     provider,
		Endpoint:     baseURL,
		APIKey:       apiKey,
		SecretID:     secretID,
		SecretKey:    secretKey,
		Region:       region,
		Model:        model,
		Organization: organization,
		Extra:        s.buildModelExtras(contract.ModImage, provider, model),
	}

	driverKey := p
	if man, ok := catalog.GetGlobalAIRegister().Manifest(p); ok && man != nil {
		if dk := strings.ToLower(strings.TrimSpace(man.Drivers["image"])); dk != "" {
			driverKey = dk
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	var invokeErr error
	switch driverKey {
	case "qwen":
		cli, err := vlmfactory.NewClient(driverKey)
		if err != nil {
			return err
		}
		_, invokeErr = cli.Invoke(ctx, contract.VLMRequest{
			Messages: []contract.Message{
				{
					Role: "user",
					Content: []contract.ContentPart{
						{Type: contract.ContentTypeText, Text: "请回复 ok"},
					},
				},
			},
			MaxTokens: 8,
			Runtime:   map[string]any{"config": &mc},
		})
	default:
		cli, err := imagefactory.NewClient(driverKey)
		if err != nil {
			return err
		}
		_, invokeErr = cli.Generate(ctx, contract.ImageRequest{
			Prompt:  prompt,
			Size:    size,
			Quality: quality,
			Format:  format,
			Runtime: map[string]any{"config": &mc},
		})
	}
	if invokeErr == nil {
		logger.InfoF(ctx, "[agent_setting] ping_image success env=%s provider=%s model=%s", env, strings.TrimSpace(provider), strings.TrimSpace(model))
	}
	return invokeErr
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

// BuildImageConfig resolves tenant image connection info and returns a ready config.
func (s *AgentSettingService) BuildImageConfig(
	ctx context.Context,
	env string,
	tenantUUID *string,
	provider string,
	model string,
	baseURL string,
	apiKey string,
	secretID string,
	secretKey string,
	region string,
	organization string,
) (*agentcfg.ModelConfig, error) {
	if err := ensureModelExists(string(contract.ModImage), provider, model); err != nil {
		return nil, err
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return nil, fmt.Errorf("provider/model 不能为空")
	}

	req := catalog.AuthReqFromCatalog(p)
	if strings.TrimSpace(baseURL) == "" {
		if v := catalog.DefaultBaseURLForModel(p, m); strings.TrimSpace(v) != "" {
			baseURL = v
		}
	}
	bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, p, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return nil, err
	}
	if err := validateEndpoint(bu); err != nil {
		return nil, err
	}

	org := strings.TrimSpace(organization)
	rg := strings.TrimSpace(region)
	azureDeployment := ""
	name := utils.Slug(env + "-" + p)
	if cred, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantUUID, name, p); err == nil && cred != nil {
		if org == "" {
			if v, ok := cred.Data["organization"].(string); ok {
				org = strings.TrimSpace(v)
			}
		}
		if rg == "" {
			if v, ok := cred.Data["region"].(string); ok {
				rg = strings.TrimSpace(v)
			}
		}
		if v, ok := cred.Data["azure_deployment"].(string); ok {
			azureDeployment = strings.TrimSpace(v)
		}
	}

	return &agentcfg.ModelConfig{
		Provider:        provider,
		Endpoint:        bu,
		APIKey:          ak,
		SecretID:        strings.TrimSpace(secretID),
		SecretKey:       strings.TrimSpace(secretKey),
		Region:          rg,
		Model:           m,
		Organization:    org,
		AzureDeployment: azureDeployment,
		Extra:           s.buildModelExtras(contract.ModImage, provider, model),
	}, nil
}

// BuildModelConfig resolves tenant provider credentials for any modality and returns runtime model config.
func (s *AgentSettingService) BuildModelConfig(
	ctx context.Context,
	env string,
	tenantUUID *string,
	modality string,
	provider string,
	model string,
	baseURL string,
	apiKey string,
	secretID string,
	secretKey string,
	region string,
	organization string,
) (*agentcfg.ModelConfig, error) {
	mod := strings.ToLower(strings.TrimSpace(modality))
	if mod == "" {
		return nil, fmt.Errorf("modality 不能为空")
	}
	if err := ensureModelExists(mod, provider, model); err != nil {
		return nil, err
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return nil, fmt.Errorf("provider/model 不能为空")
	}

	req := catalog.AuthReqFromCatalog(p)
	if strings.TrimSpace(baseURL) == "" {
		if manifest := findModelManifest(mod, p, m); manifest != nil {
			if v, ok := manifest.Defaults["base_url"].(string); ok && strings.TrimSpace(v) != "" {
				baseURL = strings.TrimSpace(v)
			}
		}
		if strings.TrimSpace(baseURL) == "" {
			if v := catalog.DefaultBaseURLForModel(p, m); strings.TrimSpace(v) != "" {
				baseURL = v
			}
		}
	}
	bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, p, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return nil, err
	}
	if err := validateEndpoint(bu); err != nil {
		return nil, err
	}

	org := strings.TrimSpace(organization)
	rg := strings.TrimSpace(region)
	sid := strings.TrimSpace(secretID)
	sk := strings.TrimSpace(secretKey)
	azureDeployment := ""

	name := utils.Slug(env + "-" + p)
	if cred, err := s.credRepo.FindByScopeNameProvider(ctx, env, tenantUUID, name, p); err == nil && cred != nil {
		if org == "" {
			if v, ok := cred.Data["organization"].(string); ok {
				org = strings.TrimSpace(v)
			}
		}
		if rg == "" {
			if v, ok := cred.Data["region"].(string); ok {
				rg = strings.TrimSpace(v)
			}
		}
		if sid == "" {
			if v, ok := cred.Data["secret_id"].(string); ok && strings.TrimSpace(v) != "" {
				sid = strings.TrimSpace(v)
			} else if v, ok := cred.Data["secretId"].(string); ok && strings.TrimSpace(v) != "" {
				sid = strings.TrimSpace(v)
			}
		}
		if sk == "" {
			if v, ok := cred.Data["secret_key"].(string); ok && strings.TrimSpace(v) != "" {
				sk = strings.TrimSpace(v)
			} else if v, ok := cred.Data["secretKey"].(string); ok && strings.TrimSpace(v) != "" {
				sk = strings.TrimSpace(v)
			}
		}
		if v, ok := cred.Data["azure_deployment"].(string); ok {
			azureDeployment = strings.TrimSpace(v)
		}
		if sid == "" || sk == "" {
			sec := map[string]any{}
			if e := s.tks.UnsealSensitive(ctx, env, s.tenantScopeKey(tenantUUID), cred.Data, &sec); e == nil {
				if sid == "" {
					if v, ok := sec["secret_id"].(string); ok && strings.TrimSpace(v) != "" {
						sid = strings.TrimSpace(v)
					}
				}
				if sk == "" {
					if v, ok := sec["secret_key"].(string); ok && strings.TrimSpace(v) != "" {
						sk = strings.TrimSpace(v)
					}
				}
			}
		}
	}

	return &agentcfg.ModelConfig{
		Provider:        provider,
		Endpoint:        bu,
		APIKey:          ak,
		SecretID:        sid,
		SecretKey:       sk,
		Region:          rg,
		Model:           m,
		Organization:    org,
		AzureDeployment: azureDeployment,
		Extra:           s.buildModelExtras(contract.Modality(mod), provider, model),
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

// QuickCallLLMResult：带 env/tenantUUID + 回退解密，返回文本与元数据。
func (s *AgentSettingService) QuickCallLLMResult(
	ctx context.Context, env string, tenantUUID *string,
	provider, model, baseURL, apiKey, secretID, secretKey, region, authMode string,
	temperature float64, maxTokens int,
	prompt string,
) (*agentcfg.InvokeResult, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Say hello in one short sentence."
	}
	if maxTokens < 0 {
		maxTokens = 0
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
				return nil, err
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
				MaxTokens:    maxTokens,
				Extra:        s.buildModelExtras(contract.ModLLM, "hunyuan", model),
			}
			cli, err := agentllm.NewClient("hunyuan")
			if err != nil {
				return nil, err
			}
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			return cli.Invoke(ctx, &mc, prompt)
		}
		// OpenAI SDK 兼容：走 openai client
		bu, ak, err := s.prepareHunyuanOpenAIInputs(ctx, env, tenantUUID, baseURL, apiKey)
		if err != nil {
			return nil, err
		}
		mc := agentcfg.ModelConfig{
			Provider:     "openai",
			Endpoint:     bu,
			APIKey:       ak,
			Model:        model,
			SystemPrompt: "You are a helpful assistant.",
			Temperature:  temperature,
			MaxTokens:    maxTokens,
			AccessToken:  ak,
			Extra:        s.buildModelExtras(contract.ModLLM, "hunyuan", model),
		}
		cli, err := agentllm.NewClient("openai")
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return cli.Invoke(ctx, &mc, prompt)
	}

	// 通用 LLM：统一走 catalog 鉴权规则与默认 base_url 回退，
	// 避免 openai-compatible provider 在未传 base_url 时误落到 OpenAI 官方默认地址。
	req := catalog.AuthReqFromCatalog(p)
	inputBaseURL := strings.TrimSpace(baseURL)
	modelDefaultBaseURL := ""
	if strings.TrimSpace(baseURL) == "" {
		if v := catalog.DefaultBaseURLForModel(p, model); strings.TrimSpace(v) != "" {
			baseURL = v
			modelDefaultBaseURL = strings.TrimSpace(v)
		}
	}
	// 防止非 openai provider 在 catalog 未命中/无默认地址时误回退到 OpenAI 官方地址。
	if strings.TrimSpace(baseURL) == "" &&
		strings.TrimSpace(req.DefaultBaseURL) == "" &&
		!strings.EqualFold(p, "openai") {
		return nil, fmt.Errorf("缺少 BaseURL（%s 需要 base_url；请在 Setup 中填写 Base URL）", provider)
	}
	bu, ak, err := s.prepareAuthInputs(ctx, env, tenantUUID, p, baseURL, apiKey, req.NeedBaseURL, req.DefaultBaseURL, req.NeedKey)
	if err != nil {
		return nil, err
	}
	if err := validateEndpoint(bu); err != nil {
		return nil, err
	}
	baseSource := "credential_store_or_runtime"
	switch {
	case inputBaseURL != "":
		baseSource = "input"
	case modelDefaultBaseURL != "" && strings.TrimSpace(bu) == modelDefaultBaseURL:
		baseSource = "model_default"
	case strings.TrimSpace(req.DefaultBaseURL) != "" && strings.TrimSpace(bu) == strings.TrimSpace(req.DefaultBaseURL):
		baseSource = "provider_default"
	}
	logger.InfoF(
		ctx,
		"llm endpoint resolved env=%s provider=%s model=%s source=%s base_url=%s",
		strings.TrimSpace(env),
		provider,
		model,
		baseSource,
		bu,
	)

	mc := agentcfg.ModelConfig{
		Provider:     provider,
		Endpoint:     bu,
		APIKey:       ak,
		Model:        model,
		SystemPrompt: "You are a helpful assistant.",
		Temperature:  temperature,
		MaxTokens:    maxTokens,
		AccessToken:  ak,
		Extra:        s.buildModelExtras(contract.ModLLM, provider, model),
	}
	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.Invoke(ctx, &mc, prompt)
}

// QuickCallLLM：兼容旧调用，仅返回文本内容。
func (s *AgentSettingService) QuickCallLLM(
	ctx context.Context, env string, tenantUUID *string,
	provider, model, baseURL, apiKey, secretID, secretKey, region, authMode string,
	temperature float64, maxTokens int,
	prompt string,
) (string, error) {
	result, err := s.QuickCallLLMResult(
		ctx, env, tenantUUID,
		provider, model, baseURL, apiKey, secretID, secretKey, region, authMode,
		temperature, maxTokens, prompt,
	)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	return result.Text, nil
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
		if resolvedBase, resolvedKey, err := s.resolveConnFromStore(ctx, env, tenantUUID, provider, bu, ak); err != nil {
			if needKey && ak == "" {
				return bu, ak, err
			}
			if needBase && bu == "" {
				return bu, ak, err
			}
		} else {
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
	_, err := s.QuickCallLLMResult(
		ctx, env, tenantUUID,
		"hunyuan", model, baseURL, apiKey, secretID, secretKey, region, authMode,
		0, 0,
		"ping",
	)
	return normalizePingLLMError("hunyuan", model, err)
}

func normalizePingLLMError(provider, model string, err error) error {
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
	if strings.EqualFold(strings.TrimSpace(provider), "ollama") &&
		strings.Contains(strings.ToLower(msg), "model") &&
		strings.Contains(strings.ToLower(msg), "not found") {
		return fmt.Errorf("OLLAMA_MODEL_NOT_FOUND model=%s: %w", model, err)
	}
	return err
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
	out := map[string]any{}
	if raw, ok := manifest.Defaults["api_path"]; ok {
		if path, ok2 := raw.(string); ok2 && strings.TrimSpace(path) != "" {
			out["api_path"] = strings.TrimSpace(path)
		}
	}
	for _, key := range []string{
		"action", "action_poll", "version", "service", "service_id",
		"req_json", "result_req_json", "force_single", "scale", "min_ratio", "max_ratio",
		"base_url", "api_path_submit", "api_path_poll", "parameters",
		"num_ctx", "context_window", "context_length", "context_len",
	} {
		if raw, ok := manifest.Defaults[key]; ok {
			out[key] = raw
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	app, pureModel := splitAppModel(reg, provider, model)
	candidates := []string{strings.TrimSpace(strings.ToLower(modality))}
	if candidates[0] == "audio_tts" || candidates[0] == "audio_asr" {
		candidates = append(candidates, "audio")
	}
	for _, mod := range candidates {
		models, err := reg.ModelsByApp(mod, provider, app)
		if err != nil {
			continue
		}
		for _, m := range models {
			if strings.EqualFold(m.ID, pureModel) || strings.EqualFold(m.ID, model) {
				copy := m
				return &copy
			}
		}
	}
	return nil
}

func splitAppModel(reg *catalog.Registry, provider, model string) (string, string) {
	raw := strings.TrimSpace(model)
	if raw == "" {
		return "", ""
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return "", raw
	}
	app := strings.ToLower(strings.TrimSpace(parts[0]))
	if app == "" {
		return "", raw
	}
	// only treat prefix as app if provider declares it
	apps := reg.Apps(provider, "")
	for _, it := range apps {
		if strings.EqualFold(it.ID, app) {
			return app, strings.TrimSpace(parts[1])
		}
	}
	return "", raw
}

// —— catalog 适配：在 service 层做轻薄封装，避免 handler 直依赖 —— //

type aiProviderItem struct {
	ID   string
	Name string
	Apps []aiAppItem
}

type aiModelItem struct {
	ID   string
	Name string
}

type aiAppItem struct {
	ID   string
	Name string
}

// 这两个函数直接调用你现有的 catalog，全局注册器：catalog.GetGlobalAIRegister()
func catalogGetProviders(mod string) []aiProviderItem {
	m := strings.TrimSpace(strings.ToLower(mod))
	reg := catalog.GetGlobalAIRegister()

	// ✅ 对齐：图像/视频两套 Provider 列表保持一致：image ∪ video
	var items []catalog.ProviderItem
	if m == "video" || m == "image" || m == "audio_tts" || m == "audio_asr" {
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
		if m == "video" || m == "image" {
			add(reg.Providers("image"))
			add(reg.Providers("video"))
		}
		if m == "audio_tts" || m == "audio_asr" {
			add(reg.Providers(m))
			add(reg.Providers("audio"))
		}
	} else {
		items = reg.Providers(m)
	}
	out := make([]aiProviderItem, 0, len(items))
	for _, it := range items {
		appItems := reg.Apps(it.ID, m)
		// 若按当前模态没有 app，回退到 provider 全量 app，避免 UI 丢失 app 层级。
		if len(appItems) == 0 {
			if allApps := reg.Apps(it.ID, ""); len(allApps) > 0 {
				appItems = allApps
			}
		}
		apps := make([]aiAppItem, 0, len(appItems))
		for _, a := range appItems {
			apps = append(apps, aiAppItem{ID: a.ID, Name: a.Name})
		}
		out = append(out, aiProviderItem{ID: it.ID, Name: it.Name, Apps: apps})
	}
	return out
}
func catalogGetModels(mod, prov, app string) ([]aiModelItem, error) {
	m := strings.TrimSpace(strings.ToLower(mod))
	reg := catalog.GetGlobalAIRegister()
	p := strings.TrimSpace(strings.ToLower(prov))
	a := strings.TrimSpace(strings.ToLower(app))

	// 对有 apps 的 provider：要求显式 app 过滤，避免跨 app 混合模型列表。
	if a == "" {
		if apps := reg.Apps(p, m); len(apps) > 0 {
			return []aiModelItem{}, nil
		}
	}

	// ✅ 对齐：图像/视频如果该模态没模型，则回退到另一模态（避免下拉为空）
	ms, err := reg.ModelsByApp(m, p, a)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		if m == "video" {
			if fallback, e := reg.ModelsByApp("image", p, a); e == nil && len(fallback) > 0 {
				ms = fallback
			}
		}
		if m == "image" {
			if fallback, e := reg.ModelsByApp("video", p, a); e == nil && len(fallback) > 0 {
				ms = fallback
			}
		}
		if m == "audio_tts" || m == "audio_asr" {
			if fallback, e := reg.ModelsByApp("audio", p, a); e == nil && len(fallback) > 0 {
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
		app, pureModel := splitAppModel(reg, provider, model)
		if models, _ := reg.ModelsByApp(modality, provider, app); len(models) > 0 {
			for _, mm := range models {
				if strings.EqualFold(mm.ID, pureModel) || strings.EqualFold(mm.ID, model) {
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
