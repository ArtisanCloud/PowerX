package tts

import (
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/qwen"
)

func NewClient(provider string) (Client, error) {
	switch normalize(provider) {
	case "qwen", "dashscope", "tongyi":
		return qwen.NewTTSClient(), nil
	default:
		return nil, fmt.Errorf("unknown tts provider: %s", provider)
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
