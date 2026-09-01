package iam

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	directoryTestTenant = "2cd8ee15-f9d8-461a-8de0-4731348f7e0a"
	directoryTestPlugin = "com.powerx.plugin.directory-contract"
)

func TestTenantMemberDirectoryRoutes(t *testing.T) {
	db := directoryRouteTestDB(t)
	member := directoryRouteTestMember(t, db, directoryTestTenant, "Ada Lovelace")
	foreignMember := directoryRouteTestMember(t, db, "5f4c25be-bb48-4b5a-9dcb-01de2a7d6a3a", "Other Tenant")
	directoryRouteGrant(t, db, directoryTestTenant, directoryTestPlugin, []string{"com.corex.iam.members.read", "com.corex.iam.directory.read", "com.corex.iam.authorization.check"})
	directoryRouteGrant(t, db, directoryTestTenant, "com.powerx.plugin.ungranted", nil)

	router := directoryRouteTestRouter(db)
	require.NoError(t, db.Create(&tenantmodel.Tenant{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(directoryTestTenant)}, Key: "directory-test", Name: "Directory Test", Status: tenantmodel.TenantStatusActive, Type: tenantmodel.TenantTypeEnterprise, Plan: tenantmodel.TenantPlanBasic}).Error)

	t.Run("current tenant and paged members derive tenant from credential", func(t *testing.T) {
		tenant := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/tenant", nil, directoryTestPlugin)
		require.Equal(t, http.StatusOK, tenant.Code)
		require.Equal(t, directoryTestTenant, directoryRouteBody(t, tenant)["data"].(map[string]any)["tenant_uuid"])

		page := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members?page=1&page_size=1", nil, directoryTestPlugin)
		require.Equal(t, http.StatusOK, page.Code)
		data := directoryRouteBody(t, page)["data"].(map[string]any)
		require.Len(t, data["items"].([]any), 1)
		require.Equal(t, float64(1), data["pagination"].(map[string]any)["page"])
		require.Equal(t, float64(1), data["pagination"].(map[string]any)["page_size"])

		override := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members?tenant_uuid="+uuid.NewString(), nil, directoryTestPlugin)
		require.Equal(t, http.StatusBadRequest, override.Code)
		require.Equal(t, "IAM_INVALID_ARGUMENT", directoryRouteBody(t, override)["reason_code"])
	})

	t.Run("success", func(t *testing.T) {
		response := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members/"+member.UUID.String(), nil, directoryTestPlugin)
		require.Equal(t, http.StatusOK, response.Code)
		body := directoryRouteBody(t, response)
		require.Equal(t, member.UUID.String(), body["data"].(map[string]any)["member_uuid"])
		require.Equal(t, "Ada Lovelace", body["data"].(map[string]any)["display_name"])
	})

	t.Run("member missing and cross tenant do not leak", func(t *testing.T) {
		missing := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members/"+uuid.NewString(), nil, directoryTestPlugin)
		require.Equal(t, http.StatusNotFound, missing.Code)
		require.Equal(t, "IAM_MEMBER_NOT_FOUND", directoryRouteBody(t, missing)["reason_code"])
		foreign := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members/"+foreignMember.UUID.String(), nil, directoryTestPlugin)
		require.Equal(t, http.StatusNotFound, foreign.Code)
		require.Equal(t, "IAM_MEMBER_NOT_FOUND", directoryRouteBody(t, foreign)["reason_code"])
	})

	t.Run("batch duplicate is rejected before member lookup", func(t *testing.T) {
		missingUUID := uuid.NewString()
		payload := map[string]any{"member_uuids": []string{missingUUID, missingUUID}}
		response := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/members:batch-get", payload, directoryTestPlugin)
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Equal(t, "IAM_INVALID_ARGUMENT", directoryRouteBody(t, response)["reason_code"])
	})

	t.Run("batch resolve returns missing and cross tenant UUIDs without hiding resolved members", func(t *testing.T) {
		missingUUID := uuid.NewString()
		payload := map[string]any{"member_uuids": []string{member.UUID.String(), missingUUID, foreignMember.UUID.String()}}
		response := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/members:batch-resolve", payload, directoryTestPlugin)
		require.Equal(t, http.StatusOK, response.Code)
		data := directoryRouteBody(t, response)["data"].(map[string]any)
		items := data["items"].([]any)
		require.Len(t, items, 1)
		item := items[0].(map[string]any)
		require.Equal(t, member.UUID.String(), item["member_uuid"])
		require.Equal(t, member.UserUUID, item["user_uuid"])
		require.Equal(t, "Ada Lovelace", item["display_name"])
		require.Equal(t, []any{missingUUID, foreignMember.UUID.String()}, data["missing_member_uuids"])
	})

	t.Run("batch resolve rejects duplicate UUIDs before lookup", func(t *testing.T) {
		missingUUID := uuid.NewString()
		payload := map[string]any{"member_uuids": []string{missingUUID, missingUUID}}
		response := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/members:batch-resolve", payload, directoryTestPlugin)
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Equal(t, "IAM_INVALID_ARGUMENT", directoryRouteBody(t, response)["reason_code"])
	})

	t.Run("missing service identity is unauthorized", func(t *testing.T) {
		response := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members/"+member.UUID.String(), nil, "")
		require.Equal(t, http.StatusUnauthorized, response.Code)
		require.Equal(t, "IAM_UNAUTHORIZED", directoryRouteBody(t, response)["reason_code"])
	})

	t.Run("same tenant ungranted plugin is forbidden", func(t *testing.T) {
		response := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members/"+member.UUID.String(), nil, "com.powerx.plugin.ungranted")
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Equal(t, "IAM_FORBIDDEN", directoryRouteBody(t, response)["reason_code"])
	})

	t.Run("batch resolve requires the directory capability", func(t *testing.T) {
		payload := map[string]any{"member_uuids": []string{member.UUID.String()}}
		response := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/members:batch-resolve", payload, "com.powerx.plugin.ungranted")
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Equal(t, "IAM_FORBIDDEN", directoryRouteBody(t, response)["reason_code"])
	})
}

