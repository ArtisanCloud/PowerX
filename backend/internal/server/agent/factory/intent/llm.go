package intent

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/intent/llm"
	"time"
)

func NewClassifierFromConfig(llmCfg config.ClassifierConfig) (llm.Classifier, error) {
	switch llmCfg.Provider {
	case "openai":
		if llmCfg.APIKey == "" {
			return nil, fmt.Errorf("openai api_key is empty")
		}
		return &llm.OpenAIClassifier{
			BaseURL: llmCfg.Endpoint, // https://api.openai.com/v1
			APIKey:  llmCfg.APIKey,
			Model:   llmCfg.Model, // gpt-4o-mini 等
			Timeout: 15 * time.Second,
		}, nil
	case "ollama":
		return &llm.OllamaClassifier{
			BaseURL: llmCfg.Endpoint, // http://localhost:11434
			Model:   llmCfg.Model,    // qwen/llama instruct
			Timeout: 20 * time.Second,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", llmCfg.Provider)
	}
}
