package ai

import (
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

const textGenerationCapability = "text_generation"

// declaredModelCapabilities 只读取正式目录声明，不解析型号、标签或供应商探测缓存。
// 没有独立声明时，llm/vlm 模态本身声明文本输出；image 不能被当作文本或生图的证据。
func declaredModelCapabilities(modality string, model catalog.ModelManifest) []string {
	capabilities := appendUniqueCapabilities(nil, model.Capabilities...)
	if len(model.Capabilities) == 0 {
		switch catalog.NormalizeModality(modality) {
		case string(contract.ModLLM):
			capabilities = []string{textGenerationCapability}
		case string(contract.ModVLM):
			capabilities = []string{textGenerationCapability, "image_understanding"}
		}
	}
	return capabilities
}

func appendUniqueCapabilities(out []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range out {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			out = append(out, value)
		}
	}
	return out
}

func declaresTextGeneration(capabilities []string) bool {
	for _, value := range capabilities {
		if value == textGenerationCapability {
			return true
		}
	}
	return false
}

func textModelProfile(reg *catalog.Registry, profile dbModelProfileView) bool {
	_, accepted := textModelProfileCapabilities(reg, profile)
	return accepted
}

// 已登记模型按精确应用及型号读取能力；自定义模型保留 profile 的明确 llm 声明。
func textModelProfileCapabilities(reg *catalog.Registry, profile dbModelProfileView) ([]string, bool) {
	if profile.Modality != string(contract.ModLLM) {
		return nil, false
	}
	if reg == nil {
		return []string{textGenerationCapability}, true
	}
	manifest, ok := reg.Manifest(profile.Provider)
	if !ok {
		return []string{textGenerationCapability}, true
	}
	modelID := strings.TrimSpace(profile.Model)
	// 冒号只有在前缀是此 provider 的已登记 app 时才作为应用分隔符。
	// qwen3:8b、phi3:vision 等原生型号不能被截断。
	if parts := strings.SplitN(modelID, ":", 2); len(parts) == 2 {
		for app, entry := range manifest.Apps {
			if strings.EqualFold(strings.TrimSpace(parts[0]), app) {
				capabilities, registered := matchingTextCapabilities(entry.Modalities, strings.TrimSpace(parts[1]))
				return profileTextDecision(capabilities, registered)
			}
		}
	}
	capabilities, registered := matchingTextCapabilities(manifest.Modalities, modelID)
	if registered {
		return profileTextDecision(capabilities, true)
	}
	// 兼容旧的未限定 app 型号；应用之间存在文本/媒体歧义时要求显式限定。
	for _, entry := range manifest.Apps {
		appCapabilities, found := matchingTextCapabilities(entry.Modalities, modelID)
		if !found {
			continue
		}
		registered = true
		if !declaresTextGeneration(appCapabilities) {
			return nil, false
		}
		capabilities = appendUniqueCapabilities(capabilities, appCapabilities...)
	}
	return profileTextDecision(capabilities, registered)
}

func profileTextDecision(capabilities []string, registered bool) ([]string, bool) {
	if !registered {
		return []string{textGenerationCapability}, true
	}
	return capabilities, declaresTextGeneration(capabilities)
}

func matchingTextCapabilities(modalities map[string]catalog.ModalityManifest, id string) ([]string, bool) {
	var capabilities []string
	registered := false
	for modality, entry := range modalities {
		for _, model := range entry.Models {
			if !strings.EqualFold(strings.TrimSpace(model.ID), id) {
				continue
			}
			registered = true
			capabilities = appendUniqueCapabilities(capabilities, declaredModelCapabilities(modality, model)...)
		}
	}
	return capabilities, registered
}

// textCatalogModels 包含有文本生成声明的模型；app 条目保留 app:model，避免跨应用混淆。
func textCatalogModels(reg *catalog.Registry, provider string) []catalog.ModelManifest {
	if reg == nil {
		return nil
	}
	manifest, ok := reg.Manifest(provider)
	if !ok {
		return nil
	}
	var result []catalog.ModelManifest
	appendModels := func(app string, modalities map[string]catalog.ModalityManifest) {
		merged := map[string]catalog.ModelManifest{}
		keys := make([]string, 0, len(modalities))
		for key := range modalities {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, modality := range keys {
			entry := modalities[modality]
			for _, model := range entry.Models {
				capabilities := declaredModelCapabilities(modality, model)
				if strings.TrimSpace(model.ID) == "" {
					continue
				}
				model.ID = strings.TrimSpace(model.ID)
				key := strings.ToLower(model.ID)
				previous, found := merged[key]
				if found {
					capabilities = appendUniqueCapabilities(previous.Capabilities, capabilities...)
				}
				if !found || modality == string(contract.ModLLM) {
					model.Capabilities = capabilities
					merged[key] = model
				} else {
					previous.Capabilities = capabilities
					merged[key] = previous
				}
			}
		}
		for _, model := range merged {
			if !declaresTextGeneration(model.Capabilities) {
				continue
			}
			if app != "" {
				model.ID = app + ":" + model.ID
			}
			result = append(result, model)
		}
	}
	appendModels("", manifest.Modalities)
	for app, entry := range manifest.Apps {
		appendModels(app, entry.Modalities)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].ID) < strings.ToLower(result[j].ID) })
	return result
}