func TestTenantDirectoryAndAuthorizationRoutes(t *testing.T) {
	db := directoryRouteTestDB(t)
	member := directoryRouteTestMember(t, db, directoryTestTenant, "Ada Lovelace")
	deniedMember := directoryRouteTestMember(t, db, directoryTestTenant, "Grace Hopper")
	foreignMember := directoryRouteTestMember(t, db, "5f4c25be-bb48-4b5a-9dcb-01de2a7d6a3a", "Other Tenant")
	directoryRouteGrant(t, db, directoryTestTenant, directoryTestPlugin, []string{"com.corex.iam.members.read", "com.corex.iam.directory.read", "com.corex.iam.authorization.check"})

	parent := modeliam.Department{TenantUUID: directoryTestTenant, Key: "engineering", Name: "Engineering"}
	require.NoError(t, db.Create(&parent).Error)
	parentUUID := uuid.NewString()
	require.NoError(t, db.Table((&modeliam.Department{}).GetTableName(true)).Where("id = ?", parent.ID).Update("department_uuid", parentUUID).Error)
	child := modeliam.Department{TenantUUID: directoryTestTenant, Key: "platform", Name: "Platform", ParentID: &parent.ID, LeaderMemberID: &member.ID}
	require.NoError(t, db.Create(&child).Error)
	childUUID := uuid.NewString()
	require.NoError(t, db.Table((&modeliam.Department{}).GetTableName(true)).Where("id = ?", child.ID).Updates(map[string]any{"department_uuid": childUUID, "parent_department_uuid": parentUUID, "leader_member_uuid": member.UUID.String()}).Error)
	require.NoError(t, db.Create(&modeliam.MemberDepartment{TenantUUID: directoryTestTenant, MemberID: member.ID, DepartmentID: child.ID}).Error)
	require.NoError(t, db.Table((&modeliam.MemberDepartment{}).GetTableName(true)).Where("member_id = ? AND department_id = ?", member.ID, child.ID).Updates(map[string]any{"member_uuid": member.UUID.String(), "department_uuid": childUUID}).Error)

	permission := modeliam.Permission{Module: "corex.iam", Resource: "members", Action: "read", Effect: "allow", Status: modeliam.PermissionStatusActive}
	require.NoError(t, db.Create(&permission).Error)
	permissionUUID := uuid.NewString()
	require.NoError(t, db.Table((&modeliam.Permission{}).GetTableName(true)).Where("id = ?", permission.ID).Update("permission_uuid", permissionUUID).Error)
	role := modeliam.Role{Scope: "tenant", TenantUUID: directoryTestTenant, Code: "directory_reader", Name: "Directory Reader"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&modeliam.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error)
	require.NoError(t, db.Table((&modeliam.RolePermission{}).GetTableName(true)).Where("role_id = ? AND permission_id = ?", role.ID, permission.ID).Updates(map[string]any{"role_uuid": role.UUID.String(), "permission_uuid": permissionUUID}).Error)
	require.NoError(t, db.Create(&modeliam.RoleBinding{TenantUUID: directoryTestTenant, RoleUUID: role.UUID.String(), RoleID: role.ID, SubjectType: modeliam.SubMember, SubjectUUID: member.UUID.String(), SubjectID: member.ID, DataScope: modeliam.ScopeTenant}).Error)

	router := directoryRouteTestRouter(db)
	t.Run("UUID directories", func(t *testing.T) {
		departments := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/departments", nil, directoryTestPlugin)
		require.Equal(t, http.StatusOK, departments.Code)
		departmentItems := directoryRouteBody(t, departments)["data"].(map[string]any)["items"].([]any)
		require.Len(t, departmentItems, 2)
		var childItem map[string]any
		for _, item := range departmentItems {
			candidate := item.(map[string]any)
			if candidate["name"] == "Platform" {
				childItem = candidate
				break
			}
		}
		require.NotNil(t, childItem)
		require.Equal(t, childUUID, childItem["department_uuid"])
		require.Equal(t, parentUUID, childItem["parent_department_uuid"])

		roles := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/roles", nil, directoryTestPlugin)
		require.Equal(t, http.StatusOK, roles.Code)
		require.Equal(t, role.UUID.String(), directoryRouteBody(t, roles)["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["role_uuid"])

		permissions := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/permissions", nil, directoryTestPlugin)
		require.Equal(t, http.StatusOK, permissions.Code)
		require.Equal(t, permissionUUID, directoryRouteBody(t, permissions)["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["permission_uuid"])

		memberResponse := directoryRouteRequest(router, http.MethodGet, "/tenant/iam/members/"+member.UUID.String(), nil, directoryTestPlugin)
		require.Equal(t, http.StatusOK, memberResponse.Code)
		require.Equal(t, []any{childUUID}, directoryRouteBody(t, memberResponse)["data"].(map[string]any)["department_uuids"])
	})

	t.Run("authorization result and contract errors", func(t *testing.T) {
		allowed := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", map[string]any{"member_uuid": member.UUID.String(), "user_uuid": member.UserUUID, "resource": "iam.member", "action": "read", "trace_id": "trace-1"}, directoryTestPlugin)
		require.Equal(t, http.StatusOK, allowed.Code)
		allowedData := directoryRouteBody(t, allowed)["data"].(map[string]any)
		require.Equal(t, true, allowedData["allowed"])
		require.Equal(t, "trace-1", allowedData["trace_id"])

		denied := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", map[string]any{"member_uuid": deniedMember.UUID.String(), "user_uuid": deniedMember.UserUUID, "resource": "iam.member", "action": "read"}, directoryTestPlugin)
		require.Equal(t, http.StatusOK, denied.Code)
		require.Equal(t, "IAM_PERMISSION_DENIED", directoryRouteBody(t, denied)["data"].(map[string]any)["reason_code"])

		mismatch := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", map[string]any{"member_uuid": member.UUID.String(), "user_uuid": deniedMember.UserUUID, "resource": "iam.member", "action": "read"}, directoryTestPlugin)
		require.Equal(t, http.StatusBadRequest, mismatch.Code)
		require.Equal(t, "IAM_SUBJECT_MISMATCH", directoryRouteBody(t, mismatch)["reason_code"])

		foreign := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", map[string]any{"member_uuid": foreignMember.UUID.String(), "user_uuid": foreignMember.UserUUID, "resource": "iam.member", "action": "read"}, directoryTestPlugin)
		require.Equal(t, http.StatusNotFound, foreign.Code)
		require.Equal(t, "IAM_MEMBER_NOT_FOUND", directoryRouteBody(t, foreign)["reason_code"])
	})
}

func directoryRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open("file:directory_routes_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &dbsetting.PluginInstanceConfig{}, &tenantmodel.Tenant{}, &modeliam.User{}, &modeliam.Member{}, &modeliam.MemberDepartment{}, &modeliam.Department{}, &modeliam.Role{}, &modeliam.Permission{}, &modeliam.RolePermission{}, &modeliam.RoleBinding{}, &modeliam.MemberAssignment{}))
	for _, model := range []struct {
		model  any
		column string
	}{
		{routePermissionUUIDColumns{}, "PermissionUUID"}, {routeDepartmentUUIDColumns{}, "DepartmentUUID"}, {routeDepartmentUUIDColumns{}, "ParentDepartmentUUID"}, {routeDepartmentUUIDColumns{}, "LeaderMemberUUID"}, {routeRolePermissionUUIDColumns{}, "RoleUUID"}, {routeRolePermissionUUIDColumns{}, "PermissionUUID"}, {routeMemberDepartmentUUIDColumns{}, "MemberUUID"}, {routeMemberDepartmentUUIDColumns{}, "DepartmentUUID"},
	} {
		if db.Migrator().HasColumn(model.model, model.column) {
			continue
		}
		require.NoError(t, db.Migrator().AddColumn(model.model, model.column))
	}
	for _, capabilityID := range []string{"com.corex.iam.members.read", "com.corex.iam.directory.read", "com.corex.iam.authorization.check"} {
		require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: capabilityID, PluginID: "com.powerx.core", PluginVersion: "v1", Title: "IAM directory", Status: "published"}).Error)
	}
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRegistration{}))
	for _, capabilityID := range []string{"com.corex.iam.members.read", "com.corex.iam.directory.read", "com.corex.iam.authorization.check"} {
		require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: capabilityID, TenantUUID: directoryTestTenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	return db
}

