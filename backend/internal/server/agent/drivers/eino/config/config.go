package config

import aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"

// 仅保留类型别名，避免 agent driver 与 ai driver 的 ModelConfig 类型分裂。
type ModelConfig = aiconfig.ModelConfig

// 直接复用 AI 层 MergeConfig。
func MergeConfig(base *ModelConfig, override *ModelConfig) *ModelConfig {
	return aiconfig.MergeConfig(base, override)
}
