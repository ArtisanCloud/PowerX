package vectorstore

import (
	"context"

	"github.com/google/uuid"
)

// Driver names exposed by built-in implementations.
const (
	DriverPGVector = "pgvector"
	DriverMilvus   = "milvus"
	DriverPinecone = "pinecone"
)

// Store 抽象向量存储的核心能力，支持批量写入、删除、查询与空间隔离。
type Store interface {
	Driver() string
	Upsert(ctx context.Context, space uuid.UUID, vectors []VectorRecord) error
	DeleteByChunkIDs(ctx context.Context, space uuid.UUID, chunkIDs []uuid.UUID) error
	DropSpace(ctx context.Context, space uuid.UUID) error
	Query(ctx context.Context, req QueryRequest) (QueryResponse, error)
	Health(ctx context.Context) error
	Close(ctx context.Context) error
}

// VectorRecord 描述需要持久化的单条向量信息。
type VectorRecord struct {
	ChunkID   uuid.UUID
	Embedding []float32
	Metadata  map[string]any
}

// QueryRequest 定义向量检索参数。
type QueryRequest struct {
	SpaceID   uuid.UUID
	Embedding []float32
	TopK      int
	Filters   map[string]string
	MinScore  float64
}

// QueryMatch 表示一次相似度查询的命中结果。
type QueryMatch struct {
	ChunkID  uuid.UUID
	Score    float64
	Metadata map[string]any
}

// QueryResponse 聚合返回的命中列表。
type QueryResponse struct {
	Matches []QueryMatch
}
