package knowledge_space

import (
	"context"

	"github.com/google/uuid"
)

// SparseIndex 抽象 BM25/FTS 等稀疏索引能力。
type SparseIndex interface {
	Query(ctx context.Context, req SparseQueryRequest) (SparseQueryResponse, error)
	Health(ctx context.Context) error
}

type SparseQueryRequest struct {
	SpaceID  uuid.UUID
	Query    string
	TopK     int
	Filters  map[string]string
	MinScore float64
}

type SparseQueryMatch struct {
	ChunkID    uuid.UUID
	Score      float64
	Provenance map[string]any
	Metadata   map[string]any
}

type SparseQueryResponse struct {
	Matches []SparseQueryMatch
}
