package ai

import (
	"context"
	"errors"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

const (
	LLMActiveModelCapability = "com.corex.ai.llm.active_model.read"
	LLMActiveModelEndpoint   = "core://ai/llm/active-model"
)

// LLMActiveModelInvoker reads the tenant default configured in Model Settings.
type LLMActiveModelInvoker struct{ Service *Service }

// InvokeCoreCapability implements the fixed STS-only default-model operation.
func (i *LLMActiveModelInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	if in.CapabilityID != LLMActiveModelCapability {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if in.Method != "INVOKE" || in.Endpoint != LLMActiveModelEndpoint || len(in.Payload) != 1 || len(in.Query) != 0 || len(in.Context) != 0 {
		return nil, MediaError(400, "AI_TYPED_OPERATION_REQUIRED")
	}
	if _, ok := in.Payload["body"]; !ok {
		return nil, MediaError(400, "AI_TYPED_OPERATION_REQUIRED")
	}
	var op struct {
		Operation string `json:"operation"`
	}
	if err := strictMediaBody(in.Body, &op); err != nil {
		return nil, err
	}
	if op.Operation != "get_active_model" {
		return nil, MediaError(400, "AI_OPERATION_INVALID")
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.Issuer != "powerx-sts" {
		return nil, MediaError(403, "AI_FORBIDDEN")
	}
	if status := reqctx.STSIssuanceStatus(ctx); status != 0 {
		return nil, &cap.DirectGrantError{Status: status}
	}
	tenant := reqctx.GetTenantUUID(ctx)
	if tenant == "" || tenant != in.TenantUUID || tenant != claims.TenantUUID {
		return nil, MediaError(403, "AI_FORBIDDEN")
	}
	if i == nil || i.Service == nil || i.Service.routes == nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	env, err := cap.ResolveSTSLLMEnvironment(ctx, tenant, "", i.Service.ResolveTenantEnv)
	if err != nil {
		return nil, err
	}
	route, err := i.Service.routes.FindEquivalentDefaultByScopeModality(ctx, env, &tenant, "llm")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, MediaError(409, "AI_DEFAULT_MODEL_NOT_CONFIGURED")
	}
	if err != nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if strings.TrimSpace(route.Provider) == "" || strings.TrimSpace(route.Model) == "" {
		return nil, MediaError(409, "AI_DEFAULT_MODEL_NOT_CONFIGURED")
	}
	return map[string]any{"env": env, "modality": "llm", "model_key": route.Provider + "/" + route.Model}, nil
}
