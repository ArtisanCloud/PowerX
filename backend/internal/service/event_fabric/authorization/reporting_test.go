package authorization

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type reportingTestEnv struct {
	t         *testing.T
	db        *gorm.DB
	authRepo  *eventfabricrepo.AuthorizationRepository
	reporting ReportingService
	now       time.Time
}

func newReportingTestEnv(t *testing.T) *reportingTestEnv {
	t.Helper()
	prev := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = prev })

	dsn := "file:reporting_test?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	require.NoError(t, db.AutoMigrate(
		&eventfabricmodel.AuthorizationGrant{},
		&eventfabricmodel.AuthorizationGrantCapability{},
		&eventfabricmodel.AuthorizationGrantCondition{},
		&auditmodel.AuditEvent{},
	))

	authRepo := eventfabricrepo.NewAuthorizationRepository(db)
	now := time.Now().UTC()
	reporting := NewReportingService(ReportingServiceOptions{
		AuditDB:                 db,
		AuthorizationRepository: authRepo,
		Logger:                  pxlog.GetGlobalLogger(),
	})

	return &reportingTestEnv{
		t:         t,
		db:        db,
		authRepo:  authRepo,
		reporting: reporting,
		now:       now,
	}
}

func (env *reportingTestEnv) insertGrant(tenant uuid.UUID, subject uuid.UUID, subjectType string) eventfabricmodel.AuthorizationGrant {
	grant := eventfabricmodel.AuthorizationGrant{
		TenantID:    tenant,
		SubjectType: subjectType,
		SubjectID:   subject,
		Status:      eventfabricmodel.GrantStatusActive,
		Source:      eventfabricmodel.GrantSourceSystemTemplate,
		Version:     1,
	}
	require.NoError(env.t, env.db.Create(&grant).Error)
	return grant
}

func (env *reportingTestEnv) insertAuditEvent(op string, outcome string, grant eventfabricmodel.AuthorizationGrant, meta map[string]string, occurred time.Time) {
	envelope := map[string]any{
		"topic":        auditTopicAuthorization,
		"principal_id": "",
		"latency_ms":   0,
		"metadata":     meta,
	}
	metaBytes, err := json.Marshal(envelope)
	require.NoError(env.t, err)
	row := auditmodel.AuditEvent{
		OccurredAt:   occurred,
		TenantID:     0,
		Source:       "event_fabric",
		Operation:    strings.ToUpper(op),
		ResourceType: "event",
		ResourceID:   grant.UUID.String(),
		ResourceName: auditTopicAuthorization,
		Outcome:      strings.ToUpper(outcome),
		Severity:     "INFO",
		Meta:         metaBytes,
	}
	require.NoError(env.t, env.db.Create(&row).Error)
}

func TestReportingQuery(t *testing.T) {
	env := newReportingTestEnv(t)
	ctx := context.Background()

	tenantID := uuid.New()
	subjectID := uuid.New()
	grant := env.insertGrant(tenantID, subjectID, SubjectTypeAgent)

	env.insertAuditEvent("grant.created", "success", grant, map[string]string{
		"grant_id":      grant.UUID.String(),
		"tenant_id":     tenantID.String(),
		"subject_id":    subjectID.String(),
		"subject_type":  SubjectTypeAgent,
		"grant_status":  grant.Status,
		"grant_version": "1",
	}, env.now.Add(-2*time.Hour))

	env.insertAuditEvent("evaluation.allow", "SUCCESS", grant, map[string]string{
		"grant_id":     grant.UUID.String(),
		"tenant_id":    tenantID.String(),
		"subject_id":   subjectID.String(),
		"subject_type": SubjectTypeAgent,
		"capability":   "event_fabric.publish",
		"decision":     "allow",
		"reason":       "authorized",
	}, env.now.Add(-time.Hour))

	filter := ReportingFilter{
		TenantID: tenantID,
		From:     env.now.Add(-3 * time.Hour),
		To:       env.now,
		Page:     1,
		PageSize: 10,
	}
	result, err := env.reporting.Query(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 2, result.Total)
	require.Len(t, result.Items, 2)
	require.Equal(t, "evaluation", result.Items[0].Category)
	require.Equal(t, "grant", result.Items[1].Category)

	// Subject filter
	filter.SubjectID = &subjectID
	filter.Decision = "allow"
	filter.Page = 1
	filter.PageSize = 5
	filter.From = env.now.Add(-2 * time.Hour)
	result, err = env.reporting.Query(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 1, result.Total)
	require.Len(t, result.Items, 1)
	require.Equal(t, "event_fabric.publish", result.Items[0].Capability)
}
