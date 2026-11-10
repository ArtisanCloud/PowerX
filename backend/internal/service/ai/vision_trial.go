package ai

import (
	"context"
	"net/url"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
)

// TrialVision 用当前表单执行单图试跑；不要求保存型号，也不修改任何运行配置。
func (s *Service) TrialVision(ctx context.Context, env, tenant, provider, model string, connection agentsvc.VisionTrialConnection, inputs []ContentItem, params map[string]any) (map[string]any, error) {
	if cap.ServiceCredential(ctx) {
		return nil, MediaError(403, "AI_FORBIDDEN")
	}
	if _, err := s.AuthorizeMedia(ctx, tenant, InvokeCapability(catalog.VisionUnderstanding)); err != nil {
		return nil, err
	}
	images, prompts := 0, 0
	for _, input := range inputs {
		if input.Role != "" && input.Role != "user" {
			return nil, MediaError(422, "AI_INPUT_INVALID")
		}
		switch input.Type {
		case "text":
			if strings.TrimSpace(input.Content) == "" || len(input.Content) > 8192 {
				return nil, MediaError(422, "AI_PROMPT_REQUIRED")
			}
			prompts++
		case "image_base64":
			images++
			if strings.TrimSpace(input.Content) == "" || input.URL != "" {
				return nil, MediaError(422, "AI_INPUT_INVALID")
			}
			if len(input.Content) > (5<<20)*4/3+128 {
				return nil, MediaError(422, "AI_INPUT_LIMIT_EXCEEDED")
			}
		case "image_url":
			images++
			u, err := url.Parse(input.URL)
			if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || len(input.URL) > 4096 {
				return nil, MediaError(422, "AI_INPUT_INVALID")
			}
		default:
			return nil, MediaError(422, "AI_INPUT_UNSUPPORTED")
		}
	}
	if images != 1 || prompts != 1 || len(inputs) != 2 {
		return nil, MediaError(422, "AI_VISION_TRIAL_IMAGE_REQUIRED")
	}
	resolved, config, err := s.settings.PrepareVisionTrialConfig(ctx, env, tenant, provider, model, connection, params)
	if err != nil {
		return nil, err
	}
	op := resolved.Model.Operations[catalog.VisionUnderstanding]
	if !mediaDriverSupported(catalog.VisionUnderstanding, op) {
		return nil, MediaError(422, "AI_DRIVER_UNSUPPORTED")
	}
	if images > op.MaxImages {
		return nil, MediaError(422, "AI_INPUT_LIMIT_EXCEEDED")
	}
	for _, input := range inputs {
		allowed := false
		for _, typ := range op.InputTypes {
			allowed = allowed || input.Type == typ
		}
		if !allowed {
			return nil, MediaError(422, "AI_INPUT_UNSUPPORTED")
		}
	}
	effective := mediaDefaults(resolved, catalog.VisionUnderstanding, nil)
	for key, value := range params {
		effective[key] = value
	}
	return s.invokeVision(ctx, resolved, config, inputs, effective)
}
