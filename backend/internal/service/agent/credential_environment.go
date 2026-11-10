package agent

import (
	"context"
	"fmt"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"gorm.io/datatypes"
)

// inheritCredentialForEnvironment 保留同一来源的连接；跨历史环境保存时，
// 必须先按原环境解封，再由正常保存流程用目标环境重新封装，不能直接复制密文。
func (s *AgentSettingService) inheritCredentialForEnvironment(ctx context.Context, env string, tenant *string, in, old *dbmodel.AIProviderCredential) error {
	if in.Data == nil {
		in.Data = datatypes.JSONMap{}
	}
	if in.Data["__sealed"] == nil && old.Data["__sealed"] != nil {
		if old.Env == env {
			in.Data["__sealed"] = old.Data["__sealed"]
		} else {
			secrets := map[string]any{}
			if err := s.tks.UnsealSensitive(ctx, old.Env, s.tenantScopeKey(tenant), old.Data, &secrets); err != nil {
				return fmt.Errorf("无法读取历史环境凭据，原配置保持不变: %w", err)
			}
			keys := make([]string, 0, len(secrets))
			for key := range secrets {
				keys = append(keys, key)
			}
			sealed, err := s.tks.SealSensitive(ctx, env, s.tenantScopeKey(tenant), datatypes.JSONMap(secrets), keys...)
			if err != nil {
				return fmt.Errorf("无法重新封装历史环境凭据，原配置保持不变: %w", err)
			}
			in.Data["__sealed"] = sealed["__sealed"]
		}
	}
	if _, provided := in.Data["base_url"]; !provided {
		if base, _ := old.Data["base_url"].(string); base != "" {
			in.Data["base_url"] = base
		}
	}
	return nil
}
