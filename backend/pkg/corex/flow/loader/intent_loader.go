package loader

// pkg/corex/flow/loader/intent_loader.go

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// 轻量结构：只关心 metadata.intent
type intentYAML struct {
	Register  bool                     `yaml:"register"`
	Group     string                   `yaml:"group,omitempty"`
	Weight    float64                  `yaml:"weight,omitempty"`
	Matchers  []schemas.IntentMatcher  `yaml:"matchers,omitempty"`
	Embedding *schemas.IntentEmbedding `yaml:"embedding,omitempty"`
	Examples  *schemas.IntentExamples  `yaml:"examples,omitempty"`
}
type flowWithIntent struct {
	Metadata struct {
		Intent *intentYAML `yaml:"intent"`
	} `yaml:"metadata"`
}

type lightFlow struct {
	FlowID   string                `yaml:"flow_id"`
	Name     string                `yaml:"name,omitempty"`
	Version  string                `yaml:"version,omitempty"`
	Metadata *schemas.FlowMetadata `yaml:"metadata,omitempty"`
	Ref      *struct {
		Enable         bool   `yaml:"enable"`
		Target         string `yaml:"target"`
		Variant        string `yaml:"variant,omitempty"`
		RegisterIntent *bool  `yaml:"register_intent,omitempty"`
		Overrides      *struct {
			Metadata *schemas.FlowMetadata `yaml:"metadata,omitempty"`
		} `yaml:"overrides,omitempty"`
	} `yaml:"ref,omitempty"`
}

type auxIntent struct {
	Register  bool                     `yaml:"register"`
	Group     string                   `yaml:"group,omitempty"`
	Weight    float64                  `yaml:"weight,omitempty"`
	Matchers  []schemas.IntentMatcher  `yaml:"matchers,omitempty"`
	Embedding *schemas.IntentEmbedding `yaml:"embedding,omitempty"`
	Examples  *schemas.IntentExamples  `yaml:"examples,omitempty"`
}
type auxFlow struct {
	FlowID   string                `yaml:"flow_id"`
	Name     string                `yaml:"name,omitempty"`
	Version  string                `yaml:"version,omitempty"`
	Metadata *schemas.FlowMetadata `yaml:"metadata"`
}
type auxFlowWithIntent struct {
	auxFlow `yaml:",inline"`
	Intent  *auxIntent `yaml:"intent"` // 注意：我们让 YAML 写法是 metadata: { ... , intent: {...} }
}

// 🔧 合并 metadata（只覆盖非零值；requires 若 overrides 非空则替换）
func mergeMeta(base, over *schemas.FlowMetadata) *schemas.FlowMetadata {
	if base == nil && over == nil {
		return nil
	}
	out := &schemas.FlowMetadata{}

	// 先拷贝 base
	if base != nil {
		*out = *base

		// 深拷贝 IO
		if base.IO != nil {
			cp := *base.IO
			if len(base.IO.Inputs) > 0 {
				cp.Inputs = append([]schemas.ParamSpec(nil), base.IO.Inputs...)
			}
			if len(base.IO.Outputs) > 0 {
				cp.Outputs = append([]schemas.ParamSpec(nil), base.IO.Outputs...)
			}
			out.IO = &cp
		}

		// 深拷贝 map/slices
		if base.ExtraInfo != nil {
			out.ExtraInfo = map[string]string{}
			for k, v := range base.ExtraInfo {
				out.ExtraInfo[k] = v
			}
		}
		if len(base.Requires) > 0 {
			out.Requires = append([]string(nil), base.Requires...)
		}
		if len(base.Tags) > 0 {
			out.Tags = append([]string(nil), base.Tags...)
		}
	}

	// 再应用 overrides（只覆盖非零值）
	if over != nil {
		if over.Domain != "" {
			out.Domain = over.Domain
		}
		if over.Version != "" {
			out.Version = over.Version
		}
		if over.Description != "" {
			out.Description = over.Description
		}
		if over.IO != nil {
			cp := *over.IO
			if len(over.IO.Inputs) > 0 {
				cp.Inputs = append([]schemas.ParamSpec(nil), over.IO.Inputs...)
			}
			if len(over.IO.Outputs) > 0 {
				cp.Outputs = append([]schemas.ParamSpec(nil), over.IO.Outputs...)
			}
			out.IO = &cp
		}
		// requires：如果 overrides 给了非空数组，直接替换
		if len(over.Requires) > 0 {
			out.Requires = append([]string(nil), over.Requires...)
		}
		if over.Priority != 0 {
			out.Priority = over.Priority
		}
		if len(over.Tags) > 0 {
			out.Tags = append([]string(nil), over.Tags...)
		}
		if over.Owner != "" {
			out.Owner = over.Owner
		}
		// IsPublic：显式覆盖为 true；否则保持原值
		if over.IsPublic {
			out.IsPublic = true
		}
		if over.ExtraInfo != nil {
			if out.ExtraInfo == nil {
				out.ExtraInfo = map[string]string{}
			}
			for k, v := range over.ExtraInfo {
				out.ExtraInfo[k] = v
			}
		}
		// 注意：Intent 不在此处处理
	}

	return out
}

