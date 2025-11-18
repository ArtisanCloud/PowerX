package devhotload

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/store"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
)

// Registry coordinates session lifecycle + cache/lock helpers.
type Registry struct {
	store           *store.Store
	redis           redis.Cmdable
	ttl             time.Duration
	maxConcurrent   int
	cleanupInterval time.Duration
	keyPrefix       string
	now             func() time.Time
}

type RegistryOptions struct {
	TTL             time.Duration
	MaxConcurrent   int
	CleanupInterval time.Duration
	KeyPrefix       string
	Now             func() time.Time
}

func NewRegistry(store *store.Store, redis redis.Cmdable, opts RegistryOptions) *Registry {
	if opts.TTL <= 0 {
		opts.TTL = 15 * time.Minute
	}
	if opts.CleanupInterval <= 0 {
		opts.CleanupInterval = time.Minute
	}
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "devhotload"
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Registry{
		store:           store,
		redis:           redis,
		ttl:             opts.TTL,
		maxConcurrent:   opts.MaxConcurrent,
		cleanupInterval: opts.CleanupInterval,
		keyPrefix:       opts.KeyPrefix,
		now:             opts.Now,
	}
}

type RegisterRequest struct {
	PluginID        string
	TenantID        uint64
	DeveloperID     uint64
	BuildHash       string
	EntryPoints     []string
	Manifest        map[string]any
	Metadata        map[string]any
	SandboxEndpoint string
	LogURL          string
	WatchFileLimit  int
}

func (r *Registry) Register(ctx context.Context, req RegisterRequest) (*model.DevHotloadSession, error) {
	if r.store == nil {
		return nil, errors.New("dev hotload store missing")
	}
	if strings.TrimSpace(req.PluginID) == "" || req.TenantID == 0 || req.DeveloperID == 0 {
		return nil, fmt.Errorf("invalid register request")
	}

	if r.maxConcurrent > 0 {
		count, err := r.store.CountActive(ctx)
		if err != nil {
			return nil, err
		}
		if int(count) >= r.maxConcurrent {
			return nil, ErrCapacityReached
		}
	}

	lockKey := sessionLockKey(r.keyPrefix, req.PluginID, req.TenantID)
	if err := r.acquireLock(ctx, lockKey); err != nil {
		if errors.Is(err, ErrSessionConflict) {
			if active, findErr := r.store.FindActiveByPlugin(ctx, req.PluginID, req.TenantID); findErr == nil {
				return nil, newSessionConflictError(active)
			}
		}
		return nil, err
	}
	defer func() {
		// ensure lock released if later operations fail
		if recoverErr := recover(); recoverErr != nil {
			_ = r.releaseLock(context.Background(), lockKey)
			panic(recoverErr)
		}
	}()

	if active, err := r.store.FindActiveByPlugin(ctx, req.PluginID, req.TenantID); err == nil {
		_ = r.releaseLock(ctx, lockKey)
		return nil, newSessionConflictError(active)
	} else if !errors.Is(err, store.ErrNotFound) {
		_ = r.releaseLock(ctx, lockKey)
		return nil, err
	}

	session := &model.DevHotloadSession{
		PluginID:        strings.TrimSpace(req.PluginID),
		TenantID:        req.TenantID,
		DeveloperID:     req.DeveloperID,
		BuildHash:       strings.TrimSpace(req.BuildHash),
		ReloadToken:     generateReloadToken(),
		Status:          model.DevHotloadSessionStatusActive,
		SandboxEndpoint: strings.TrimSpace(req.SandboxEndpoint),
		LogURL:          strings.TrimSpace(req.LogURL),
		WatchFileLimit:  req.WatchFileLimit,
		EntryPoints:     toJSON(req.EntryPoints, []byte("[]")),
		Manifest:        toJSON(req.Manifest, []byte("{}")),
		Metadata:        toJSON(req.Metadata, []byte("{}")),
		ExpiresAt:       r.now().Add(r.ttl),
	}
	if err := r.store.CreateSession(ctx, session); err != nil {
		_ = r.releaseLock(ctx, lockKey)
		return nil, err
	}
	if err := r.store.AppendEvent(ctx, session.UUID, "session.started", map[string]any{
		"pluginId":    session.PluginID,
		"tenantId":    session.TenantID,
		"developerId": session.DeveloperID,
	}); err != nil {
		return nil, err
	}
	cacheKey := sessionCacheKey(r.keyPrefix, session.UUID)
	if r.redis != nil {
		_ = r.redis.Set(ctx, cacheKey, session.ReloadToken, r.ttl).Err()
	}
	return session, nil
}

