package agent_lifecycle

import (
	"context"
)

// ShareValidator 校验共享申请（白名单、权限等）。
type ShareValidator interface {
	Validate(ctx context.Context, agent *Agent, tenantID string, quotas []ShareQuota, metadata map[string]string) error
}

// QuotaProvisioner 负责复制/回收配额与凭证。
type QuotaProvisioner interface {
	Provision(ctx context.Context, agent *Agent, tenantID string, quotas []ShareQuota, metadata map[string]string) error
	Release(ctx context.Context, share *AgentShare) error
}

type noopShareValidator struct{}

func (noopShareValidator) Validate(_ context.Context, _ *Agent, _ string, _ []ShareQuota, _ map[string]string) error {
	return nil
}

type noopQuotaProvisioner struct{}

func (noopQuotaProvisioner) Provision(_ context.Context, _ *Agent, _ string, _ []ShareQuota, _ map[string]string) error {
	return nil
}

func (noopQuotaProvisioner) Release(_ context.Context, _ *AgentShare) error {
	return nil
}
