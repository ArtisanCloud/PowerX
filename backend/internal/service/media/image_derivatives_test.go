package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type derivativeFixture struct {
	s     *MediaService
	redis *redis.Client
	db    *gorm.DB
	ctx   context.Context
	mini  *miniredis.Miniredis
}

func newDerivativeFixture(t *testing.T) derivativeFixture {
	t.Helper()
	ctx := context.Background()
	store, err := local.New(local.Options{BasePath: t.TempDir()})
	require.NoError(t, err)
	manager := mediamgr.New("local")
	manager.RegisterDriver(store)
	var db *gorm.DB

	if dsn := os.Getenv("POWERX_MEDIA_TEST_DSN"); dsn != "" {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		pool, e := db.DB()
		require.NoError(t, e)
		schema := "media_derivative_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		previous := core.PowerXSchema
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		core.PowerXSchema = schema
		t.Cleanup(func() {
			core.PowerXSchema = previous
			require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
			require.NoError(t, pool.Close())
		})

	} else {
		previous := core.PowerXSchema
		core.PowerXSchema = "main"
		t.Cleanup(func() { core.PowerXSchema = previous })
		db, err = gorm.Open(sqlite.Open(t.TempDir()+"/media.db?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		pool, e := db.DB()
		require.NoError(t, e)
		pool.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = pool.Close() })
		for _, model := range []any{&m.MediaAsset{}, &m.MediaAssetVariant{}, &ef.TaskHistory{}} {
			stmt := &gorm.Statement{DB: db}
			require.NoError(t, stmt.Parse(model))
			for _, field := range stmt.Schema.Fields {
				field.DefaultValue = strings.TrimSuffix(field.DefaultValue, "::jsonb")
				if field.DataType == "timestamp with time zone" {
					field.DataType = "datetime"
					field.TagSettings["TYPE"] = "datetime"
				}
				if strings.Contains(strings.ToLower(field.TagSettings["INDEX"]), "gin") {
					delete(field.TagSettings, "INDEX")
				}
			}
		}
	}
	require.NoError(t, db.AutoMigrate(&m.MediaAsset{}, &m.MediaAssetVariant{}, &ef.TaskHistory{}))
	s := NewMediaService(db, nil, manager, &stubAuditService{}, time.Hour)
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	queue, err := event_bus.NewRedisStreamTaskDriver(client, 2*time.Minute)
	require.NoError(t, err)
	s.ConfigureDerivativeQueue(taskqueue.NewStreamDriverWithRetryDelay(db, queue, time.Millisecond))
	return derivativeFixture{s, client, db, ctx, mini}
}
func imageFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1800, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 1800; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: uint8(x % 255), B: 20, A: 128})
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, img))
	return out.Bytes()
}
func (f derivativeFixture) upload(t *testing.T, raw []byte) (*Asset, CreateAssetInput) {
	t.Helper()
	in := CreateAssetInput{TenantUUID: mediaTenantUUID, Name: "image.png", MimeType: "image/png", SizeBytes: int64(len(raw)), ContentSHA256: payloadHash(raw), UploadMethod: UploadMethodPresign}
	a, err := f.s.CreateAsset(f.ctx, in)
	require.NoError(t, err)
	require.Equal(t, "waiting_upload", a.Derivatives.Status)
	_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: a.MimeType})
	require.NoError(t, err)
	a, err = f.s.CompleteAssetUpload(f.ctx, CompleteAssetUploadInput{TenantUUID: mediaTenantUUID, UUID: a.UUID, Checksum: payloadHash(raw)})
	require.NoError(t, err)
	require.Equal(t, "ready", a.UploadState)
	require.Equal(t, "queued", a.Derivatives.Status)
	return a, in
}
func (f derivativeFixture) consume(t *testing.T) {
	t.Helper()
	ds, err := f.s.derivativeQueue.Dequeue(f.ctx, derivativeQueueTenant, derivativeSubscriber, derivativeGroup, "test-worker", 10, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, ds, 1)
	require.NoError(t, f.s.handleDerivativeDelivery(f.ctx, ds[0]))
}

