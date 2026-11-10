package media

import (
	"bytes"
	"encoding/base64"
	"image"
	"io"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func pngCRCRepairFixture(t *testing.T) (derivativeFixture, *Asset, []byte) {
	t.Helper()
	f := newDerivativeFixture(t)
	raw, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	require.NoError(t, err)
	a, _ := f.upload(t, raw)
	f.consume(t)
	status, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "failed", status.Status)
	require.Equal(t, "MEDIA_IMAGE_DECODE_FAILED", status.ErrorCode)
	return f, a, raw
}

func TestPNGChecksumRepairPreservesSourceHistoryAndStrictDecoder(t *testing.T) {
	f, a, raw := pngCRCRepairFixture(t)
	_, _, err := renderSource(f.ctx, bytes.NewReader(raw), payloadHash(raw))
	require.ErrorContains(t, err, "MEDIA_IMAGE_DECODE_FAILED")
	fixed, err := recoverKnownPNGCRC(raw)
	require.NoError(t, err)
	require.Equal(t, raw[:52], fixed[:52])
	require.Equal(t, raw[56:], fixed[56:])
	img, format, err := image.Decode(bytes.NewReader(fixed))
	require.NoError(t, err)
	require.Equal(t, "png", format)
	require.Equal(t, image.Rect(0, 0, 1, 1), img.Bounds())
	_, err = recoverKnownPNGCRC(fixed)
	require.ErrorContains(t, err, "MEDIA_PNG_REPAIR_SOURCE_NOT_APPROVED")
	before, err := f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	var history []ef.TaskHistory
	require.NoError(t, f.db.Find(&history).Error)
	plan, err := f.s.ScanPNGChecksumRepair(f.ctx, mediaTenantUUID, a.UUID, "fixture")
	require.NoError(t, err)
	var count int64
	require.NoError(t, f.db.Model(&m.MediaAssetVariant{}).Count(&count).Error)
	require.Zero(t, count)
	variants, err := f.s.ApplyPNGChecksumRepair(f.ctx, plan, "fixture")
	require.NoError(t, err)
	require.Len(t, variants, 3)
	for _, v := range variants {
		require.True(t, variantRecordUsable(before, v))
		obj, err := f.s.manager.Get(f.ctx, v.Driver, driver.GetObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey})
		require.NoError(t, err)
		body, err := io.ReadAll(obj.Body)
		require.NoError(t, err)
		require.NoError(t, obj.Body.Close())
		require.Equal(t, v.ExpectedChecksum, payloadHash(body))
		require.EqualValues(t, len(body), v.SizeBytes)
		img, format, err := renderSource(f.ctx, bytes.NewReader(body), v.ExpectedChecksum)
		require.NoError(t, err)
		require.Equal(t, "image/"+format, v.MimeType)
		require.Equal(t, image.Rect(0, 0, 1, 1), img.Bounds())
		require.Equal(t, "known_png_idat_crc_v1", extractMeta(v.Meta)["recovery"].(map[string]any)["method"])
	}
	after, err := f.s.repo.FindByUUID(f.ctx, mediaTenantUUID, a.UUID, false)
	require.NoError(t, err)
	require.Equal(t, before, after)
	var historyAfter []ef.TaskHistory
	require.NoError(t, f.db.Find(&historyAfter).Error)
	require.Equal(t, history, historyAfter)
	obj, err := f.s.manager.Get(f.ctx, a.Driver, driver.GetObjectInput{Bucket: a.Bucket, ObjectKey: a.StorageKey})
	require.NoError(t, err)
	source, err := io.ReadAll(obj.Body)
	require.NoError(t, err)
	require.NoError(t, obj.Body.Close())
	require.Equal(t, raw, source)
	status, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", status.Status)
	require.Empty(t, status.ErrorCode)
	for _, item := range status.Items {
		require.Equal(t, "ready", item.Status)
	}
	_, err = f.s.ApplyPNGChecksumRepair(f.ctx, plan, "fixture")
	require.ErrorContains(t, err, "MEDIA_PNG_REPAIR_VARIANT_CONFLICT")
	require.NoError(t, f.db.Model(&m.MediaAssetVariant{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
	_, _, err = renderSource(f.ctx, bytes.NewReader(raw), payloadHash(raw))
	require.ErrorContains(t, err, "MEDIA_IMAGE_DECODE_FAILED")
}

func TestPNGChecksumRepairRejectsChangesAndScope(t *testing.T) {
	for _, change := range []string{"source", "parent", "variant", "tenant", "seal", "environment", "checksum"} {
		t.Run(change, func(t *testing.T) {
			f, a, _ := pngCRCRepairFixture(t)
			p, err := f.s.ScanPNGChecksumRepair(f.ctx, mediaTenantUUID, a.UUID, "fixture")
			require.NoError(t, err)
			switch change {
			case "source":
				_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader([]byte("changed")), Size: 7, ContentType: "image/png", Overwrite: true})
				require.NoError(t, err)
			case "parent":
				require.NoError(t, f.db.Model(&m.MediaAsset{}).Where("uuid=?", a.UUID).UpdateColumn("business_status", "changed").Error)
			case "variant":
				v := m.MediaAssetVariant{TenantUUID: mediaTenantUUID, AssetUUID: a.UUID, Variant: "preview", Driver: "local", StorageKey: "manual.png", UploadState: m.UploadStatePending}
				v.UUID = uuid.New()
				require.NoError(t, f.db.Create(&v).Error)
			case "tenant":
				p.Tenant = uuid.NewString()
				p.Seal = ""
				p.Seal = pngRepairHash(p)
			case "seal":
				p.Method = "changed"
			case "environment":
				p.Environment = "other"
				p.Seal = ""
				p.Seal = pngRepairHash(p)
			case "checksum":
				require.NoError(t, f.db.Model(&m.MediaAsset{}).Where("uuid=?", a.UUID).UpdateColumn("expected_checksum", payloadHash([]byte("other"))).Error)
			}
			var before, after []m.MediaAssetVariant
			require.NoError(t, f.db.Find(&before).Error)
			created, err := f.s.ApplyPNGChecksumRepair(f.ctx, p, "fixture")
			require.Error(t, err)
			require.Empty(t, created)
			require.NoError(t, f.db.Find(&after).Error)
			require.Equal(t, before, after)
		})
	}
}

func TestPNGChecksumRepairRollbackReportsObjects(t *testing.T) {
	f, a, _ := pngCRCRepairFixture(t)
	p, err := f.s.ScanPNGChecksumRepair(f.ctx, mediaTenantUUID, a.UUID, "fixture")
	require.NoError(t, err)
	require.NoError(t, f.db.Exec(`CREATE TRIGGER reject_png_repair BEFORE INSERT ON media_asset_variants BEGIN SELECT RAISE(ABORT, 'fixture_reject'); END`).Error)
	created, err := f.s.ApplyPNGChecksumRepair(f.ctx, p, "fixture")
	require.ErrorContains(t, err, "MEDIA_PNG_REPAIR_RECOVERY_REQUIRED")
	require.Len(t, created, 1)
	var count int64
	require.NoError(t, f.db.Model(&m.MediaAssetVariant{}).Count(&count).Error)
	require.Zero(t, count)
	_, err = f.s.manager.Stat(f.ctx, created[0].Driver, driver.StatObjectInput{ObjectKey: created[0].StorageKey})
	require.NoError(t, err)
}
