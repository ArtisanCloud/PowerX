package agent

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	model "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
)

// VisionTrialConnection 仅在本次管理试跑中使用；不进入画像、在线目录或凭据存储。
type VisionTrialConnection struct {
	CurrentForm                                                                           bool
	BaseURL, APIKey, AuthMode, Organization, Region, SecretID, SecretKey, AzureDeployment string
}

// PrepareVisionTrialConfig 不要求已有画像或在线目录，复用已发布的固定视觉驱动合同。
// 当前表单优先；只有空白密钥复用同租户、同环境凭据，显式空地址使用供应商默认值。
func (s *AgentSettingService) PrepareVisionTrialConfig(ctx context.Context, env, tenant, provider, key string, in VisionTrialConnection, params map[string]any) (*catalog.MediaModel, *aiconfig.ModelConfig, error) {
	resolved, err := visionTrialModel(provider, key, params)
	if err != nil {
		return nil, nil, err
	}
	if tenant == "" || env == "" {
		return nil, nil, connectionConfigError()
	}
	provider, id := resolved.Provider, resolved.Model.ID
	var saved *model.AIProviderCredential
	// 仅在确实需要复用时读取；全新连接无需创建任何持久化记录。
	if !in.CurrentForm || strings.TrimSpace(in.APIKey) == "" {
		var err error
		saved, err = s.credRepo.FindByEquivalentScopeNameProvider(ctx, env, &tenant, utils.Slug(env+"-"+provider), provider)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, &dto.AppError{HTTPCode: 503, Code: "AI_DEPENDENCY_UNAVAILABLE", Message: "AI_DEPENDENCY_UNAVAILABLE"}
		}
	}
	if !in.CurrentForm && saved != nil {
		// 兼容旧管理客户端省略连接字段的请求；新页面显式发送 CurrentForm。
		in.BaseURL, _ = saved.Data["base_url"].(string)
		in.AuthMode, _ = saved.Data["auth_mode"].(string)
		in.Organization, _ = saved.Data["organization"].(string)
		in.Region, _ = saved.Data["region"].(string)
		in.AzureDeployment, _ = saved.Data["azure_deployment"].(string)
	}
	base, err := ResolveFormBaseURL(provider, in.AuthMode, in.BaseURL)
	if err != nil || validateEndpoint(base) != nil {
		return nil, nil, connectionConfigError()
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, nil, connectionConfigError()
	}
	apiKey := strings.TrimSpace(in.APIKey)
	if apiKey == "" && saved != nil {
		// 只继承密钥；不允许旧地址、组织或 deployment 覆盖当前表单。
		_, apiKey, err = s.resolveConnFromCredential(ctx, env, &tenant, provider, saved, base, "")
		if err != nil {
			return nil, nil, connectionConfigError()
		}
	}
	if catalog.AuthReqFromCatalog(provider).NeedKey && apiKey == "" {
		return nil, nil, connectionConfigError()
	}
	extra := s.buildModelExtras(contract.ModVLM, provider, key)
	delete(extra, "base_url")
	mc := &aiconfig.ModelConfig{Provider: provider, Model: id, Endpoint: base, APIKey: apiKey, Organization: strings.TrimSpace(in.Organization), Region: strings.TrimSpace(in.Region), SecretID: strings.TrimSpace(in.SecretID), SecretKey: strings.TrimSpace(in.SecretKey), AzureDeployment: strings.TrimSpace(in.AzureDeployment), Extra: extra}
	return resolved, mc, nil
}

// ValidateVLMTrialParameters 只校验当前表单的型号协议与参数，不读取画像或提交请求。
func ValidateVLMTrialParameters(provider, key string, params map[string]any) error {
	_, err := visionTrialModel(provider, key, params)
	return err
}

func visionTrialModel(provider, key string, params map[string]any) (*catalog.MediaModel, error) {
	reg := catalog.GetGlobalAIRegister()
	provider = reg.CanonicalProvider(provider)
	app, id := splitAppModel(reg, provider, key)
	if provider == "" || id == "" || len(id) > 256 {
		return nil, connectionConfigError()
	}
	manifest := reg.ProtocolModel("vlm", provider, app, id)
	if manifest == nil {
		return nil, &dto.AppError{HTTPCode: 422, Code: "AI_DRIVER_UNSUPPORTED", Message: "AI_DRIVER_UNSUPPORTED"}
	}
	usage := resolveVisionUsage(provider, OnlineCatalogModel{ID: id})
	if usage.Status == "unsupported" {
		return nil, &dto.AppError{HTTPCode: 422, Code: usage.Reason, Message: usage.Reason}
	}
	if err := validateVLMManifest(provider, manifest, params); err != nil {
		return nil, err
	}
	return &catalog.MediaModel{Provider: provider, App: app, Modality: "vlm", Model: *manifest, Vision: &usage}, nil
}
