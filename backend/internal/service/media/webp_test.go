package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	"github.com/stretchr/testify/require"
)

func webpFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "webp", name+".webp"))
	require.NoError(t, err)
	return raw
}

// Real WebP ingestion runs through completion, TaskBus and stored resource reads.
func TestWebPImageDerivatives(t *testing.T) {
	for _, name := range []string{"lossy", "lossless", "alpha"} {
		t.Run(name, func(t *testing.T) {
			f := newDerivativeFixture(t)
			raw := webpFixture(t, name)
			a, err := f.s.CreateAsset(f.ctx, CreateAssetInput{TenantUUID: mediaTenantUUID, Name: name + ".webp", MimeType: "image/webp", SizeBytes: int64(len(raw)), ContentSHA256: payloadHash(raw), UploadMethod: UploadMethodPresign})
			require.NoError(t, err)
			require.Equal(t, "waiting_upload", a.Derivatives.Status)
			_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: "image/webp"})
			require.NoError(t, err)
			a, err = f.s.CompleteAssetUpload(f.ctx, CompleteAssetUploadInput{TenantUUID: mediaTenantUUID, UUID: a.UUID, Checksum: payloadHash(raw)})
			require.NoError(t, err)
			require.Equal(t, "queued", a.Derivatives.Status)
			f.consume(t)
			state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
			require.NoError(t, err)
			require.Equal(t, "ready", state.Status)
			require.Len(t, state.Items, len(imageProfiles))
			before := map[string]string{}
			for _, p := range imageProfiles {
				v, obj, err := f.s.OpenAssetVariantResource(f.ctx, mediaTenantUUID, a.UUID, p.Name)
				require.NoError(t, err)
				body, err := io.ReadAll(obj.Body)
				require.NoError(t, err)
				require.NoError(t, obj.Body.Close())
				require.Equal(t, "image/png", v.MimeType)
				decoded, format, err := image.Decode(bytes.NewReader(body))
				require.NoError(t, err)
				require.Equal(t, "png", format)
				w := min(p.Edge, 160)
				require.Equal(t, image.Rect(0, 0, w, w/2), decoded.Bounds())
				_, _, _, alpha := decoded.At(0, 0).RGBA()
				if name == "alpha" {
					require.InDelta(t, 96*257, alpha, 257)
				} else {
					require.EqualValues(t, 65535, alpha)
				}
				if p.Name == "thumbnail_64" {
					require.LessOrEqual(t, len(body), Thumbnail64MaxBytes)
				}
				before[p.Name] = v.UUID
			}
			_, source, err := f.s.OpenAssetResource(f.ctx, mediaTenantUUID, a.UUID)
			require.NoError(t, err)
			body, err := io.ReadAll(source.Body)
			require.NoError(t, err)
			require.NoError(t, source.Body.Close())
			require.Equal(t, raw, body)

			// Explicit repair regenerates only the missing preview, preserving ready variants.
			preview, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, "preview")
			require.NoError(t, err)
			require.NoError(t, f.s.manager.Delete(f.ctx, preview.Driver, driver.DeleteObjectInput{Bucket: preview.Bucket, ObjectKey: preview.StorageKey}))
			originalBefore, err := f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
			require.NoError(t, err)
			o := ImageDerivativeBackfillOptions{TenantUUID: mediaTenantUUID, AssetUUIDs: []string{a.UUID}, BatchSize: 1, Concurrency: 1}
			report, err := f.s.BackfillImageDerivatives(f.ctx, o, func(item ImageDerivativeBackfillItem) error {
				require.Equal(t, "would_enqueue", item.Outcome)
				require.Equal(t, []string{"preview"}, item.Missing)
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, 1, report.Missing)
			originalAfterScan, err := f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
			require.NoError(t, err)
			require.Equal(t, originalBefore, originalAfterScan)
			o.Apply = true
			_, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
			require.NoError(t, err)
			f.consume(t)
			state, err = f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
			require.NoError(t, err)
			require.Equal(t, "ready", state.Status)
			for _, item := range state.Items {
				require.Equal(t, before[item.Variant], item.VariantUUID)
			}
		})
	}
}

func TestWebPSourceGuards(t *testing.T) {
	raw := webpFixture(t, "alpha")
	_, _, err := renderSource(context.Background(), bytes.NewReader(raw), payloadHash([]byte("fixture_other")))
	require.ErrorContains(t, err, "MEDIA_IMAGE_SOURCE_CHECKSUM_MISMATCH")
	_, _, err = renderSource(context.Background(), bytes.NewReader(raw[:20]), "")
	require.ErrorContains(t, err, "MEDIA_IMAGE_DECODE_FAILED")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = renderSource(ctx, bytes.NewReader(raw), "")
	require.ErrorIs(t, err, context.Canceled)
	_, _, err = renderSource(context.Background(), bytes.NewReader(webpFixture(t, "animated")), "")
	require.ErrorContains(t, err, "MEDIA_IMAGE_DECODE_FAILED")

	mismatch := append([]byte(nil), raw...)
	mismatch[24], mismatch[25], mismatch[26] = 1, 0, 0
	_, _, err = renderSource(context.Background(), bytes.NewReader(mismatch), "")
	require.ErrorContains(t, err, "MEDIA_IMAGE_DECODE_FAILED")
	oversizedFrame := append([]byte(nil), raw...)
	frame := bytes.Index(oversizedFrame, []byte("VP8 ")) + 8
	require.Greater(t, frame, 8)
	binary.LittleEndian.PutUint16(oversizedFrame[frame+6:frame+8], 16383)
	binary.LittleEndian.PutUint16(oversizedFrame[frame+8:frame+10], 16383)
	_, _, err = renderSource(context.Background(), bytes.NewReader(oversizedFrame), "")
	require.ErrorContains(t, err, "MEDIA_IMAGE_PIXEL_LIMIT")
	for i := 24; i < 30; i++ {
		raw[i] = 255
	}
	_, _, err = renderSource(context.Background(), bytes.NewReader(raw), "")
	require.ErrorContains(t, err, "MEDIA_IMAGE_PIXEL_LIMIT")
}
