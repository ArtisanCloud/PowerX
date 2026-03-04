package image

import (
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/google"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/openai"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/volcengine"
)

// NewClient returns provider-specific Image driver.
func NewClient(provider string) (Client, error) {
	switch normalize(provider) {
	case "openai":
		return openai.NewImageClient(provider), nil
	case "google", "gemini":
		return google.NewImageClient(provider), nil
	case "volcengine", "volc", "volcano", "bytedance":
		return volcengine.NewImageClient(provider), nil
	default:
		return nil, fmt.Errorf("unknown image provider: %s", provider)
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
