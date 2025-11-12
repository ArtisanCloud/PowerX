package agent_lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ManifestValidator 定义 manifest 校验器。
type ManifestValidator interface {
	Validate(ctx context.Context, input ManifestRegistrationInput) error
}

// SandboxRunner 定义沙箱执行器。
type SandboxRunner interface {
	Run(ctx context.Context, input SandboxRunInput) (*SandboxRunResult, error)
}

type defaultManifestValidator struct{}

func (defaultManifestValidator) Validate(_ context.Context, in ManifestRegistrationInput) error {
	if strings.TrimSpace(in.PluginID) == "" ||
		strings.TrimSpace(in.PluginVersion) == "" ||
		strings.TrimSpace(in.ManifestVersion) == "" ||
		strings.TrimSpace(in.TenantID) == "" ||
		strings.TrimSpace(in.Alias) == "" ||
		strings.TrimSpace(in.TelemetryContractVersion) == "" {
		return ErrInvalidManifestPayload
	}
	if strings.TrimSpace(in.Signature) == "" {
		return ErrInvalidManifestSignature
	}
	return nil
}

type noopSandboxRunner struct{}

func (noopSandboxRunner) Run(_ context.Context, input SandboxRunInput) (*SandboxRunResult, error) {
	return &SandboxRunResult{
		Status:     "skipped",
		Profile:    input.Profile,
		ExecutedAt: time.Now().UTC(),
	}, nil
}

// RegisterManifest 处理插件 manifest 自动注册流程。
func (s *Service) RegisterManifest(ctx context.Context, in ManifestRegistrationInput) (*ManifestRegistrationResult, error) {
	if s == nil {
		return nil, fmt.Errorf("service not initialized")
	}
	if s.manifestValidator == nil {
		s.manifestValidator = defaultManifestValidator{}
	}
	if err := s.manifestValidator.Validate(ctx, in); err != nil {
		return nil, err
	}
	if in.DryRun {
		return &ManifestRegistrationResult{DryRun: true}, nil
	}

	input := s.buildRegisterInputFromManifest(in)
	result, err := s.Register(ctx, input)
	if err != nil {
		return nil, err
	}

	var sandboxResult *SandboxRunResult
	if !in.SkipSandbox && s.sandboxRunner != nil {
		sbResult, err := s.sandboxRunner.Run(ctx, SandboxRunInput{
			AgentID:       result.Agent.ID,
			PluginID:      in.PluginID,
			PluginVersion: in.PluginVersion,
			Profile:       in.SandboxProfile,
			RequestedBy:   chooseRequestedBy(in.RequestedBy, in.PluginID, in.PluginVersion),
			TraceID:       in.TraceID,
			Metadata:      cloneStringMap(in.Metadata),
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSandboxExecutionFailed, err)
		}
		sandboxResult = sbResult
	}

	return &ManifestRegistrationResult{
		Agent:   result.Agent,
		Sandbox: sandboxResult,
		DryRun:  false,
	}, nil
}

// RunSandbox 允许手动重跑沙箱验证。
func (s *Service) RunSandbox(ctx context.Context, in SandboxRunInput) (*SandboxRunResult, error) {
	if s == nil || s.sandboxRunner == nil {
		return nil, ErrSandboxExecutionFailed
	}
	if in.AgentID == uuid.Nil {
		return nil, fmt.Errorf("%w: agent_id required", ErrInvalidManifestPayload)
	}
	result, err := s.sandboxRunner.Run(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSandboxExecutionFailed, err)
	}
	return result, nil
}

// SetManifestValidator 允许在运行时替换 manifest 校验器（用于测试）。
func (s *Service) SetManifestValidator(v ManifestValidator) {
	if v == nil {
		return
	}
	s.manifestValidator = v
}

// SetSandboxRunner 允许在运行时替换沙箱执行器（用于测试）。
func (s *Service) SetSandboxRunner(r SandboxRunner) {
	if r == nil {
		return
	}
	s.sandboxRunner = r
}

func (s *Service) buildRegisterInputFromManifest(in ManifestRegistrationInput) RegisterInput {
	metadata := cloneStringMap(in.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["plugin_id"] = in.PluginID
	metadata["plugin_version"] = in.PluginVersion
	metadata["manifest_version"] = in.ManifestVersion
	if len(in.Capabilities) > 0 {
		metadata["capabilities"] = strings.Join(in.Capabilities, ",")
	}
	if len(in.Permissions) > 0 {
		metadata["permissions"] = strings.Join(in.Permissions, ",")
	}

	requestedBy := chooseRequestedBy(in.RequestedBy, in.PluginID, in.PluginVersion)
	defaultCapacity := in.DefaultCapacityInstances
	if defaultCapacity <= 0 {
		defaultCapacity = int32(s.config.DefaultCapacityInstances)
	}

	return RegisterInput{
		TenantID:                 in.TenantID,
		Alias:                    strings.TrimSpace(in.Alias),
		DisplayName:              defaultDisplayName(in.DisplayName, in.Alias),
		ToolGrants:               in.ToolGrants,
		TelemetryContractVersion: in.TelemetryContractVersion,
		DefaultCapacityInstances: defaultCapacity,
		MaxCapacityInstances:     in.MaxCapacityInstances,
		EventTopicPrefix:         "",
		NotificationChannel:      in.NotificationChannel,
		Metadata:                 metadata,
		RequestedBy:              requestedBy,
		TraceID:                  in.TraceID,
	}
}

func chooseRequestedBy(requestedBy, pluginID, pluginVersion string) string {
	if strings.TrimSpace(requestedBy) != "" {
		return requestedBy
	}
	if pluginID == "" {
		return "plugin-autoreg"
	}
	if pluginVersion == "" {
		return fmt.Sprintf("plugin:%s", pluginID)
	}
	return fmt.Sprintf("plugin:%s@%s", pluginID, pluginVersion)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	cp := make(map[string]string, len(src))
	for k, v := range src {
		cp[k] = v
	}
	return cp
}
