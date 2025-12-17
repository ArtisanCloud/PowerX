package toolstore

import capregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"

// ToRegistrySnapshot converts a ToolStore snapshot to capability registry format.
func (s SelectorPolicySnapshot) ToRegistrySnapshot() capregistry.SelectorPolicySnapshot {
	return capregistry.SelectorPolicySnapshot{
		TenantID:           s.TenantID,
		CapabilitiesHash:   s.CapabilitiesHash,
		IntentMappings:     toGenericIntentMap(s.IntentMappings),
		PreferMatrix:       toGenericPreferMatrix(s.PreferMatrix),
		RateLimitOverrides: toGenericOverrides(s.RateLimitOverrides),
		GeneratedAt:        s.GeneratedAt,
		Metadata:           copyStringMap(s.Metadata),
	}
}
