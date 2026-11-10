package backup_ops

import (
	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestQuarantineInventoryMappingAndFilters(t *testing.T) {
	now := time.Now().UTC()
	r := recovery.RetentionRecord{Operation: "quarantine_cleanup", RequestID: uuid.NewString(), ArtifactID: 3, JobID: 4, PolicyID: 5, Token: uuid.NewString(), State: "quarantined", ExpectedDiagnosis: "missing", ActorID: 1, CreatedAt: now, CompletedAt: &now}
	row := repo.CleanupReviewRow{ID: 3, JobID: 4, PolicyID: 5, JobFound: true, CleanupToken: r.Token, CleanupState: "quarantined", SizeBytes: 1024, UpdatedAt: now}
	item := inventoryItem(r, nil, &row, "generation", true)
	require.Equal(t, "sql_missing", item.HistoryStatus)
	require.Equal(t, "quarantined", item.Current)
	require.True(t, item.CanInspect)
	require.Len(t, item.Snapshot, 64)
	require.False(t, item.ValidBackupProof)
	row.CleanupToken = uuid.NewString()
	changed := inventoryItem(r, nil, &row, "generation", true)
	require.Equal(t, "identity_changed", changed.Current)
	require.Empty(t, changed.Snapshot)
	require.NotEqual(t, row.CleanupToken, changed.Token)
	row.CleanupToken = r.Token
	row.JobFound = false
	row.PolicyID = 0
	require.Equal(t, "unavailable", inventoryItem(r, nil, &row, "generation", true).Current)
	row.JobFound = true
	row.PolicyID = r.PolicyID
	row.ArtifactDeleted = true
	require.Equal(t, "unavailable", inventoryItem(r, nil, &row, "generation", true).Current)
	require.Equal(t, "missing", inventoryItem(r, nil, nil, "generation", true).Current)
	a, b := item, item
	b.RequestID = uuid.NewString()
	b.ArtifactID = "10"
	size := int64(2048)
	b.SizeBytes = &size
	b.Current = "not_quarantined"
	data := quarantineInventory{items: []QuarantineInventoryItem{a, b}, unattributed: 2}
	o := QuarantineInventoryOptions{1, 1, "all", "all", "all", "", "size", "desc"}
	require.True(t, validInventoryOptions(o))
	page := quarantineInventoryPage(data, o)
	require.Equal(t, 2, page.Total)
	require.Equal(t, b.RequestID, page.Items[0].RequestID)
	require.Equal(t, 2, page.UnattributedQuarantinedArtifacts)
	o.Page = 2
	require.Equal(t, a.RequestID, quarantineInventoryPage(data, o).Items[0].RequestID)
	o.Page = 1
	o.Current = "quarantined"
	require.Equal(t, 1, quarantineInventoryPage(data, o).Total)
	o.Current = "all"
	o.Query = b.RequestID
	require.Equal(t, b.RequestID, quarantineInventoryPage(data, o).Items[0].RequestID)
	o.Query = "not found"
	require.Empty(t, quarantineInventoryPage(data, o).Items)
	o.Query = ""
	o.Sort = "artifact"
	o.Direction = "asc"
	require.Equal(t, "3", quarantineInventoryPage(data, o).Items[0].ArtifactID)
	for _, bad := range []QuarantineInventoryOptions{{0, 20, "all", "all", "all", "", "time", "desc"}, {1, 101, "all", "all", "all", "", "time", "desc"}, {1, 20, "all", "all", "all", "", "DROP TABLE", "desc"}, {1, 20, "all", "all", "all", "", "time", "sideways"}} {
		require.False(t, validInventoryOptions(bad))
	}
}
