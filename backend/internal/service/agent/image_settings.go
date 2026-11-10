package agent

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/datatypes"
)

// normalizeImageProfile 在凭据、画像、路由产生写入前校验最终有效默认值。
func (s *AgentSettingService) normalizeImageProfile(ctx context.Context, env string, tenant *string, profile *dbmodel.AIModelProfile) error {
	provider := catalog.GetGlobalAIRegister().CanonicalProvider(profile.Provider)
	manifest := findModelManifest("image", provider, profile.Model)
	if manifest == nil {
		var err error
		manifest, err = s.TenantModelManifest(ctx, env, tenant, "image", provider, profile.Model)
		if err != nil {
			return err
		}
	}
	op, ok := manifest.Operations[catalog.ImageGeneration]
	if !ok {
		return &dto.AppError{HTTPCode: 422, Code: "AI_MODEL_CAPABILITY_UNSUPPORTED", Message: "AI_MODEL_CAPABILITY_UNSUPPORTED"}
	}
	values := datatypes.JSONMap{}
	for key, value := range profile.Defaults {
		// 表单未填写的可选字符串表示使用合同默认；已保存的非法非空值绝不自动改写。
		if text, isString := value.(string); isString && text == "" {
			if spec, known := op.Parameters[key]; known && spec.Type == "string" {
				continue
			}
		}
		values[key] = value
	}
	effective := catalog.MediaDefaults(manifest, catalog.ImageGeneration, values)
	if field, reason := catalog.ValidateMediaParameters(op, effective); reason != "" {
		details := map[string]any{"parameter": field, "reason": reason}
		if len(op.Parameters[field].Enum) > 0 {
			details["allowed_values"] = op.Parameters[field].Enum
		}
		return &dto.AppError{HTTPCode: 422, Code: "AI_MODEL_DEFAULTS_INVALID", Message: "AI_MODEL_DEFAULTS_INVALID", Details: details}
	}
	for key, value := range effective {
		values[key] = value
	}
	profile.Defaults = values
	return nil
}