func TestImageDerivativesUploadReuseRepairAndIsolation(t *testing.T) {
	f := newDerivativeFixture(t)
	raw := imageFixture(t)
	a, in := f.upload(t, raw)
	require.False(t, f.db.Migrator().HasTable(core.PowerXSchema+".media_derivative_jobs"))
	originalRow, err := f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	ref := derivativeRef(originalRow)
	history, err := f.s.derivativeQueue.Status(f.ctx, mediaTenantUUID, derivativeSubscriber, ref.TaskID)
	require.NoError(t, err)
	require.NotNil(t, history)
	require.Equal(t, ef.TaskHistoryStatusPending, history.Status)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(originalRow.Meta, &meta))
	reference := meta["image_derivatives"].(map[string]any)
	for _, forbidden := range []string{"status", "attempts", "run_token", "lease_until", "error_code"} {
		require.NotContains(t, reference, forbidden, "Media 不保存统一任务生命周期")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	f.consume(t)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
	require.Len(t, state.Items, 2)
	// 兼容尚未迁移的客户端：Core 标准预览可复用，调用方不必生成相同 JPEG/PNG 字节。
	legacy, err := f.s.CreateAssetVariant(f.ctx, CreateAssetVariantInput{TenantUUID: mediaTenantUUID, AssetUUID: a.UUID, Variant: "preview", MimeType: "image/jpeg", SizeBytes: 42, Checksum: strings.Repeat("a", 64)})
	require.NoError(t, err)
	require.Equal(t, "ready", legacy.UploadState)
	for _, p := range imageProfiles {
		v, obj, e := f.s.OpenAssetVariantResource(f.ctx, mediaTenantUUID, a.UUID, p.Name)
		require.NoError(t, e)
		require.Equal(t, "ready", v.UploadState)
		data, e := io.ReadAll(obj.Body)
		require.NoError(t, e)
		require.NoError(t, obj.Body.Close())
		img, _, e := image.Decode(bytes.NewReader(data))
		require.NoError(t, e)
		require.Equal(t, p.Edge, img.Bounds().Dx())
		_, _, _, alpha := img.At(0, 0).RGBA()
		require.InDelta(t, 0x8080, alpha, 256)
	}
	_, original, err := f.s.OpenAssetResource(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	originalBytes, err := io.ReadAll(original.Body)
	require.NoError(t, err)
	_ = original.Body.Close()
	require.Equal(t, raw, originalBytes)
	// 重复创建不再上传原图；缺失的预览对象从存量原图恢复，变体 UUID 稳定。
	v, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, "preview")
	require.NoError(t, err)
	require.NoError(t, f.s.manager.Delete(f.ctx, v.Driver, driver.DeleteObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey}))
	reused, err := f.s.CreateAsset(f.ctx, in)
	require.NoError(t, err)
	require.Equal(t, a.UUID, reused.UUID)
	require.Equal(t, "queued", reused.Derivatives.Status)
	f.consume(t)
	repaired, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, "preview")
	require.NoError(t, err)
	require.Equal(t, v.UUID, repaired.UUID)
	_, err = f.s.EnsureImageDerivatives(f.ctx, uuid.NewString(), a.UUID, false)
	require.ErrorIs(t, err, ErrAssetNotFound)
	_, err = f.s.ImageDerivativeStatus(f.ctx, uuid.NewString(), a.UUID)
	require.ErrorIs(t, err, ErrAssetNotFound)
}

func TestImageDerivativesRestartRecoveryAndQueueFailure(t *testing.T) {
	f := newDerivativeFixture(t)
	raw := imageFixture(t)
	a, _ := f.upload(t, raw)
	// 模拟入库后队列丢失/Core 重启：从持久任务重投，仍只生成一份。
	require.NoError(t, f.redis.FlushDB(f.ctx).Err())
	require.NoError(t, f.s.derivativeQueue.Recover(f.ctx, derivativeQueueTenant, derivativeSubscriber))
	f.consume(t)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
	v, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, "thumbnail")
	require.NoError(t, err)
	require.NoError(t, f.s.manager.Delete(f.ctx, v.Driver, driver.DeleteObjectInput{ObjectKey: v.StorageKey}))
	queue := f.s.derivativeQueue
	f.s.ConfigureDerivativeQueue(nil)
	state, err = f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false)
	require.Error(t, err)
	f.s.ConfigureDerivativeQueue(queue)
	_, err = f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.NoError(t, f.s.derivativeQueue.Recover(f.ctx, derivativeQueueTenant, derivativeSubscriber))
	f.consume(t)
}

