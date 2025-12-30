package capability_registry

import (
	"context"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkflowTemplateServiceApproveUpgrade(t *testing.T) {
	db := newTemplateTestDB(t)
	templateRepo := repo.NewWorkflowTemplateRepository(db)
	approvalRepo := repo.NewWorkflowTemplateApprovalRepository(db)
	service := NewWorkflowTemplateService(WorkflowTemplateServiceOptions{
		TemplateRepo: templateRepo,
		ApprovalRepo: approvalRepo,
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})
	require.NotNil(t, service)

	template := &models.WorkflowTemplateRef{
		CapabilityID:          "demo.capability",
		TemplateID:            "tpl.demo",
		Name:                  "Demo Template",
		CapabilitiesHash:      "hash-demo",
		TemplateHash:          "tpl-hash",
		RequiresManualUpgrade: true,
	}
	ctx := context.Background()
	_, err := templateRepo.Upsert(ctx, template)
	require.NoError(t, err)

	approval, err := service.ApproveUpgrade(ctx, TemplateUpgradeInput{
		TemplateID:       template.TemplateID,
		CapabilitiesHash: "hash-demo",
		Reason:           "accept",
		Operator:         "admin-1",
	})
	require.NoError(t, err)
	require.Equal(t, "tpl.demo", approval.TemplateID)
	require.Equal(t, "hash-demo", approval.CapabilitiesHash)
	require.Equal(t, "admin-1", approval.ApprovedBy)
	require.WithinDuration(t, time.Unix(1700000000, 0).UTC(), approval.ApprovedAt, time.Millisecond)

	_, err = service.ApproveUpgrade(ctx, TemplateUpgradeInput{
		TemplateID:       template.TemplateID,
		CapabilitiesHash: "mismatch",
	})
	require.ErrorIs(t, err, ErrWorkflowTemplateHashMismatch)
}

func newTemplateTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})
	require.NoError(t, db.AutoMigrate(
		&models.WorkflowTemplateRef{},
		&models.WorkflowTemplateApproval{},
	))
	return db
}
