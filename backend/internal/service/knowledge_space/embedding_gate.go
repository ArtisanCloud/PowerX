package knowledge_space

import (
	"context"
	"net/http"
	"strings"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func (s *Service) ensureTenantEmbeddingConfigured(ctx context.Context, tenantUUID string) error {
	if s == nil || s.db == nil {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "AI Settings 未初始化，无法创建空间", ErrEmbeddingNotConfigured)
	}
	tid := strings.TrimSpace(tenantUUID)
	if tid == "" {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "缺少租户上下文，无法确认 embedding 配置", ErrEmbeddingNotConfigured)
	}
	settings := agentsettings.NewAgentSettingService(s.db)
	env, _, err := settings.GetTenantCurrentAIEnv(ctx, tid)
	if err != nil && !isMissingTableError(err) {
		return dto.NewErrorWithCode(http.StatusInternalServerError, "embedding_failed", "读取 AI 环境失败", err)
	}
	if strings.TrimSpace(env) == "" {
		env = "dev"
	}
	profiles, err := settings.ListProfiles(ctx, env, &tid, "embedding")
	if err != nil && !isMissingTableError(err) {
		return dto.NewErrorWithCode(http.StatusInternalServerError, "embedding_failed", "读取 embedding 配置失败", err)
	}
	if hasReadyEmbeddingProfile(profiles) {
		return nil
	}
	return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "请先在 AI Settings 配置 embedding 模型并完成测试", ErrEmbeddingNotConfigured)
}

func hasReadyEmbeddingProfile(profiles []agentmodel.AIModelProfile) bool {
	for i := range profiles {
		if agentsettings.EmbeddingProfileReady(&profiles[i]) {
			return true
		}
	}
	return false
}
