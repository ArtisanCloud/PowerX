package diagnostics

import (
	"context"
	"regexp"
	"testing"
	"time"

	ticketbridge "github.com/ArtisanCloud/PowerX/internal/service/integration/ticketbridge"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_debug"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_debug"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const diagnosticsTenantUUID = "a928b03c-5a21-4c61-a4a9-0ce274164a9b"

func TestDiagnosticsLifecycle(t *testing.T) {
	db, cleanup := openTestDB(t)
	defer cleanup()

	repository := repo.NewReportRepository(db)
	masker := &Masker{
		rules: []maskRule{
			{
				Name:        "token",
				regex:       regexp.MustCompile("token-[A-Za-z0-9]+"),
				Replacement: "token-***",
			},
		},
	}
	template := &ReportTemplate{
		Version: "v1",
		Sections: []TemplateSection{
			{ID: "overview", Title: "Overview", Fields: []string{"severity", "status"}},
		},
	}
	ticketSvc := ticketbridge.NewService(ticketbridge.Options{
		Provider: "noop",
		Project:  "dbg",
		Endpoint: "https://tickets.local",
	})
	svc := NewService(repository, nil, func() time.Time { return time.Unix(0, 0) }, Options{
		Template:        template,
		Masker:          masker,
		TicketBridge:    ticketSvc,
		FallbackLogBase: "https://logs.local",
	})

	report, err := svc.CreateReport(context.Background(), CreateRequest{
		TenantUUID: diagnosticsTenantUUID,
		PluginID:   "plugin.demo",
		TraceID:    "trace-1",
		Notes:      "token-SECRET",
		Summary: map[string]any{
			"severity": "P1",
			"detail":   "token-SECRET",
		},
		LogPointers: []string{"https://storage.local/token-SECRET"},
		Metadata: map[string]string{
			"owner": "token-OWNER",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.Masked)

	err = svc.CompleteReport(context.Background(), report.UUID, map[string]any{
		"severity": "P1",
		"status":   "failed",
	})
	require.NoError(t, err)

	fetched, err := repository.Get(context.Background(), report.UUID)
	require.NoError(t, err)
	require.NotEmpty(t, fetched.TicketRef)

	export, err := svc.ExportLogs(context.Background(), report.UUID)
	require.NoError(t, err)
	require.NotEmpty(t, export.URL)
}

func openTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.DiagnosticReport{}))
	return db, func() {
		coremodel.PowerXSchema = previous
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}
