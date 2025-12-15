package distribution

import (
	"context"
	"crypto/sha256"
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
		&models.MarketplaceListing{},
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

	pkg, err := svc.StoreOfflinePackage(context.Background(), StoreOfflinePackageInput{
		CandidateID: candidate.UUID,
		Content:     content,
		Checksum:    checksum,
		Actor:       "dist-test",
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

	job, err := svc.StartOfflineImport(context.Background(), OfflineImportInput{
		TenantUUID:      "878f6af7-2f76-4853-8a39-29e22983b05e",
		PackageURI:      pkg.PackageURI,
		Checksum:        checksum,
		DryRun:          true,
		LicenseAccepted: true,
		Actor:           "tenant-admin",
	})
	require.NoError(t, err)
	require.Equal(t, "completed", job.Status)
	require.NotEmpty(t, job.ID)

	fetched, err := svc.GetImportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, job.ID, fetched.ID)
	require.Equal(t, job.PackageURI, fetched.PackageURI)
}
