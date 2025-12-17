package capability_registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PolicyGenerator 根据 Registry 数据生成 SelectorPolicySnapshot 并可选写入缓存。
type PolicyGenerator struct {
	records *repo.CapabilityRecordRepository
	cache   *CacheManager
	now     func() time.Time
}

// SelectorPolicySnapshot 描述写入缓存的策略视图。
type SelectorPolicySnapshot struct {
	TenantID           string                        `json:"tenant_id"`
	CapabilitiesHash   string                        `json:"capabilities_hash"`
	IntentMappings     map[string]map[string]string  `json:"intent_mappings"`
	PreferMatrix       map[string]ProtocolPreference `json:"prefer_matrix"`
	RateLimitOverrides map[string]RateLimitOverride  `json:"rate_limit_overrides,omitempty"`
	GeneratedAt        time.Time                     `json:"generated_at"`
	Metadata           map[string]string             `json:"metadata,omitempty"`
}

// ProtocolPreference 描述协议优先级。
type ProtocolPreference struct {
	Prefer               string   `json:"prefer"`
	Fallback             []string `json:"fallback,omitempty"`
	RollbackCapabilityID string   `json:"rollback_capability_id,omitempty"`
}

// RateLimitOverride 描述限流覆盖。
type RateLimitOverride struct {
	Limit         uint64 `json:"limit"`
	Burst         uint64 `json:"burst"`
	WindowSeconds int    `json:"window_seconds"`
	Scope         string `json:"scope,omitempty"`
}

// PolicyGeneratorOptions 配置生成器依赖。
type PolicyGeneratorOptions struct {
	DB         *gorm.DB
	RecordRepo *repo.CapabilityRecordRepository
	Cache      *CacheManager
	Clock      func() time.Time
}

// GeneratePolicyInput 描述快照生成所需输入。
type GeneratePolicyInput struct {
	TenantUUID   string
	ToolGrantIDs []string
	SkipCache    bool
}

var (
	errPolicyTenantRequired = errors.New("tenant uuid is required")
	errPolicyNoCapability   = errors.New("no published capabilities for tenant")
)

// NewPolicyGenerator 构建策略快照生成器。
func NewPolicyGenerator(opts PolicyGeneratorOptions) *PolicyGenerator {
	repository := opts.RecordRepo
	if repository == nil {
		if opts.DB == nil {
			panic("policy generator requires DB when repository is nil")
		}
		repository = repo.NewCapabilityRecordRepository(opts.DB, nil)
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &PolicyGenerator{
		records: repository,
		cache:   opts.Cache,
		now:     clock,
	}
}

// GenerateSnapshot 为租户生成 SelectorPolicySnapshot。
func (g *PolicyGenerator) GenerateSnapshot(ctx context.Context, in GeneratePolicyInput) (SelectorPolicySnapshot, error) {
	var snapshot SelectorPolicySnapshot
	tenant := strings.TrimSpace(in.TenantUUID)
	if tenant == "" {
		return snapshot, errPolicyTenantRequired
	}

	records, err := g.records.List(ctx, repo.CapabilityRecordFilter{
		Status: []string{"published"},
	})
	if err != nil {
		return snapshot, err
	}

	filtered := filterCapabilities(records, tenant, in.ToolGrantIDs)
	if len(filtered) == 0 {
		return snapshot, fmt.Errorf("%w: %s", errPolicyNoCapability, tenant)
	}

	sort.Slice(filtered, func(i, j int) bool {
		left := strings.ToLower(filtered[i].CapabilityID)
		right := strings.ToLower(filtered[j].CapabilityID)
		return left < right
	})

	intentMappings := make(map[string]map[string]string)
	preferMatrix := make(map[string]ProtocolPreference, len(filtered))
	hashBuilder := sha256.New()

	for i := range filtered {
		record := filtered[i]
		intents := stringListFromJSON(record.Intents, defaultIntentKey)
		scopes := stringListFromJSON(record.ToolScope, defaultScopeKey)
		for _, intent := range intents {
			scopeMap := intentMappings[intent]
			if scopeMap == nil {
				scopeMap = make(map[string]string)
				intentMappings[intent] = scopeMap
			}
			for _, scope := range scopes {
				scopeMap[scope] = record.CapabilityID
			}
		}

		policy := parseCapabilityPolicy(record.Policy)
		preferMatrix[record.CapabilityID] = buildProtocolPreference(policy)

		hashBuilder.Write([]byte(strings.ToLower(record.CapabilityID)))
		hashBuilder.Write([]byte(":"))
		hashBuilder.Write([]byte(record.CapabilitiesHash))
		hashBuilder.Write([]byte(";"))
	}

	snapshot = SelectorPolicySnapshot{
		TenantID:         tenant,
		CapabilitiesHash: hex.EncodeToString(hashBuilder.Sum(nil)),
		IntentMappings:   intentMappings,
		PreferMatrix:     preferMatrix,
		GeneratedAt:      g.now().UTC(),
	}

	if g.cache != nil && !in.SkipCache {
		if err := g.cache.CachePolicySnapshot(ctx, snapshot.CapabilitiesHash, snapshot); err != nil {
			return snapshot, err
		}
		_ = g.cache.Broadcast(ctx, CacheBroadcastMessage{
			Event:            "selector.policy.generated",
			CapabilitiesHash: snapshot.CapabilitiesHash,
			PolicyHash:       snapshot.CapabilitiesHash,
			TenantUUID:       tenant,
			Timestamp:        snapshot.GeneratedAt,
		})
	}

	return snapshot, nil
}

func filterCapabilities(records []models.CapabilityRecord, tenant string, grants []string) []models.CapabilityRecord {
	result := make([]models.CapabilityRecord, 0, len(records))
	for _, record := range records {
		policy := parseCapabilityPolicy(record.Policy)
		if !policy.AllowsTenant(tenant) {
			continue
		}
		if len(grants) > 0 && !policy.AllowsToolGrants(grants) {
			continue
		}
		if !strings.EqualFold(record.Status, "published") {
			continue
		}
		result = append(result, record)
	}
	return result
}

func buildProtocolPreference(policy capabilityPolicy) ProtocolPreference {
	prefer := strings.TrimSpace(policy.Prefer)
	if prefer == "" {
		prefer = "mcp"
	}
	fallback := sanitizeList(policy.Fallback)
	return ProtocolPreference{
		Prefer:               prefer,
		Fallback:             fallback,
		RollbackCapabilityID: strings.TrimSpace(policy.RollbackCapabilityID),
	}
}

const (
	defaultIntentKey = "*"
	defaultScopeKey  = "default"
)

func stringListFromJSON(raw datatypes.JSON, fallback string) []string {
	values := make([]string, 0)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			values = nil
		}
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	if len(normalized) == 0 && fallback != "" {
		return []string{fallback}
	}
	return dedupStrings(normalized)
}

func sanitizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
