package agent

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	openaidriver "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/openai"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
)

// ValidateVLMProfile 只校验已发布的视觉操作和参数，不访问供应商、不执行推理。
func ValidateVLMProfile(provider, model string, params map[string]any) error {
	provider = catalog.GetGlobalAIRegister().CanonicalProvider(provider)
	manifest := findModelManifest("vlm", provider, model)
	return validateVLMManifest(provider, manifest, params)
}

func (s *AgentSettingService) ValidateVLMProfile(ctx context.Context, env string, tenant *string, provider, model string, params map[string]any) error {
	manifest, err := s.TenantModelManifest(ctx, env, tenant, "vlm", provider, model)
	if err != nil {
		return err
	}
	return validateVLMManifest(catalog.GetGlobalAIRegister().CanonicalProvider(provider), manifest, params)
}

func validateVLMManifest(provider string, manifest *catalog.ModelManifest, params map[string]any) error {
	if manifest == nil {
		return &dto.AppError{HTTPCode: 422, Code: "AI_MODEL_CAPABILITY_UNSUPPORTED", Message: "AI_MODEL_CAPABILITY_UNSUPPORTED"}
	}
	op, ok := manifest.Operations[catalog.VisionUnderstanding]
	if !ok {
		return &dto.AppError{HTTPCode: 422, Code: "AI_MODEL_CAPABILITY_UNSUPPORTED", Message: "AI_MODEL_CAPABILITY_UNSUPPORTED"}
	}
	if field, reason := catalog.ValidateMediaParameters(op, params); reason != "" {
		return &dto.AppError{HTTPCode: 422, Code: "AI_PARAMETER_INVALID", Message: "AI_PARAMETER_INVALID", Details: map[string]any{"parameter": field, "reason": reason}}
	}
	effective := map[string]any{}
	for key, spec := range op.Parameters {
		if spec.Default != nil {
			effective[key] = spec.Default
		}
	}
	for key, value := range manifest.Defaults {
		if key == "maxTokens" {
			key = "max_tokens"
		}
		if _, ok := op.Parameters[key]; ok {
			effective[key] = value
		}
	}
	for key, value := range params {
		effective[key] = value
	}
	if provider == "openai" {
		return openaidriver.ValidateVLMParameters(manifest.ID, effective)
	}
	return nil
}

func (s *AgentSettingService) RequireVLMStoredCredential(ctx context.Context, env string, tenant *string, provider string) error {
	if tenant == nil || strings.TrimSpace(*tenant) == "" {
		return &dto.AppError{HTTPCode: 401, Code: "AI_UNAUTHORIZED", Message: "AI_UNAUTHORIZED"}
	}
	provider = catalog.GetGlobalAIRegister().CanonicalProvider(provider)
	cred, err := s.credRepo.FindByEquivalentScopeNameProvider(ctx, env, tenant, utils.Slug(env+"-"+provider), provider)
	if err == nil && cred != nil {
		_, key, resolveErr := s.resolveConnFromCredential(ctx, env, tenant, provider, cred, "", "")
		if resolveErr == nil && (!catalog.AuthReqFromCatalog(provider).NeedKey || strings.TrimSpace(key) != "") {
			return nil
		}
	}
	return &dto.AppError{HTTPCode: 409, Code: "AI_MODEL_NOT_CONFIGURED", Message: "AI_MODEL_NOT_CONFIGURED"}
}
