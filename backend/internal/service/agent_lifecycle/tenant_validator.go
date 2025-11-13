package agent_lifecycle

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// TenantShareValidator 编排租户共享校验，包含白名单、策略冲突与沙箱验证。
type TenantShareValidator struct {
	policy  PolicyConflictEngine
	sandbox SandboxRunner
	logger  *pxlog.Logger
}

// NewTenantShareValidator 构建默认的共享校验器。
func NewTenantShareValidator(policy PolicyConflictEngine, sandbox SandboxRunner) ShareValidator {
	return &TenantShareValidator{
		policy:  policy,
		sandbox: sandbox,
		logger:  pxlog.GetGlobalLogger(),
	}
}

func (v *TenantShareValidator) Validate(ctx context.Context, agent *Agent, tenantID string, quotas []ShareQuota, metadata map[string]string) error {
	if strings.TrimSpace(tenantID) == "" {
		return fmt.Errorf("tenant_id is required for sharing")
	}
	if agent != nil && agent.TenantID == tenantID {
		return fmt.Errorf("cannot share agent to its owning tenant")
	}
	for _, q := range quotas {
		if q.Limit < 0 {
			return fmt.Errorf("quota %s has invalid limit %d", q.Type, q.Limit)
		}
	}

	if v.policy != nil {
		input := PolicyConflictInput{
			TenantID:    tenantID,
			Alias:       agentAlias(agent),
			Permissions: metadataList(metadata["permissions"]),
			RateLimit:   parseRateLimit(metadata["rate_limit"]),
		}
		if conflicts, err := v.policy.Evaluate(ctx, input); err != nil {
			return err
		} else if len(conflicts) > 0 {
			return fmt.Errorf("policy conflict: %s", conflicts[0].Message)
		}
	}

	if v.sandbox != nil && agent != nil {
		runCtx, traceID := agentinstr.EnsureTraceContext(ctx)
		profile := metadata["sandbox_profile"]
		result, err := v.sandbox.Run(runCtx, SandboxRunInput{
			AgentID:     agent.ID,
			Profile:     profile,
			RequestedBy: metadata["requested_by"],
			TraceID:     traceID,
			Metadata:    metadata,
		})
		if err != nil {
			return fmt.Errorf("%w: %v", ErrSandboxExecutionFailed, err)
		}
		if result != nil && !strings.EqualFold(result.Status, "completed") {
			return fmt.Errorf("sandbox validation failed: %s", result.Status)
		}
	}
	return nil
}

func agentAlias(agent *Agent) string {
	if agent == nil {
		return ""
	}
	return agent.Alias
}

func metadataList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func parseRateLimit(value string) int32 {
	if value == "" {
		return 0
	}
	if lim, err := strconv.Atoi(value); err == nil {
		return int32(lim)
	}
	return 0
}
