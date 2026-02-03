package vlm

import (
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/google"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/hunyuan"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/openai"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/qwen"
)

// NewClient returns provider-specific VLM driver (placeholder for now).
func NewClient(provider string) (Client, error) {
	switch normalize(provider) {
	case "openai":
		return openai.NewVLMClient(), nil
	case "google", "gemini":
		return google.NewVLMClient(), nil
	case "hunyuan":
		return hunyuan.NewVLMClient(), nil
	case "qwen":
		return qwen.NewVLMClient(), nil
	default:
		return nil, fmt.Errorf("unknown vlm provider: %s", provider)
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
