package context_snapshot

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// Citation represents a stored QA citation delta.
type Citation struct {
	ChunkID     string   `json:"chunkId"`
	SpaceID     string   `json:"spaceId"`
	Status      string   `json:"status"`
	Citations   []string `json:"citations"`
	SourceType  string   `json:"sourceType"`
	Confidence  float64  `json:"confidence"`
	DeltaReason string   `json:"deltaReason"`
}

// Store keeps conversation memory snapshots per (tenant, session).
type Store struct {
	mu       sync.RWMutex
	sessions map[string][]Citation
}

// NewStore returns an in-memory snapshot store.
func NewStore() *Store {
	return &Store{
		sessions: make(map[string][]Citation),
	}
}

func key(tenant uuid.UUID, sessionID string) string {
	return tenant.String() + ":" + sessionID
}

// Upsert merges citations for the given session and returns the latest view.
func (s *Store) Upsert(_ context.Context, tenant uuid.UUID, sessionID string, updates []Citation) []Citation {
	if tenant == uuid.Nil || sessionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(tenant, sessionID)
	if _, ok := s.sessions[k]; !ok {
		s.sessions[k] = []Citation{}
	}
	if len(updates) > 0 {
		merged := make(map[string]Citation)
		for _, existing := range s.sessions[k] {
			merged[existing.ChunkID] = existing
		}
		for _, upd := range updates {
			if upd.ChunkID == "" {
				continue
			}
			merged[upd.ChunkID] = upd
		}
		s.sessions[k] = make([]Citation, 0, len(merged))
		for _, v := range merged {
			s.sessions[k] = append(s.sessions[k], v)
		}
		sort.SliceStable(s.sessions[k], func(i, j int) bool {
			return s.sessions[k][i].ChunkID < s.sessions[k][j].ChunkID
		})
	}
	copied := make([]Citation, len(s.sessions[k]))
	copy(copied, s.sessions[k])
	return copied
}

// Snapshot returns the stored citations for the requested session.
func (s *Store) Snapshot(_ context.Context, tenant uuid.UUID, sessionID string) []Citation {
	if tenant == uuid.Nil || sessionID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := key(tenant, sessionID)
	if entries, ok := s.sessions[k]; ok {
		copied := make([]Citation, len(entries))
		copy(copied, entries)
		return copied
	}
	return nil
}
