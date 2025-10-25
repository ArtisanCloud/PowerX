package contract

import (
	"context"
)

// 文本向量嵌入
type EmbeddingClient interface {
	Embed(ctx context.Context, in EmbedRequest) (*EmbedResponse, error)

	Cap() ModelCapabilities
	Health(ctx context.Context) error
}

type EmbedRequest struct {
	Texts      []string
	Dimensions int    // 维度（不填则走模型默认）
	Truncate   string // "none" | "start" | "end"
	BatchSize  int    // 驱动可参考以分批
	Runtime    map[string]any
}

type EmbedResponse struct {
	Vectors  [][]float32
	Provider string
	Model    string

	// 可选：用量/时延/追踪
	Usage     map[string]int
	LatencyMS int
	TraceID   string
}
