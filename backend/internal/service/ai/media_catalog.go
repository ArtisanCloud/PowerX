package ai

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	openaidriver "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/openai"
	imagefactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/image"
	ttsfactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/tts"
	videofactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/video"
	vlmfactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/vlm"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const MediaCatalogCapability = "com.corex.ai.catalog.models.read"

func MediaError(status int, code string) *dto.AppError {
	return &dto.AppError{HTTPCode: status, Code: code, Message: code}
}
func InvokeCapability(capability string) string {
	switch catalog.MediaCapability(capability) {
	case catalog.VisionUnderstanding:
		return "com.corex.ai.vlm.invoke"
	case catalog.ImageGeneration:
		return "com.corex.ai.image.invoke"
	case catalog.VideoGeneration:
		return "com.corex.ai.video.invoke"
	case catalog.SpeechSynthesis:
		return "com.corex.ai.tts.invoke"
	}
	return ""
}

// AuthorizeMedia 在每次调用时检查实时租户授权；Admin 与服务身份分别判断。
func (s *Service) AuthorizeMedia(ctx context.Context, tenant, capability string) (string, error) {
	if s == nil || s.db == nil {
		return "", MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if tenant == "" || tenant != reqctx.GetTenantUUID(ctx) || reqctx.GetClaims(ctx) == nil {
		return "", MediaError(401, "AI_UNAUTHORIZED")
	}
	if cap.ServiceCredential(ctx) {
		access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
		if err != nil {
			return "", err
		}
		if err = access.Require(capability); err != nil {
			return "", err
		}
		return access.Subject, nil
	}
	if reqctx.IsRoot(ctx) {
		return "user:" + reqctx.GetClaims(ctx).Subject, nil
	}
	resource, action := "", "invoke"
	switch capability {
	case ConnectionTestCapability:
		resource, action = "connection", "test"
	case MediaCatalogCapability:
		resource, action = "catalog_models", "read"
	case "com.corex.ai.catalog.providers.read":
		resource, action = "catalog_providers", "read"
	case "com.corex.ai.vlm.invoke":
		resource = "vlm"
	case "com.corex.ai.image.invoke":
		resource = "image"
	case "com.corex.ai.video.invoke":
		resource = "video"
	case "com.corex.ai.tts.invoke":
		resource = "tts"
	case MediaTaskReadCapability:
		resource, action = "media_tasks", "read"
	case MediaTaskCancelCapability:
		resource, action = "media_tasks", "cancel"
	default:
		return "", MediaError(403, "AI_FORBIDDEN")
	}
	ok, err := iamrepo.NewPermissionRepository(s.db).MemberHasLivePlatformPermission(ctx, tenant, reqctx.GetMemberUUID(ctx), reqctx.GetMemberID(ctx), "ai", resource, action)
	if err != nil {
		return "", MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if !ok {
		return "", MediaError(403, "AI_FORBIDDEN")
	}
	return "member:" + reqctx.GetMemberUUID(ctx), nil
}

type MediaModelItem struct {
	ModelKey          string                         `json:"model_key"`
	Provider          string                         `json:"provider"`
	Model             string                         `json:"model"`
	App               string                         `json:"app,omitempty"`
	Label             string                         `json:"label"`
	Capabilities      []string                       `json:"capabilities"`
	Configured        bool                           `json:"configured"`
	Available         bool                           `json:"available"`
	ReasonCode        string                         `json:"reason_code,omitempty"`
	Operations        map[string]MediaModelOperation `json:"operations"`
	UsageSource       string                         `json:"usage_source,omitempty"`
	InferenceVerified bool                           `json:"inference_verified"`
	Vision            *catalog.VisionUsage           `json:"vision,omitempty"`
}
type MediaModelOperation struct {
	catalog.MediaOperation
	Available       bool           `json:"available"`
	ReasonCode      string         `json:"reason_code,omitempty"`
	ParameterSchema map[string]any `json:"parameter_schema"`
	Defaults        map[string]any `json:"defaults"`
}

func (s *Service) mediaRegistry() *catalog.Registry {
	if s.catalog != nil {
		return s.catalog
	}
	return catalog.GetGlobalAIRegister()
}
func (s *Service) mediaModel(ctx context.Context, env, tenant, key, capability string) (*catalog.MediaModel, error) {
	provider, model := splitModelKey(key)
	models, err := s.settings.MediaModelsForTenant(ctx, env, &tenant, provider, "", capability)
	if err != nil {
		return nil, err
	}
	for _, item := range models {
		id := item.Model.ID
		if item.App != "" {
			id = item.App + ":" + id
		}
		if id == model {
			if catalog.MediaCapability(capability) == catalog.VisionUnderstanding && item.Vision != nil && item.Vision.Status == "unsupported" {
				return nil, MediaError(422, item.Vision.Reason)
			}
			return &item, nil
		}
	}
	return nil, MediaError(422, "AI_MODEL_CAPABILITY_UNSUPPORTED")
}
func mediaDefaults(model *catalog.MediaModel, capability string, profile map[string]any) map[string]any {
	return catalog.MediaDefaults(&model.Model, capability, profile)
}
func (s *Service) mediaConfig(ctx context.Context, env, tenant string, model *catalog.MediaModel, capability string) (*aiconfig.ModelConfig, map[string]any, error) {
	key := strings.TrimPrefix(model.ModelKey(), model.Provider+"/")
	profile, err := s.profiles.FindByEquivalentScopeModalityProviderModel(ctx, env, &tenant, model.Modality, model.Provider, key)
	// 历史视觉画像曾存于 image；仅对已明确登记的视觉操作保留读取兼容。
	if (err != nil || profile == nil) && capability == catalog.VisionUnderstanding && model.Modality == "vlm" {
		if legacy, legacyErr := s.profiles.FindByEquivalentScopeModalityProviderModel(ctx, env, &tenant, "image", model.Provider, key); legacyErr == nil && legacy != nil {
			profile, err = legacy, nil
		}
	}
	if err != nil || profile == nil {
		modalities := []string{model.Modality}
		if capability == catalog.VisionUnderstanding && model.Modality == "vlm" {
			modalities = append(modalities, "image")
		}
		for _, modality := range modalities {
			if profiles, e := s.profiles.ListByEquivalentScope(ctx, env, &tenant, modality); e == nil {
				for index := range profiles {
					candidate := &profiles[index]
					if s.mediaRegistry().CanonicalProvider(candidate.Provider) == model.Provider && candidate.Model == key {
						profile = candidate
						break
					}
				}
			}
			if profile != nil {
				break
			}
		}
	}
	if isProtocolTemplate(model.Model) && (profile == nil || !hasOnlineUsageSelection(profile.Tags)) {
		// 新在线型号的用途由管理员保存画像明确选择，连接健康不能代替该选择。
		return nil, nil, MediaError(409, "AI_MODEL_NOT_CONFIGURED")
	}
	credentials, err := s.settings.ListCredentials(ctx, env, &tenant)
	if err != nil {
		return nil, nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	for _, credential := range credentials {
		if s.mediaRegistry().CanonicalProvider(credential.Provider) != model.Provider {
			continue
		}
		if profile == nil && !s.allowUnprofiled(ctx, env, tenant, model.Modality, credential.Provider) {
			return nil, nil, MediaError(409, "AI_MODEL_NOT_CONFIGURED")
		}
		var defaults map[string]any
		if profile != nil {
			defaults = profile.Defaults
		}
		baseURL, _ := credential.Data["base_url"].(string)
		secrets := map[string]any{}
		if tenantkeys.NewTenantKeyService(s.db).UnsealSensitive(ctx, credential.Env, tenant, credential.Data, &secrets) != nil {
			return nil, nil, MediaError(409, "AI_MODEL_NOT_CONFIGURED")
		}
		apiKey, _ := secrets["api_key"].(string)
		if strings.TrimSpace(apiKey) == "" {
			return nil, nil, MediaError(409, "AI_MODEL_NOT_CONFIGURED")
		}
		mc, e := s.settings.BuildModelConfig(ctx, env, &tenant, model.Modality, credential.Provider, key, baseURL, apiKey, "", "", "", "")
		if e != nil {
			return nil, nil, MediaError(409, "AI_MODEL_NOT_CONFIGURED")
		}
		// 多鉴权供应商的通用设置可能选用 AK/SK；媒体 operation 使用已解封的 API Key。
		mc.APIKey = apiKey
		// 通道能力来自已保存的连接，不接受调用 payload 覆盖编辑路径或上限。
		for _, field := range []string{"image_edit_api_path", "image_edit_max_images"} {
			if value, exists := credential.Data[field]; exists {
				if mc.Extra == nil {
					mc.Extra = map[string]any{}
				}
				mc.Extra[field] = value
			}
		}
		if baseURL != "" && mc.Extra != nil {
			delete(mc.Extra, "base_url")
		}
		return mc, mediaDefaults(model, capability, defaults), nil
	}
	return nil, nil, MediaError(409, "AI_MODEL_NOT_CONFIGURED")
}

func (s *Service) ListMediaModels(ctx context.Context, env, tenant, provider, app, capability string) ([]MediaModelItem, error) {
	if _, err := s.AuthorizeMedia(ctx, tenant, MediaCatalogCapability); err != nil {
		return nil, err
	}
	if capability != "" && InvokeCapability(capability) == "" {
		return nil, MediaError(400, "AI_CAPABILITY_INVALID")
	}
	items := []MediaModelItem{}
	models, err := s.settings.MediaModelsForTenant(ctx, env, &tenant, provider, app, capability)
	if err != nil {
		return nil, err
	}
	profileCache := map[string]map[string]bool{}
	permissionCache := map[string]error{}
	for _, m := range models {
		item := MediaModelItem{ModelKey: m.ModelKey(), Provider: m.Provider, Model: m.Model.ID, App: m.App, Label: m.Model.Label, Capabilities: []string{}, Operations: map[string]MediaModelOperation{}}
		item.UsageSource = "local_adaptation"
		item.Vision = m.Vision
		for _, tag := range m.Model.Tags {
			if tag == "protocol-template" {
				item.UsageSource = "unspecified"
			}
			if tag == "provider-usage" {
				item.UsageSource = "provider"
			}
		}
		if m.Vision != nil && m.Vision.Source != "unknown" {
			item.UsageSource = m.Vision.Source
		}
		for ability, operation := range m.Model.Operations {
			if capability != "" && ability != catalog.MediaCapability(capability) {
				continue
			}
			if InvokeCapability(ability) == "" {
				continue
			}
			var defaults map[string]any
			var configErr error
			if isProtocolTemplate(m.Model) {
				known, loaded := profileCache[m.Modality]
				if !loaded {
					known = map[string]bool{}
					profiles, err := s.profiles.ListByEquivalentScope(ctx, env, &tenant, m.Modality)
					if err != nil {
						return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
					}
					for _, p := range profiles {
						if hasOnlineUsageSelection(p.Tags) {
							known[s.mediaRegistry().CanonicalProvider(p.Provider)+"/"+p.Model] = true
						}
					}
					profileCache[m.Modality] = known
				}
				if !known[m.ModelKey()] {
					configErr = MediaError(409, "AI_MODEL_NOT_CONFIGURED")
				}
			}
			if configErr == nil {
				var mc *aiconfig.ModelConfig
				mc, defaults, configErr = s.mediaConfig(ctx, env, tenant, &m, ability)
				if configErr == nil && ability == catalog.ImageGeneration {
					operation, configErr = imageOperationForConfig(operation, mc)
				}
			}
			reason := ""
			if !mediaDriverSupported(ability, operation) {
				reason = "AI_DRIVER_UNSUPPORTED"
			}
			if configErr != nil {
				reason = "AI_MODEL_NOT_CONFIGURED"
				var configuration *dto.AppError
				if errors.As(configErr, &configuration) && configuration.Code == "AI_MODEL_CONFIGURATION_INVALID" {
					reason = configuration.Code
				}
			} else {
				item.Configured = true
			}
			permissionErr, checked := permissionCache[ability]
			if !checked {
				_, permissionErr = s.AuthorizeMedia(ctx, tenant, InvokeCapability(ability))
				permissionCache[ability] = permissionErr
			}
			if permissionErr != nil {
				reason = "AI_FORBIDDEN"
			}
			if _, why := catalog.ValidateMediaParameters(operation, defaults); reason == "" && why != "" {
				reason = "AI_MODEL_DEFAULTS_INVALID"
			}
			if reason == "" && m.Provider == "openai" && ability == catalog.VisionUnderstanding {
				if err := openaidriver.ValidateVLMParameters(m.Model.ID, defaults); err != nil {
					reason = "AI_MODEL_DEFAULTS_INVALID"
				}
			}
			if ability == catalog.VisionUnderstanding && m.Vision != nil && m.Vision.Status == "unsupported" && permissionErr == nil {
				reason = m.Vision.Reason
			}
			if defaults == nil {
				defaults = mediaDefaults(&m, ability, nil)
			}
			item.Capabilities = append(item.Capabilities, ability)
			item.Operations[ability] = MediaModelOperation{operation, reason == "", reason, map[string]any{"type": "object", "additionalProperties": false, "properties": operation.Parameters}, defaults}
			item.Available = item.Available || reason == ""
			if item.ReasonCode == "" {
				item.ReasonCode = reason
			}
		}
		if item.Available {
			item.ReasonCode = ""
		}
		sort.Strings(item.Capabilities)
		items = append(items, item)
	}
	return items, nil
}

func isProtocolTemplate(model catalog.ModelManifest) bool {
	for _, tag := range model.Tags {
		if tag == "protocol-template" {
			return true
		}
	}
	return false
}

func hasOnlineUsageSelection(tags []string) bool {
	for _, tag := range tags {
		if tag == catalog.OnlineUsageSelectedTag {
			return true
		}
	}
	return false
}

func (s *Service) prepareMedia(ctx context.Context, env, tenant, key, capability string, inputs []ContentItem, params map[string]any) (*catalog.MediaModel, *aiconfig.ModelConfig, map[string]any, error) {
	if _, err := s.AuthorizeMedia(ctx, tenant, InvokeCapability(capability)); err != nil {
		return nil, nil, nil, err
	}
	model, err := s.mediaModel(ctx, env, tenant, key, capability)
	if err != nil {
		return nil, nil, nil, err
	}
	op := model.Model.Operations[capability]
	if !mediaDriverSupported(capability, op) {
		return nil, nil, nil, MediaError(422, "AI_DRIVER_UNSUPPORTED")
	}
	if field, reason := catalog.ValidateMediaParameters(op, params); reason != "" {
		e := MediaError(422, "AI_PARAMETER_INVALID")
		e.Details = map[string]any{"parameter": field, "reason": reason}
		return nil, nil, nil, e
	}
	mc, defaults, err := s.mediaConfig(ctx, env, tenant, model, capability)
	if err != nil {
		return nil, nil, nil, err
	}
	if capability == catalog.ImageGeneration {
		op, err = imageOperationForConfig(op, mc)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	images, videos := 0, 0
	for _, input := range inputs {
		found := false
		for _, typ := range op.InputTypes {
			if typ == input.Type {
				found = true
			}
		}
		if !found {
			return nil, nil, nil, MediaError(422, "AI_INPUT_UNSUPPORTED")
		}
		if (input.Type == "image_url" || input.Type == "video_url") && strings.TrimSpace(input.URL) == "" {
			return nil, nil, nil, MediaError(422, "AI_INPUT_INVALID")
		}
		if input.Type == "image_base64" && strings.TrimSpace(input.Content) == "" && strings.TrimSpace(input.URL) == "" {
			return nil, nil, nil, MediaError(422, "AI_INPUT_INVALID")
		}
		if input.Type == "image_url" || input.Type == "image_base64" {
			images++
		}
		if input.Type == "video_url" {
			videos++
		}
	}
	if images > op.MaxImages || videos > op.MaxVideos {
		return nil, nil, nil, MediaError(422, "AI_INPUT_LIMIT_EXCEEDED")
	}
	if strings.TrimSpace(BuildPrompt(inputs)) == "" {
		return nil, nil, nil, MediaError(422, "AI_PROMPT_REQUIRED")
	}
	if field, reason := catalog.ValidateMediaParameters(op, defaults); reason != "" {
		e := MediaError(409, "AI_MODEL_DEFAULTS_INVALID")
		e.Details = map[string]any{"parameter": field, "reason": reason}
		return nil, nil, nil, e
	}
	for key, v := range params {
		defaults[key] = v
	}
	if capability == catalog.ImageGeneration {
		client, e := imagefactory.NewClient(op.Driver)
		if e != nil {
			return nil, nil, nil, e
		}
		if validator, ok := client.(contract.ImageRequestValidator); ok {
			if e = validator.Validate(imageTaskRequest(&mediaTaskRequest{MediaSubmitInput: MediaSubmitInput{Inputs: inputs}, EffectiveParams: defaults}, mc)); e != nil {
				return nil, nil, nil, e
			}
		}
	}
	if capability == catalog.VideoGeneration {
		client, e := videofactory.NewClient(op.Driver)
		if e != nil {
			return nil, nil, nil, e
		}
		if validator, ok := client.(contract.VideoRequestValidator); ok {
			if e = validator.Validate(videoTaskRequest(&mediaTaskRequest{MediaSubmitInput: MediaSubmitInput{Inputs: inputs}, EffectiveParams: defaults}, mc)); e != nil {
				return nil, nil, nil, e
			}
		}
	}
	return model, mc, defaults, nil
}

func mediaErrorCode(err error) string {
	var e *dto.AppError
	if errors.As(err, &e) {
		return e.Code
	}
	return "AI_PROVIDER_FAILED"
}

func mediaDriverSupported(capability string, operation catalog.MediaOperation) bool {
	switch capability {
	case catalog.ImageGeneration:
		_, e := imagefactory.NewClient(operation.Driver)
		return e == nil
	case catalog.VisionUnderstanding:
		_, e := vlmfactory.NewClient(operation.Driver)
		return e == nil
	case catalog.SpeechSynthesis:
		_, e := ttsfactory.NewClient(operation.Driver)
		return e == nil
	case catalog.VideoGeneration:
		c, e := videofactory.NewClient(operation.Driver)
		if e != nil {
			return false
		}
		_, ok := c.(interface {
			Submit(context.Context, contract.VideoRequest) (*contract.VideoTaskStatus, error)
		})
		return operation.Async && ok
	}
	return false
}

// imageOperationForConfig 目录与提交入口使用同一份通道限制，文字能力不因关闭参考图而消失。
func imageOperationForConfig(operation catalog.MediaOperation, mc *aiconfig.ModelConfig) (catalog.MediaOperation, error) {
	if operation.Driver != "openai" {
		return operation, nil
	}
	limit, err := openaidriver.ImageReferenceLimit(mc)
	if err != nil {
		var failure *dto.AppError
		if !errors.As(err, &failure) || failure.Code != "AI_INPUT_UNSUPPORTED" {
			return operation, err
		}
		limit = 0
	}
	if operation.MaxImages > limit {
		operation.MaxImages = limit
	}
	if operation.MaxImages == 0 {
		operation.InputTypes = []string{"text"}
		operation.MaxImageBytes, operation.MaxTotalImageBytes, operation.MaxImagePixels = 0, 0, 0
	}
	return operation, nil
}
