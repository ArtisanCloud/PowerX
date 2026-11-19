package capability_registry

import (
	"context"
	"strings"

	aimodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// TenantAIEnvironmentLookup reads the tenant's current AI environment setting.
type TenantAIEnvironmentLookup func(context.Context, string) (string, bool, error)

// ResolveSTSLLMEnvironment prefers the signed environment binding, then the
// tenant's AI setting. A supplied environment can only confirm that choice.
func ResolveSTSLLMEnvironment(ctx context.Context, tenant, requested string, lookup TenantAIEnvironmentLookup) (string, error) {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.Issuer != "powerx-sts" || claims.TenantUUID != tenant || tenant == "" {
		return "", &DirectGrantError{401}
	}
	// reqctx.GetEnv also contains the JWT middleware's synthetic dev value
	// for tokens issued without an environment. Only the claim is a binding.
	env := aimodel.CanonicalAIEnvironment(claims.Env)
	if env != "" {
		if status := reqctx.STSIssuanceStatus(ctx); status != 0 {
			return "", &DirectGrantError{Status: status}
		}
	}
	if env == "" {
		if lookup == nil {
			return "", dto.NewErrorWithCode(503, "AI_DEPENDENCY_UNAVAILABLE", "AI_DEPENDENCY_UNAVAILABLE", nil)
		}
		current, configured, err := lookup(ctx, tenant)
		if err != nil {
			return "", dto.NewErrorWithCode(503, "AI_DEPENDENCY_UNAVAILABLE", "AI_DEPENDENCY_UNAVAILABLE", nil)
		}
		if !configured || strings.TrimSpace(current) == "" {
			return "", dto.NewErrorWithCode(409, "AI_ENV_NOT_CONFIGURED", "AI_ENV_NOT_CONFIGURED", nil)
		}
		env = aimodel.CanonicalAIEnvironment(current)
	}
	if env != "dev" && env != "staging" && env != "prod" {
		return "", dto.NewErrorWithCode(400, "AI_ENV_INVALID", "AI_ENV_INVALID", nil)
	}
	if requested = aimodel.CanonicalAIEnvironment(requested); requested != "" && requested != env {
		return "", dto.NewErrorWithCode(403, "AI_FORBIDDEN", "AI_FORBIDDEN", nil)
	}
	return env, nil
}

type resolvedLLMEnvironmentKey struct{}

func resolvedLLMEnvironment(ctx context.Context) string {
	env, _ := ctx.Value(resolvedLLMEnvironmentKey{}).(string)
	return env
}
