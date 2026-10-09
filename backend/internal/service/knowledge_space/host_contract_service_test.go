package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func newHostContractTestService(t *testing.T, startWorker ...bool) (*HostContractService, *gorm.DB, string, string) {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open("file:knowledge_host_contract_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.KnowledgeSpace{}, &models.TenantDocument{}, &models.IndexJob{}, &models.HostDocumentChunk{}, &models.IngestionProfileVersion{}, &models.IndexProfileVersion{}, &models.RAGProfileVersion{}))
	tenantUUID := uuid.NewString()
	require.NoError(t, db.Create(&models.KnowledgeSpace{TenantUUID: tenantUUID, SpaceName: "contract", DepartmentCode: "knowledge", Status: models.KnowledgeSpaceStatusActive}).Error)
	var space models.KnowledgeSpace
	require.NoError(t, db.Where("tenant_uuid = ?", tenantUUID).First(&space).Error)
	svc := NewHostContractService(db)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	if len(startWorker) == 0 || startWorker[0] {
		go func() { defer close(done); _ = svc.Run(ctx) }()
	} else {
		close(done)
	}
	t.Cleanup(func() { cancel(); <-done; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	return svc, db, tenantUUID, space.UUID.String()
}

func hostChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestHostContractUpsertReturnsQueuedJobThenSearchableCitation(t *testing.T) {
	svc, _, tenantUUID, spaceUUID := newHostContractTestService(t)
	ctx := context.Background()
	content := "退款政策支持七日内申请退款。"
	accepted, err := svc.UpsertDocument(ctx, tenantUUID, spaceUUID, HostDocumentInput{Title: "退款政策", URI: "powerx://policy/refund", Content: content, ContentType: "text/markdown", Checksum: hostChecksum(content), Version: "2026-09-02", Tags: []string{"refund"}})
	require.NoError(t, err)
	require.Equal(t, HostIndexStatusQueued, accepted.Status)
	require.Equal(t, HostIndexOperationUpsert, accepted.Operation)
	require.NotEmpty(t, accepted.JobUUID)

	require.Eventually(t, func() bool {
		job, getErr := svc.GetIndexJob(ctx, tenantUUID, accepted.JobUUID)
		return getErr == nil && job.Status == HostIndexStatusSucceeded
	}, time.Second, 10*time.Millisecond)

	items, err := svc.Search(ctx, tenantUUID, "退款", []string{spaceUUID}, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, accepted.DocumentUUID, items[0].DocumentUUID)
	require.Equal(t, "退款政策", items[0].Title)
	require.Equal(t, "powerx://policy/refund", items[0].URI)
}

func TestHostContractRejectsCrossTenantAndDuplicateChecksum(t *testing.T) {
	svc, _, tenantUUID, spaceUUID := newHostContractTestService(t)
	ctx := context.Background()
	content := "stable knowledge content"
	in := HostDocumentInput{Title: "policy", URI: "powerx://policy/a", Content: content, ContentType: "text/markdown", Checksum: hostChecksum(content), Version: "v1"}
	_, err := svc.UpsertDocument(ctx, tenantUUID, spaceUUID, in)
	require.NoError(t, err)
	_, err = svc.UpsertDocument(ctx, tenantUUID, spaceUUID, in)
	require.Equal(t, KnowledgeReasonIndexConflict, dto.CodeOf(err))
	_, err = svc.UpsertDocument(ctx, uuid.NewString(), spaceUUID, in)
	require.Equal(t, KnowledgeReasonSpaceNotFound, dto.CodeOf(err))
}

func TestHostContractRejectsNumericAndDuplicateUUIDInput(t *testing.T) {
	svc, _, tenantUUID, spaceUUID := newHostContractTestService(t)
	_, err := svc.Search(context.Background(), tenantUUID, "x", []string{"123"}, 20)
	require.Equal(t, KnowledgeReasonInvalidArgument, dto.CodeOf(err))
	_, err = svc.Search(context.Background(), tenantUUID, "x", []string{spaceUUID, spaceUUID}, 20)
	require.Equal(t, KnowledgeReasonInvalidArgument, dto.CodeOf(err))
}

func TestUpsertLegacyDocumentInitializesMissingVisibilityEpoch(t *testing.T) {
	service, db, tenant, space := newHostContractTestService(t, false)
	input := HostDocumentInput{Title: "legacy-visibility", URI: "powerx://test/legacy-visibility", Content: "原文保持不变", ContentType: "text/plain", Checksum: hostChecksum("原文保持不变"), Version: "v1"}
	accepted, err := service.UpsertDocument(context.Background(), tenant, space, input)
	require.NoError(t, err)
	_, err = service.ProcessNextDocumentJob(context.Background())
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.TenantDocument{}).Where("uuid = ?", accepted.DocumentUUID).Update("visibility_epoch", nil).Error)
	input.Version = "v2"
	updated, err := service.UpsertDocument(context.Background(), tenant, space, input)
	require.NoError(t, err)
	require.Equal(t, accepted.DocumentUUID, updated.DocumentUUID)
	var row models.TenantDocument
	require.NoError(t, db.Where("uuid = ?", accepted.DocumentUUID).First(&row).Error)
	require.True(t, validSemanticUUID(row.VisibilityEpoch))
}