func TestImageDerivativesEnqueueFailureUsesSharedHistory(t *testing.T) {
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.mini.SetError("ERR fixture unavailable")
	state, err := f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.Equal(t, "scheduling_failed", state.Status)
	require.Equal(t, "ready", state.UploadState)
	f.mini.SetError("")
	require.NoError(t, f.s.derivativeQueue.Recover(f.ctx, derivativeQueueTenant, derivativeSubscriber))
	f.consume(t)
	state, err = f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
}

func TestImageDerivativesDecodeLimitsAndBoundedRetry(t *testing.T) {
	f := newDerivativeFixture(t)
	raw := []byte("not an image")
	in := CreateAssetInput{TenantUUID: mediaTenantUUID, Name: "legacy.png", MimeType: "image/png", SizeBytes: int64(len(raw)), ContentSHA256: payloadHash(raw), UploadMethod: UploadMethodDirect}
	a, err := f.s.CreateAsset(f.ctx, in)
	require.NoError(t, err)
	_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw))})
	require.NoError(t, err)
	_, err = f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	f.consume(t)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "failed", state.Status)
	require.Equal(t, "MEDIA_IMAGE_DECODE_FAILED", state.ErrorCode)
	require.Equal(t, "ready", state.UploadState)
	reused, err := f.s.CreateAsset(f.ctx, in)
	require.NoError(t, err)
	require.Equal(t, "failed", reused.Derivatives.Status)
	_, err = f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, true)
	require.NoError(t, err)
	f.consume(t)
	_, _, err = renderSource(f.ctx, bytes.NewReader(make([]byte, maxImageSourceBytes+1)), "")
	require.Equal(t, "MEDIA_IMAGE_BYTE_LIMIT", derivativeErrorCode(err))
	_, _, err = renderSource(f.ctx, bytes.NewReader(imageFixture(t)), strings.Repeat("0", 64))
	require.Equal(t, "MEDIA_IMAGE_SOURCE_CHECKSUM_MISMATCH", derivativeErrorCode(err))
	// 临时丢失原图最多执行三次；状态不会永远停在 processing。
	b, _ := f.upload(t, imageFixture(t))
	require.NoError(t, f.s.manager.Delete(f.ctx, b.Driver, driver.DeleteObjectInput{ObjectKey: b.StorageKey}))
	for range taskqueue.DefaultMaxAttempts {
		time.Sleep(2 * time.Millisecond)
		f.consume(t)
	}
	state, err = f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, b.UUID)
	require.NoError(t, err)
	require.Equal(t, "failed", state.Status)
	require.Equal(t, taskqueue.DefaultMaxAttempts, state.Attempts)
}

