package agent

import (
	"context"
	"sort"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
)

func containsModality(modalities []string, modality string) bool {
	for _, candidate := range modalities {
		if candidate == modality {
			return true
		}
	}
	return false
}

// legacyConfiguredCandidate 仅保留同租户、连接、环境、模态下已保存配置的运行兼容性。
// 它不重新开放下拉候选，也不允许图像等已知用途进入文本调用。
func (s *AgentSettingService) legacyConfiguredCandidate(ctx context.Context, env string, tenant *string, mod, provider, model string) bool {
	if tenant == nil || (mod != "llm" && mod != "embedding") {
		return false
	}
	reg := catalog.GetGlobalAIRegister()
	provider = reg.CanonicalProvider(provider)
	manifest, ok := reg.Manifest(provider)
	if !ok || manifest == nil {
		return false
	}
	app, id := splitAppModel(reg, provider, model)
	item, err := findOnlineCatalogSetting(ctx, s.settingRepo, *tenant, env, provider, app, false)
	if err != nil {
		return false
	}
	snapshot, err := decodeOnlineCatalog(item)
	if err != nil || snapshot == nil {
		return false
	}
	conn, err := s.savedCatalogConnection(ctx, env, *tenant, provider)
	if err != nil || snapshot.Fingerprint != connectionFingerprint(conn) {
		return false
	}
	for _, stored := range snapshot.Models {
		if stored.ID != id {
			continue
		}
		entry, _ := classifyOnlineModel(reg, manifest, provider, app, stored)
		if !entry.ReviewRequired || len(entry.Modalities) > 0 || (len(entry.ReportedModalities) > 0 && !containsModality(entry.ReportedModalities, mod)) {
			return false
		}
		profile, err := s.GetProfile(ctx, env, tenant, mod, provider, model)
		return err == nil && profile != nil && profile.TenantUUID != nil && *profile.TenantUUID == *tenant
	}
	return false
}

// classifyOnlineModel 在同步和读取时使用同一套分类规则。
// 在线用途声明与本地可执行合同分开保存；旧版未逐条确认的未知候选必须重新审核。
func classifyOnlineModel(reg *catalog.Registry, manifest *catalog.Manifest, provider, app string, stored OnlineCatalogModel) (OnlineCatalogModel, bool) {
	entry := stored
	entry.Modalities, entry.ManualModalities = []string{}, []string{}
	entry.Match, entry.ManualConfirmed = "unmatched", false
	modalities := map[string]bool{}
	for mod := range manifest.Modalities {
		modalities[mod] = true
	}
	if group, ok := manifest.Apps[app]; ok {
		for mod := range group.Modalities {
			modalities[mod] = true
		}
	}
	for mod := range modalities {
		models, _ := reg.ModelsByApp(mod, provider, app)
		for _, local := range models {
			if local.ID == entry.ID {
				entry.Modalities = appendUnique(entry.Modalities, mod)
				entry.Match = "local"
			}
		}
	}
	if entry.Match == "unmatched" {
		for _, mod := range entry.ReportedModalities {
			if (mod == "llm" || mod == "embedding") && manifest.Drivers[mod] != "" {
				entry.Modalities = appendUnique(entry.Modalities, mod)
				entry.Match = "provider"
			}
		}
	}
	if len(entry.Modalities) == 0 && stored.ManualConfirmed {
		for _, mod := range stored.ManualModalities {
			if (mod == "llm" || mod == "embedding") && manifest.Drivers[mod] != "" && (len(entry.ReportedModalities) == 0 || containsModality(entry.ReportedModalities, mod)) {
				entry.ManualModalities = appendUnique(entry.ManualModalities, mod)
				entry.Match, entry.ManualConfirmed = "manual", true
			}
		}
	}
	corrected := false
	for _, oldMod := range stored.ManualModalities {
		if !containsModality(entry.Modalities, oldMod) && !containsModality(entry.ManualModalities, oldMod) {
			corrected = true
		}
	}
	entry.ReviewRequired = entry.Match == "unmatched" && (stored.ReviewRequired || corrected)
	sort.Strings(entry.Modalities)
	return entry, corrected
}
