package agent

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modelparams"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func enrichParameterProfiles(provider string, entries []OnlineCatalogModel) {
	for i := range entries {
		if entries[i].ParameterProfile == nil {
			entries[i].ParameterProfile = modelparams.Known(provider, entries[i].ID)
		}
	}
}

// ModelParameterProfile 只读当前连接的已保存快照；不触发在线抓取，不读其他租户。
func (s *AgentSettingService) ModelParameterProfile(ctx context.Context, env string, tenant *string, provider, model string) *modelparams.Profile {
	reg := catalog.GetGlobalAIRegister()
	provider = reg.CanonicalProvider(provider)
	app, id := splitAppModel(reg, provider, model)
	fallback := modelparams.Known(provider, id)
	if s == nil || s.db == nil || tenant == nil {
		return fallback
	}
	item, err := findOnlineCatalogSetting(ctx, s.settingRepo, *tenant, env, provider, app, false)
	if err != nil {
		return fallback
	}
	snapshot, err := decodeOnlineCatalog(item)
	if err != nil || snapshot == nil {
		return fallback
	}
	conn, err := s.savedCatalogConnection(ctx, env, *tenant, provider)
	if err != nil || snapshot.Fingerprint != connectionFingerprint(conn) {
		return fallback
	}
	for _, entry := range snapshot.Models {
		if entry.ID == id && entry.ParameterProfile != nil {
			return entry.ParameterProfile
		}
	}
	return fallback
}

func (s *AgentSettingService) ValidateLLMParameters(ctx context.Context, env string, tenant *string, provider, model string, params map[string]any) error {
	p := s.ModelParameterProfile(ctx, env, tenant, provider, model)
	if p.MaxOutputTokens > 0 {
		for _, key := range []string{"maxTokens", "max_tokens"} {
			if value, ok := asInt(params[key]); ok && value > p.MaxOutputTokens {
				return &dto.AppError{HTTPCode: 422, Code: "AI_PARAMETER_INVALID", Message: "AI_PARAMETER_INVALID", Details: map[string]any{"parameter": "max_tokens", "reason": "above_model_output_limit"}}
			}
		}
	}
	effort, _ := params["reasoning_effort"].(string)
	if effort != "" && len(p.ReasoningEfforts) > 0 {
		valid := false
		for _, candidate := range p.ReasoningEfforts {
			if candidate == effort {
				valid = true
			}
		}
		if !valid {
			return &dto.AppError{HTTPCode: 422, Code: "AI_PARAMETER_INVALID", Message: "AI_PARAMETER_INVALID", Details: map[string]any{"parameter": "reasoning_effort", "reason": "unsupported_value"}}
		}
	}
	for _, key := range []string{"temperature", "top_p", "topP"} {
		name := key
		if name == "topP" {
			name = "top_p"
		}
		if _, ok := params[key]; ok && (!p.SupportsSampling(effort) || !p.Supports(name)) {
			return &dto.AppError{HTTPCode: 422, Code: "AI_PARAMETER_INVALID", Message: "AI_PARAMETER_INVALID", Details: map[string]any{"parameter": name, "reason": "unsupported_for_reasoning_effort"}}
		}
	}
	return nil
}

type LLMCallParameters struct {
	ReasoningEffort string
	TopP            float32
	TemperatureSet  bool
	TopPSet         bool
}
