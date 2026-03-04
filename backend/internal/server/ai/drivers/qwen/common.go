package qwen

import (
	"errors"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
)

const defaultQwenEndpoint = "https://dashscope.aliyuncs.com/compatible-mode/v1"

func modelConfigFromRuntime(runtime map[string]any) (*config.ModelConfig, error) {
	if runtime == nil {
		return nil, errors.New("qwen runtime config missing")
	}
	if raw, ok := runtime["config"]; ok {
		switch v := raw.(type) {
		case *config.ModelConfig:
			return v, nil
		case config.ModelConfig:
			return &v, nil
		}
	}
	if raw, ok := runtime["model_config"]; ok {
		switch v := raw.(type) {
		case *config.ModelConfig:
			return v, nil
		case config.ModelConfig:
			return &v, nil
		}
	}
	return nil, errors.New("qwen runtime config invalid")
}

func resolveEndpoint(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return defaultQwenEndpoint
	}
	return strings.TrimRight(base, "/")
}

func resolveModel(model string) string {
	raw := strings.TrimSpace(model)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 2)
		return strings.TrimSpace(parts[1])
	}
	return raw
}
