package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/connectionprobe"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
)

type ConnectionInput struct {
	Provider, Model, BaseURL, APIKey, AuthMode, Organization string
}

// VerifyConnection 只验证入口及鉴权；不调用文本、图片、视频或 embedding 生成。
func (s *AgentSettingService) VerifyConnection(ctx context.Context, env string, tenantUUID *string, modality contract.Modality, in ConnectionInput) (*connectionprobe.Result, error) {
	return s.verifyConnection(ctx, env, tenantUUID, modality, in, false)
}

// VerifySavedConnection 仅探测当前租户已保存的连接；不保存凭据、健康状态或模型画像。
func (s *AgentSettingService) VerifySavedConnection(ctx context.Context, env, tenant string, modality contract.Modality, provider, model string) (*connectionprobe.Result, error) {
	if s == nil || s.credRepo == nil || tenant == "" || env == "" {
		return nil, connectionConfigError()
	}
	provider = catalog.GetGlobalAIRegister().CanonicalProvider(provider)
	cred, err := s.credRepo.FindByEquivalentScopeNameProvider(ctx, env, &tenant, utils.Slug(env+"-"+provider), provider)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, &dto.AppError{HTTPCode: 409, Code: "AI_SETTINGS_PROVIDER_NOT_CONFIGURED", Message: "AI_SETTINGS_PROVIDER_NOT_CONFIGURED", Details: map[string]any{"generation_submitted": false}}
	}
	if err != nil {
		return nil, &dto.AppError{HTTPCode: 503, Code: "AI_DEPENDENCY_UNAVAILABLE", Message: "AI_DEPENDENCY_UNAVAILABLE", Details: map[string]any{"generation_submitted": false}}
	}
	authMode, _ := cred.Data["auth_mode"].(string)
	organization, _ := cred.Data["organization"].(string)
	if _, err := connectionProbeStrategy(provider, authMode); err != nil {
		return nil, err
	}
	base, key, err := s.resolveConnFromCredential(ctx, env, &tenant, provider, cred, "", "")
	if err != nil {
		return nil, connectionConfigError()
	}
	return s.verifyConnection(ctx, env, &tenant, modality, ConnectionInput{Provider: provider, Model: model, BaseURL: base, APIKey: key, AuthMode: authMode, Organization: organization}, true)
}

func (s *AgentSettingService) verifyConnection(ctx context.Context, env string, tenantUUID *string, modality contract.Modality, in ConnectionInput, credentialsResolved bool) (*connectionprobe.Result, error) {
	reg := catalog.GetGlobalAIRegister()
	p := reg.CanonicalProvider(in.Provider)
	if manifest, ok := reg.Manifest(p); !ok || manifest == nil {
		return nil, connectionConfigError()
	}
	// 入口／鉴权与型号使用分离：首次连接尚无在线目录，不能先要求型号已登记。
	strategy, err := connectionProbeStrategy(p, in.AuthMode)
	if err != nil {
		return nil, err
	}
	rule := catalog.AuthReqFromCatalog(p)
	base, key := in.BaseURL, in.APIKey
	if !credentialsResolved {
		base, err = ResolveFormBaseURL(p, in.AuthMode, base)
		if err != nil || strings.TrimSpace(base) == "" {
			return nil, connectionConfigError()
		}
		base, key, err = s.prepareAuthInputs(ctx, env, tenantUUID, p, base, key, false, rule.DefaultBaseURL, strategy != connectionprobe.OllamaTags)
		if err != nil {
			return nil, connectionConfigError()
		}
	}
	if strings.TrimSpace(base) == "" {
		base = rule.DefaultBaseURL
	}
	return connectionprobe.Verify(ctx, connectionprobe.Input{Provider: p, Endpoint: base, APIKey: key, Organization: in.Organization, Strategy: strategy})
}

func connectionProbeStrategy(p, authMode string) (string, error) {
	manifest, ok := catalog.GetGlobalAIRegister().Manifest(p)
	if !ok || manifest == nil {
		return "", connectionConfigError()
	}
	strategy := ""
	switch p {
	case "agnes", "openai", "qwen", "qwen-cn":
		strategy = connectionprobe.Models
	case "openrouter":
		strategy = connectionprobe.KeyInfo
	case "ollama":
		strategy = connectionprobe.OllamaTags
	default:
		if manifest.Auth.Scheme == "bearer" && manifest.Drivers["llm"] == "openai" {
			strategy = connectionprobe.Models
		}
	}
	if strategy == "" || strings.EqualFold(authMode, "tc3") {
		return "", &dto.AppError{HTTPCode: 501, Code: "AI_CONNECTION_PROBE_UNSUPPORTED", Message: "AI_CONNECTION_PROBE_UNSUPPORTED", Details: map[string]any{"generation_submitted": false}}
	}
	return strategy, nil
}

func connectionConfigError() *dto.AppError {
	return &dto.AppError{HTTPCode: 422, Code: "AI_CONNECTION_CONFIG_INVALID", Message: "AI_CONNECTION_CONFIG_INVALID", Details: map[string]any{"generation_submitted": false}}
}
