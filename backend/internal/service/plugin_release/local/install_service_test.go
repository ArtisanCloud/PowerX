package local

import (
	"context"
	"testing"
	"time"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testTenantUUID = "f1c221cb-97fd-4f9e-9359-24da3b6f9001"

type metadataResolverStub struct {
	meta *ArtifactMetadata
	err  error
}

func (m metadataResolverStub) Resolve(ctx context.Context, tenantUUID string, artifactURI string) (*ArtifactMetadata, error) {
	return m.meta, m.err
}

func TestInstallServiceStartPersistsSession(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(&models.LocalInstallSession{}))

	repository := repo.NewLocalInstallSessionRepository(db)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	svc := NewInstallService(InstallServiceDeps{
		Repository:       repository,
		Auditor:          audit.Noop{},
		Clock:            func() time.Time { return now },
		MetadataResolver: metadataResolverStub{meta: &ArtifactMetadata{SizeBytes: 1024, Checksum: "sha", Signature: "sig"}},
	}, Options{
		SessionTTL:        10 * time.Minute,
		MaxArtifactSizeMB: 50,
		FeatureEnabled:    true,
	})

	session, err := svc.Start(context.Background(), StartInput{
		TenantUUID:          testTenantUUID,
		DeveloperMemberUUID: "b4f90ea5-80e7-4d8d-a18d-af6ef7f5f540",
		ArtifactURI:         "s3://bucket/hotload.zip",
		FeatureFlags:        []string{"beta_ui"},
		Actor:               "tester",
	})
	require.NoError(t, err)
	require.Equal(t, models.LocalInstallStatusInProgress, session.Status)
	require.NotNil(t, session.ExpiredAt)
	require.Equal(t, now.Add(10*time.Minute), session.ExpiredAt.UTC())

	stored, err := repository.GetSessionByTenantUUID(context.Background(), testTenantUUID, session.UUID)
	require.NoError(t, err)
	require.Equal(t, session.UUID, stored.UUID)
	require.Equal(t, testTenantUUID, stored.TenantUUID)
}

func TestInstallServiceRejectsLargeArtifact(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(&models.LocalInstallSession{}))

	repository := repo.NewLocalInstallSessionRepository(db)
	svc := NewInstallService(InstallServiceDeps{
		Repository:       repository,
		MetadataResolver: metadataResolverStub{meta: &ArtifactMetadata{SizeBytes: 200 * 1024 * 1024}},
	}, Options{
		SessionTTL:        5 * time.Minute,
		MaxArtifactSizeMB: 10,
		FeatureEnabled:    true,
	})

	_, err = svc.Start(context.Background(), StartInput{
		TenantUUID:          testTenantUUID,
		DeveloperMemberUUID: "c4f90ea5-80e7-4d8d-a18d-af6ef7f5f540",
		ArtifactURI:         "s3://bucket/huge.zip",
		FeatureFlags:        []string{},
	})
	require.ErrorIs(t, err, ErrArtifactTooLarge)
}

func TestInstallServiceStopMarksSession(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(&models.LocalInstallSession{}))

	repository := repo.NewLocalInstallSessionRepository(db)
	svc := NewInstallService(InstallServiceDeps{
		Repository: repository,
		Auditor:    audit.Noop{},
		Clock:      func() time.Time { return time.Unix(0, 0) },
	}, Options{
		SessionTTL:        5 * time.Minute,
		MaxArtifactSizeMB: 5,
		FeatureEnabled:    true,
	})

	session, err := svc.Start(context.Background(), StartInput{
		TenantUUID:          testTenantUUID,
		DeveloperMemberUUID: "d4f90ea5-80e7-4d8d-a18d-af6ef7f5f540",
		ArtifactURI:         "file://bundle.zip",
	})
	require.NoError(t, err)

	err = svc.Stop(context.Background(), StopInput{
		SessionID:  session.UUID,
		TenantUUID: testTenantUUID,
		Actor:      "tester",
	})
	require.NoError(t, err)

	stored, err := repository.GetSessionByTenantUUID(context.Background(), testTenantUUID, session.UUID)
	require.NoError(t, err)
	require.Equal(t, models.LocalInstallStatusSuccess, stored.Status)
	require.NotNil(t, stored.ExpiredAt)
}

func TestInstallServiceStopRejectsCrossTenant(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(&models.LocalInstallSession{}))

	repository := repo.NewLocalInstallSessionRepository(db)
	svc := NewInstallService(InstallServiceDeps{
		Repository: repository,
	}, Options{
		SessionTTL:        5 * time.Minute,
		MaxArtifactSizeMB: 5,
		FeatureEnabled:    true,
	})

	session, err := svc.Start(context.Background(), StartInput{
		TenantUUID:          testTenantUUID,
		DeveloperMemberUUID: "e4f90ea5-80e7-4d8d-a18d-af6ef7f5f540",
		ArtifactURI:         "file://bundle.zip",
	})
	require.NoError(t, err)

	err = svc.Stop(context.Background(), StopInput{
		SessionID:  session.UUID,
		TenantUUID: "11111111-2222-3333-4444-555555555555",
	})
	require.ErrorIs(t, err, ErrSessionNotFound)
}
