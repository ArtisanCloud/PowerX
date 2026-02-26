package apikeycache

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
)

const (
	globalVersionKey = "igw:apikey:cache:version"
	authTTL          = 2 * time.Minute
	permTTL          = 60 * time.Second
)

type AuthSnapshot struct {
	KeyID      uint64 `json:"key_id"`
	TenantUUID string `json:"tenant_uuid"`
	ProfileID  uint64 `json:"profile_id"`
}

type permissionDecision struct {
	Allowed bool `json:"allowed"`
}

func GetAuthSnapshot(ctx context.Context, keyHash string) (*AuthSnapshot, bool, error) {
	store := cache.GetCache()
	keyHash = strings.TrimSpace(keyHash)
	if store == nil || keyHash == "" {
		return nil, false, nil
	}
	version, err := cacheVersion(ctx, store)
	if err != nil {
		return nil, false, err
	}
	raw, err := store.Get(ctx, authCacheKey(version, keyHash))
	if err != nil || len(raw) == 0 {
		return nil, false, err
	}
	var out AuthSnapshot
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false, nil
	}
	return &out, true, nil
}

func SetAuthSnapshot(ctx context.Context, keyHash string, snapshot AuthSnapshot) error {
	store := cache.GetCache()
	keyHash = strings.TrimSpace(keyHash)
	if store == nil || keyHash == "" {
		return nil
	}
	version, err := cacheVersion(ctx, store)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return store.Set(ctx, authCacheKey(version, keyHash), raw, authTTL)
}

func GetPermissionDecision(ctx context.Context, keyHash string, action string, resourceType string, resource string) (bool, bool, error) {
	store := cache.GetCache()
	keyHash = strings.TrimSpace(keyHash)
	if store == nil || keyHash == "" {
		return false, false, nil
	}
	version, err := cacheVersion(ctx, store)
	if err != nil {
		return false, false, err
	}
	raw, err := store.Get(ctx, permissionCacheKey(version, keyHash, action, resourceType, resource))
	if err != nil || len(raw) == 0 {
		return false, false, err
	}
	var out permissionDecision
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, false, nil
	}
	return out.Allowed, true, nil
}

func SetPermissionDecision(ctx context.Context, keyHash string, action string, resourceType string, resource string, allowed bool) error {
	store := cache.GetCache()
	keyHash = strings.TrimSpace(keyHash)
	if store == nil || keyHash == "" {
		return nil
	}
	version, err := cacheVersion(ctx, store)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(permissionDecision{Allowed: allowed})
	if err != nil {
		return err
	}
	return store.Set(ctx, permissionCacheKey(version, keyHash, action, resourceType, resource), raw, permTTL)
}

func InvalidateAll(ctx context.Context) error {
	store := cache.GetCache()
	if store == nil {
		return nil
	}
	_, err := store.Increment(ctx, globalVersionKey, 1)
	return err
}

func cacheVersion(ctx context.Context, store cache.ICache) (int64, error) {
	raw, err := store.Get(ctx, globalVersionKey)
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		if err := store.Set(ctx, globalVersionKey, "1", 0); err != nil {
			return 0, err
		}
		return 1, nil
	}
	value, parseErr := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if parseErr != nil || value <= 0 {
		if err := store.Set(ctx, globalVersionKey, "1", 0); err != nil {
			return 0, err
		}
		return 1, nil
	}
	return value, nil
}

func authCacheKey(version int64, keyHash string) string {
	return fmt.Sprintf("igw:apikey:auth:v%d:%s", version, strings.TrimSpace(keyHash))
}

func permissionCacheKey(version int64, keyHash string, action string, resourceType string, resource string) string {
	return fmt.Sprintf(
		"igw:apikey:perm:v%d:%s:%s:%s:%s",
		version,
		strings.TrimSpace(keyHash),
		strings.ToLower(strings.TrimSpace(action)),
		strings.ToLower(strings.TrimSpace(resourceType)),
		strings.TrimSpace(resource),
	)
}
