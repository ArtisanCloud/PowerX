package intent

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/services/agent/config"
	"github.com/ArtisanCloud/PowerX/services/agent/contract/embed"
	embed2 "github.com/ArtisanCloud/PowerX/services/agent/intent/embed"
	"time"
)

func NewVectorizerFromConfig(llm config.EmbeddingConfig) (embed.Vectorizer, error) {
	switch llm.Provider {
	case "openai":
		if llm.APIKey == "" {
			return nil, fmt.Errorf("openai api_key is empty")
		}
		return &embed2.OpenAIEmbedder{
			BaseURL:  llm.Endpoint, // e.g. https://api.openai.com/v1
			APIKey:   llm.APIKey,
			Model:    llm.Model, // e.g. text-embedding-3-small
			Timeout:  15 * time.Second,
			MaxBatch: 128,
		}, nil
	case "ollama":
		return &embed2.OllamaEmbedder{
			BaseURL:  llm.Endpoint, // e.g. http://localhost:11434
			Model:    llm.Model,    // e.g. bge-m3
			Timeout:  15 * time.Second,
			MaxBatch: 128,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", llm.Provider)
	}
}
