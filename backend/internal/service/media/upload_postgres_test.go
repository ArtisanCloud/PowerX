package media

import (
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 原生 PostgreSQL 独立 schema 验证行锁及重试；不会修改现有业务表。
func TestMediaUploadRetryPostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_MEDIA_TEST_DSN")
	if dsn == "" {
		t.Skip("set POWERX_MEDIA_TEST_DSN for native PostgreSQL upload acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	schema := "media_upload_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	previousSchema := model.PowerXSchema
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		model.PowerXSchema = previousSchema
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&m.MediaAsset{}, &m.MediaAssetVariant{}))
	storage, err := local.New(local.Options{BasePath: t.TempDir()})
	require.NoError(t, err)
	manager := mediamgr.New("local")
	manager.RegisterDriver(storage)
	svc := NewMediaService(db, nil, manager, &stubAuditService{}, time.Hour)
	svc.SetPublicResourceTokenSecret("postgres-media-test-secret")
	ctx := context.Background()
	data, declared := officeFixture(t, "xlsx")
	if path := os.Getenv("POWERX_MEDIA_TEST_XLSX"); path != "" {
		data, err = os.ReadFile(path)
		require.NoError(t, err)
	}
	checksum := payloadHash(data)
	owner := uuid.NewString()
	in := CreateAssetInput{TenantUUID: mediaTenantUUID, UploadMethod: UploadMethodPresign, Name: "book.xlsx", MimeType: declared, SizeBytes: int64(len(data)), ContentSHA256: checksum, OwnerSubjectUUID: owner}
	asset, err := svc.CreateAsset(ctx, in)
	require.NoError(t, err)
	require.NoError(t, db.Model(&m.MediaAsset{}).Where("uuid = ?", asset.UUID).Updates(map[string]any{"upload_state": m.UploadStateFailed, "upload_expires_at": nil, "upload_ticket_version": 2, "meta": `{"custom":"preserved","upload_failures":[{"reason":"mime_mismatch"}]}`}).Error)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := svc.CreateAsset(ctx, in); results <- e }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	var row m.MediaAsset
	require.NoError(t, db.Where("uuid = ?", asset.UUID).First(&row).Error)
	require.Equal(t, uint64(3), row.UploadTicketVersion)
	require.Equal(t, m.UploadStatePending, row.UploadState)
	require.Equal(t, owner, row.OwnerSubjectUUID.String())
	require.Contains(t, string(row.Meta), "mime_mismatch")
	// 错误租户只能得到未找到，不能改变原资产的状态或版本。
	_, err = svc.CompleteAssetUpload(ctx, CompleteAssetUploadInput{TenantUUID: uuid.NewString(), UUID: asset.UUID, Checksum: checksum})
	require.ErrorIs(t, err, ErrAssetNotFound)
	ticket, err := svc.IssueHostTransferTicket(ctx, mediaTenantUUID, asset.UUID, "upload", time.Minute)
	require.NoError(t, err)
	snapshot := authorizeUpload(t, svc, ticket)
	require.NoError(t, svc.PutHostTransfer(ctx, snapshot, bytes.NewReader(data), int64(len(data)), declared))
	ready, err := svc.CompleteAssetUpload(ctx, CompleteAssetUploadInput{TenantUUID: mediaTenantUUID, UUID: asset.UUID, Checksum: checksum})
	require.NoError(t, err)
	require.Equal(t, m.UploadStateReady, ready.UploadState)
	_, object, err := svc.OpenAssetResource(ctx, mediaTenantUUID, asset.UUID)
	require.NoError(t, err)
	read, err := io.ReadAll(object.Body)
	require.NoError(t, err)
	require.NoError(t, object.Body.Close())
	require.Equal(t, data, read)
	// Variant 的失败重试及元数据也真实落库。
	variantInput := CreateAssetVariantInput{TenantUUID: mediaTenantUUID, AssetUUID: asset.UUID, Variant: "preview", MimeType: declared, SizeBytes: int64(len(data)), Checksum: checksum}
	v, err := svc.CreateAssetVariant(ctx, variantInput)
	require.NoError(t, err)
	require.NoError(t, db.Model(&m.MediaAssetVariant{}).Where("uuid = ?", v.UUID).Updates(map[string]any{"upload_state": m.UploadStateFailed, "ticket_version": 2, "upload_expires_at": nil, "meta": `{"upload_failures":[{"reason":"mime_mismatch"}]}`}).Error)
	v, err = svc.CreateAssetVariant(ctx, variantInput)
	require.NoError(t, err)
	vt, err := svc.IssueVariantTransferTicket(ctx, mediaTenantUUID, asset.UUID, v.UUID, "upload", time.Minute)
	require.NoError(t, err)
	_, err = svc.ConsumeVariantTransfer(ctx, asset.UUID, v.UUID, "upload", strconv.FormatInt(vt.ExpiresAt.Unix(), 10), strconv.FormatUint(vt.Version, 10), strconv.FormatUint(vt.ParentVersion, 10), vt.Token, bytes.NewReader(data), int64(len(data)), declared)
	require.NoError(t, err)
	v, err = svc.CompleteVariantUpload(ctx, mediaTenantUUID, asset.UUID, v.UUID, checksum)
	require.NoError(t, err)
	require.Equal(t, m.UploadStateReady, v.UploadState)
	var variantRow m.MediaAssetVariant
	require.NoError(t, db.Where("uuid = ?", v.UUID).First(&variantRow).Error)
	require.Contains(t, string(variantRow.Meta), "mime_mismatch")
	if path := os.Getenv("POWERX_MEDIA_TEST_MARKDOWN"); path != "" {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		input := CreateAssetInput{TenantUUID: mediaTenantUUID, UploadMethod: UploadMethodPresign, Name: "README.md", MimeType: "text/markdown", SizeBytes: int64(len(data)), ContentSHA256: payloadHash(data)}
		asset, err := svc.CreateAsset(ctx, input)
		require.NoError(t, err)
		require.NoError(t, db.Model(&m.MediaAsset{}).Where("uuid = ?", asset.UUID).Updates(map[string]any{"upload_state": m.UploadStateFailed, "upload_expires_at": nil, "upload_ticket_version": 2}).Error)
		asset, err = svc.CreateAsset(ctx, input)
		require.NoError(t, err)
		ticket, err := svc.IssueHostTransferTicket(ctx, mediaTenantUUID, asset.UUID, "upload", time.Minute)
		require.NoError(t, err)
		require.NoError(t, svc.PutHostTransfer(ctx, authorizeUpload(t, svc, ticket), bytes.NewReader(data), int64(len(data)), input.MimeType))
		asset, err = svc.CompleteAssetUpload(ctx, CompleteAssetUploadInput{TenantUUID: mediaTenantUUID, UUID: asset.UUID, Checksum: input.ContentSHA256})
		require.NoError(t, err)
		require.Equal(t, m.UploadStateReady, asset.UploadState)
	}
}
