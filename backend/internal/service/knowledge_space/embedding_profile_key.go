package knowledge_space

import (
	"fmt"
	"strings"
)

// ParseEmbeddingProfileKey parses a logical key in the form:
// - "provider/model"
// - "provider:model"
func ParseEmbeddingProfileKey(key string) (provider string, model string, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("embeddingProfileKey 不能为空")
	}
	var parts []string
	if strings.Contains(key, "/") {
		parts = strings.SplitN(key, "/", 2)
	} else if strings.Contains(key, ":") {
		parts = strings.SplitN(key, ":", 2)
	} else {
		return "", "", fmt.Errorf("embeddingProfileKey 格式错误（期望 provider/model 或 provider:model），got=%q", key)
	}
	provider = strings.ToLower(strings.TrimSpace(parts[0]))
	model = strings.TrimSpace(parts[1])
	if provider == "" || model == "" {
		return "", "", fmt.Errorf("embeddingProfileKey 格式错误（provider/model）")
	}
	return provider, model, nil
}

