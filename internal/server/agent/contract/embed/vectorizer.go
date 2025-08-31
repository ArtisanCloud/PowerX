// services/agent/intent/embed/vectorizer.go

package embed

import "context"

type Vectorizer interface {
	// 单批多文本 -> 向量
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
