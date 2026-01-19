package context_snapshot

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
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

	redis      *redis.Client
	keyPrefix  string
	traceTTL   time.Duration
	snapshotTTL time.Duration
}

type Options struct {
	Redis       *redis.Client
	KeyPrefix   string
	SnapshotTTL time.Duration
	TraceTTL    time.Duration
}

// NewStore returns a snapshot store (Redis-backed when configured, otherwise in-memory).
func NewStore(opts ...Options) *Store {
	var cfg Options
	if len(opts) > 0 {
		cfg = opts[0]
	}
	prefix := strings.TrimSpace(cfg.KeyPrefix)
	if prefix == "" {
		prefix = "qa:snapshot"
	}
	snapshotTTL := cfg.SnapshotTTL
	if snapshotTTL <= 0 {
		snapshotTTL = 24 * time.Hour
	}
	traceTTL := cfg.TraceTTL
	if traceTTL <= 0 {
		traceTTL = 48 * time.Hour
	}
	return &Store{
		sessions: make(map[string][]Citation),
		redis:      cfg.Redis,
		keyPrefix:  prefix,
		snapshotTTL: snapshotTTL,
		traceTTL:   traceTTL,
	}
}

func key(tenant uuid.UUID, sessionID string) string {
	return tenant.String() + ":" + sessionID
}

func (s *Store) snapshotKey(tenant uuid.UUID, sessionID string) string {
	return s.keyPrefix + ":session:" + key(tenant, sessionID)
}

func (s *Store) traceKey(traceID string) string {
	return s.keyPrefix + ":trace:" + strings.TrimSpace(traceID)
}

// Upsert merges citations for the given session and returns the latest view.
func (s *Store) Upsert(ctx context.Context, tenant uuid.UUID, sessionID string, updates []Citation, traceID ...string) []Citation {
	if tenant == uuid.Nil || sessionID == "" {
		return nil
	}

	if s.redis != nil {
		return s.upsertRedis(ctx, tenant, sessionID, updates, traceID...)
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

func (s *Store) upsertRedis(ctx context.Context, tenant uuid.UUID, sessionID string, updates []Citation, traceID ...string) []Citation {
	snapshotKey := s.snapshotKey(tenant, sessionID)

	var existing []Citation
	if raw, err := s.redis.Get(ctx, snapshotKey).Bytes(); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &existing)
	}

	merged := make(map[string]Citation, len(existing)+len(updates))
	for _, item := range existing {
		if item.ChunkID == "" {
			continue
		}
		merged[item.ChunkID] = item
	}
	for _, upd := range updates {
		if upd.ChunkID == "" {
			continue
		}
		merged[upd.ChunkID] = upd
	}

	result := make([]Citation, 0, len(merged))
	for _, v := range merged {
		result = append(result, v)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].ChunkID < result[j].ChunkID })

	if payload, err := json.Marshal(result); err == nil {
		_ = s.redis.Set(ctx, snapshotKey, payload, s.snapshotTTL).Err()
	}

	if len(traceID) > 0 {
		trace := strings.TrimSpace(traceID[0])
		if trace != "" {
			_ = s.redis.Set(ctx, s.traceKey(trace), snapshotKey, s.traceTTL).Err()
		}
	}

	return result
}

// Snapshot returns the stored citations for the requested session.
func (s *Store) Snapshot(ctx context.Context, tenant uuid.UUID, sessionID string) []Citation {
	if tenant == uuid.Nil || sessionID == "" {
		return nil
	}
	if s.redis != nil {
		raw, err := s.redis.Get(ctx, s.snapshotKey(tenant, sessionID)).Bytes()
		if err != nil || len(raw) == 0 {
			return nil
		}
		var out []Citation
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil
		}
		return out
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

// SnapshotByTrace resolves a trace_id -> session snapshot mapping when available.
func (s *Store) SnapshotByTrace(ctx context.Context, tenant uuid.UUID, traceID string) []Citation {
	if tenant == uuid.Nil || strings.TrimSpace(traceID) == "" {
		return nil
	}
	if s.redis == nil {
		return nil
	}
	key, err := s.redis.Get(ctx, s.traceKey(traceID)).Result()
	if err != nil || strings.TrimSpace(key) == "" {
		return nil
	}
	raw, err := s.redis.Get(ctx, key).Bytes()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var out []Citation
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
