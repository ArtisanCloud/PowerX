package toolstore

import (
	"encoding/json"
	"fmt"
	"time"
)

// SelectorPolicySnapshot 用于缓存 Selector 运行所需的策略结果。
type SelectorPolicySnapshot struct {
	TenantID           string                        `json:"tenant_id"`
	CapabilitiesHash   string                        `json:"capabilities_hash"`
	IntentMappings     IntentMapping                 `json:"intent_mappings"`
	PreferMatrix       map[string]ProtocolPreference `json:"prefer_matrix"`
	RateLimitOverrides map[string]RateLimitOverride  `json:"rate_limit_overrides,omitempty"`
	GeneratedAt        time.Time                     `json:"generated_at"`
	Metadata           map[string]string             `json:"metadata,omitempty"`
}

// IntentMapping 表示 intent → tool_scope → capability_id 的映射。
type IntentMapping map[string]map[string]string

// ProtocolPreference 描述能力在不同协议下的优先级。
type ProtocolPreference struct {
	Prefer               string   `json:"prefer"`
	Fallback             []string `json:"fallback,omitempty"`
	RollbackCapabilityID string   `json:"rollback_capability_id,omitempty"`
}

// RateLimitOverride 描述针对某能力/租户的限流覆盖。
type RateLimitOverride struct {
	Limit         uint64 `json:"limit"`
	Burst         uint64 `json:"burst"`
	WindowSeconds int    `json:"window_seconds"`
	Scope         string `json:"scope,omitempty"`
}

// RedisKey 返回在 Redis 中存储该快照的 key。
func (s SelectorPolicySnapshot) RedisKey(prefix string) string {
	if prefix == "" {
		prefix = "capability_registry:selector_policy"
	}
	return fmt.Sprintf("%s:%s:%s", prefix, s.TenantID, s.CapabilitiesHash)
}

// MarshalBinary 实现 redis.BinaryMarshaler，方便直接写入缓存。
func (s SelectorPolicySnapshot) MarshalBinary() ([]byte, error) {
	return json.Marshal(s)
}

// UnmarshalBinary 实现 redis.BinaryUnmarshaler，便于从缓存读取。
func (s *SelectorPolicySnapshot) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, s)
}

// IsValid 用于快速校验快照是否匹配当前 Registry 版本。
func (s SelectorPolicySnapshot) IsValid(expectedHash string) bool {
	return s.TenantID != "" && s.CapabilitiesHash != "" && s.CapabilitiesHash == expectedHash
}
