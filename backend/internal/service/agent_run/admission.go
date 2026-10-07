package agent_run

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// Admit binds one tenant/environment/session/idempotency key to one stable
// Run ID. A retry may carry a new trace ID but receives the original Run.
// The caller must durably record a low-frequency admission anchor before
// replying to the client; Redis alone cannot recover an acknowledged Run
// after data loss beyond its configured RPO.
func (s *RedisStore) Admit(ctx context.Context, initial Snapshot, idempotencyKey string) (Snapshot, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" || len(key) > 256 || strings.TrimSpace(initial.SessionID) == "" ||
		strings.TrimSpace(initial.MessageID) == "" || strings.TrimSpace(initial.TraceID) == "" {
		return Snapshot{}, ErrInvalid
	}
	// Derivation avoids a cross-slot transaction between a global idempotency
	// index and the per-Run state/event keys in Redis Cluster.
	scope := Snapshot{TenantUUID: initial.TenantUUID, Env: initial.Env, RunID: uuid.NewString()}
	if !validScope(scope) {
		return Snapshot{}, ErrInvalid
	}
	name := initial.TenantUUID + "\x00" + initial.Env + "\x00" + initial.SessionID + "\x00" + key
	initial.RunID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("powerx-agent-run\x00"+name)).String()
	existing, err := s.Get(ctx, initial.TenantUUID, initial.Env, initial.RunID)
	if err == nil {
		if existing.SessionID != initial.SessionID || existing.MessageID != initial.MessageID {
			return Snapshot{}, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Snapshot{}, err
	}
	created, err := s.Create(ctx, initial)
	if !errors.Is(err, ErrConflict) {
		return created, err
	}
	// A competing same-key request may have created the original trace first.
	existing, err = s.Get(ctx, initial.TenantUUID, initial.Env, initial.RunID)
	if err == nil && existing.SessionID == initial.SessionID && existing.MessageID == initial.MessageID {
		return existing, nil
	}
	return Snapshot{}, ErrConflict
}