func TestImageDerivativesReclaimCrashedWorkerAndRejectStaleResult(t *testing.T) {
	f := newDerivativeFixture(t)
	queue, err := event_bus.NewRedisStreamTaskDriver(f.redis, time.Second)
	require.NoError(t, err)
	f.s.ConfigureDerivativeQueue(taskqueue.NewStreamDriverWithRetryDelay(f.db, queue, time.Millisecond))
	a, _ := f.upload(t, imageFixture(t))
	first, err := f.s.derivativeQueue.Dequeue(f.ctx, derivativeQueueTenant, derivativeSubscriber, derivativeGroup, "crashed-worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, first, 1)
	var task derivativeTask
	require.NoError(t, json.Unmarshal(first[0].Message.Payload, &task))
	f.mini.FastForward(2 * time.Second)
	time.Sleep(1100 * time.Millisecond)
	// 新 worker 通过正式 TaskBus XAUTOCLAIM 接管未 ACK 消息。
	f.consume(t)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
	require.Equal(t, 2, state.Attempts)
	source, err := f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.ErrorIs(t, f.s.generateImageVariants(f.ctx, source, task, first[0]), event_bus.ErrLeaseStale)
	require.ErrorIs(t, queue.Ack(f.ctx, first[0]), event_bus.ErrLeaseStale)
}

func TestImageDerivativesKeepExistingManualPreview(t *testing.T) {
	f := newDerivativeFixture(t)
	raw := imageFixture(t)
	a, _ := f.upload(t, raw)
	key := a.UUID + "/manual-preview"
	_, err := f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: key, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: "image/png"})
	require.NoError(t, err)
	manual := &m.MediaAssetVariant{TenantUUID: mediaTenantUUID, AssetUUID: a.UUID, Variant: "preview", Name: "plugin-preview", Driver: a.Driver, StorageKey: key, SizeBytes: int64(len(raw)), MimeType: "image/png", UploadState: m.UploadStateReady}
	manual.UUID = uuid.New()
	_, err = f.s.repo.CreateVariant(f.ctx, manual)
	require.NoError(t, err)
	f.consume(t)
	v, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, "preview")
	require.NoError(t, err)
	require.Equal(t, manual.UUID, v.UUID)
	require.Equal(t, key, v.StorageKey)
	require.Equal(t, "plugin-preview", v.Name)
	_, err = f.s.CreateAssetVariant(f.ctx, CreateAssetVariantInput{TenantUUID: mediaTenantUUID, AssetUUID: a.UUID, Variant: "preview", MimeType: "image/jpeg", SizeBytes: 42, Checksum: strings.Repeat("a", 64)})
	require.ErrorIs(t, err, ErrInvalidStatusTransition, "插件自行上传的变体仍拒绝 checksum/MIME 冲突")
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
}

func TestImageDerivativesConcurrentLegacyVariantPostgres(t *testing.T) {
	if os.Getenv("POWERX_MEDIA_TEST_DSN") == "" {
		t.Skip("native PostgreSQL parent-row locking acceptance")
	}
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.s.SetPublicResourceTokenSecret("media-legacy-compatibility-test")
	deliveries, err := f.s.derivativeQueue.Dequeue(f.ctx, derivativeQueueTenant, derivativeSubscriber, derivativeGroup, "legacy-race", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	delivery := deliveries[0]
	var task derivativeTask
	require.NoError(t, json.Unmarshal(delivery.Message.Payload, &task))
	var preview bytes.Buffer
	require.NoError(t, png.Encode(&preview, image.NewRGBA(image.Rect(0, 0, 25, 50))))
	in := CreateAssetVariantInput{TenantUUID: mediaTenantUUID, AssetUUID: a.UUID, Variant: "preview", Name: "plugin.png", MimeType: "image/png", SizeBytes: int64(preview.Len()), Checksum: payloadHash(preview.Bytes())}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var variant *AssetVariant
	var createErr, processErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; variant, createErr = f.s.CreateAssetVariant(f.ctx, in) }()
	go func() { defer wg.Done(); <-start; processErr = f.s.processImageDerivative(f.ctx, task, delivery) }()
	close(start)
	wg.Wait()
	require.NoError(t, createErr)
	if processErr != nil {
		require.Contains(t, processErr.Error(), "variant upload still pending")
	}
	if variant.UploadState == m.UploadStatePending {
		ticket, err := f.s.IssueVariantTransferTicket(f.ctx, mediaTenantUUID, a.UUID, variant.UUID, "upload", time.Minute)
		require.NoError(t, err)
		_, err = f.s.ConsumeVariantTransfer(f.ctx, a.UUID, variant.UUID, "upload", fmt.Sprint(ticket.ExpiresAt.Unix()), fmt.Sprint(ticket.Version), fmt.Sprint(ticket.ParentVersion), ticket.Token, bytes.NewReader(preview.Bytes()), int64(preview.Len()), "image/png")
		require.NoError(t, err)
		_, err = f.s.CompleteVariantUpload(f.ctx, mediaTenantUUID, a.UUID, variant.UUID, in.Checksum)
		require.NoError(t, err)
	} else {
		require.Equal(t, m.UploadStateReady, variant.UploadState)
	}
	require.NoError(t, f.s.processImageDerivative(f.ctx, task, delivery))
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
	variants, err := f.s.repo.ListVariants(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.Len(t, variants, 2)
}
