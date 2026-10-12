package knowledge_space

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func seedHostRebuildDocuments(t *testing.T) (*HostContractService, string, string, string, string, HostTaskProfileRef) {
	t.Helper()
	svc, db, tenant, space := newHostContractTestService(t, false)
	profile := models.IngestionProfileVersion{TenantUUID: tenant, ProfileKey: "rebuild", Version: 1, Status: models.ProfileStatusPublished, Config: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Model(&models.KnowledgeSpace{}).Where("uuid = ?", space).Update("ingestion_profile_uuid", profile.UUID).Error)
	ids := []string{}
	for _, name := range []string{"first", "second"} {
		text := name + strings.Repeat("甲乙丙丁", 500)
		accepted, err := svc.UpsertDocument(context.Background(), tenant, space, HostDocumentInput{Title: name, URI: "powerx://rebuild/" + name, Content: text, Checksum: hostChecksum(text), ContentType: "text/plain", Version: "v1", Ingestion: &HostIngestionSettings{Schema: HostIngestionSnapshotSchema, ChunkSize: ptr(640), ChunkOverlap: ptr(80)}})
		require.NoError(t, err)
		handled, err := svc.ProcessNextDocumentJob(context.Background())
		require.NoError(t, err)
		require.True(t, handled)
		job, err := svc.GetIndexJob(context.Background(), tenant, accepted.JobUUID)
		require.NoError(t, err)
		require.Equal(t, HostIndexStatusSucceeded, job.Status)
		ids = append(ids, accepted.DocumentUUID)
	}
	return svc, tenant, space, ids[0], ids[1], HostTaskProfileRef{profile.UUID.String(), 1}
}
func activeHostJob(t *testing.T, svc *HostContractService, document string) string {
	t.Helper()
	var row models.TenantDocument
	require.NoError(t, svc.db.Where("uuid = ?", document).First(&row).Error)
	require.NotNil(t, row.ActiveIndexJobUUID)
	return *row.ActiveIndexJobUUID
}
func TestHostDocumentRebuildModesIdempotencyAndAtomicActivation(t *testing.T) {
	svc, tenant, space, first, second, profile := seedHostRebuildDocuments(t)
	ctx := context.Background()
	oldFirst, oldSecond := activeHostJob(t, svc, first), activeHostJob(t, svc, second)
	// Profile 归档后 reuse 仍使用上次成功任务，而 apply 必须重新校验 published。
	require.NoError(t, svc.db.Model(&models.IngestionProfileVersion{}).Where("uuid = ?", profile.UUID).Update("status", models.ProfileStatusArchived).Error)
	req := HostRebuildInput{Mode: HostRebuildReuseSnapshot, IdempotencyKey: "click-first"}
	accepted, err := svc.RebuildDocument(ctx, tenant, space, first, req)
	require.NoError(t, err)
	require.Equal(t, first, accepted.DocumentUUID)
	again, err := svc.RebuildDocument(ctx, tenant, space, first, req)
	require.NoError(t, err)
	require.Equal(t, accepted.JobUUID, again.JobUUID)
	_, err = svc.RebuildDocument(ctx, tenant, space, second, req)
	require.Equal(t, KnowledgeReasonIndexConflict, dto.CodeOf(err))
	_, err = svc.RebuildDocument(ctx, tenant, space, first, HostRebuildInput{Mode: HostRebuildReuseSnapshot, IdempotencyKey: "another-click"})
	require.Equal(t, KnowledgeReasonIndexConflict, dto.CodeOf(err))
	_, err = svc.RebuildSpace(ctx, tenant, space, HostRebuildInput{})
	require.Equal(t, KnowledgeReasonIndexConflict, dto.CodeOf(err))
	require.Equal(t, oldFirst, activeHostJob(t, svc, first))
	require.Equal(t, oldSecond, activeHostJob(t, svc, second))
	hits, err := svc.Search(ctx, tenant, "first", []string{space}, 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	_, err = svc.ProcessNextDocumentJob(ctx)
	require.NoError(t, err)
	job, err := svc.GetIndexJob(ctx, tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Equal(t, HostIndexStatusSucceeded, job.Status)
	require.Equal(t, 640, job.EffectiveConfig.ChunkSize)
	require.Equal(t, 1, job.DocumentCount)
	require.Equal(t, 1, job.ProcessedDocuments)
	require.Equal(t, accepted.JobUUID, activeHostJob(t, svc, first))
	require.Equal(t, oldSecond, activeHostJob(t, svc, second))
	require.NoError(t, svc.db.Model(&models.IngestionProfileVersion{}).Where("uuid = ?", profile.UUID).Update("status", models.ProfileStatusPublished).Error)
	newReq := HostRebuildInput{Mode: HostRebuildApplyConfig, IdempotencyKey: "new-config", Ingestion: &HostIngestionSettings{Schema: HostIngestionSnapshotSchema, IngestionProfile: &profile, ChunkSize: ptr(320), ChunkOverlap: ptr(40)}}
	next, err := svc.RebuildDocument(ctx, tenant, space, first, newReq)
	require.NoError(t, err)
	_, err = svc.ProcessNextDocumentJob(ctx)
	require.NoError(t, err)
	job, err = svc.GetIndexJob(ctx, tenant, next.JobUUID)
	require.NoError(t, err)
	require.Equal(t, 320, job.EffectiveConfig.ChunkSize)
	require.Greater(t, job.ChunkCount, 4)
	oldChunks, err := svc.GetJobChunks(ctx, tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Len(t, oldChunks, 4)
}
func TestHostRebuildFailureRetainsIndexAndAllowsRetry(t *testing.T) {
	svc, tenant, space, first, _, _ := seedHostRebuildDocuments(t)
	ctx := context.Background()
	old := activeHostJob(t, svc, first)
	accepted, err := svc.RebuildDocument(ctx, tenant, space, first, HostRebuildInput{IdempotencyKey: "failure"})
	require.NoError(t, err)
	// 错误快照触发真实 Worker 失败路径，不能切换 active 或污染检索。
	require.NoError(t, svc.db.Model(&models.IndexJob{}).Where("uuid = ?", accepted.JobUUID).Update("config_snapshot", datatypes.JSON(`{"corrupt":true}`)).Error)
	_, err = svc.ProcessNextDocumentJob(ctx)
	require.NoError(t, err)
	var row models.IndexJob
	require.NoError(t, svc.db.Where("uuid = ?", accepted.JobUUID).First(&row).Error)
	require.Equal(t, HostIndexStatusFailed, row.Status)
	require.Equal(t, old, activeHostJob(t, svc, first))
	hits, err := svc.Search(ctx, tenant, "first", []string{space}, 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	retry, err := svc.RebuildDocument(ctx, tenant, space, first, HostRebuildInput{IdempotencyKey: "retry-failure"})
	require.NoError(t, err)
	require.NotEqual(t, accepted.JobUUID, retry.JobUUID)
	_, err = svc.ProcessNextDocumentJob(ctx)
	require.NoError(t, err)
	job, err := svc.GetIndexJob(ctx, tenant, retry.JobUUID)
	require.NoError(t, err)
	require.Equal(t, HostIndexStatusSucceeded, job.Status)
}
func TestHostSpaceRebuildReprocessesAllAndRejectsCrossTenant(t *testing.T) {
	svc, tenant, space, first, second, profile := seedHostRebuildDocuments(t)
	ctx := context.Background()
	_, err := svc.RebuildDocument(ctx, uuid.NewString(), space, first, HostRebuildInput{})
	require.Equal(t, KnowledgeReasonSpaceNotFound, dto.CodeOf(err))
	_, err = svc.RebuildDocument(ctx, tenant, space, uuid.NewString(), HostRebuildInput{})
	require.Equal(t, KnowledgeReasonDocumentNotFound, dto.CodeOf(err))
	_, err = svc.RebuildDocument(ctx, tenant, space, first, HostRebuildInput{Mode: HostRebuildApplyConfig})
	require.Equal(t, KnowledgeReasonInvalidArgument, dto.CodeOf(err))
	accepted, err := svc.RebuildSpace(ctx, tenant, space, HostRebuildInput{Mode: HostRebuildApplyConfig, IdempotencyKey: "whole-space", Ingestion: &HostIngestionSettings{Schema: HostIngestionSnapshotSchema, IngestionProfile: &profile, ChunkSize: ptr(400), ChunkOverlap: ptr(50)}})
	require.NoError(t, err)
	_, err = svc.ProcessNextDocumentJob(ctx)
	require.NoError(t, err)
	job, err := svc.GetIndexJob(ctx, tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Equal(t, 2, job.DocumentCount)
	require.Equal(t, 2, job.ProcessedDocuments)
	require.Len(t, job.DocumentConfigs, 2)
	require.Equal(t, accepted.JobUUID, activeHostJob(t, svc, first))
	require.Equal(t, accepted.JobUUID, activeHostJob(t, svc, second))
	// 空间任务之后仍可从其中单篇的配置开始重建，不扩大到其他文档。
	next, err := svc.RebuildDocument(ctx, tenant, space, first, HostRebuildInput{IdempotencyKey: "after-space"})
	require.NoError(t, err)
	_, err = svc.ProcessNextDocumentJob(ctx)
	require.NoError(t, err)
	job, err = svc.GetIndexJob(ctx, tenant, next.JobUUID)
	require.NoError(t, err)
	require.Equal(t, 400, job.EffectiveConfig.ChunkSize)
	require.Equal(t, accepted.JobUUID, activeHostJob(t, svc, second))
	chunks, err := svc.GetJobChunks(ctx, tenant, next.JobUUID)
	require.NoError(t, err)
	for _, ch := range chunks {
		require.Equal(t, first, ch.DocumentUUID)
	}
	_, _ = json.Marshal(job)
}
