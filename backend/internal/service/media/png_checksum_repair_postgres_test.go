package media

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestPNGChecksumRepairPostgresReadonlyAndLocking(t *testing.T) {
	if !strings.Contains(os.Getenv("POWERX_MEDIA_TEST_DSN"), "host=/") || os.Getenv("PNG_REPAIR_TEST_PG_OWNED") != "1" {
		t.Skip("PNG_REPAIR_TEST_PG_OWNED_REQUIRED")
	}
	f, a, _ := pngCRCRepairFixture(t)
	p, err := f.s.ScanPNGChecksumRepair(f.ctx, mediaTenantUUID, a.UUID, "fixture")
	require.NoError(t, err)
	err = f.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&m.MediaAsset{}).Where("uuid=?", a.UUID).UpdateColumn("name", "forbidden").Error
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	require.ErrorContains(t, err, "25006")
	locked := f.db.Begin()
	require.NoError(t, locked.Error)
	var row m.MediaAsset
	require.NoError(t, locked.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid=?", a.UUID).First(&row).Error)
	created, err := f.s.ApplyPNGChecksumRepair(f.ctx, p, "fixture")
	require.Error(t, err)
	require.Empty(t, created)
	require.NoError(t, locked.Rollback().Error)
	created, err = f.s.ApplyPNGChecksumRepair(f.ctx, p, "fixture")
	require.NoError(t, err)
	require.Len(t, created, 3)
}
