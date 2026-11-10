package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestThumbnail64ActualPixelsFormatsAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		name         string
		w, h, dw, dh int
		format       string
		alpha        bool
		orientation  int
	}{
		{"portrait", 1200, 1600, 48, 64, "png", false, 1},
		{"landscape", 1600, 900, 64, 36, "png", false, 1},
		{"transparent", 100, 100, 64, 64, "png", true, 1},
		{"small", 17, 9, 17, 9, "png", true, 1},
		{"jpeg-oriented", 120, 240, 64, 32, "jpeg", false, 6},
		{"gif-first-frame", 80, 40, 64, 32, "gif", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, tc.w, tc.h))
			alpha := uint8(255)
			if tc.alpha {
				alpha = 80
			}
			for y := 0; y < tc.h; y++ {
				for x := 0; x < tc.w; x++ {
					img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 90, B: 20, A: alpha})
				}
			}
			var buf bytes.Buffer
			switch tc.format {
			case "jpeg":
				require.NoError(t, jpeg.Encode(&buf, img, nil))
			case "gif":
				require.NoError(t, gif.Encode(&buf, img, nil))
			default:
				require.NoError(t, png.Encode(&buf, img))
			}
			raw := buf.Bytes()
			if tc.orientation == 6 {
				// Little-endian TIFF orientation tag inside a JPEG APP1 segment.
				exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
				segment := []byte{0xff, 0xe1, 0, 0}
				binary.BigEndian.PutUint16(segment[2:], uint16(len(exif)+2))
				withEXIF := append([]byte{}, raw[:2]...)
				withEXIF = append(withEXIF, segment...)
				withEXIF = append(withEXIF, exif...)
				raw = append(withEXIF, raw[2:]...)
			}
			src, format, err := renderSource(context.Background(), bytes.NewReader(raw), payloadHash(raw))
			require.NoError(t, err)
			out, mime, w, h, err := encodeDerivative(context.Background(), src, format, 64)
			require.NoError(t, err)
			decoded, actual, err := image.Decode(bytes.NewReader(out))
			require.NoError(t, err)
			require.Equal(t, tc.dw, w)
			require.Equal(t, tc.dh, h)
			require.Equal(t, image.Rect(0, 0, w, h), decoded.Bounds())
			require.LessOrEqual(t, len(out), Thumbnail64MaxBytes)
			if tc.format == "jpeg" {
				require.Equal(t, "jpeg", actual)
				require.Equal(t, "image/jpeg", mime)
				require.NotContains(t, string(out), "Exif")
			} else {
				require.Equal(t, "image/png", mime)
			}
			if tc.alpha {
				_, _, _, a := decoded.At(0, 0).RGBA()
				require.InDelta(t, uint16(alpha)*257, a, 257)
			}
		})
	}
}

// Simulate an actual pre-v2 asset without scheduling a new recipe first.
func legacyDerivativeTask(t *testing.T, f derivativeFixture) (*Asset, event_bus.StreamTaskDelivery, derivativeTask) {
	t.Helper()
	raw := imageFixture(t)
	a, err := f.s.CreateAsset(f.ctx, CreateAssetInput{TenantUUID: mediaTenantUUID, Name: "legacy.png", MimeType: "image/png", SizeBytes: int64(len(raw)), ContentSHA256: payloadHash(raw), UploadMethod: UploadMethodDirect})
	require.NoError(t, err)
	_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: "image/png"})
	require.NoError(t, err)
	ref := derivativeReference{TaskID: uuid.NewString(), Recipe: legacyImageDerivativeRecipe, SourceChecksum: payloadHash(raw), Missing: []string{"preview", "thumbnail"}}
	require.NoError(t, f.s.derivatives.WithAsset(f.ctx, mediaTenantUUID, a.UUID, func(st *r.DerivativeState) error { setDerivativeRef(st.Asset, ref); return nil }))
	task := derivativeTask{Tenant: mediaTenantUUID, Asset: a.UUID, TaskID: ref.TaskID, Recipe: ref.Recipe, SourceChecksum: ref.SourceChecksum}
	payload, _ := json.Marshal(task)
	msg := event_bus.TaskMessage{ID: task.TaskID, TenantKey: derivativeQueueTenant, SubscriberID: legacyDerivativeSubscriber, Topic: ImageDerivativeChangedTopic, Payload: payload, Metadata: map[string]string{"taskbus_record_tenant": mediaTenantUUID, "kind": "media_image_derivatives"}}
	require.NoError(t, f.s.derivativeQueue.Enqueue(f.ctx, msg))
	ds, err := f.s.derivativeQueue.Dequeue(f.ctx, derivativeQueueTenant, legacyDerivativeSubscriber, derivativeGroup, "v2-worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, ds, 1)
	return a, ds[0], task
}
func puts(f derivativeFixture) int64 {
	for _, d := range f.s.manager.MetricsSnapshot() {
		if d.Driver == "local" {
			return d.Metrics["put"].Calls
		}
	}
	return 0
}

