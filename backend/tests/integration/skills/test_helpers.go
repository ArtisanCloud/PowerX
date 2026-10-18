package skillsintegration

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSkillsDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&parseTime=true&_loc=UTC", dbName)), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&skillmodel.SkillRegistryRecord{},
		&skillmodel.SkillLifecycleAudit{},
		&skillmodel.SkillExecutionTrace{},
		&skillmodel.SkillCapabilityBinding{},
		&skillmodel.OfficialSkillCatalogEntry{},
	))
	return db
}

// Runtime 的 message Trace 需要完整归属；仅在独立夹具缺失时生成新标识。
func withMessageTrace(ctx context.Context) context.Context {
	for _, key := range []string{"session_id", "message_id"} {
		if ctx.Value(key) == nil {
			ctx = context.WithValue(ctx, key, uuid.NewString())
		}
	}
	return ctx
}
