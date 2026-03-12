package skills

import (
	"context"
	"encoding/json"
	"strings"

	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const TenantSettingKeySkillSourceAllowlist = "ai.skills.source_allowlist"

type SourcePolicyInput struct {
	TenantUUID string
	Env        string
	AgentID    uint64
	Context    map[string]interface{}
}

type SourcePolicyResolver interface {
	ResolveAllowedSources(ctx context.Context, in SourcePolicyInput) []string
}

type DBSourcePolicyResolver struct {
	tenantSettings *settingrepo.TenantSettingRepository
	agentSettings  *agentrepo.AgentSettingRepository
	defaultSources []string
}

func NewDBSourcePolicyResolver(db *gorm.DB) *DBSourcePolicyResolver {
	if db == nil {
		return nil
	}
	return &DBSourcePolicyResolver{
		tenantSettings: settingrepo.NewTenantSettingRepository(db),
		agentSettings:  agentrepo.NewAgentSettingRepository(db),
		defaultSources: defaultAllowedSources(),
	}
}

func (r *DBSourcePolicyResolver) ResolveAllowedSources(ctx context.Context, in SourcePolicyInput) []string {
	if r == nil {
		return defaultAllowedSources()
	}
	// 1) request context override
	if sources := parseSourcesFromContext(in.Context); len(sources) > 0 {
		return sources
	}
	// 2) agent-level policy
	if in.AgentID > 0 && strings.TrimSpace(in.TenantUUID) != "" && r.agentSettings != nil {
		tenant := strings.TrimSpace(in.TenantUUID)
		env := strings.TrimSpace(in.Env)
		if env == "" {
			env = "default"
		}
		agentSetting, err := r.agentSettings.FindByAgent(ctx, env, &tenant, in.AgentID)
		if err == nil && agentSetting != nil {
			if sources := parseSourcesFromAgentQuota(agentSetting.QuotaPolicy); len(sources) > 0 {
				return sources
			}
		}
	}
	// 3) tenant-level policy
	if strings.TrimSpace(in.TenantUUID) != "" && r.tenantSettings != nil {
		setting, err := r.tenantSettings.GetByTenantAndKey(ctx, strings.TrimSpace(in.TenantUUID), TenantSettingKeySkillSourceAllowlist)
		if err == nil && setting != nil {
			if sources := parseSourcesFromTenantSetting(setting); len(sources) > 0 {
				return sources
			}
		}
	}
	return append([]string(nil), r.defaultSources...)
}

func parseSourcesFromContext(ctxMap map[string]interface{}) []string {
	if len(ctxMap) == 0 {
		return nil
	}
	if raw, ok := ctxMap["skill_source_allowlist"]; ok {
		if parsed := parseSourceRawValue(raw); len(parsed) > 0 {
			return parsed
		}
	}
	if raw, ok := ctxMap["skills_source_allowlist"]; ok {
		if parsed := parseSourceRawValue(raw); len(parsed) > 0 {
			return parsed
		}
	}
	return nil
}

func parseSourcesFromTenantSetting(setting *dbsetting.TenantSetting) []string {
	if setting == nil || len(setting.ValueJSON) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(setting.ValueJSON, &arr); err == nil {
		return normalizeAllowedSources(arr)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(setting.ValueJSON, &obj); err == nil {
		if raw, ok := obj["allowlist"]; ok {
			return parseSourceRawValue(raw)
		}
	}
	return nil
}

func parseSourcesFromAgentQuota(quota datatypes.JSONMap) []string {
	if len(quota) == 0 {
		return nil
	}
	if raw, ok := quota["skill_source_allowlist"]; ok {
		if parsed := parseSourceRawValue(raw); len(parsed) > 0 {
			return parsed
		}
	}
	if skillsRaw, ok := quota["skills"]; ok {
		if skillsMap, ok := skillsRaw.(map[string]interface{}); ok {
			if raw, ok := skillsMap["source_allowlist"]; ok {
				return parseSourceRawValue(raw)
			}
		}
	}
	return nil
}

func parseSourceRawValue(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		return normalizeAllowedSources(v)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return normalizeAllowedSources(out)
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		return normalizeAllowedSources(parts)
	default:
		return nil
	}
}

func normalizeAllowedSources(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"builtin":     {},
		"plugin":      {},
		"third_party": {},
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		v := strings.ToLower(strings.TrimSpace(item))
		if v == "" {
			continue
		}
		if _, ok := allowed[v]; !ok {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
