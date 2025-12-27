package capability_registry

import (
	"context"
	"testing"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestWorkflowCatalogRefreshAndSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	recordRepo := repo.NewCapabilityRecordRepository(db, nil)
	templateRepo := repo.NewWorkflowTemplateRepository(db)

	now := time.Unix(1700000000, 0).UTC()
	require.NoError(t, db.Create(&models.CapabilityRecord{
		CapabilityID:     "demo.capability.workflow",
		PluginID:         "demo.plugin",
		PluginVersion:    "1.0.0",
		Title:            "Demo Capability",
		Description:      "workflow demo",
		Intents:          datatypes.JSON([]byte(`["demo.intent"]`)),
		ToolScope:        datatypes.JSON([]byte(`["default"]`)),
		Protocols:        datatypes.JSON([]byte(`[]`)),
		Policy:           datatypes.JSON([]byte(`{"prefer":"mcp"}`)),
		CapabilitiesHash: "hash-demo-v1",
		ProtocolHash:     "proto-demo-v1",
		Status:           "published",
	}).Error)

	require.NoError(t, db.Create(&models.WorkflowTemplateRef{
		CapabilityID:          "demo.capability.workflow",
		TemplateID:            "tpl.demo.workflow",
		Name:                  "Demo Workflow Template",
		Description:           "template description",
		Steps:                 datatypes.JSON([]byte(`[{"id":"step1","type":"task"}]`)),
		ParamsSchema:          datatypes.JSON([]byte(`{"type":"object"}`)),
		ProtocolRequirements:  datatypes.JSON([]byte(`[]`)),
		CapabilitiesHash:      "hash-demo-v1",
		TemplateHash:          "tpl-hash-v1",
		RequiresManualUpgrade: true,
		LastSyncedAt:          &now,
	}).Error)

	_, redisClient := newTestRedis(t)

	catalog := NewWorkflowCatalog(WorkflowCatalogOptions{
		TemplateRepo: templateRepo,
		RecordRepo:   recordRepo,
		Redis:        redisClient,
		Clock: func() time.Time {
			return now
		},
	})

	snapshot, err := catalog.Refresh(ctx)
	require.NoError(t, err)
	require.Equal(t, now, snapshot.GeneratedAt)
	require.Len(t, snapshot.Templates, 1)
	entry := snapshot.Templates[0]
	require.Equal(t, "demo.plugin", entry.PluginID)
	require.Equal(t, "Demo Capability", entry.CapabilityTitle)
	require.Equal(t, "hash-demo-v1", entry.CapabilitiesHash)
	require.NotEmpty(t, entry.TemplateHash)

	cached, err := catalog.Snapshot(ctx)
	require.NoError(t, err)
	require.Equal(t, snapshot.Version, cached.Version)
	require.Len(t, cached.Templates, 1)
}
