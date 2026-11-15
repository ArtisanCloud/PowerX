package iam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestIAM434SignedSessionCannotEscalateThroughAnyRoleRoute(t *testing.T) {
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&m.User{}, &m.Member{}, &m.Role{}, &m.Permission{}, &m.RolePermission{}, &m.RoleBinding{}, &m.MemberAssignment{}))
	tenant := uuid.NewString()
	user := m.User{Email: "ordinary@example.invalid", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	member := m.Member{TenantUUID: tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "ordinary", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	role := m.Role{Scope: "tenant", TenantUUID: tenant, Code: "role_user", Name: "Tenant User"}
	require.NoError(t, db.Create(&role).Error)
	binding := m.RoleBinding{TenantUUID: tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: m.SubMember, SubjectID: member.ID, SubjectUUID: member.UUID.String()}
	require.NoError(t, db.Create(&binding).Error)
	backup := m.Permission{Module: "platform_ops", Resource: "backup", Action: "execute", Source: "core", Status: "active", Effect: "allow"}
	require.NoError(t, db.Create(&backup).Error)
	secret := []byte("isolated-iam434-http-session-key")
	claims := reqctx.CoreXClaims{Scope: "access", UserID: user.ID, UserUUID: user.UUID.String(), TenantUUID: tenant, MemberID: member.ID, MemberUUID: member.UUID.String(), Roles: []string{"role_admin", "system_admin"}, RegisteredClaims: jwt.RegisteredClaims{Issuer: "iam434-test", Audience: jwt.ClaimStrings{"user"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)
	rootClaims := claims
	rootClaims.IsRoot = true
	rootToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, rootClaims).SignedString(secret)
	require.NoError(t, err)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(middleware.JwtMiddleware(secret, "iam434-test", []string{"user"}, []string{"access"}, nil))
	deps := &shared.Deps{DB: db}
	registerRoleRoutes(api, NewRoleHandler(deps), NewRBACHandler(deps))
	api.GET("/iam434-protected", func(c *gin.Context) {
		ctx := c.Request.Context()
		allowed, err := svc.NewRBACService(db).Enforce(ctx, svc.ActorContext{TenantUUID: tenant}, tenant, reqctx.GetMemberID(ctx), "platform_ops", "backup", "execute")
		if err != nil {
			c.Status(500)
			return
		}
		if !allowed {
			c.Status(403)
			return
		}
		c.Status(200)
	})
	request := func(method, path string, body any, bearer string, expected int) {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, expected, response.Code, "%s %s: %s", method, path, response.Body.String())
	}
	base := fmt.Sprintf("/admin/iam/roles/%d", role.ID)
	permissionIDs := map[string]any{"perm_ids": []uint64{backup.ID}}
	request("GET", "/iam434-protected", nil, token, 403)
	cases := []struct {
		method, path string
		body         any
	}{
		{"GET", "/admin/iam/roles", nil},
		{"GET", base, nil},
		{"POST", "/admin/iam/roles", map[string]any{"scope": "tenant", "code": "injected", "name": "Injected", "perm_ids": []uint64{backup.ID}}},
		{"PATCH", base, map[string]any{"name": "Hijacked"}},
		{"DELETE", base, nil},
		{"POST", base + "/permissions/grant-ids", permissionIDs},
		{"POST", base + "/permissions/grant", map[string]any{"items": []any{map[string]any{"module": "iam", "resource": "injected", "action": "write"}}}},
		{"POST", base + "/permissions/revoke-ids", permissionIDs},
		{"GET", base + "/permissions", nil},
		{"GET", base + "/permissions/ids", nil},
		{"PUT", base + "/permissions/set-ids", map[string]any{"ids": []uint64{backup.ID}}},
		{"POST", base + "/bind/member", map[string]any{"member_id": member.ID}},
		{"DELETE", fmt.Sprintf("%s/bindings/%d", base, binding.ID), nil},
	}
	for _, tc := range cases {
		request(tc.method, tc.path, tc.body, token, 403)
	}
	request("POST", base+"/permissions/grant-ids", permissionIDs, "", 401)
	request("GET", "/iam434-protected", nil, token, 403)
	var count int64
	require.NoError(t, db.Model(&m.RolePermission{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&m.Role{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&m.Permission{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&m.RoleBinding{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	// Root retains access without a tenant management grant.
	request("GET", base, nil, rootToken, 200)
	// A custom delegated manager is authorized by live permissions, not JWT roles.
	permission := m.Permission{Module: "iam", Resource: "role_management", Action: "write", Source: "platform_capability", Status: "active", Effect: "allow"}
	require.NoError(t, db.Create(&permission).Error)
	grant := m.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID}
	require.NoError(t, db.Create(&grant).Error)
	request("POST", base+"/permissions/grant-ids", permissionIDs, token, 200)
	request("GET", "/iam434-protected", nil, token, 200)
	request("POST", base+"/bind/member", map[string]any{"member_id": member.ID}, token, 403)
	require.NoError(t, db.Where("permission_uuid = ?", permission.PermissionUUID).Delete(&m.RolePermission{}).Error)
	request("POST", base+"/permissions/grant-ids", permissionIDs, token, 403)
}
