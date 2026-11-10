package customer

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClientApplicationRequiresLiveOperationGrant(t *testing.T) {
	s, db, tenant, _ := clientAppTestService(t)
	require.NoError(t, db.AutoMigrate(&im.Permission{}, &im.Role{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}))
	member := uuid.NewString()
	ctx := reqctx.WithMemberUUID(reqctx.WithMemberID(reqctx.WithUserID(reqctx.WithTenantUUID(context.Background(), tenant), 42), 7), member)
	role := im.Role{TenantUUID: tenant, Scope: "tenant", Code: "role_admin", Name: "Admin"}
	require.NoError(t, db.Create(&role).Error)
	binding := im.RoleBinding{TenantUUID: tenant, RoleUUID: role.UUID.String(), RoleID: role.ID, SubjectType: im.SubMember, SubjectUUID: member, SubjectID: 7}
	require.NoError(t, db.Create(&binding).Error)
	_, file, _, _ := runtime.Caller(0)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", filepath.Clean(filepath.Join(filepath.Dir(file), "../../../config/platform_capabilities")))
	formal, err := apikeypermissions.BuildPlatformCapabilityPermissions()
	require.NoError(t, err)
	grant := func(resource, action string) im.Permission {
		var p im.Permission
		for _, row := range formal {
			if row.Module == "customer" && row.Resource == resource && row.Action == action {
				p = row
				break
			}
		}
		require.NotEmpty(t, p.Resource, "permission must be materialized by the checked-in formal capability")
		require.False(t, p.AllowAPIKey, "credential administration must not be API-key grantable")
		require.NoError(t, db.Create(&p).Error)
		require.NoError(t, db.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: p.ID, PermissionUUID: p.PermissionUUID}).Error)
		return p
	}
	_, err = s.Catalog(ctx)
	require.Equal(t, 403, dto.StatusCode(err))
	grant("admin_customers_client_apps", "list")
	grant("admin_customers_client_apps_catalog", "list")
	read := grant("admin_customers_client_apps_app_uuid", "read")
	_, err = s.Catalog(ctx)
	require.NoError(t, err)
	_, err = s.Save(ctx, "", clientAppInput("credential"))
	require.Equal(t, 403, dto.StatusCode(err), "read does not imply write")
	grant("admin_customers_client_apps", "create")
	created, err := s.Save(ctx, "", clientAppInput("credential"))
	require.NoError(t, err)
	_, err = s.Save(ctx, created.UUID, clientAppUpdate(created))
	require.Equal(t, 403, dto.StatusCode(err), "create does not imply edit")
	edit := grant("admin_customers_client_apps_app_uuid", "update")
	_, err = s.Save(ctx, created.UUID, clientAppUpdate(created))
	require.NoError(t, err)
	require.NoError(t, db.Model(&edit).Update("effect", "deny").Error)
	_, err = s.Save(ctx, created.UUID, clientAppUpdate(created))
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, db.Model(&read).Update("status", "deprecated").Error)
	_, err = s.Get(ctx, created.UUID)
	require.Equal(t, 403, dto.StatusCode(err))
}
