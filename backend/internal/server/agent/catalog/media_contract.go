package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	VisionUnderstanding = "vision_understanding"
	ImageGeneration     = "image_generation"
	VideoGeneration     = "video_generation"
	SpeechSynthesis     = "speech_synthesis"
)

// ParameterSchema 使用 JSON Schema 子集；目录和执行校验读取同一份声明。
type ParameterSchema struct {
	Type    string   `json:"type" yaml:"type"`
	Enum    []any    `json:"enum,omitempty" yaml:"enum,omitempty"`
	Minimum *float64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	Default any      `json:"default,omitempty" yaml:"default,omitempty"`
}
type MediaOperation struct {
	MaxImageBytes      int64                      `json:"max_image_bytes,omitempty" yaml:"max_image_bytes,omitempty"`
	MaxTotalImageBytes int64                      `json:"max_total_image_bytes,omitempty" yaml:"max_total_image_bytes,omitempty"`
	MaxImagePixels     int64                      `json:"max_image_pixels,omitempty" yaml:"max_image_pixels,omitempty"`
	Driver             string                     `json:"driver" yaml:"driver"`
	InputTypes         []string                   `json:"input_types" yaml:"input_types"`
	MaxImages          int                        `json:"max_images" yaml:"max_images"`
	MaxVideos          int                        `json:"max_videos" yaml:"max_videos"`
	Parameters         map[string]ParameterSchema `json:"parameters" yaml:"parameters"`
	Async              bool                       `json:"async" yaml:"async"`
}

func MediaCapability(value string) string {
	if value == "image_understanding" {
		return VisionUnderstanding
	}
	return strings.TrimSpace(value)
}

// VisionUsage 区分用途证据与真实推理验收；未知型号仍可显式选择试跑。
type VisionUsage struct {
	Status    string `json:"status"` // supported / unsupported / unknown
	Reason    string `json:"reason_code,omitempty"`
	Source    string `json:"source"` // provider_api / official_docs / unknown
	URL       string `json:"source_url,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
}

type MediaModel struct {
	Provider string
	App      string
	Modality string
	Model    ModelManifest
	Vision   *VisionUsage
}

// MediaModels 保留 app 与供应商原始型号，通配条目永远不是能力证明。
func (r *Registry) MediaModels(provider, app, capability string) []MediaModel {
	result := []MediaModel{}
	providers := r.Providers("")
	if provider != "" {
		providers = []ProviderItem{{ID: r.CanonicalProvider(provider)}}
	}
	for _, p := range providers {
		m, ok := r.Manifest(p.ID)
		if !ok {
			continue
		}
		appendModels := func(application string, mods map[string]ModalityManifest) {
			if app != "" && app != application {
				return
			}
			for modality, list := range mods {
				for _, model := range list.Models {
					if model.ID == "" || model.ID == "*" || len(model.Operations) == 0 {
						continue
					}
					if capability != "" {
						if _, ok := model.Operations[MediaCapability(capability)]; !ok {
							continue
						}
					}
					result = append(result, MediaModel{Provider: p.ID, App: application, Modality: modality, Model: model})
				}
			}
		}
		appendModels("", m.Modalities)
		for application, a := range m.Apps {
			appendModels(application, a.Modalities)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		return a.Provider+"/"+a.App+":"+a.Model.ID < b.Provider+"/"+b.App+":"+b.Model.ID
	})
	return result
}
func (m MediaModel) ModelKey() string {
	id := m.Model.ID
	if m.App != "" {
		id = m.App + ":" + id
	}
	return m.Provider + "/" + id
}

// MediaDefaults 统一设置保存、目录读取与执行时的默认值合并顺序。
// 画像的显式值优先；未知业务辅助字段不成为供应商调用参数。
func MediaDefaults(model *ModelManifest, capability string, profile map[string]any) map[string]any {
	result := map[string]any{}
	op := model.Operations[capability]
	for name, schema := range op.Parameters {
		if schema.Default != nil {
			result[name] = schema.Default
		}
	}
	aliases := map[string]string{"maxTokens": "max_tokens", "topP": "top_p", "maxDurationSec": "max_duration_sec", "output_format": "format"}
	for _, values := range []map[string]any{model.Defaults, profile} {
		for key, value := range values {
			if alias := aliases[key]; alias != "" {
				key = alias
			}
			if _, ok := op.Parameters[key]; ok {
				result[key] = value
			}
		}
	}
	return result
}

func ValidateMediaParameters(operation MediaOperation, params map[string]any) (string, string) {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		value := params[k]
		schema, ok := operation.Parameters[k]
		if !ok {
			return k, "unsupported_parameter"
		}
		valid := false
		switch schema.Type {
		case "string":
			_, valid = value.(string)
		case "boolean":
			_, valid = value.(bool)
		case "number", "integer":
			raw, e := json.Marshal(value)
			var n float64
			if e == nil && json.Unmarshal(raw, &n) == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
				valid = schema.Type != "integer" || math.Trunc(n) == n
				if schema.Minimum != nil && n < *schema.Minimum || schema.Maximum != nil && n > *schema.Maximum {
					return k, "out_of_range"
				}
			}
		}
		if !valid {
			return k, "invalid_type"
		}
		if len(schema.Enum) > 0 {
			found := false
			for _, candidate := range schema.Enum {
				if fmt.Sprint(candidate) == fmt.Sprint(value) {
					found = true
					break
				}
			}
			if !found {
				return k, "invalid_enum"
			}
		}
	}
	return "", ""
}