// ParseIntentSpecsFromDir 读取目录下所有 YAML，并抽出 metadata.intent
func ParseIntentSpecsFromDir(root string) ([]*schemas.IntentSpec, error) {
	out := make([]*schemas.IntentSpec, 0, 64)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !utils.IsYAML(d.Name()) {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// 1) 轻量解析：拿 flow_id / metadata / ref.overrides.metadata
		var lf lightFlow
		if err := yaml.Unmarshal(b, &lf); err != nil {
			return err
		}
		if strings.TrimSpace(lf.FlowID) == "" {
			// 不是 flow 蓝图，跳过
			return nil
		}

		// 2) 单独解析 intent（matchers/examples/register）
		var aux flowWithIntent
		_ = yaml.Unmarshal(b, &aux) // 容错：没有 intent 也不报错

		// 3) 计算“有效 metadata”：metadata ⊕ ref.overrides.metadata
		effMeta := mergeMeta(lf.Metadata, nil)
		if lf.Ref != nil && lf.Ref.Overrides != nil && lf.Ref.Overrides.Metadata != nil {
			effMeta = mergeMeta(effMeta, lf.Ref.Overrides.Metadata)
		}
		if effMeta == nil {
			effMeta = &schemas.FlowMetadata{} // 防御空指针
		}

		// 4) 组装 IntentSpec
		var matchers []schemas.IntentMatcher
		var examples schemas.IntentExamples

		reg := true // 默认注册
		if aux.Metadata.Intent != nil {
			matchers = aux.Metadata.Intent.Matchers
			if aux.Metadata.Intent.Examples != nil {
				examples = *aux.Metadata.Intent.Examples
			}
			if !aux.Metadata.Intent.Register {
				reg = false
			}
		}
		if lf.Ref != nil && lf.Ref.RegisterIntent != nil && !*lf.Ref.RegisterIntent {
			// ref 层显式要求不注册
			reg = false
		}

		// 即便不注册匹配器，也要回传 spec 以便 BuildPlan 读取 requires
		if !reg {
			matchers = nil
			examples = schemas.IntentExamples{}
		}

		spec := &schemas.IntentSpec{
			FlowID:   lf.FlowID,
			Name:     lf.Name,
			Version:  lf.Version,
			Matchers: matchers,
			Examples: examples,
			Metadata: effMeta,
		}
		out = append(out, spec)
		return nil
	})
	return out, err
}

func extractIntentFromYAML(data []byte) ([]schemas.IntentMatcher, schemas.IntentExamples) {
	var aux flowWithIntent
	if err := yaml.Unmarshal(data, &aux); err != nil || aux.Metadata.Intent == nil {
		return nil, schemas.IntentExamples{}
	}
	var ex schemas.IntentExamples
	if aux.Metadata.Intent.Examples != nil {
		ex = *aux.Metadata.Intent.Examples // 安全解引用
	}
	return aux.Metadata.Intent.Matchers, ex
}

// —— 如果你已有调用点写成了 extractMatchersFromFlow(...) —— //
// 这里给一个薄封装，避免改太多签名（会返回空，因为 Flow 里没有 intent）。
// 建议优先用 extractIntentFromYAML(data) 上面的版本。
func extractMatchersFromFlow(_ *schemas.Flow) []schemas.IntentMatcher {
	return nil
}
func extractExamplesFromFlow(_ *schemas.Flow) schemas.IntentExamples {
	return schemas.IntentExamples{}
}

func normalizeType(s string) string {
	switch s {
	case "Keyword", "KEYWORD", "keyword":
		return "keyword"
	case "Pattern", "REGEX", "pattern", "regex":
		return "pattern"
	default:
		return s
	}
}

func normalizeWords(arr []string) []string {
	out := make([]string, 0, len(arr))
	seen := map[string]struct{}{}
	for _, w := range arr {
		w = strings.TrimSpace(strings.ToLower(w))
		if w == "" {
			continue
		}
		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		out = append(out, w)
	}
	return out
}
