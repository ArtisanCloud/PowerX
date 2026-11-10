package media

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestApprovedBackfillDoesNotExpandAfterAnotherVariantDisappears(t *testing.T) {
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.consume(t)
	require.NoError(t, f.db.Unscoped().Where("asset_uuid = ? AND variant = ?", a.UUID, "thumbnail_64").Delete(&m.MediaAssetVariant{}).Error)
	o := ImageDerivativeBackfillOptions{TenantUUID: mediaTenantUUID, AssetUUIDs: []string{a.UUID}, BatchSize: 10, Concurrency: 1}
	var scan ImageDerivativeBackfillItem
	_, err := f.s.BackfillImageDerivatives(f.ctx, o, func(item ImageDerivativeBackfillItem) error { scan = item; return nil })
	require.NoError(t, err)
	require.Equal(t, []string{"thumbnail_64"}, scan.Missing)
	expected := ImageDerivativeBackfillExpectation{TenantUUID: mediaTenantUUID, SourceChecksum: scan.SourceChecksum, Missing: scan.Missing}
	require.NoError(t, f.db.Unscoped().Where("asset_uuid = ? AND variant = ?", a.UUID, "preview").Delete(&m.MediaAssetVariant{}).Error)
	var before, after int64
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&before).Error)
	// Check the parent-lock guard directly as well as the outer planning read.
	_, err = f.s.ensureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false, &expected)
	require.ErrorIs(t, err, errBackfillPlanChanged)
	o.Apply = true
	o.Expected = map[string]ImageDerivativeBackfillExpectation{a.UUID: expected}
	report, err := f.s.BackfillImageDerivatives(f.ctx, o, func(item ImageDerivativeBackfillItem) error {
		require.Equal(t, "MEDIA_BACKFILL_PLAN_CHANGED", item.ErrorCode)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, report.Failed)
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&after).Error)
	require.Equal(t, before, after)
	// A fresh review authorizes both missing specs; the 320px variant is retained.
	o.Apply = false
	o.Expected = nil
	_, err = f.s.BackfillImageDerivatives(f.ctx, o, func(item ImageDerivativeBackfillItem) error { scan = item; return nil })
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"preview", "thumbnail_64"}, scan.Missing)
	o.Apply = true
	o.Expected = map[string]ImageDerivativeBackfillExpectation{a.UUID: {TenantUUID: mediaTenantUUID, SourceChecksum: scan.SourceChecksum, Missing: scan.Missing}}
	report, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, report.Submitted)
	f.consume(t)
	state, err := f.s.ImageDerivativeStatus(f.ctx, mediaTenantUUID, a.UUID)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Status)
}

func TestGlobalBackfillDoesNotStopAtOneHundredAssets(t *testing.T) {
	f := newDerivativeFixture(t)
	tenants := []string{mediaTenantUUID, uuid.NewString()}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 8, 6))))
	raw := encoded.Bytes()
	for i := 0; i < 105; i++ {
		id := uuid.New()
		row := m.MediaAsset{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: id}, TenantUUID: tenants[i%2], Name: "historical.png", Driver: "local", StorageKey: "history/" + id.String() + ".png", SizeBytes: int64(len(raw)), MimeType: "image/png", UploadState: m.UploadStateReady, ExpectedChecksum: payloadHash(raw), Tags: datatypes.JSON(`[]`), Meta: datatypes.JSON(`{}`)}
		require.NoError(t, f.db.Create(&row).Error)
		_, err := f.s.manager.Put(f.ctx, row.Driver, driver.PutObjectInput{ObjectKey: row.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: row.MimeType})
		require.NoError(t, err)
	}
	report, err := f.s.BackfillImageDerivatives(f.ctx, ImageDerivativeBackfillOptions{AllTenants: true, BatchSize: 17, Concurrency: 2}, nil)
	require.NoError(t, err)
	require.Equal(t, 105, report.Scanned)
	require.Equal(t, 105, report.Missing)
	require.Equal(t, 2, report.TenantCount)
	var histories int64
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&histories).Error)
	require.Zero(t, histories)
}

