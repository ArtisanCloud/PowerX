// Package modelparams 管理有来源的型号参数；不访问供应商、不保存用户配置。
package modelparams

import (
	_ "embed"
	"encoding/json"
	"math"
	"strings"
)

type Profile struct {
	Version                  int            `json:"version"`
	Provider                 string         `json:"provider"`
	Model                    string         `json:"model"`
	Source                   string         `json:"source"`
	SourceURL                string         `json:"source_url,omitempty"`
	CheckedAt                string         `json:"checked_at,omitempty"`
	Defaults                 map[string]any `json:"defaults"`
	Sampling                 string         `json:"sampling"` // supported | reasoning_none | unsupported | unknown
	ReasoningEfforts         []string       `json:"reasoning_efforts,omitempty"`
	TokenParameter           string         `json:"token_parameter,omitempty"`
	SupportedParameters      []string       `json:"supported_parameters,omitempty"`
	SupportedParametersKnown bool           `json:"supported_parameters_known,omitempty"`
	MaxOutputTokens          int            `json:"max_output_tokens,omitempty"` // 上限，不是默认请求预算。
}

//go:embed openai.json
var officialJSON []byte
var officialRules = func() []struct {
	Families []string `json:"families"`
	Profile
} {
	var rules []struct {
		Families []string `json:"families"`
		Profile
	}
	if err := json.Unmarshal(officialJSON, &rules); err != nil {
		panic(err)
	}
	return rules
}()

func Known(provider, model string) *Profile {
	p := &Profile{Version: 1, Provider: strings.ToLower(strings.TrimSpace(provider)), Model: model, Source: "unknown", Sampling: "unknown", Defaults: map[string]any{}}
	if p.Provider != "openai" {
		return p
	}
	for _, rule := range officialRules {
		for _, family := range rule.Families {
			if model != family && !strings.HasPrefix(model, family+"-") {
				continue
			}
			raw, _ := json.Marshal(rule.Profile)
			_ = json.Unmarshal(raw, p) // 每次返回独立 map，不污染其他租户或请求。
			p.Provider, p.Model, p.Version = provider, model, 1
			return p
		}
	}
	return p
}

func (p *Profile) SupportsSampling(effort string) bool {
	if p == nil || p.Sampling == "unknown" || p.Sampling == "supported" {
		return true
	}
	if effort == "" {
		effort, _ = p.Defaults["reasoning_effort"].(string)
	}
	return p.Sampling == "reasoning_none" && effort == "none"
}

func (p *Profile) Supports(name string) bool {
	if p == nil || !p.SupportedParametersKnown {
		return true
	}
	for _, key := range p.SupportedParameters {
		if key == name {
			return true
		}
	}
	return false
}

// Provider 从模型列表响应中提取白名单字段。null 是未提供，显式 0 必须保留。
func Provider(provider, model, source, checkedAt string, defaults map[string]any, supported []string, maxOutput int) *Profile {
	p := Known(provider, model)
	clean := map[string]any{}
	for key, value := range defaults {
		switch key {
		case "temperature", "top_p":
			if n, ok := value.(float64); ok && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 2 && (key != "top_p" || n <= 1) {
				clean[key] = n
			}
		case "max_tokens":
			if n, ok := value.(float64); ok && n > 0 && n <= 10000000 && n == math.Trunc(n) {
				clean[key] = n
			}
		case "reasoning_effort":
			if effort, ok := value.(string); ok {
				for _, known := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
					if effort == known {
						clean[key] = effort
					}
				}
			}
		}
	}
	if len(clean) == 0 && supported == nil && maxOutput <= 0 {
		return p
	}
	// OpenAI /models 不声明这些扩展；已核实官方合同不被兼容网关扩展覆盖。
	if p.Source == "official_docs" {
		return p
	}
	p.Source, p.SourceURL, p.CheckedAt = "provider_api", source, checkedAt
	p.Defaults = clean
	if supported != nil {
		p.SupportedParametersKnown = true
		for _, key := range supported {
			switch key {
			case "temperature", "top_p", "max_tokens", "max_completion_tokens", "reasoning_effort", "logprobs", "top_logprobs":
				p.SupportedParameters = append(p.SupportedParameters, key)
			}
		}
	}
	if maxOutput > 0 && maxOutput <= 10000000 {
		p.MaxOutputTokens = maxOutput
	}
	if p.SupportedParametersKnown && p.Supports("max_completion_tokens") && !p.Supports("max_tokens") {
		p.TokenParameter = "max_completion_tokens"
	}
	return p
}
