package knowledge_space

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	reprocessEmbeddingNotConfigured  = "REPROCESS_EMBEDDING_NOT_CONFIGURED"
	reprocessEmbeddingProbeRequired  = "REPROCESS_EMBEDDING_PROBE_REQUIRED"
	reprocessEmbeddingProfileInvalid = "REPROCESS_EMBEDDING_PROFILE_INVALID"
)

type embeddingGateError struct {
	code   string
	reason string
}

func (e embeddingGateError) Error() string {
	return e.reason
}

func (w *ReprocessWorker) ensureEmbeddingReady(ctx context.Context, space *models.KnowledgeSpace) error {
	if w == nil || w.db == nil {
		return embeddingGateError{
			code:   reprocessEmbeddingNotConfigured,
			reason: "AI Settings 未初始化，无法执行 reprocess",
		}
	}
	if space == nil || space.UUID == uuid.Nil {
		return embeddingGateError{
			code:   reprocessEmbeddingNotConfigured,
			reason: "知识空间不存在，无法执行 reprocess",
		}
	}
	tenantUUID := strings.TrimSpace(space.TenantUUID)
	if tenantUUID == "" {
		return embeddingGateError{
			code:   reprocessEmbeddingNotConfigured,
			reason: "缺少租户上下文，无法确认 embedding 配置",
		}
	}
	profileKey := strings.TrimSpace(space.EmbeddingProfileKey)
	if profileKey == "" {
		return embeddingGateError{
			code:   reprocessEmbeddingNotConfigured,
			reason: "请先在 AI Settings 配置 embedding 模型并完成测试",
		}
	}
	provider, model, err := parseEmbeddingProfileKey(profileKey)
	if err != nil {
		return embeddingGateError{
			code:   reprocessEmbeddingProfileInvalid,
			reason: "embedding profile 配置无效，请重新设置",
		}
	}

	settings := agentsettings.NewAgentSettingService(w.db)
	env, _, err := settings.GetTenantCurrentAIEnv(ctx, tenantUUID)
	if err != nil && !isMissingTableError(err) {
		return err
	}
	if strings.TrimSpace(env) == "" {
		env = "default"
	}
	profile, err := settings.GetProfile(ctx, env, &tenantUUID, "embedding", provider, model)
	if err != nil {
		if isMissingTableError(err) || errors.Is(err, gorm.ErrRecordNotFound) {
			return embeddingGateError{
				code:   reprocessEmbeddingNotConfigured,
				reason: "embedding 配置不存在或未启用，请先在 AI Settings 完成配置",
			}
		}
		return err
	}
	if profile == nil {
		return embeddingGateError{
			code:   reprocessEmbeddingNotConfigured,
			reason: "embedding 配置不存在或未启用，请先在 AI Settings 完成配置",
		}
	}
	if !agentsettings.EmbeddingProfileReady(profile) {
		return embeddingGateError{
			code:   reprocessEmbeddingProbeRequired,
			reason: fmt.Sprintf("%s/%s 尚未完成测试（probe），请先在 AI Settings 执行测试", provider, model),
		}
	}
	return nil
}

func (w *ReprocessWorker) blockReprocess(
	ctx context.Context,
	jobs *repo.IngestionJobRepository,
	cases *repo.FeedbackCaseRepository,
	audits *repo.AuditTrailRepository,
	input ReprocessInput,
	jobSeq uint64,
	caseModel *models.FeedbackCase,
	gate embeddingGateError,
) error {
	if w == nil || jobs == nil || cases == nil || audits == nil {
		return nil
	}
	now := w.clock()
	job := &models.IngestionJob{
		SpaceUUID:     input.SpaceID,
		SourceID:      fmt.Sprintf("feedback:%s", input.CaseID.String()),
		SourceType:    "reprocess",
		Status:        models.IngestionStatusBlocked,
		Priority:      priorityFromSeverity(input.Severity),
		SubmittedBy:   strings.TrimSpace(input.RequestedBy),
		StartedAt:     &now,
		CompletedAt:   &now,
		ErrorCode:     gate.code,
		BlockedReason: gate.reason,
	}
	job.UUID = uuid.New()
	if _, err := jobs.Create(ctx, job); err != nil {
		return err
	}

	if caseModel != nil {
		caseModel.Status = models.FeedbackStatusEscalated
		caseModel.EscalatedAt = &now
		caseModel.ResolutionNotes = "reprocess blocked: " + gate.reason
		_, _ = cases.Update(ctx, caseModel)
	}

	_, _ = audits.Create(ctx, &models.AuditTrailEntry{
		SpaceUUID:     input.SpaceID,
		Action:        "feedback.reprocess.blocked",
		Actor:         strings.TrimSpace(input.RequestedBy),
		PayloadHash:   hexHash(gate.reason),
		Metadata:      marshalMap(map[string]any{"job_seq": jobSeq, "case_id": input.CaseID.String(), "error_code": gate.code}),
		OccurredAt:    now,
		RollbackToken: input.CaseID.String(),
	})
	return nil
}

func parseEmbeddingProfileKey(key string) (provider string, model string, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("embeddingProfileKey 不能为空")
	}
	var parts []string
	if strings.Contains(key, "/") {
		parts = strings.SplitN(key, "/", 2)
	} else if strings.Contains(key, ":") {
		parts = strings.SplitN(key, ":", 2)
	} else {
		return "", "", fmt.Errorf("embeddingProfileKey 格式错误（期望 provider/model 或 provider:model），got=%q", key)
	}
	provider = strings.ToLower(strings.TrimSpace(parts[0]))
	model = strings.TrimSpace(parts[1])
	if provider == "" || model == "" {
		return "", "", fmt.Errorf("embeddingProfileKey 格式错误（provider/model）")
	}
	return provider, model, nil
}

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "undefined_table") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "unknown table")
}
