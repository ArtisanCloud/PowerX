package ai

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
)

const (
	settingsProvidersCapability = "com.corex.ai.catalog.providers.read"
	settingsProvidersOperation  = "list_settings_providers"
	settingsModelsOperation     = "list_settings_models"
)

type settingsProvidersRequest struct {
	Operation string `json:"operation"`
	Modality  string `json:"modality"`
}

type settingsModelsRequest struct {
	Operation string `json:"operation"`
	Modality  string `json:"modality"`
	Provider  string `json:"provider"`
	App       string `json:"app,omitempty"`
}

// SettingsCatalogApp 保留底座正式应用标识和显示名称。
type SettingsCatalogApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SettingsCatalogProvider 直接来自管理页的 Provider 服务，不从型号反推。
type SettingsCatalogProvider struct {
	ID   string               `json:"id"`
	Name string               `json:"name"`
	Apps []SettingsCatalogApp `json:"apps"`
}

// SettingsProviderCatalog 是 AI Settings 配置候选目录，不是生成授权证明。
type SettingsProviderCatalog struct {
	ContractVersion int                       `json:"contract_version"`
	CatalogKind     string                    `json:"catalog_kind"`
	Env             string                    `json:"env"`
	Modality        string                    `json:"modality"`
	Providers       []SettingsCatalogProvider `json:"providers"`
}

// SettingsCatalogModel 原样保留实例 tag；只有正式 App 才添加应用前缀。
type SettingsCatalogModel struct {
	ModelKey string `json:"model_key"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Label    string `json:"label"`
}

// SettingsModelCatalog 与管理页共用型号读取逻辑，连接信息不进入响应。
type SettingsModelCatalog struct {
	ContractVersion int                    `json:"contract_version"`
	CatalogKind     string                 `json:"catalog_kind"`
	Env             string                 `json:"env"`
	Modality        string                 `json:"modality"`
	Provider        string                 `json:"provider"`
	App             string                 `json:"app"`
	Items           []SettingsCatalogModel `json:"items"`
}

// settingsCatalog 由已完成实时 grant 和 tenant 校验的固定 core_internal 入口调用。
func (s *Service) settingsCatalog(ctx context.Context, capabilityID, operation string, body any) (map[string]any, error) {
	provider, app := "", ""
	switch operation {
	case settingsProvidersOperation:
		var request settingsProvidersRequest
		if capabilityID != settingsProvidersCapability || strictMediaBody(body, &request) != nil || request.Modality != "llm" {
			return nil, MediaError(400, "AI_OPERATION_INVALID")
		}
	case settingsModelsOperation:
		var request settingsModelsRequest
		if capabilityID != MediaCatalogCapability || strictMediaBody(body, &request) != nil || request.Modality != "llm" || strings.TrimSpace(request.Provider) == "" {
			return nil, MediaError(400, "AI_OPERATION_INVALID")
		}
		provider, app = request.Provider, request.App
	default:
		return nil, MediaError(400, "AI_OPERATION_INVALID")
	}
	tenant := reqctx.GetTenantUUID(ctx)
	env, configured, err := s.ResolveTenantEnv(ctx, tenant)
	if err != nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if !configured {
		return nil, MediaError(409, "AI_ENV_NOT_CONFIGURED")
	}
	providers := s.settings.Providers("llm")
	if operation == settingsProvidersOperation {
		items := make([]SettingsCatalogProvider, 0, len(providers))
		for _, entry := range providers {
			apps := make([]SettingsCatalogApp, 0, len(entry.Apps))
			for _, item := range entry.Apps {
				apps = append(apps, SettingsCatalogApp{ID: item.ID, Name: item.Name})
			}
			items = append(items, SettingsCatalogProvider{ID: entry.ID, Name: entry.Name, Apps: apps})
		}
		return map[string]any{"result": SettingsProviderCatalog{ContractVersion: 1, CatalogKind: "ai_settings", Env: env, Modality: "llm", Providers: items}}, nil
	}
	found := false
	for _, entry := range providers {
		if entry.ID != provider {
			continue
		}
		found = true
		if len(entry.Apps) > 0 && app == "" {
			return nil, MediaError(422, "AI_SETTINGS_APP_REQUIRED")
		}
		appFound := app == ""
		for _, item := range entry.Apps {
			appFound = appFound || item.ID == app
		}
		if !appFound {
			return nil, MediaError(422, "AI_SETTINGS_APP_INVALID")
		}
		break
	}
	if !found {
		return nil, MediaError(422, "AI_SETTINGS_PROVIDER_INVALID")
	}
	baseURL := ""
	if provider == "ollama" {
		// 服务读取不能因租户未配置而探测 Core 主机的默认 Ollama。
		// 使用已保存连接的快照，避免二次查询失败后退回另一实例。
		baseURL, err = s.savedSettingsOllamaBase(ctx, env, tenant)
		if err != nil {
			return nil, err
		}
	}
	models, err := s.settings.ModelsForTenant(ctx, env, &tenant, "llm", provider, app, baseURL)
	if err != nil {
		return nil, err
	}
	items := make([]SettingsCatalogModel, 0, len(models))
	for _, model := range models {
		key := provider + "/" + model
		if app != "" {
			key = provider + "/" + app + ":" + model
		}
		items = append(items, SettingsCatalogModel{ModelKey: key, Provider: provider, Model: model, Label: model})
	}
	return map[string]any{"result": SettingsModelCatalog{ContractVersion: 1, CatalogKind: "ai_settings", Env: env, Modality: "llm", Provider: provider, App: app, Items: items}}, nil
}

func (s *Service) savedSettingsOllamaBase(ctx context.Context, env, tenant string) (string, error) {
	credentials, err := s.settings.ListCredentials(ctx, env, &tenant)
	if err != nil {
		return "", MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	for _, credential := range credentials {
		if credential.Provider != "ollama" || credential.Name != utils.Slug(credential.Env+"-ollama") {
			continue
		}
		base, _ := credential.Data["base_url"].(string)
		if strings.TrimSpace(base) == "" {
			// 显式保存的默认连接仍由 Core 的正式 Provider 配置决定地址。
			base = catalog.AuthReqFromCatalog("ollama").DefaultBaseURL
		}
		if strings.TrimSpace(base) != "" {
			return strings.TrimSpace(base), nil
		}
	}
	return "", MediaError(409, "AI_SETTINGS_PROVIDER_NOT_CONFIGURED")
}