type routePermissionUUIDColumns struct {
	PermissionUUID string `gorm:"column:permission_uuid;type:uuid"`
}

func (routePermissionUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMPermission
}

type routeDepartmentUUIDColumns struct {
	DepartmentUUID       string `gorm:"column:department_uuid;type:uuid"`
	ParentDepartmentUUID string `gorm:"column:parent_department_uuid;type:uuid"`
	LeaderMemberUUID     string `gorm:"column:leader_member_uuid;type:uuid"`
}

func (routeDepartmentUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMDepartment
}

type routeRolePermissionUUIDColumns struct {
	RoleUUID       string `gorm:"column:role_uuid;type:uuid"`
	PermissionUUID string `gorm:"column:permission_uuid;type:uuid"`
}

func (routeRolePermissionUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMRolePermission
}

type routeMemberDepartmentUUIDColumns struct {
	MemberUUID     string `gorm:"column:member_uuid;type:uuid"`
	DepartmentUUID string `gorm:"column:department_uuid;type:uuid"`
}

func (routeMemberDepartmentUUIDColumns) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableIAMMemberDepartment
}

func directoryRouteTestRouter(db *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		pluginID := c.GetHeader("X-Test-Plugin")
		if pluginID != "" {
			claims := &reqctx.CoreXClaims{TenantUUID: directoryTestTenant, PluginID: pluginID, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: []string{"powerx:api"}}}
			ctx := reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), directoryTestTenant), claims)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})
	RegisterTenantRoutes(router.Group(""), &shared.Deps{DB: db})
	return router
}

