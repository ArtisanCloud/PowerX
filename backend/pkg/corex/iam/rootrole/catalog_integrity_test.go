package rootrole

import (
	"testing"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureRejectsNumericKeyCollisionWithDifferentRoleUUID(t *testing.T) {
	oldSchema := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&im.Role{}, &im.Permission{}, &im.RolePermission{}))
	_, err = Ensure(db)
	require.NoError(t, err)
	var root im.Role
	require.NoError(t, db.Where("code=? AND scope=?", "root", "system").Take(&root).Error)
	p := im.Permission{Module: "fixture", Resource: "order", Action: "read", Status: im.PermissionStatusActive}
	require.NoError(t, db.Create(&p).Error)
	stale := im.RolePermission{RoleID: root.ID, RoleUUID: uuid.New(), PermissionID: p.ID, PermissionUUID: p.PermissionUUID}
	require.NoError(t, db.Create(&stale).Error)
	_, err = Ensure(db)
	require.ErrorContains(t, err, "IAM_ROOT_ROLE_CATALOG_INCONSISTENT")
	var after im.RolePermission
	require.NoError(t, db.Where("role_id=? AND permission_id=?", root.ID, p.ID).Take(&after).Error)
	require.Equal(t, stale, after)
}
