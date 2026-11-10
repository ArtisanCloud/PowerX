package agent

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modeldiscovery"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// TenantModelCatalog 在原有型号数组之外提供目录来源与供应商声明。
type TenantModelCatalog struct {
	Models    []string             `json:"models"`
	Source    string               `json:"catalog_source"`
	UpdatedAt string               `json:"updated_at,omitempty"`
	URL       string               `json:"source,omitempty"`
	Items     []OnlineCatalogModel `json:"items,omitempty"`
}

// ModelCatalogForTenant 保留完整在线型号，类型声明与可执行协议分别返回。
func (s *AgentSettingService) ModelCatalogForTenant(ctx context.Context, env string, tenant *string, modality, provider, app, baseURL string) (*TenantModelCatalog, error) {
	reg := catalog.GetGlobalAIRegister()
	provider = reg.CanonicalProvider(provider)
	models, err := s.ModelsForTenant(ctx, env, tenant, modality, provider, app, baseURL)
	if err != nil {
		return nil, err
	}
	out := &TenantModelCatalog{Models: models, Source: "local"}
	for _, id := range models {
		out.Items = append(out.Items, OnlineCatalogModel{ID: id})
	}
	enrichParameterProfiles(provider, out.Items)
	enrichImageOperations(reg, provider, app, modality, out.Items)
	if provider == "ollama" {
		out.Source = "instance"
		return out, nil
	}
	if _, ok := modeldiscovery.ProviderSpec(provider); !ok {
		return out, nil
	}
	if tenant != nil {
		if _, err := s.savedCatalogConnection(ctx, env, *tenant, provider); dto.CodeOf(err) == "AI_MODEL_SYNC_UNSUPPORTED" {
			return out, nil
		}
	}
	out.Source = "online"
	if tenant == nil {
		return out, nil
	}
	item, err := findOnlineCatalogSetting(ctx, s.settingRepo, *tenant, env, provider, app, false)
	if err != nil {
		return nil, err
	}
	snapshot, err := decodeOnlineCatalog(item)
	if err != nil {
		return nil, err
	}
	if snapshot != nil {
		out.Items, out.UpdatedAt, out.URL = snapshot.Models, snapshot.UpdatedAt, snapshot.Source
		enrichVisionUsage(provider, out.Items)
		enrichParameterProfiles(provider, out.Items)
		enrichImageOperations(reg, provider, app, modality, out.Items)
	}
	return out, nil
}

// 生图选项直接来自可执行合同，不把 LLM 参数推测或旧在线快照当成生图 Schema。
func enrichImageOperations(reg *catalog.Registry, provider, app, modality string, entries []OnlineCatalogModel) {
	if modality != "image" {
		return
	}
	for i := range entries {
		model := reg.ProtocolModel(modality, provider, app, entries[i].ID)
		if model != nil {
			if operation, ok := model.Operations[catalog.ImageGeneration]; ok {
				entries[i].Operations = map[string]catalog.MediaOperation{catalog.ImageGeneration: operation}
			}
		}
	}
}

// TenantModelManifest 不向全局目录添加租户型号；只在当前连接目录内解析驱动参数。
func (s *AgentSettingService) TenantModelManifest(ctx context.Context, env string, tenant *string, modality, provider, model string) (*catalog.ModelManifest, error) {
	if err := s.ensureTenantModelExists(ctx, env, tenant, modality, provider, model); err != nil {
		return nil, err
	}
	reg := catalog.GetGlobalAIRegister()
	provider = reg.CanonicalProvider(provider)
	app, id := splitAppModel(reg, provider, model)
	manifest := reg.ProtocolModel(modality, provider, app, id)
	if manifest == nil {
		return nil, &dto.AppError{HTTPCode: 422, Code: "AI_DRIVER_UNSUPPORTED", Message: "AI_DRIVER_UNSUPPORTED"}
	}
	return manifest, nil
}

func modalityForMediaCapability(capability string) string {
	switch capability {
	case catalog.VisionUnderstanding:
		return "vlm"
	case catalog.ImageGeneration:
		return "image"
	case catalog.SpeechSynthesis:
		return "audio_tts"
	case catalog.VideoGeneration:
		return "video"
	}
	return ""
}

// MediaModelsForTenant 让服务能力目录使用与设置页相同的租户在线型号来源。
func (s *AgentSettingService) MediaModelsForTenant(ctx context.Context, env string, tenant *string, provider, app, capability string) ([]catalog.MediaModel, error) {
	reg := catalog.GetGlobalAIRegister()
	providers := reg.Providers("")
	if provider != "" {
		providers = []catalog.ProviderItem{{ID: reg.CanonicalProvider(provider)}}
	}
	result := []catalog.MediaModel{}
	for _, p := range providers {
		if _, online := modeldiscovery.ProviderSpec(p.ID); !online {
			result = append(result, reg.MediaModels(p.ID, app, capability)...)
			continue
		}
		manifest, ok := reg.Manifest(p.ID)
		if !ok {
			continue
		}
		modalities := []string{"vlm", "image", "audio_tts", "video"}
		if capability != "" {
			modalities = []string{modalityForMediaCapability(capability)}
		}
		for _, modality := range modalities {
			if modality == "" || manifest.Drivers[modality] == "" {
				continue
			}
			view, err := s.ModelCatalogForTenant(ctx, env, tenant, modality, p.ID, app, "")
			if err != nil {
				// 没有配置的供应商没有可查询的在线型号；不能回填本地示例。
				if dto.CodeOf(err) == "AI_SETTINGS_PROVIDER_NOT_CONFIGURED" {
					continue
				}
				return nil, err
			}
			for _, id := range view.Models {
				model := reg.ProtocolModel(modality, p.ID, app, id)
				if model == nil || len(model.Operations) == 0 {
					continue
				}
				model.Tags = append([]string{}, model.Tags...)
				var vision *catalog.VisionUsage
				for _, entry := range view.Items {
					if entry.ID == id {
						if containsModality(entry.ReportedModalities, modality) {
							model.Tags = append(model.Tags, "provider-usage")
						}
						if modality == "vlm" {
							vision = entry.Vision
						}
						break
					}
				}
				result = append(result, catalog.MediaModel{Provider: p.ID, App: app, Modality: modality, Model: *model, Vision: vision})
			}
		}
	}
	return result, nil
}
