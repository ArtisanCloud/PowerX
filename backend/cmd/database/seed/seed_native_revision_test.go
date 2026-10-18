package seed

import (
	"context"
	"errors"
	"testing"

	skillsvc "github.com/ArtisanCloud/PowerX/internal/service/skills"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type revisionPackageStore struct {
	fail  bool
	calls int
}

func (s *revisionPackageStore) PutSkillPackage(_ context.Context, key, _ string, _ []byte) (string, error) {
	s.calls++
	if s.fail {
		return "", errors.New("fixture.storage_unavailable")
	}
	return "local://" + key, nil
}

func TestNativeRevisionUpgradeAtomicAndIdempotent(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&skillmodel.SkillPackageSource{}, &skillmodel.SkillDefinitionDraft{}, &skillmodel.SkillDefinitionRevision{}, &modelagent.EvidenceSnapshot{}))
	ctx := context.Background()
	tenant, actor := uuid.NewString(), uuid.NewString()
	store := &revisionPackageStore{}
	publisher := skillsvc.NewPackagePublisher(store)
	require.NoError(t, seedNativeMarketingRevisions(ctx, db, publisher, tenant, actor))
	repo := skillrepo.NewSkillDefinitionRepository(db)
	draft, err := repo.GetDraftBySkillID(ctx, tenant, MarketingReviewSummarizeSkillID)
	require.NoError(t, err)
	original := draft.CurrentRevisionUUID
	var count int64
	require.NoError(t, db.Model(&skillmodel.SkillDefinitionRevision{}).Count(&count).Error)
	beforeCalls := store.calls
	require.NoError(t, seedNativeMarketingRevisions(ctx, db, publisher, tenant, actor))
	require.Equal(t, beforeCalls, store.calls)
	// 模拟现有旧 Revision；新发布写存储失败，必须保留旧 current/status，不能遗留新草稿。
	revision, err := repo.GetCurrentRevision(ctx, tenant, draft.UUID.String())
	require.NoError(t, err)
	require.NoError(t, db.Model(revision).Update("definition_json", `{"schema":"powerx.skill.definition/v2","executor":{"type":"llm_prompt","prompt_template_i18n":{"en-US":"fixture"}},"entrypoints":["runbook.default"]}`).Error)
	store.fail = true
	require.ErrorContains(t, seedNativeMarketingRevisions(ctx, db, publisher, tenant, actor), "fixture.storage_unavailable")
	current, err := repo.GetDraftBySkillID(ctx, tenant, MarketingReviewSummarizeSkillID)
	require.NoError(t, err)
	require.Equal(t, original, current.CurrentRevisionUUID)
	require.Equal(t, skillmodel.SkillDefinitionDraftStatusPublished, current.Status)
	var after int64
	require.NoError(t, db.Model(&skillmodel.SkillDefinitionRevision{}).Count(&after).Error)
	require.Equal(t, count, after)
	// 缺归档表时预检先失败，连存储发布也不能发生。
	store.fail = false
	beforeCalls = store.calls
	require.NoError(t, db.Migrator().DropTable(&modelagent.EvidenceSnapshot{}))
	require.Error(t, seedNativeMarketingRevisions(ctx, db, publisher, tenant, actor))
	require.Equal(t, beforeCalls, store.calls)
	current, err = repo.GetDraftBySkillID(ctx, tenant, MarketingReviewSummarizeSkillID)
	require.NoError(t, err)
	require.Equal(t, original, current.CurrentRevisionUUID)
}