func TestLegacyInFlightCompletesThenOnlyEncodesMissing64(t *testing.T) {
	f := newDerivativeFixture(t)
	a, d, task := legacyDerivativeTask(t, f)
	require.NoError(t, f.s.handleDerivativeDelivery(f.ctx, d))
	prior, err := f.s.repo.ListVariants(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.Len(t, prior, 2)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, ImageDerivativeRecipe, state.Recipe)
	require.Equal(t, "queued", state.Status)
	items := map[string]ImageDerivativeItem{}
	for _, item := range state.Items {
		items[item.Variant] = item
	}
	require.Equal(t, "ready", items["preview"].Status)
	require.Equal(t, "ready", items["thumbnail"].Status)
	require.Equal(t, "queued", items["thumbnail_64"].Status)
	// Late v1 results cannot overwrite the successor's association.
	require.ErrorIs(t, f.s.processImageDerivative(f.ctx, task, d), errDerivativeStale)
	before := puts(f)
	f.consume(t)
	require.EqualValues(t, 1, puts(f)-before)
	for _, v := range prior {
		now, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, v.Variant)
		require.NoError(t, err)
		require.Equal(t, v.UUID, now.UUID)
		require.Equal(t, v.StorageKey, now.StorageKey)
		require.Equal(t, v.TicketVersion, now.TicketVersion)
		require.Equal(t, v.Meta, now.Meta)
	}
	state, err = f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
}

func TestCompletedHistoryNeverMakesAbsentVariantReadyAndGetIsReadOnly(t *testing.T) {
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.consume(t)
	require.NoError(t, f.db.Unscoped().Where("asset_uuid = ? AND variant = ?", a.UUID, "thumbnail_64").Delete(&m.MediaAssetVariant{}).Error)
	var before, after int64
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&before).Error)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.NotEqual(t, "ready", state.Status)
	for _, item := range state.Items {
		if item.Variant == "thumbnail_64" {
			require.NotEqual(t, "ready", item.Status)
			require.Empty(t, item.VariantUUID)
		}
	}
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&after).Error)
	require.Equal(t, before, after)
}

func TestMissing64FailureKeepsExistingPreviewsAndOriginal(t *testing.T) {
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.consume(t)
	v, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, "thumbnail_64")
	require.NoError(t, err)
	require.NoError(t, f.s.manager.Delete(f.ctx, v.Driver, driver.DeleteObjectInput{ObjectKey: v.StorageKey}))
	_, err = f.s.EnsureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader([]byte("damaged")), Size: 7, Overwrite: true})
	require.NoError(t, err)
	f.consume(t)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "failed", state.Status)
	require.Equal(t, "ready", state.UploadState)
	for _, name := range []string{"preview", "thumbnail"} {
		_, obj, err := f.s.OpenAssetVariantResource(f.ctx, mediaTenantUUID, a.UUID, name)
		require.NoError(t, err)
		raw, err := io.ReadAll(obj.Body)
		_ = obj.Body.Close()
		require.NoError(t, err)
		_, _, err = image.Decode(bytes.NewReader(raw))
		require.NoError(t, err)
	}
	for _, item := range state.Items {
		if item.Variant == "thumbnail_64" {
			require.Equal(t, "failed", item.Status)
			require.Empty(t, item.VariantUUID)
			require.NotEmpty(t, item.ErrorCode)
		}
	}
}

func TestBackfillDryRunResumeApplyAndDuplicateAreBounded(t *testing.T) {
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.consume(t)
	require.NoError(t, f.db.Unscoped().Where("asset_uuid = ? AND variant = ?", a.UUID, "thumbnail_64").Delete(&m.MediaAssetVariant{}).Error)
	o := ImageDerivativeBackfillOptions{TenantUUID: mediaTenantUUID, BatchSize: 1, Concurrency: 2, Limit: 10}
	var before, after int64
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&before).Error)
	var items []ImageDerivativeBackfillItem
	result, err := f.s.BackfillImageDerivatives(f.ctx, o, func(v ImageDerivativeBackfillItem) error { items = append(items, v); return nil })
	require.NoError(t, err)
	require.True(t, result.DryRun)
	require.Equal(t, 1, result.Missing)
	require.Equal(t, "would_enqueue", items[0].Outcome)
	require.Equal(t, []string{"thumbnail_64"}, items[0].Missing)
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&after).Error)
	require.Equal(t, before, after)
	o.AfterID = result.AfterID
	o.ThroughID = result.ThroughID
	resumed, err := f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Zero(t, resumed.Scanned)
	o.AfterID = 0
	o.Apply = true
	result, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.Submitted)
	result, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.Active)
	f.consume(t)
	result, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.Skipped)
	require.Zero(t, result.Missing)
	o.TenantUUID = uuid.NewString()
	result, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Zero(t, result.Scanned)
	o.TenantUUID = ""
	_, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.Error(t, err)
}

