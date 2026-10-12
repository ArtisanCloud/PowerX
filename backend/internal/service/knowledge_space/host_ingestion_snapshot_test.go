package knowledge_space

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func ptr[T any](value T) *T { return &value }
func TestHostSnapshot64080ExecutesAndSurvivesProfileAndDocumentChange(t *testing.T) {
	svc, db, tenant, space := newHostContractTestService(t)
	profile := models.IngestionProfileVersion{TenantUUID: tenant, ProfileKey: "frozen", Version: 3, Status: models.ProfileStatusPublished, Config: datatypes.JSON(`{"chunking":{"chunk_size":256,"chunk_overlap":16}}`)}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Model(&models.KnowledgeSpace{}).Where("uuid = ?", space).Update("ingestion_profile_uuid", profile.UUID).Error)
	content := strings.Repeat("甲乙丙丁戊己庚辛壬癸", 200)
	input := HostDocumentInput{Title: "snapshot", URI: "powerx://snapshot/test", Content: content, ContentType: "text/plain", Checksum: hostChecksum(content), Version: "source-v1", Ingestion: &HostIngestionSettings{Schema: HostIngestionSnapshotSchema, IngestionProfile: &HostTaskProfileRef{profile.UUID.String(), 3}, ChunkSize: ptr(640), ChunkOverlap: ptr(80), PagePriority: ptr(false), Separators: &[]string{}}}
	accepted, err := svc.UpsertDocument(context.Background(), tenant, space, input)
	require.NoError(t, err)
	// 修改已发布 Profile 的配置甚至归档，已受理任务仍只能使用冻结副本。
	require.NoError(t, db.Model(&profile).Updates(map[string]any{"config": datatypes.JSON(`{"chunking":{"chunk_size":128}}`), "status": models.ProfileStatusArchived}).Error)
	var job HostJob
	require.Eventually(t, func() bool {
		job, err = svc.GetIndexJob(context.Background(), tenant, accepted.JobUUID)
		return err == nil && job.Status == HostIndexStatusSucceeded
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 640, job.EffectiveConfig.ChunkSize)
	require.Equal(t, 80, job.EffectiveConfig.ChunkOverlap)
	require.Equal(t, 3, job.EffectiveConfig.IngestionProfile.Version)
	require.False(t, job.EffectiveConfig.PagePriority)
	require.Equal(t, checksumJSON([]byte(`{"chunking":{"chunk_size":256,"chunk_overlap":16}}`)), job.EffectiveConfig.IngestionProfile.ConfigChecksum)
	var persisted models.IndexJob
	require.NoError(t, db.First(&persisted, "uuid = ?", accepted.JobUUID).Error)
	var frozen HostIngestionSnapshot
	require.NoError(t, json.Unmarshal(persisted.ConfigSnapshot, &frozen))
	require.JSONEq(t, `{"chunking":{"chunk_size":256,"chunk_overlap":16}}`, string(frozen.IngestionProfile.Config))
	chunks, err := svc.GetJobChunks(context.Background(), tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Len(t, chunks, 4)
	for i, ch := range chunks {
		require.LessOrEqual(t, utf8.RuneCountInString(ch.Content), 640)
		require.Equal(t, hostChecksum(ch.Content), ch.Checksum)
		if i > 0 {
			left, right := []rune(chunks[i-1].Content), []rune(ch.Content)
			require.Equal(t, string(left[len(left)-80:]), string(right[:80]))
		}
	}
	before, _ := json.Marshal(job.EffectiveConfig)
	require.NoError(t, db.Model(&models.TenantDocument{}).Where("uuid = ?", accepted.DocumentUUID).Updates(map[string]any{"content": "changed source", "version": "source-v2"}).Error)
	require.NoError(t, db.Model(&models.KnowledgeSpace{}).Where("uuid = ?", space).Update("ingestion_profile_uuid", nil).Error)
	reread, err := svc.GetIndexJob(context.Background(), tenant, accepted.JobUUID)
	require.NoError(t, err)
	after, _ := json.Marshal(reread.EffectiveConfig)
	require.Equal(t, string(before), string(after))
	oldChunks, err := svc.GetJobChunks(context.Background(), tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Equal(t, chunks[0].Content, oldChunks[0].Content)
}
func TestHostSnapshotRejectsInvalidWithoutLeavingJobsOrDocuments(t *testing.T) {
	svc, db, tenant, space := newHostContractTestService(t)
	profile := models.IngestionProfileVersion{TenantUUID: tenant, ProfileKey: "valid", Version: 1, Status: models.ProfileStatusPublished, Config: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Model(&models.KnowledgeSpace{}).Where("uuid = ?", space).Update("ingestion_profile_uuid", profile.UUID).Error)
	for _, tc := range []struct {
		name     string
		settings HostIngestionSettings
		code     string
	}{
		{"bad_overlap", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, ChunkSize: ptr(640), ChunkOverlap: ptr(640)}, KnowledgeReasonInvalidArgument},
		{"negative", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, ChunkSize: ptr(-1)}, KnowledgeReasonInvalidArgument},
		{"mode", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, SegmentMode: ptr("semantic")}, KnowledgeReasonUnsupportedSetting},
		{"order", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, SegmentOrder: &[]string{"size", "segment", "invalid", "size"}}, KnowledgeReasonInvalidArgument},
		{"profile", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, IngestionProfile: &HostTaskProfileRef{profile.UUID.String(), 99}}, KnowledgeReasonProfileUnavailable},
		{"cross_profile", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, IngestionProfile: &HostTaskProfileRef{uuid.NewString(), 1}}, KnowledgeReasonProfileUnavailable},
		{"processor", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, ProcessorProfile: &HostTaskProfileRef{uuid.NewString(), 1}}, KnowledgeReasonUnsupportedSetting},
		{"page", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, PagePriority: ptr(true)}, KnowledgeReasonUnsupportedSetting},
		{"anchor", HostIngestionSettings{Schema: HostIngestionSnapshotSchema, AnchorSpeaker: ptr(true)}, KnowledgeReasonUnsupportedSetting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.UpsertDocument(context.Background(), tenant, space, HostDocumentInput{Title: "invalid", URI: "powerx://" + tc.name, Content: "text", ContentType: "text/plain", Checksum: hostChecksum("text"), Version: "v1", Ingestion: &tc.settings})
			require.Equal(t, tc.code, dto.CodeOf(err))
		})
	}
	var count int64
	require.NoError(t, db.Model(&models.TenantDocument{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&models.IndexJob{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestHostQueuePriorityAndClaimFencing(t *testing.T) {
	svc, db, tenant, space := newHostContractTestService(t, false)
	normal := models.IndexJob{TenantUUID: tenant, SpaceUUID: space, Operation: "upsert", Status: HostIndexStatusQueued, Priority: "normal"}
	high := models.IndexJob{TenantUUID: tenant, SpaceUUID: space, Operation: "upsert", Status: HostIndexStatusQueued, Priority: "high"}
	require.NoError(t, db.Create(&normal).Error)
	require.NoError(t, db.Create(&high).Error)
	handled, err := svc.ProcessNextDocumentJob(context.Background())
	require.NoError(t, err)
	require.True(t, handled)
	require.NoError(t, db.First(&normal, "uuid = ?", normal.UUID).Error)
	require.Equal(t, HostIndexStatusQueued, normal.Status)
	job, err := svc.GetIndexJob(context.Background(), tenant, high.UUID.String())
	require.NoError(t, err)
	require.Equal(t, HostIndexStatusFailed, job.Status)
	require.Equal(t, KnowledgeReasonSnapshotInvalid, job.ErrorCode)
	handled, err = svc.ProcessNextDocumentJob(context.Background())
	require.NoError(t, err)
	require.True(t, handled)
	expired := time.Now().Add(-time.Minute)
	interrupted := models.IndexJob{TenantUUID: tenant, SpaceUUID: space, Operation: "upsert", Status: HostIndexStatusRunning, Priority: "normal", ClaimToken: "stale", LeaseUntil: &expired}
	require.NoError(t, db.Create(&interrupted).Error)
	_, err = svc.ProcessNextDocumentJob(context.Background())
	require.NoError(t, err)
	job, err = svc.GetIndexJob(context.Background(), tenant, interrupted.UUID.String())
	require.NoError(t, err)
	require.Equal(t, "KNOWLEDGE_WORKER_INTERRUPTED", job.ErrorCode)
}
func TestHostSnapshotChecksumSurvivesJSONBSerialization(t *testing.T) {
	require.Equal(t, checksumJSON([]byte(`{"b":false,"a":640}`)), checksumJSON([]byte(`{ "a": 640, "b": false }`)))
}

func TestHostSnapshotLiteralNewlineSeparatorAndChunkBudget(t *testing.T) {
	svc, db, tenant, space := newHostContractTestService(t)
	profile := models.IngestionProfileVersion{TenantUUID: tenant, ProfileKey: "literal", Version: 1, Status: models.ProfileStatusPublished, Config: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Model(&models.KnowledgeSpace{}).Where("uuid = ?", space).Update("ingestion_profile_uuid", profile.UUID).Error)
	source := "第一段完整文本\n\n第二段完整文本"
	input := HostDocumentInput{Title: "literal", URI: "powerx://literal/separator", Content: source, ContentType: "text/plain", Checksum: hostChecksum(source), Version: "v1", Ingestion: &HostIngestionSettings{Schema: HostIngestionSnapshotSchema, ChunkSize: ptr(0), ChunkOverlap: ptr(0), Separators: &[]string{"\n\n"}}}
	accepted, err := svc.UpsertDocument(context.Background(), tenant, space, input)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		job, _ := svc.GetIndexJob(context.Background(), tenant, accepted.JobUUID)
		return job.Status == HostIndexStatusSucceeded
	}, 2*time.Second, 10*time.Millisecond)
	chunks, err := svc.GetJobChunks(context.Background(), tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	require.Equal(t, "第一段完整文本", chunks[0].Content)
	require.Equal(t, "第二段完整文本", chunks[1].Content)
	config := HostIngestionSnapshot{ChunkSize: 16, ChunkOverlap: 15}
	require.Equal(t, KnowledgeReasonInvalidArgument, dto.CodeOf(validateHostChunkBudget(config, strings.Repeat("x", 4097))))
}
