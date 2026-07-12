package agent_authz

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	effectivePermissionsCacheVersion = "v2"
	effectivePermissionsDefaultVer   = int64(1)
)

func (s *Service) getCachedEffectivePermissions(ctx context.Context, env, tenantUUID, userUUID, memberUUID string, memberID uint64, isRoot bool, agentUUID uuid.UUID) (EffectivePermissionsResult, bool, error) {
	if s == nil || s.cache == nil {
		return EffectivePermissionsResult{}, false, nil
	}
	key, err := s.effectivePermissionsCacheKey(ctx, env, tenantUUID, userUUID, memberUUID, memberID, isRoot, agentUUID)
	if err != nil {
		return EffectivePermissionsResult{}, false, err
	}
	raw, err := s.cache.Get(ctx, key)
	if err != nil {
		return EffectivePermissionsResult{}, false, err
	}
	if len(raw) == 0 {
		return EffectivePermissionsResult{}, false, nil
	}
	var out EffectivePermissionsResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return EffectivePermissionsResult{}, false, fmt.Errorf("decode agent effective permissions cache: %w", err)
	}
	return out, true, nil
}

func (s *Service) setCachedEffectivePermissions(ctx context.Context, env, tenantUUID, userUUID, memberUUID string, memberID uint64, isRoot bool, agentUUID uuid.UUID, result EffectivePermissionsResult) error {
	if s == nil || s.cache == nil {
		return nil
	}
	key, err := s.effectivePermissionsCacheKey(ctx, env, tenantUUID, userUUID, memberUUID, memberID, isRoot, agentUUID)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.cache.Set(ctx, key, raw, s.cacheTTL)
}

func (s *Service) invalidateAgentEffectivePermissionsCache(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID) error {
	if s == nil || s.cache == nil {
		return nil
	}
	_, err := s.cache.Increment(ctx, agentEffectivePermissionsVersionKey(env, tenantUUID, agentUUID), 1)
	return err
}

func (s *Service) effectivePermissionsCacheKey(ctx context.Context, env, tenantUUID, userUUID, memberUUID string, memberID uint64, isRoot bool, agentUUID uuid.UUID) (string, error) {
	agentVersion, err := s.effectivePermissionsVersion(ctx, agentEffectivePermissionsVersionKey(env, tenantUUID, agentUUID))
	if err != nil {
		return "", err
	}
	tenantIAMVersion, err := s.effectivePermissionsVersion(ctx, tenantIAMEffectivePermissionsVersionKey(tenantUUID))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"agentauthz:effective:%s:av%d:iv%d:%s:%s:%s:%s:%d:%s:%t",
		effectivePermissionsCacheVersion,
		agentVersion,
		tenantIAMVersion,
		normalizeCachePart(env),
		normalizeCachePart(tenantUUID),
		agentUUID.String(),
		normalizeCachePart(memberUUID),
		memberID,
		normalizeCachePart(userUUID),
		isRoot,
	), nil
}

func (s *Service) effectivePermissionsVersion(ctx context.Context, key string) (int64, error) {
	raw, err := s.cache.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		if err := s.cache.Set(ctx, key, strconv.FormatInt(effectivePermissionsDefaultVer, 10), 0); err != nil {
			return 0, err
		}
		return effectivePermissionsDefaultVer, nil
	}
	version, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || version < effectivePermissionsDefaultVer {
		return 0, fmt.Errorf("invalid agent effective permissions cache version: %s", key)
	}
	return version, nil
}

func agentEffectivePermissionsVersionKey(env, tenantUUID string, agentUUID uuid.UUID) string {
	return fmt.Sprintf(
		"agentauthz:effective:agent-version:%s:%s:%s",
		normalizeCachePart(env),
		normalizeCachePart(tenantUUID),
		agentUUID.String(),
	)
}

func tenantIAMEffectivePermissionsVersionKey(tenantUUID string) string {
	return fmt.Sprintf("agentauthz:effective:iam-version:%s", normalizeCachePart(tenantUUID))
}

func normalizeCachePart(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
