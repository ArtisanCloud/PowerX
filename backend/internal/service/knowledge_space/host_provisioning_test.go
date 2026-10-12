package knowledge_space

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func hostProvisioningFixture(t *testing.T) (*Service, *gorm.DB, string, string) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&iam.Department{}, &models.KnowledgeSpace{}, &models.PolicyTemplateVersion{}, &models.IngestionProfileVersion{}, &models.IndexProfileVersion{}, &models.RAGProfileVersion{}, &models.IAMSyncTask{}, &models.AuditTrailEntry{}))
	// PostgreSQL JSON defaults are not SQLite syntax. This fixture exercises the
	// actual repository queries against the same model columns.
	require.NoError(t, db.Exec("CREATE TABLE ai_model_profiles (id integer primary key, env text, tenant_uuid text, modality text, provider text, model text, defaults text, cap_cache text, tags text, deleted_at datetime)").Error)
	tenant := uuid.NewString()
	// Use the model's actual table name (the plural name is not assumed).
	svc := NewService(ServiceOptions{DB: db})
	department := iam.Department{TenantUUID: tenant, Key: "knowledge", Name: "Knowledge", Status: 1}
	require.NoError(t, db.Create(&department).Error)
	require.NoError(t, db.Create(&models.PolicyTemplateVersion{TemplateName: "default", Version: "v1"}).Error)
	require.NoError(t, db.Create(&models.IngestionProfileVersion{TenantUUID: tenant, ProfileKey: "p0_basic", Version: 1, Status: models.ProfileStatusPublished, DisplayName: "General"}).Error)
	require.NoError(t, db.Create(&models.IndexProfileVersion{TenantUUID: tenant, ProfileKey: "p0_basic", Version: 1, Status: models.ProfileStatusPublished, DisplayName: "General"}).Error)
	require.NoError(t, db.Create(&models.RAGProfileVersion{TenantUUID: tenant, ProfileKey: "p0_basic", Version: 1, Status: models.ProfileStatusPublished, DisplayName: "General"}).Error)
	require.NoError(t, db.Exec("INSERT INTO ai_model_profiles (env,tenant_uuid,modality,provider,model,defaults,cap_cache,tags) VALUES (?,?,?,?,?,?,?,?)", "dev", tenant, "embedding", "ollama", "embed", `{"dimensions":768}`, `{"probed_at":"tested"}`, "[]").Error)
	return svc, db, tenant, department.DepartmentUUID.String()
}

func TestHostProvisioningCatalogCreateAndTenantIsolation(t *testing.T) {
	svc, db, tenant, department := hostProvisioningFixture(t)
	ctx := context.Background()
	catalog, err := svc.GetHostCatalog(ctx, tenant)
	require.NoError(t, err)
	var selected *HostStrategyPackage
	for i := range catalog.StrategyPackages {
		if catalog.StrategyPackages[i].Key == "A_simple" {
			selected = &catalog.StrategyPackages[i]
		}
	}
	require.NotNil(t, selected)
	require.True(t, selected.Available, selected.UnavailableReasons)
	request := HostCreateSpaceRequest{Name: "Contract space", DepartmentUUID: department, StrategyKey: selected.Key}
	item, err := svc.CreateHostSpace(ctx, tenant, request)
	require.NoError(t, err)
	require.Equal(t, models.KnowledgeSpaceStatusPending, item.Status)
	var row models.KnowledgeSpace
	require.NoError(t, db.Where("uuid = ?", item.SpaceUUID).First(&row).Error)
	require.Equal(t, department, row.DepartmentUUID.String())
	require.Equal(t, item.Profiles.RAG.UUID, row.RAGProfileUUID.String())
	require.Equal(t, "KNOWLEDGE", row.DepartmentCode)
	var tasks int64
	require.NoError(t, db.Model(&models.IAMSyncTask{}).Count(&tasks).Error)
	require.EqualValues(t, 1, tasks)
	_, err = svc.CreateHostSpace(ctx, tenant, request)
	require.Equal(t, http.StatusConflict, dto.StatusCode(err))
	_, err = svc.CreateHostSpace(ctx, uuid.NewString(), request)
	require.Equal(t, http.StatusForbidden, dto.StatusCode(err))
	request.Name = "wrong profile"
	request.RAGProfileUUID = uuid.NewString()
	_, err = svc.CreateHostSpace(ctx, tenant, request)
	require.Equal(t, http.StatusBadRequest, dto.StatusCode(err))
}

func TestHostProvisioningRollsBackWhenAuditFails(t *testing.T) {
	svc, db, tenant, department := hostProvisioningFixture(t)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == coremodel.TableKnowledgeAuditTrailEntries {
			tx.AddError(errors.New("audit unavailable"))
		}
	}))
	_, err := svc.CreateHostSpace(context.Background(), tenant, HostCreateSpaceRequest{Name: "Rollback", DepartmentUUID: department, StrategyKey: "A_simple"})
	require.ErrorContains(t, err, "audit unavailable")
	for _, model := range []any{&models.KnowledgeSpace{}, &models.IAMSyncTask{}, &models.AuditTrailEntry{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestHostCatalogDoesNotRepairOrFabricateReferences(t *testing.T) {
	svc, db, tenant, _ := hostProvisioningFixture(t)
	require.NoError(t, db.Model(&models.PolicyTemplateVersion{}).Where("template_name = ?", "default").Update("uuid", nil).Error)
	_, err := svc.GetHostCatalog(context.Background(), tenant)
	require.Equal(t, http.StatusServiceUnavailable, dto.StatusCode(err))
	var count int64
	require.NoError(t, db.Model(&models.PolicyTemplateVersion{}).Where("uuid IS NULL").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestHostCatalogUnavailableProfilesDoNotCreateSpace(t *testing.T) {
	svc, db, tenant, department := hostProvisioningFixture(t)
	require.NoError(t, db.Where("tenant_uuid = ?", tenant).Delete(&models.RAGProfileVersion{}).Error)
	catalog, err := svc.GetHostCatalog(context.Background(), tenant)
	require.NoError(t, err)
	for _, strategy := range catalog.StrategyPackages {
		if strategy.Key == "A_simple" {
			require.False(t, strategy.Available)
			require.Contains(t, strategy.UnavailableReasons, "rag_profile_not_published")
		}
	}
	_, err = svc.CreateHostSpace(context.Background(), tenant, HostCreateSpaceRequest{Name: "Unavailable", DepartmentUUID: department, StrategyKey: "A_simple"})
	require.Equal(t, http.StatusPreconditionFailed, dto.StatusCode(err))
	var count int64
	require.NoError(t, db.Model(&models.KnowledgeSpace{}).Count(&count).Error)
	require.Zero(t, count)
}
