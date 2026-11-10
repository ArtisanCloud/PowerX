package media

import (
	"bytes"
	"context"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	"github.com/stretchr/testify/require"
)

func TestImageProcessingLimitsAndCancelledAdmission(t *testing.T) {
	o := defaultImageDerivativeOptions()
	// 真实稿件尺寸必须能通过默认资源检查，超限在完整解码前拒绝。
	large := image.Config{Width: 6250, Height: 8307}
	require.NoError(t, validateImageDimensions(large, 11<<20, o))
	require.Equal(t, "MEDIA_IMAGE_PIXEL_LIMIT", derivativeErrorCode(validateImageDimensions(image.Config{Width: 10001, Height: 8000}, 1, o)))
	o.MaxWorkingBytes = 64 << 20
	require.Equal(t, "MEDIA_IMAGE_MEMORY_LIMIT", derivativeErrorCode(validateImageDimensions(large, 11<<20, o)))
	o = defaultImageDerivativeOptions()
	o.MaxSourceBytes = 8
	_, _, err := renderSourceWithOptions(context.Background(), bytes.NewReader([]byte("012345678")), "", o)
	require.Equal(t, "MEDIA_IMAGE_BYTE_LIMIT", derivativeErrorCode(err))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = renderSourceWithOptions(ctx, bytes.NewReader(nil), "", defaultImageDerivativeOptions())
	require.ErrorIs(t, err, context.Canceled)
	policy := newImageProcessingPolicy(ImageDerivativeOptions{MaxParallelJobs: 1})
	release, err := policy.acquire(context.Background())
	require.NoError(t, err)
	_, err = policy.acquire(ctx)
	require.ErrorIs(t, err, context.Canceled)
	deadline, done := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer done()
	_, err = policy.acquire(deadline)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	release()
	release, err = policy.acquire(context.Background())
	require.NoError(t, err)
	release()
	_, err = newImageProcessingPolicy(ImageDerivativeOptions{MaxParallelJobs: -1}).acquire(context.Background())
	require.Equal(t, "MEDIA_IMAGE_CONFIGURATION_INVALID", derivativeErrorCode(err))
}

func TestImageResourceFailurePreservesOriginalAndExplicitRetry(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		options    ImageDerivativeOptions
	}{
		{"pixels", "MEDIA_IMAGE_PIXEL_LIMIT", ImageDerivativeOptions{MaxPixels: 100}},
		{"memory", "MEDIA_IMAGE_MEMORY_LIMIT", ImageDerivativeOptions{MaxWorkingBytes: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDerivativeFixture(t)
			f.s.ConfigureImageDerivatives(tc.options)
			raw := imageFixture(t)
			a, _ := f.upload(t, raw)
			f.consume(t)
			state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
			require.NoError(t, err)
			require.Equal(t, "failed", state.Status)
			require.Equal(t, tc.code, state.ErrorCode)
			require.Equal(t, "ready", state.UploadState)
			object, err := f.s.manager.Get(f.ctx, a.Driver, driver.GetObjectInput{ObjectKey: a.StorageKey})
			require.NoError(t, err)
			stored, err := io.ReadAll(object.Body)
			require.NoError(t, err)
			require.NoError(t, object.Body.Close())
			require.Equal(t, raw, stored)
			// 普通查询不偷偷重试；配置恢复后仍由明确的 retryFailed 操作提交。
			same, err := f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false)
			require.NoError(t, err)
			require.Equal(t, state.JobUUID, same.JobUUID)
			f.s.ConfigureImageDerivatives(ImageDerivativeOptions{})
			retry, err := f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, true)
			require.NoError(t, err)
			require.NotEqual(t, state.JobUUID, retry.JobUUID)
			f.consume(t)
			ready, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
			require.NoError(t, err)
			require.Equal(t, "ready", ready.Status)
		})
	}
}

// 本机验收 opt-in：只读取指定目录的两张原稿，上传与处理均使用临时库／对象目录／Redis。
func TestLargeImageOriginalFixturesThroughTaskBus(t *testing.T) {
	dir := os.Getenv("POWERX_MEDIA_LARGE_IMAGE_FIXTURES")
	if dir == "" {
		t.Skip("set POWERX_MEDIA_LARGE_IMAGE_FIXTURES for read-only source fixture acceptance")
	}
	for _, name := range []string{"category2.png", "shaker cat1.jpg"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			before := payloadHash(raw)
			f := newDerivativeFixture(t)
			mime := http.DetectContentType(raw)
			a, err := f.s.CreateAsset(f.ctx, CreateAssetInput{TenantUUID: mediaTenantUUID, Name: name, MimeType: mime, SizeBytes: int64(len(raw)), ContentSHA256: before, UploadMethod: UploadMethodPresign})
			require.NoError(t, err)
			_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: mime})
			require.NoError(t, err)
			a, err = f.s.CompleteAssetUpload(f.ctx, CompleteAssetUploadInput{TenantUUID: mediaTenantUUID, UUID: a.UUID, Checksum: before})
			require.NoError(t, err)
			require.Equal(t, "ready", a.UploadState)
			start := time.Now()
			f.consume(t)
			state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
			require.NoError(t, err)
			require.Equal(t, "ready", state.Status, "%+v", state)
			require.Len(t, state.Items, 3)
			variants, err := f.s.repo.ListVariants(f.ctx, mediaTenantUUID, a.UUID, false)
			require.NoError(t, err)
			require.Len(t, variants, 3)
			for _, v := range variants {
				obj, err := f.s.manager.Get(f.ctx, v.Driver, driver.GetObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey})
				require.NoError(t, err)
				body, err := io.ReadAll(obj.Body)
				require.NoError(t, err)
				require.NoError(t, obj.Body.Close())
				cfg, format, err := image.DecodeConfig(bytes.NewReader(body))
				require.NoError(t, err)
				require.Contains(t, []string{"png", "jpeg"}, format)
				require.Equal(t, v.SizeBytes, int64(len(body)))
				require.Equal(t, v.ExpectedChecksum, payloadHash(body))
				for _, profile := range imageProfiles {
					if profile.Name == v.Variant {
						require.LessOrEqual(t, cfg.Width, profile.Edge)
						require.LessOrEqual(t, cfg.Height, profile.Edge)
					}
				}
				if v.Variant == "thumbnail_64" {
					require.LessOrEqual(t, len(body), Thumbnail64MaxBytes)
				}
				t.Logf("variant=%s dimensions=%dx%d bytes=%d checksum_verified=true", v.Variant, cfg.Width, cfg.Height, len(body))
			}
			obj, err := f.s.manager.Get(f.ctx, a.Driver, driver.GetObjectInput{ObjectKey: a.StorageKey})
			require.NoError(t, err)
			stored, err := io.ReadAll(obj.Body)
			require.NoError(t, err)
			require.NoError(t, obj.Body.Close())
			require.Equal(t, before, payloadHash(stored))
			reused, err := f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, true)
			require.NoError(t, err)
			require.Equal(t, state.JobUUID, reused.JobUUID)
			sourceAfter, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			require.Equal(t, before, payloadHash(sourceAfter))
			t.Logf("original_preserved=true TaskBus_status=ready elapsed=%s", time.Since(start))
		})
	}
}
