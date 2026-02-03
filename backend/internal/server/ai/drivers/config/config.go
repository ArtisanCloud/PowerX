package config

import (
	"time"
)

// 统一配置（融合运行时请求 + agent 默认配置）
type ModelConfig struct {
	Provider     string         `yaml:"provider" json:"provider"`
	Endpoint     string         `yaml:"endpoint" json:"endpoint"`
	APIKey       string         `yaml:"api_key" json:"api_key"`
	SecretID     string         `yaml:"secret_id" json:"secret_id"`
	SecretKey    string         `yaml:"secret_key" json:"secret_key"`
	Region       string         `yaml:"region" json:"region"`
	Model        string         `yaml:"model" json:"model"`
	SystemPrompt string         `yaml:"system_prompt" json:"system_prompt"`
	Temperature  float64        `yaml:"temperature" json:"temperature"`
	MaxTokens    int            `yaml:"max_tokens" json:"max_tokens"`
	TopP         float32        `yaml:"top_p" json:"top_p"`
	Extra        map[string]any `yaml:"extra" json:"extra"`
	Timeout      time.Duration  `yaml:"timeout" json:"timeout"`

	// 可选：OpenAI
	Organization    string `yaml:"organization" json:"organization"`
	AzureDeployment string `yaml:"azure_deployment" json:"azure_deployment"`
	APIVersion      string `yaml:"api_version" json:"api_version"`
	APIType         string `yaml:"api_type" json:"api_type"`

	// 可选：百度
	AccessToken string `yaml:"access_token" json:"access_token"`
	BaiduAK     string `yaml:"baidu_ak" json:"baidu_ak"`
	BaiduSK     string `yaml:"baidu_sk" json:"baidu_sk"`
}

// ——— 配置融合 ———
// 以 Agent 默认配置为“底”，运行时覆盖它（req.Config > agent cfg）
func MergeConfig(base *ModelConfig, override *ModelConfig) *ModelConfig {
	out := &ModelConfig{}
	if base != nil {
		*out = *base
	}
	if override == nil {
		return out
	}
	// string fields
	if override.Provider != "" {
		out.Provider = override.Provider
	}
	if override.Endpoint != "" {
		out.Endpoint = override.Endpoint
	}
	if override.APIKey != "" {
		out.APIKey = override.APIKey
	}
	if override.SecretID != "" {
		out.SecretID = override.SecretID
	}
	if override.SecretKey != "" {
		out.SecretKey = override.SecretKey
	}
	if override.Region != "" {
		out.Region = override.Region
	}
	if override.Model != "" {
		out.Model = override.Model
	}
	if override.SystemPrompt != "" {
		out.SystemPrompt = override.SystemPrompt
	}
	if override.Organization != "" {
		out.Organization = override.Organization
	}
	if override.AzureDeployment != "" {
		out.AzureDeployment = override.AzureDeployment
	}
	if override.APIVersion != "" {
		out.APIVersion = override.APIVersion
	}
	if override.APIType != "" {
		out.APIType = override.APIType
	}
	if override.AccessToken != "" {
		out.AccessToken = override.AccessToken
	}
	if override.BaiduAK != "" {
		out.BaiduAK = override.BaiduAK
	}
	if override.BaiduSK != "" {
		out.BaiduSK = override.BaiduSK
	}

	// numeric fields (0 usually means "unspecified" in this project)
	if override.Temperature > 0 {
		out.Temperature = override.Temperature
	}
	if override.MaxTokens > 0 {
		out.MaxTokens = override.MaxTokens
	}
	if override.TopP > 0 {
		out.TopP = override.TopP
	}
	if override.Timeout > 0 {
		out.Timeout = override.Timeout
	}

	// maps
	if len(override.Extra) > 0 {
		if out.Extra == nil {
			out.Extra = map[string]any{}
		}
		for k, v := range override.Extra {
			out.Extra[k] = v
		}
	}
	return out
}
