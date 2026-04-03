package skills

import (
	"context"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSourcePolicyAdminService_GetDefault(t *testing.T) {
	db := setupSourcePolicyAdminDB(t)
	svc := NewSourcePolicyAdminService(db)

	view, err := svc.GetTenantSourcePolicy(context.Background(), "tenant-admin-default")
	require.NoError(t, err)
	require.Equal(t, "default", view.EffectiveSource)
	require.Equal(t, []string{"builtin", "plugin", "third_party"}, view.Allowlist)
}

func TestSourcePolicyAdminService_SetAndGet(t *testing.T) {
	db := setupSourcePolicyAdminDB(t)
	svc := NewSourcePolicyAdminService(db)

	view, err := svc.SetTenantSourcePolicy(context.Background(), "tenant-admin-set", []string{"PLUGIN", "builtin", "plugin"})
	require.NoError(t, err)
	require.Equal(t, "tenant", view.EffectiveSource)
	require.Equal(t, []string{"plugin", "builtin"}, view.Allowlist)

	got, err := svc.GetTenantSourcePolicy(context.Background(), "tenant-admin-set")
	require.NoError(t, err)
	require.Equal(t, "tenant", got.EffectiveSource)
	require.Equal(t, []string{"plugin", "builtin"}, got.Allowlist)
}

func TestSourcePolicyAdminService_SetInvalid(t *testing.T) {
	db := setupSourcePolicyAdminDB(t)
	svc := NewSourcePolicyAdminService(db)

	_, err := svc.SetTenantSourcePolicy(context.Background(), "tenant-admin-invalid", []string{"unknown"})
	require.ErrorIs(t, err, ErrSkillSourcePolicyInvalid)
}

func setupSourcePolicyAdminDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(&settingmodel.TenantSetting{}))
	return db
}
