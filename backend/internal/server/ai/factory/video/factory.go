package video

import (
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/volcengine"
)

func NewClient(provider string) (Client, error) {
	switch normalize(provider) {
	case "volcengine", "volc", "volcano", "bytedance":
		return volcengine.NewVideoClient(provider), nil
	default:
		return nil, fmt.Errorf("unknown video provider: %s", provider)
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
