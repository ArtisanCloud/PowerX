package intent

import (
	"fmt"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract/embed"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/baidu"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/hash"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/huggingface"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/ollama"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/openai"
)

func NewVectorizerFromConfig(llm config.EmbeddingConfig) (embed.Vectorizer, error) {
	switch llm.Provider {
	case "", "none", "disabled":
		return nil, nil
	case "hash", "hash32", "local_hash":
		dim := llm.Dim
		if dim <= 0 {
			dim = 1536
		}
		return &hash.HashEmbedder{Dim: dim}, nil
	case "huggingface", "hf":
		if llm.APIKey == "" {
			return nil, fmt.Errorf("huggingface api_key is empty")
		}
		return &huggingface.HuggingFaceEmbedder{
			BaseURL:  llm.Endpoint,
			APIKey:   llm.APIKey,
			Model:    llm.Model,
			Timeout:  20 * time.Second,
			MaxBatch: 64,
		}, nil
	case "baidu", "qianfan":
		if llm.APIKey == "" {
			return nil, fmt.Errorf("baidu api_key is empty")
		}
		return &baidu.BaiduQianfanEmbedder{
			BaseURL:  llm.Endpoint,
			APIKey:   llm.APIKey,
			Model:    llm.Model,
			Timeout:  20 * time.Second,
			MaxBatch: 64,
		}, nil
	case "sentence_transformers", "sentence-transformers", "sbert":
		// 无官方统一 HTTP 协议；若你本地已部署 OpenAI-compatible embeddings 网关，请改用 provider=openai_compatible。
		return nil, fmt.Errorf("sentence-transformers embedding 未提供内置直连实现，请使用 OpenAI-compatible 网关（provider=openai_compatible）或在后端补充该 provider 的 HTTP 协议适配")
	case "openai_compatible", "openai-compatible", "openai_compat":
		return &openai.OpenAIEmbedder{
			BaseURL:  llm.Endpoint, // e.g. http://localhost:8080/v1
			APIKey:   llm.APIKey,   // optional for local gateways
			Model:    llm.Model,
			Timeout:  60 * time.Second,
			MaxBatch: llm.MaxBatch,
		}, nil
	case "openai":
		return &openai.OpenAIEmbedder{
			BaseURL:  llm.Endpoint, // e.g. https://api.openai.com/v1
			APIKey:   llm.APIKey,
			Model:    llm.Model, // e.g. text-embedding-3-small
			Timeout:  60 * time.Second,
			MaxBatch: llm.MaxBatch,
		}, nil
	case "ollama":
		return &ollama.OllamaEmbedder{
			BaseURL:  llm.Endpoint, // e.g. http://127.0.0.1:11434
			Model:    llm.Model,    // e.g. bge-m3
			Timeout:  60 * time.Second,
			MaxBatch: llm.MaxBatch,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", llm.Provider)
	}
}