func directoryRouteTestMember(t *testing.T, db *gorm.DB, tenantUUID, displayName string) *modeliam.Member {
	t.Helper()
	user := &modeliam.User{Email: uuid.NewString() + "@example.test", DisplayName: displayName, Status: 1}
	require.NoError(t, db.Create(user).Error)
	member := &modeliam.Member{TenantUUID: tenantUUID, UserUUID: user.UUID.String(), UserID: user.ID, Username: "member_" + uuid.NewString()[:8], DisplayName: displayName, Status: 1}
	require.NoError(t, db.Create(member).Error)
	return member
}

func directoryRouteGrant(t *testing.T, db *gorm.DB, tenantUUID, pluginID string, capabilities []string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"allowed_capabilities": capabilities})
	require.NoError(t, err)
	require.NoError(t, db.Create(&dbsetting.PluginInstanceConfig{TenantUUID: tenantUUID, PluginID: pluginID, Key: "auth.credentials", ValueJSON: datatypes.JSON(payload), Enabled: true}).Error)
}

func directoryRouteRequest(router *gin.Engine, method, path string, payload any, pluginID string) *httptest.ResponseRecorder {
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if pluginID != "" {
		req.Header.Set("X-Test-Plugin", pluginID)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func directoryRouteBody(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body
}
