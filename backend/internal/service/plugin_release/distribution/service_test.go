package distribution

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const distributionTenantUUID = "1a23831a-3a65-4e1c-97ad-68fde1c4e5d2"

func TestDistributionServiceWorkflow(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(
		&models.PluginReleaseCandidate{},
		&models.ReleasePlan{},
		&models.CanaryDeploymentRecord{},
		&models.OfflineDistributionPackage{},
		&models.SigningKey{},
		&models.MarketplaceListing{},
		&models.PluginImportRun{},
	))

	candidateRepo := repo.NewReleaseCandidateRepository(db)
	distRepo := repo.NewDistributionRepository(db)

	candidate, err := candidateRepo.CreateCandidate(context.Background(), &models.PluginReleaseCandidate{
		TenantUUID:       distributionTenantUUID,
		PluginID:         "px.demo",
		Version:          "v5.0.0",
		BuildArtifactURI: "s3://bucket/demo.zip",
		CommitHash:       "commit-dist",
		ReleaseNotes:     "distribution test",
		GateStatus:       models.PluginReleaseGateStatusPassed,
		ApprovalStatus:   models.PluginReleaseApprovalApproved,
	})
	require.NoError(t, err)

	svc := NewService(Dependencies{
		Candidates: candidateRepo,
		Repository: distRepo,
		ImportRuns: repo.NewImportRepository(db),
		Clock: func() time.Time {
			return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		},
	}, Options{
		FeatureEnabled:      true,
		OfflineBucket:       "bucket",
		OfflinePrefix:       "packages",
		EscalationThreshold: 2,
		ArtifactRetention:   30 * 24 * time.Hour,
		ReviewSLA:           48 * time.Hour,
	})

	content := []byte("hello distribution")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signature := ed25519.Sign(privateKey, content)
	require.NoError(t, db.Create(&models.SigningKey{
		KeyID:     "test-key",
		PublicKey: base64.RawStdEncoding.EncodeToString(publicKey),
		Enabled:   true,
	}).Error)

	pkg, err := svc.StoreOfflinePackage(context.Background(), StoreOfflinePackageInput{
		CandidateID:          candidate.UUID,
		Content:              content,
		Checksum:             checksum,
		SignatureFingerprint: base64.RawStdEncoding.EncodeToString(signature),
		SigningKeyID:         "test-key",
		Actor:                "dist-test",
		LicenseReport: map[string]any{
			"apache": 2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, models.OfflinePackageStatusSubmitted, pkg.Status)
	require.NotEmpty(t, pkg.PackageURI)

	listing, err := svc.SubmitListing(context.Background(), SubmitListingInput{
		OfflinePackageID: pkg.ID,
		Channel:          "online",
		Pricing:          map[string]any{"tier": "enterprise"},
		SupportPolicy:    map[string]any{"sla": "24x7"},
		Actor:            "ops",
	})
	require.NoError(t, err)
	require.Equal(t, models.MarketplaceListingStatusPending, listing.ReviewStatus)

	for i := 0; i < 2; i++ {
		listing, err = svc.ReviewListing(context.Background(), ReviewListingInput{
			ListingID: listing.ID,
			Decision:  "need_fix",
			Actor:     "ops",
		})
		require.NoError(t, err)
	}
	require.Equal(t, models.MarketplaceListingStatusNeedFix, listing.ReviewStatus)
	require.Equal(t, 2, listing.ReviewCount)
	require.NotNil(t, listing.EscalatedAt)

	listing, err = svc.ReviewListing(context.Background(), ReviewListingInput{
		ListingID: listing.ID,
		Decision:  "approved",
		Actor:     "ops",
	})
	require.NoError(t, err)
	require.Equal(t, models.MarketplaceListingStatusApproved, listing.ReviewStatus)
	require.NoError(t, db.Model(&models.OfflineDistributionPackage{}).Where("id = ?", pkg.ID).Update("status", models.OfflinePackageStatusApproved).Error)

	job, err := svc.StartOfflineImport(context.Background(), OfflineImportInput{
		TenantUUID:      "878f6af7-2f76-4853-8a39-29e22983b05e",
		PackageUUID:     pkg.PackageUUID.String(),
		DryRun:          true,
		LicenseAccepted: true,
		Actor:           "tenant-admin",
	})
	require.NoError(t, err)
	require.Equal(t, "completed", job.Status)
	require.NotEmpty(t, job.ID)

	fetched, err := svc.GetImportJob(context.Background(), job.TenantUUID, job.ID)
	require.NoError(t, err)
	require.Equal(t, job.ID, fetched.ID)
	require.Equal(t, job.PackageURI, fetched.PackageURI)
}

func TestStoreOfflinePackageRequiresEnabledCoreOwnedSigningKey(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file:signing-key-test?mode=memory&cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.PluginReleaseCandidate{}, &models.OfflineDistributionPackage{}, &models.SigningKey{}))
	candidateRepo := repo.NewReleaseCandidateRepository(db)
	distRepo := repo.NewDistributionRepository(db)
	candidate, err := candidateRepo.CreateCandidate(context.Background(), &models.PluginReleaseCandidate{
		TenantUUID:       distributionTenantUUID,
		PluginID:         "px.signing-test",
		Version:          "v1.0.0",
		BuildArtifactURI: "s3://bucket/signing-test.zip",
		CommitHash:       "commit-signing-test",
		GateStatus:       models.PluginReleaseGateStatusPassed,
		ApprovalStatus:   models.PluginReleaseApprovalApproved,
	})
	require.NoError(t, err)
	svc := NewService(Dependencies{Candidates: candidateRepo, Repository: distRepo}, Options{FeatureEnabled: true})
	content := []byte("signed package")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signature := base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, content))

	_, err = svc.StoreOfflinePackage(context.Background(), StoreOfflinePackageInput{
		CandidateID: candidate.UUID, Content: content, Checksum: checksum,
		SignatureFingerprint: signature, SigningKeyID: "unregistered",
		LicenseReport: map[string]any{"license": "Apache-2.0"},
	})
	require.ErrorIs(t, err, ErrInvalidInput)

	key, err := svc.RegisterSigningKey(context.Background(), RegisterSigningKeyInput{
		KeyID: "disabled-key", PublicKey: base64.RawStdEncoding.EncodeToString(publicKey),
	})
	require.NoError(t, err)
	require.NoError(t, svc.DisableSigningKey(context.Background(), key.UUID))
	_, err = svc.StoreOfflinePackage(context.Background(), StoreOfflinePackageInput{
		CandidateID: candidate.UUID, Content: content, Checksum: checksum,
		SignatureFingerprint: signature, SigningKeyID: key.KeyID,
		LicenseReport: map[string]any{"license": "Apache-2.0"},
	})
	require.ErrorIs(t, err, ErrInvalidInput)
}
