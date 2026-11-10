package iam

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	svc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTenantAdminMemberAccountLifecycleAndIsolation(t *testing.T) {
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&m.User{}, &m.Member{}, &m.Credential{}, &m.MemberDepartment{}, &m.Role{}, &m.RoleBinding{}, &m.RolePermission{}, &m.Permission{}, &m.MemberAssignment{}))
	tenant, otherTenant := uuid.NewString(), uuid.NewString()
	admin := m.User{Email: "admin@example.invalid", DisplayName: "Admin", Status: 1}
	require.NoError(t, db.Create(&admin).Error)
	actor := m.Member{TenantUUID: tenant, UserID: admin.ID, UserUUID: admin.UUID.String(), Username: "admin", Status: 1}
	require.NoError(t, db.Create(&actor).Error)
	adminRole := m.Role{Scope: "tenant", TenantUUID: tenant, Code: "role_admin", Name: "Tenant Admin"}
	userRole := m.Role{Scope: "tenant", TenantUUID: tenant, Code: "role_user", Name: "Tenant User"}
	foreignRole := m.Role{Scope: "tenant", TenantUUID: otherTenant, Code: "role_user", Name: "Other User"}
	for _, role := range []*m.Role{&adminRole, &userRole, &foreignRole} {
		require.NoError(t, db.Create(role).Error)
	}
	require.NoError(t, db.Create(&m.RoleBinding{TenantUUID: tenant, RoleUUID: adminRole.UUID.String(), RoleID: adminRole.ID, SubjectType: m.SubMember, SubjectUUID: actor.UUID.String(), SubjectID: actor.ID, DataScope: m.ScopeTenant}).Error)
	var writePermission m.Permission
	for _, action := range []string{"read", "write", "delete", "bind"} {
		resource := "member_accounts"
		if action == "bind" {
			resource = "role"
		}
		permission := m.Permission{Module: "iam", Resource: resource, Action: action, Source: "platform_capability", Status: m.PermissionStatusActive, Effect: "allow"}
		require.NoError(t, db.Create(&permission).Error)
		require.NoError(t, db.Create(&m.RolePermission{RoleUUID: adminRole.UUID, RoleID: adminRole.ID, PermissionID: permission.ID}).Error)
		if action == "write" {
			writePermission = permission
		}
	}
	secret := []byte("isolated-member-http-test-key")
	claims := reqctx.CoreXClaims{TenantUUID: tenant, UserID: admin.ID, UserUUID: admin.UUID.String(), MemberID: actor.ID, MemberUUID: actor.UUID.String(), Roles: []string{"role_admin"}, RegisteredClaims: jwt.RegisteredClaims{Issuer: "member-test", Audience: jwt.ClaimStrings{"user"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(middleware.JwtMiddleware(secret, "member-test", []string{"user"}, nil, nil))
	registerMemberRoutes(api, NewMemberHandler(svc.NewMemberService(db)))
	root := api.Group("/admin/system")
	root.Use(middleware.JwtMiddleware(secret, "member-test", []string{"user"}, nil, reqctx.RootOnlyCB()))
	root.POST("/users", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func(method, path string, body any, expected int) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, expected, response.Code, "%s %s: %s", method, path, response.Body.String())
		var result map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		return result
	}
	create := func(username, email, role string) map[string]any {
		return map[string]any{"member": map[string]any{"username": username, "display_name": username, "tenant_uuid": otherTenant}, "user": map[string]any{"email": email, "is_root": true}, "initial_password": "Fixture-secret-123", "role_uuids": []string{role}}
	}
	// 有效的非 Root Admin JWT 经正式成员路由成功创建；租户及 Root 字段不能由 body 覆盖。
	request("POST", "/admin/iam/members", create("newperson", "newperson@example.invalid", userRole.UUID.String()), 200)
	var member m.Member
	require.NoError(t, db.Where("tenant_uuid = ? AND username = ?", tenant, "newperson").First(&member).Error)
	var user m.User
	require.NoError(t, db.First(&user, member.UserID).Error)
	require.False(t, user.IsRoot)
	var credential m.Credential
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&credential).Error)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(credential.SecretHash), []byte("Fixture-secret-123")))
	path := "/admin/iam/members/" + member.UUID.String()
	request("PATCH", path, map[string]any{"user": map[string]any{"status": 2}}, 403)
	roles := request("GET", path+"/roles", nil, 200)
	require.Equal(t, []any{userRole.UUID.String()}, roles["data"].(map[string]any)["role_uuids"])
	listed := request("GET", "/admin/iam/members?page=1&page_size=1&q=newperson", nil, 200)
	data := listed["data"].(map[string]any)
	require.Len(t, data["items"], 1)
	require.EqualValues(t, 1, data["pagination"].(map[string]any)["total"])
	request("PATCH", path, map[string]any{"member": map[string]any{"username": "renamed", "display_name": "Renamed"}, "contact": map[string]any{"email": "renamed@example.invalid", "phone": ""}, "role_uuids": []string{adminRole.UUID.String()}}, 200)
	request("PATCH", path, map[string]any{"member": map[string]any{"display_name": "Must rollback"}, "contact": map[string]any{"email": "must-rollback@example.invalid"}, "role_uuids": []string{foreignRole.UUID.String()}}, 400)
	require.NoError(t, db.First(&member, member.ID).Error)
	require.Equal(t, "Renamed", member.DisplayName)
	require.NoError(t, db.First(&credential, credential.ID).Error)
	require.Equal(t, "renamed@example.invalid", credential.Identifier)
	roles = request("GET", path+"/roles", nil, 200)
	require.Equal(t, []any{adminRole.UUID.String()}, roles["data"].(map[string]any)["role_uuids"])
	request("PUT", path+"/status", map[string]any{"status": 2}, 200)
	require.NoError(t, db.First(&user, user.ID).Error)
	require.EqualValues(t, 1, user.Status, "租户停用不能全局停用登录账号")
	request("PUT", path+"/password", map[string]any{"new_password": "Fixture-new-secret-123"}, 200)
	require.NoError(t, db.First(&credential, credential.ID).Error)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(credential.SecretHash), []byte("Fixture-new-secret-123")))
	request("POST", "/admin/iam/members", create("rollback", "rollback@example.invalid", foreignRole.UUID.String()), 400)
	var count int64
	require.NoError(t, db.Model(&m.User{}).Where("email = ?", "rollback@example.invalid").Count(&count).Error)
	require.Zero(t, count, "无效角色必须回滚用户、成员和密码")
	foreign := m.Member{TenantUUID: otherTenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "foreign", Status: 1}
	require.NoError(t, db.Create(&foreign).Error)
	for _, suffix := range []string{"", "/roles", "/password", "/status"} {
		method, body := "PATCH", map[string]any{"member": map[string]any{"display_name": "intruder"}}
		if suffix != "" {
			method, body = "PUT", map[string]any{"role_uuids": []string{userRole.UUID.String()}, "new_password": "No-overwrite-123", "status": 2}
		}
		request(method, "/admin/iam/members/"+foreign.UUID.String()+suffix, body, 404)
	}
	rootUser := m.User{Email: "root@example.invalid", IsRoot: true, Status: 1}
	require.NoError(t, db.Create(&rootUser).Error)
	rootMember := m.Member{TenantUUID: tenant, UserID: rootUser.ID, UserUUID: rootUser.UUID.String(), Username: "root", Status: 1}
	require.NoError(t, db.Create(&rootMember).Error)
	request("PUT", "/admin/iam/members/"+rootMember.UUID.String()+"/password", map[string]any{"new_password": "No-overwrite-123"}, 403)
	request("POST", "/admin/iam/members", create("root-link", "root@example.invalid", userRole.UUID.String()), 403)
	request("DELETE", "/admin/iam/members/"+foreign.UUID.String(), nil, 404)
	request("POST", "/admin/system/users", create("blocked", "blocked@example.invalid", userRole.UUID.String()), 403)
	request("DELETE", path, nil, 200)
	require.NoError(t, db.First(&user, user.ID).Error)
	require.NoError(t, db.First(&foreign, foreign.ID).Error)
	request("PUT", path+"/restore", nil, 200)
	// user:write 足以创建默认普通用户；其他角色与已有角色变更需 role:bind。
	var bindPermission m.Permission
	require.NoError(t, db.Where("module = ? AND resource = ? AND action = ?", "iam", "role", "bind").First(&bindPermission).Error)
	require.NoError(t, db.Where("permission_id = ?", bindPermission.ID).Delete(&m.RolePermission{}).Error)
	request("POST", "/admin/iam/members", create("ordinary", "ordinary@example.invalid", userRole.UUID.String()), 200)
	request("POST", "/admin/iam/members", create("elevated", "elevated@example.invalid", adminRole.UUID.String()), 403)
	request("PUT", path+"/roles", map[string]any{"role_uuids": []string{adminRole.UUID.String()}}, 403)
	// 同一 JWT 的写权限撤销即时生效，角色名称本身不绕过 RBAC。
	require.NoError(t, db.Where("permission_id = ?", writePermission.ID).Delete(&m.RolePermission{}).Error)
	request("POST", "/admin/iam/members", create("denied", "denied@example.invalid", userRole.UUID.String()), 403)
}
