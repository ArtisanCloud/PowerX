package hash

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

// HashEmbedder 提供“零依赖”的本地向量化兜底：
// - 纯确定性 hash → float32 向量（不具备语义能力）
// - 仅用于开发/演示/联调，生产环境应替换为真实 embedding 模型
type HashEmbedder struct {
	Dim int
}

func (e *HashEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	_ = ctx
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	dim := e.Dim
	if dim <= 0 {
		dim = 1536
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		sum := sha256.Sum256([]byte(t))
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			offset := (j * 4) % len(sum)
			u := binary.BigEndian.Uint32(sum[offset : offset+4])
			vec[j] = float32(u%10_000) / 10_000.0
		}
		out[i] = vec
	}
	return out, nil
}
