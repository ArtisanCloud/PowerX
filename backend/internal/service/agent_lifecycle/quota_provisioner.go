package agent_lifecycle

import (
	"context"
	"fmt"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// ProvisioningStep 定义单个配额复制步骤。
type ProvisioningStep interface {
	Name() string
	Provision(ctx context.Context, agent *Agent, tenantID string, quotas []ShareQuota, metadata map[string]string) error
	Rollback(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error
	Release(ctx context.Context, share *AgentShare) error
}

// DefaultQuotaProvisioner 顺序执行注册的步骤并在失败时回滚。
type DefaultQuotaProvisioner struct {
	steps []ProvisioningStep
	log   *pxlog.Logger
}

// NewDefaultQuotaProvisioner 构造默认的配额复制器。
func NewDefaultQuotaProvisioner(steps ...ProvisioningStep) QuotaProvisioner {
	if len(steps) == 0 {
		steps = []ProvisioningStep{
			IAMProvisionStep{},
			SecretReplicationStep{},
			RateLimitProvisionStep{},
		}
	}
	return &DefaultQuotaProvisioner{
		steps: steps,
		log:   pxlog.GetGlobalLogger(),
	}
}

// Provision 执行配额复制流程。
func (p *DefaultQuotaProvisioner) Provision(ctx context.Context, agent *Agent, tenantID string, quotas []ShareQuota, metadata map[string]string) error {
	if len(p.steps) == 0 {
		return nil
	}
	executed := make([]ProvisioningStep, 0, len(p.steps))
	for _, step := range p.steps {
		if err := step.Provision(ctx, agent, tenantID, quotas, metadata); err != nil {
			p.rollback(ctx, executed, agent, tenantID, metadata)
			return fmt.Errorf("quota provision step %s failed: %w", step.Name(), err)
		}
		executed = append(executed, step)
	}
	return nil
}

// Release 回收配额。
func (p *DefaultQuotaProvisioner) Release(ctx context.Context, share *AgentShare) error {
	if len(p.steps) == 0 || share == nil {
		return nil
	}
	var releaseErr error
	for i := len(p.steps) - 1; i >= 0; i-- {
		if err := p.steps[i].Release(ctx, share); err != nil && releaseErr == nil {
			releaseErr = err
		}
	}
	return releaseErr
}

func (p *DefaultQuotaProvisioner) rollback(ctx context.Context, steps []ProvisioningStep, agent *Agent, tenantID string, metadata map[string]string) {
	for i := len(steps) - 1; i >= 0; i-- {
		if err := steps[i].Rollback(ctx, agent, tenantID, metadata); err != nil && p.log != nil {
			p.log.Warn(ctx, fmt.Sprintf("quota rollback failed for step %s: %v", steps[i].Name(), err))
		}
	}
}

// IAMProvisionStep 将 IAM 策略复制到目标租户（默认空操作）。
type IAMProvisionStep struct {
	Binder IAMBinder
}

func (s IAMProvisionStep) Name() string { return "iam" }

func (s IAMProvisionStep) Provision(ctx context.Context, agent *Agent, tenantID string, _ []ShareQuota, metadata map[string]string) error {
	if s.Binder == nil {
		return nil
	}
	return s.Binder.BindTenant(ctx, agent, tenantID, metadata)
}

func (s IAMProvisionStep) Rollback(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error {
	if s.Binder == nil {
		return nil
	}
	return s.Binder.UnbindTenant(ctx, agent, tenantID, metadata)
}

func (s IAMProvisionStep) Release(ctx context.Context, share *AgentShare) error {
	if s.Binder == nil {
		return nil
	}
	agent := &Agent{ID: share.AgentID}
	return s.Binder.UnbindTenant(ctx, agent, share.TenantID, share.Metadata)
}

// SecretReplicationStep 复制敏感凭证。
type SecretReplicationStep struct {
	Replicator SecretReplicator
}

func (s SecretReplicationStep) Name() string { return "secret" }

func (s SecretReplicationStep) Provision(ctx context.Context, agent *Agent, tenantID string, _ []ShareQuota, metadata map[string]string) error {
	if s.Replicator == nil {
		return nil
	}
	return s.Replicator.Replicate(ctx, agent, tenantID, metadata)
}

func (s SecretReplicationStep) Rollback(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error {
	if s.Replicator == nil {
		return nil
	}
	return s.Replicator.Revoke(ctx, agent, tenantID, metadata)
}

func (s SecretReplicationStep) Release(ctx context.Context, share *AgentShare) error {
	if s.Replicator == nil {
		return nil
	}
	agent := &Agent{ID: share.AgentID}
	return s.Replicator.Revoke(ctx, agent, share.TenantID, share.Metadata)
}

// RateLimitProvisionStep 配置共享配额/限流。
type RateLimitProvisionStep struct {
	Allocator RateLimitAllocator
}

func (s RateLimitProvisionStep) Name() string { return "rate_limit" }

func (s RateLimitProvisionStep) Provision(ctx context.Context, agent *Agent, tenantID string, quotas []ShareQuota, metadata map[string]string) error {
	if s.Allocator == nil {
		return nil
	}
	return s.Allocator.Allocate(ctx, agent, tenantID, quotas, metadata)
}

func (s RateLimitProvisionStep) Rollback(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error {
	if s.Allocator == nil {
		return nil
	}
	return s.Allocator.Release(ctx, agent, tenantID, metadata)
}

func (s RateLimitProvisionStep) Release(ctx context.Context, share *AgentShare) error {
	if s.Allocator == nil {
		return nil
	}
	agent := &Agent{ID: share.AgentID}
	return s.Allocator.Release(ctx, agent, share.TenantID, share.Metadata)
}

// IAMBinder 定义 IAM 策略绑定行为。
type IAMBinder interface {
	BindTenant(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error
	UnbindTenant(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error
}

// SecretReplicator 复制密钥/凭证。
type SecretReplicator interface {
	Replicate(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error
	Revoke(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error
}

// RateLimitAllocator 负责限流/配额分配。
type RateLimitAllocator interface {
	Allocate(ctx context.Context, agent *Agent, tenantID string, quotas []ShareQuota, metadata map[string]string) error
	Release(ctx context.Context, agent *Agent, tenantID string, metadata map[string]string) error
}
