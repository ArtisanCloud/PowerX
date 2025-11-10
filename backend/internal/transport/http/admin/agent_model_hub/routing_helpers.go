package agentmodelhub

import (
	"encoding/json"
	"strings"
	"time"

	modelrouting "github.com/ArtisanCloud/PowerX/internal/service/model_routing"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	"gorm.io/datatypes"
)

func buildPolicyDTO(policy *model.RoutingPolicy) map[string]any {
	if policy == nil {
		return map[string]any{}
	}
	return map[string]any{
		"policyId":           policy.UUID.String(),
		"tenantScope":        policy.TenantScope,
		"version":            policy.Version,
		"status":             policy.Status,
		"env":                policy.Env,
		"rules":              decodeJSON(policy.Rules, []any{}),
		"fallbackChain":      decodeJSON(policy.FallbackChain, []any{}),
		"safeModeThresholds": mapFromJSON(policy.SafeModeThresholds),
		"approvalRecord":     mapFromJSON(policy.ApprovalRecord),
	}
}

func decodeJSON(raw datatypes.JSON, def any) any {
	if len(raw) == 0 {
		return def
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return def
	}
	return v
}

func mapFromJSON(raw datatypes.JSONMap) map[string]any {
	if raw == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	return out
}

func pickPrimaryAndFallback(policy *model.RoutingPolicy) (string, []string) {
	primary := ""
	var fallback []string
	var rules []struct {
		Candidates []struct {
			ProviderID string `json:"providerId"`
		} `json:"candidates"`
	}
	_ = json.Unmarshal(policy.Rules, &rules)
	if len(rules) > 0 && len(rules[0].Candidates) > 0 {
		primary = strings.TrimSpace(rules[0].Candidates[0].ProviderID)
	}
	_ = json.Unmarshal(policy.FallbackChain, &fallback)
	return primary, fallback
}

func buildSafeModeDTO(state *modelrouting.SafeModeState) map[string]any {
	if state == nil {
		return map[string]any{}
	}
	dto := map[string]any{
		"tenantScope": state.TenantScope,
		"env":         state.Env,
		"enabled":     state.Enabled,
		"reason":      state.Reason,
		"actor":       state.Actor,
	}
	if !state.UpdatedAt.IsZero() {
		dto["updatedAt"] = state.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if state.ExpiresAt != nil && !state.ExpiresAt.IsZero() {
		dto["expiresAt"] = state.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return dto
}