func TestUploadedThumbnail64ValidatesActualSizeAndCannotForgeMetadata(t *testing.T) {
	for _, width := range []int{32, 128} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			f := newDerivativeFixture(t)
			a, _ := f.upload(t, imageFixture(t))
			f.s.SetPublicResourceTokenSecret("controlled-thumbnail64-ticket")
			var raw bytes.Buffer
			require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, width, 24))))
			checksum := payloadHash(raw.Bytes())
			v, err := f.s.CreateAssetVariant(f.ctx, CreateAssetVariantInput{TenantUUID: mediaTenantUUID, AssetUUID: a.UUID, Variant: "thumbnail_64", MimeType: "image/png", SizeBytes: int64(raw.Len()), Checksum: checksum, Metadata: map[string]any{"generator": "powerx-media", "width": 1, "height": 1, "validated_profile": "thumbnail_64"}})
			require.NoError(t, err)
			require.NotContains(t, v.Metadata, "generator")
			_, err = f.s.IssueVariantTransferTicket(f.ctx, mediaTenantUUID, a.UUID, v.UUID, "download", time.Minute)
			require.Error(t, err)
			ticket, err := f.s.IssueVariantTransferTicket(f.ctx, mediaTenantUUID, a.UUID, v.UUID, "upload", time.Minute)
			require.NoError(t, err)
			_, err = f.s.ConsumeVariantTransfer(f.ctx, a.UUID, v.UUID, "upload", fmt.Sprint(ticket.ExpiresAt.Unix()), fmt.Sprint(ticket.Version), fmt.Sprint(ticket.ParentVersion), ticket.Token, bytes.NewReader(raw.Bytes()), int64(raw.Len()), "image/png")
			require.NoError(t, err)
			ready, err := f.s.CompleteVariantUpload(f.ctx, mediaTenantUUID, a.UUID, v.UUID, checksum)
			if width > 64 {
				require.ErrorIs(t, err, ErrUploadValidationFailed)
				require.Equal(t, m.UploadStateFailed, ready.UploadState)
				return
			}
			require.NoError(t, err)
			require.Equal(t, m.UploadStateReady, ready.UploadState)
			state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
			require.NoError(t, err)
			for _, item := range state.Items {
				if item.Variant == "thumbnail_64" {
					require.Equal(t, "ready", item.Status)
					require.Equal(t, v.UUID, item.VariantUUID)
				}
			}
			_, err = f.s.IssueVariantTransferTicket(f.ctx, mediaTenantUUID, a.UUID, v.UUID, "download", time.Minute)
			require.NoError(t, err)
			f.consume(t)
			actual, err := f.s.repo.FindVariant(f.ctx, mediaTenantUUID, a.UUID, "thumbnail_64")
			require.NoError(t, err)
			require.Equal(t, v.UUID, actual.UUID.String())
		})
	}
}

func TestLegacySuccessorSurvivesCrashBeforeAckOrEnqueue(t *testing.T) {
	f := newDerivativeFixture(t)
	a, d, task := legacyDerivativeTask(t, f)
	// Complete SQL admission only: simulate termination before ACK/Ensure/enqueue.
	require.NoError(t, f.s.processImageDerivative(f.ctx, task, d))
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "queued", state.Status)
	require.NotEqual(t, task.TaskID, state.JobUUID)
	h, err := f.s.derivativeQueue.Status(f.ctx, mediaTenantUUID, derivativeSubscriber, state.JobUUID)
	require.NoError(t, err)
	require.NotNil(t, h)
	require.Equal(t, ef.TaskHistoryStatusPending, h.Status)
	require.NoError(t, f.s.derivativeQueue.Recover(f.ctx, derivativeQueueTenant, derivativeSubscriber))
	f.consume(t)
	state, err = f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
	require.NoError(t, f.s.derivativeQueue.Ack(f.ctx, d))
}

func TestHistoricalSourceWithoutChecksumIsPinnedOnlyOnExplicitApply(t *testing.T) {
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.consume(t)
	require.NoError(t, f.db.Model(&m.MediaAsset{}).Where("uuid = ?", a.UUID).Update("expected_checksum", "").Error)
	require.NoError(t, f.db.Unscoped().Where("asset_uuid = ? AND variant = ?", a.UUID, "thumbnail_64").Delete(&m.MediaAssetVariant{}).Error)
	o := ImageDerivativeBackfillOptions{TenantUUID: mediaTenantUUID, BatchSize: 10, Concurrency: 1}
	report, err := f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, report.Missing)
	original, err := f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.Empty(t, original.ExpectedChecksum, "dry-run never writes a checksum")
	o.Apply = true
	report, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, report.Submitted)
	original, err = f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.Equal(t, payloadHash(imageFixture(t)), original.ExpectedChecksum)
	before := puts(f)
	f.consume(t)
	require.EqualValues(t, 1, puts(f)-before, "verified historical v1/v2 previews are reused after pinning")
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
}