func TestGlobalBackfillCoversEveryTenantAndKeepsJobsTenantScoped(t *testing.T) {
	f := newDerivativeFixture(t)
	tenants := []string{mediaTenantUUID, uuid.NewString()}
	var assets []*Asset
	for i := 0; i < 3; i++ {
		var encoded bytes.Buffer
		require.NoError(t, png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 120+i, 80))))
		raw := encoded.Bytes()
		tenant := tenants[min(i, 1)]
		a, err := f.s.CreateAsset(f.ctx, CreateAssetInput{TenantUUID: tenant, Name: "historical.png", MimeType: "image/png", SizeBytes: int64(len(raw)), ContentSHA256: payloadHash(raw), UploadMethod: UploadMethodDirect})
		require.NoError(t, err)
		assets = append(assets, a)
		if i < 2 {
			_, err = f.s.manager.Put(f.ctx, a.Driver, driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: a.MimeType})
			require.NoError(t, err)
		}
	}
	o := ImageDerivativeBackfillOptions{AllTenants: true, BatchSize: 1, Concurrency: 1}
	var scan []ImageDerivativeBackfillItem
	report, err := f.s.BackfillImageDerivatives(f.ctx, o, func(item ImageDerivativeBackfillItem) error { scan = append(scan, item); return nil })
	require.NoError(t, err)
	require.Equal(t, 3, report.Scanned)
	require.Equal(t, 2, report.TenantCount)
	require.Equal(t, 1, report.SourceMissing)
	var histories int64
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&histories).Error)
	require.Zero(t, histories)
	o.AssetUUIDs = []string{assets[0].UUID, assets[1].UUID}
	o.Expected = map[string]ImageDerivativeBackfillExpectation{}
	for _, item := range scan {
		if item.Outcome == "would_enqueue" {
			o.Expected[item.AssetUUID] = ImageDerivativeBackfillExpectation{TenantUUID: item.TenantUUID, SourceChecksum: item.SourceChecksum, Missing: item.Missing}
		}
	}
	o.Apply = true
	report, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 2, report.Submitted)
	deliveries, err := f.s.derivativeQueue.Dequeue(f.ctx, derivativeQueueTenant, derivativeSubscriber, derivativeGroup, "global-test", 10, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	var actualTenants []string
	for _, delivery := range deliveries {
		var task derivativeTask
		require.NoError(t, json.Unmarshal(delivery.Message.Payload, &task))
		actualTenants = append(actualTenants, task.Tenant)
		require.NoError(t, f.s.handleDerivativeDelivery(f.ctx, delivery))
	}
	require.ElementsMatch(t, tenants, actualTenants)
	for _, a := range assets[:2] {
		state, err := f.s.ImageDerivativeStatus(f.ctx, a.TenantUUID, a.UUID)
		require.NoError(t, err)
		require.Equal(t, "ready", state.Status)
		_, err = f.s.ImageDerivativeStatus(f.ctx, tenants[1], assets[0].UUID)
		require.ErrorIs(t, err, ErrAssetNotFound)
	}
	report, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 2, report.Skipped)
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&histories).Error)
	require.EqualValues(t, 2, histories)
	// Tenant-filtered operations and repository reads retain their old boundary.
	o = ImageDerivativeBackfillOptions{TenantUUID: tenants[0], BatchSize: 50, Concurrency: 1}
	report, err = f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, report.Scanned)
	_, _, err = r.NewAssetRepository(f.db).DerivativeCandidates(f.ctx, "", nil, 0, 0, 50)
	require.Error(t, err)
}

func TestApprovedBackfillRejectsChangedSourceWithoutRegisteringJob(t *testing.T) {
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.consume(t)
	expected := ImageDerivativeBackfillExpectation{TenantUUID: mediaTenantUUID, SourceChecksum: strings.Repeat("0", 64), Missing: []string{"thumbnail_64"}}
	var before, after int64
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&before).Error)
	_, err := f.s.ensureImageDerivatives(f.ctx, mediaTenantUUID, a.UUID, false, &expected)
	require.ErrorIs(t, err, errBackfillPlanChanged)
	o := ImageDerivativeBackfillOptions{TenantUUID: mediaTenantUUID, AssetUUIDs: []string{a.UUID}, BatchSize: 10, Concurrency: 1, Apply: true, Expected: map[string]ImageDerivativeBackfillExpectation{a.UUID: expected}}
	report, err := f.s.BackfillImageDerivatives(f.ctx, o, nil)
	require.NoError(t, err)
	require.Equal(t, 1, report.Failed)
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&after).Error)
	require.Equal(t, before, after)
}
