package plugin_governance

import (
	"context"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	govmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_governance"
	pluginrelease "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	govrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const governanceTenantUUID = "d52bcd75-87c7-4ba4-95b3-7e0f588f956f"

func TestScanCreatesReport(t *testing.T) {
	db := openGovernanceDB(t)
	govRepository := govrepo.NewReportRepository(db)
	releaseRepo := plugin_release.NewReleaseCandidateRepository(db)
	service := NewService(govRepository, releaseRepo, func() time.Time { return time.Unix(0, 0) })

	candidate := &pluginrelease.PluginReleaseCandidate{
		TenantUUID:       governanceTenantUUID,
		PluginID:         "plugin.demo",
		Version:          "2.0.0",
		BuildArtifactURI: "s3://demo",
		CommitHash:       "abcd",
		GateStatus:       pluginrelease.PluginReleaseGateStatusPassed,
		ApprovalStatus:   pluginrelease.PluginReleaseApprovalApproved,
	}
	require.NoError(t, db.Create(candidate).Error)

	report, err := service.Scan(context.Background(), ScanInput{
		TenantUUID:    governanceTenantUUID,
		PluginID:      "plugin.demo",
		TargetVersion: "2.0.0",
	})
	require.NoError(t, err)
	require.Equal(t, "pass", report.RiskLevel)

	board, err := service.Board(context.Background(), BoardFilter{TenantUUID: governanceTenantUUID, Limit: 5})
	require.NoError(t, err)
	require.Equal(t, int64(1), board.Total)
}

func openGovernanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&pluginrelease.PluginReleaseCandidate{}, &govmodel.VersionGovernanceReport{}))
	return db
}