func (r *Registry) Get(ctx context.Context, id uuid.UUID) (*model.DevHotloadSession, error) {
	session, err := r.store.FindSession(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return session, nil
}

func (r *Registry) VerifyReloadToken(ctx context.Context, id uuid.UUID, token string) error {
	cacheKey := sessionCacheKey(r.keyPrefix, id)
	if r.redis != nil {
		if cached, err := r.redis.Get(ctx, cacheKey).Result(); err == nil {
			if subtleConstantTimeEquals(cached, token) {
				return nil
			}
			return ErrReloadToken
		}
	}
	session, err := r.store.FindSession(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrSessionNotFound
		}
		return err
	}
	if !subtleConstantTimeEquals(session.ReloadToken, token) {
		return ErrReloadToken
	}
	return nil
}

func (r *Registry) Terminate(ctx context.Context, id uuid.UUID, note string) error {
	session, err := r.store.FindSession(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrSessionNotFound
		}
		return err
	}
	if session.Status == model.DevHotloadSessionStatusTerminated {
		return nil
	}
	now := r.now()
	session.Status = model.DevHotloadSessionStatusTerminated
	session.TerminationNote = note
	session.EndedAt = &now
	session.ExpiresAt = now
	if err := r.store.SaveSession(ctx, session); err != nil {
		return err
	}

	if err := r.store.AppendEvent(ctx, session.UUID, "session.terminated", map[string]any{"note": note}); err != nil {
		return err
	}
	cacheKey := sessionCacheKey(r.keyPrefix, session.UUID)
	lockKey := sessionLockKey(r.keyPrefix, session.PluginID, session.TenantID)
	_ = r.releaseLock(ctx, lockKey)
	if r.redis != nil {
		_ = r.redis.Del(ctx, cacheKey).Err()
	}
	return nil
}

func (r *Registry) RecordReload(ctx context.Context, id uuid.UUID, payload any) error {
	return r.store.AppendEvent(ctx, id, "session.reload", payload)
}

func (r *Registry) CleanupExpired(ctx context.Context) (int, error) {
	expired, err := r.store.ListExpired(ctx, r.now())
	if err != nil {
		return 0, err
	}
	count := 0
	for _, session := range expired {
		_ = r.Terminate(ctx, session.UUID, "expired")
		count++
	}
	return count, nil
}

func (r *Registry) acquireLock(ctx context.Context, key string) error {
	if r.redis == nil {
		return nil
	}
	ok, err := r.redis.SetNX(ctx, key, "1", r.ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return ErrSessionConflict
	}
	return nil
}

func (r *Registry) releaseLock(ctx context.Context, key string) error {
	if r.redis == nil {
		return nil
	}
	return r.redis.Del(ctx, key).Err()
}

func sessionLockKey(prefix, pluginID string, tenantID uint64) string {
	return fmt.Sprintf("%s:lock:%s:%d", prefix, pluginID, tenantID)
}

func sessionCacheKey(prefix string, sessionID uuid.UUID) string {
	return fmt.Sprintf("%s:session:%s", prefix, sessionID.String())
}

func toJSON(value any, def []byte) datatypes.JSON {
	if value == nil {
		return datatypes.JSON(def)
	}
	data, err := jsonMarshal(value)
	if err != nil || len(data) == 0 {
		return datatypes.JSON(def)
	}
	return datatypes.JSON(data)
}

func jsonMarshal(value any) ([]byte, error) {
	switch v := value.(type) {
	case datatypes.JSON:
		return v, nil
	case []byte:
		return v, nil
	default:
		return json.Marshal(v)
	}
}

func generateReloadToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return uuid.NewString()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func subtleConstantTimeEquals(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
