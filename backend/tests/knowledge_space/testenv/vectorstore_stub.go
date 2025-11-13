package testenv

import (
	"context"
	"sync"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/google/uuid"
)

// VectorStoreStub 提供简单的内存实现，便于在测试中验证向量写入/清理。
type VectorStoreStub struct {
	mu   sync.Mutex
	data map[uuid.UUID]map[uuid.UUID]vectorstore.VectorRecord
}

// NewVectorStoreStub 构造内存驱动。
func NewVectorStoreStub() *VectorStoreStub {
	return &VectorStoreStub{
		data: make(map[uuid.UUID]map[uuid.UUID]vectorstore.VectorRecord),
	}
}

// Driver returns the stub identifier.
func (s *VectorStoreStub) Driver() string {
	return "stub"
}

// Upsert stores/overwrites chunk vectors per space.
func (s *VectorStoreStub) Upsert(_ context.Context, space uuid.UUID, vectors []vectorstore.VectorRecord) error {
	if len(vectors) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	spaceRecords, ok := s.data[space]
	if !ok {
		spaceRecords = make(map[uuid.UUID]vectorstore.VectorRecord)
		s.data[space] = spaceRecords
	}
	for _, vec := range vectors {
		spaceRecords[vec.ChunkID] = vec
	}
	return nil
}

// DeleteByChunkIDs removes vector entries for specified chunk IDs.
func (s *VectorStoreStub) DeleteByChunkIDs(_ context.Context, space uuid.UUID, chunkIDs []uuid.UUID) error {
	if len(chunkIDs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if spaceRecords, ok := s.data[space]; ok {
		for _, chunkID := range chunkIDs {
			delete(spaceRecords, chunkID)
		}
	}
	return nil
}

// DropSpace purges every record for the space.
func (s *VectorStoreStub) DropSpace(_ context.Context, space uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, space)
	return nil
}

// Query returns an empty match set; retrieval coverage is validated in later stories.
func (s *VectorStoreStub) Query(_ context.Context, _ vectorstore.QueryRequest) (vectorstore.QueryResponse, error) {
	return vectorstore.QueryResponse{}, nil
}

// Health reports stub availability.
func (s *VectorStoreStub) Health(context.Context) error { return nil }

// Close releases resources (noop for stub).
func (s *VectorStoreStub) Close(context.Context) error { return nil }

// Records returns a snapshot of stored vectors for assertions.
func (s *VectorStoreStub) Records(space uuid.UUID) []vectorstore.VectorRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	spaceRecords := s.data[space]
	out := make([]vectorstore.VectorRecord, 0, len(spaceRecords))
	for _, rec := range spaceRecords {
		out = append(out, rec)
	}
	return out
}
