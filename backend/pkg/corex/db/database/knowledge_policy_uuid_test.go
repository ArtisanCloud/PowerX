package database

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestKnowledgePolicyUUIDBackfillPreservesIDsAndReferences(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.PolicyTemplateVersion{}))
	require.NoError(t, db.Exec("INSERT INTO knowledge_policy_template_versions (id,template_name,version,immutable_hash) VALUES (17,'default','v1','old17'),(18,'old','v1','old18')").Error)
	require.NoError(t, backfillKnowledgePolicyUUIDs(db))
	var first []models.PolicyTemplateVersion
	require.NoError(t, db.Order("id").Find(&first).Error)
	require.Len(t, first, 2)
	require.EqualValues(t, 17, first[0].ID)
	require.NotNil(t, first[0].UUID)
	require.NotEqual(t, *first[0].UUID, *first[1].UUID)
	require.NoError(t, backfillKnowledgePolicyUUIDs(db))
	var second []models.PolicyTemplateVersion
	require.NoError(t, db.Order("id").Find(&second).Error)
	require.Equal(t, first[0].UUID, second[0].UUID)
	require.Equal(t, first[1].UUID, second[1].UUID)
}
