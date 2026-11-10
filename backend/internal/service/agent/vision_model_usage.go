package agent

import (
	_ "embed"
	"encoding/json"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
)

// 仅补充已核对的官方用途证据，不提供候选 ID、不推导名称前缀，也不代表推理已验收。
//
//go:embed openai_vision_usage.json
var openAIVisionUsageJSON []byte

type visionUsageDocument struct {
	CheckedAt string `json:"checked_at"`
	Models    []struct {
		IDs    []string `json:"ids"`
		Page   string   `json:"page"`
		Status string   `json:"status"`
		Reason string   `json:"reason_code"`
	} `json:"models"`
}

var openAIVisionUsage = func() map[string]catalog.VisionUsage {
	var doc visionUsageDocument
	if err := json.Unmarshal(openAIVisionUsageJSON, &doc); err != nil {
		panic(err)
	}
	result := map[string]catalog.VisionUsage{}
	for _, entry := range doc.Models {
		for _, id := range entry.IDs {
			result[id] = catalog.VisionUsage{Status: entry.Status, Reason: entry.Reason, Source: "official_docs", URL: "https://developers.openai.com/api/docs/models/" + entry.Page, CheckedAt: doc.CheckedAt}
		}
	}
	return result
}()

func resolveVisionUsage(provider string, entry OnlineCatalogModel) catalog.VisionUsage {
	// 读取时重算，不信任旧目录中可能保存的归类结果。
	usage := catalog.VisionUsage{Status: "unknown", Source: "unknown"}
	if len(entry.InputModalities) > 0 && len(entry.OutputModalities) > 0 {
		usage.Source = "provider_api"
		switch {
		case !containsModality(entry.InputModalities, "image"):
			usage.Status, usage.Reason = "unsupported", "AI_VISION_IMAGE_INPUT_UNSUPPORTED"
		case !containsModality(entry.OutputModalities, "text"):
			usage.Status, usage.Reason = "unsupported", "AI_VISION_TEXT_OUTPUT_UNSUPPORTED"
		default:
			usage.Status = "supported"
		}
	} else if containsModality(entry.ReportedModalities, "vlm") {
		usage.Status, usage.Source = "supported", "provider_api"
	}
	if provider == "openai" {
		if official, ok := openAIVisionUsage[entry.ID]; ok {
			// 明确的协议限制/下线信息不能被仅声明 I/O 的目录绕过。
			if official.Reason == "AI_VISION_PROTOCOL_UNSUPPORTED" || official.Reason == "AI_MODEL_RETIRED" || usage.Source == "unknown" {
				return official
			}
		}
	}
	return usage
}

func enrichVisionUsage(provider string, entries []OnlineCatalogModel) {
	for i := range entries {
		usage := resolveVisionUsage(provider, entries[i])
		entries[i].Vision = &usage
	}
}
