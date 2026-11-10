package iam

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestPluginAuthorizationHTTPAndTypedEntry(t *testing.T) {
	db := directoryRouteTestDB(t)
	require.NoError(t, db.Create(&tenantmodel.Tenant{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(directoryTestTenant)}, Key: uuid.NewString(), Name: "test", Status: tenantmodel.TenantStatusActive}).Error)
	member := directoryRouteTestMember(t, db, directoryTestTenant, "Plugin member")
	other := directoryRouteTestMember(t, db, uuid.NewString(), "Other tenant")
	permission := model.Permission{Module: "messages", Resource: "conversation", Action: "reply", Effect: "allow", Status: model.PermissionStatusActive, Source: "plugin:" + directoryTestPlugin, Meta: datatypes.JSON(`{"type":"action"}`)}
	require.NoError(t, db.Create(&permission).Error)
	role := model.Role{TenantUUID: directoryTestTenant, Code: "message-agent", Name: "Message Agent"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&model.RolePermission{RoleID: role.ID, PermissionID: permission.ID, RoleUUID: role.UUID, PermissionUUID: permission.PermissionUUID}).Error)
	binding := model.RoleBinding{TenantUUID: directoryTestTenant, SubjectType: model.SubMember, SubjectID: member.ID, SubjectUUID: member.UUID.String(), RoleID: role.ID, RoleUUID: role.UUID.String()}
	require.NoError(t, db.Create(&binding).Error)
	credential := setting.PluginInstanceConfig{TenantUUID: directoryTestTenant, PluginID: directoryTestPlugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"client_id":"permission-test","allowed_capabilities":["com.corex.iam.authorization.check"]}`)}
	require.NoError(t, db.Create(&credential).Error)
	var ctxClaims *reqctx.CoreXClaims
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctxClaims = &reqctx.CoreXClaims{TenantUUID: directoryTestTenant, PluginID: c.GetHeader("X-Test-Plugin"), RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:permission-test", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
		c.Request = c.Request.WithContext(reqctx.WithSTSIssuanceStatus(reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), directoryTestTenant), ctxClaims), 0))
		c.Next()
	})
	RegisterTenantRoutes(router.Group(""), &shared.Deps{DB: db})
	payload := map[string]any{"member_uuid": member.UUID.String(), "user_uuid": member.UserUUID, "plugin_id": directoryTestPlugin, "permission_code": "messages.conversation:reply"}
	call := func() map[string]any {
		resp := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", payload, directoryTestPlugin)
		require.Equal(t, 200, resp.Code, resp.Body.String())
		return directoryRouteBody(t, resp)["data"].(map[string]any)
	}
	require.Equal(t, true, call()["allowed"])
	// 实时撤权，不依赖 token 内权限码。
	require.NoError(t, db.Delete(&binding).Error)
	require.Equal(t, false, call()["allowed"])
	require.Equal(t, "IAM_PERMISSION_DENIED", call()["reason_code"])
	for _, effect := range []string{"deny", "allow"} {
		require.NoError(t, db.Model(&permission).Update("effect", effect).Error)
		require.Equal(t, false, call()["allowed"])
	}
	payload["member_uuid"], payload["user_uuid"] = other.UUID.String(), other.UserUUID
	resp := directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", payload, directoryTestPlugin)
	require.Equal(t, 404, resp.Code)
	payload["member_uuid"], payload["user_uuid"] = member.UUID.String(), member.UserUUID
	payload["plugin_id"] = "com.example.foreign"
	resp = directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", payload, directoryTestPlugin)
	require.Equal(t, 404, resp.Code)
	payload["plugin_id"] = directoryTestPlugin
	// 另一插件有全局查询 grant，也不能枚举其他插件权限。
	foreignCred := credential
	foreignCred.ID = 0
	foreignCred.PluginID = "com.example.unrelated"
	require.NoError(t, db.Create(&foreignCred).Error)
	resp = directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", payload, foreignCred.PluginID)
	require.Equal(t, 403, resp.Code)
	// typed core_internal 同样拒绝用户 JWT 与自由代理参数。
	invoker := iamsvc.AuthorizationInvoker{Service: iamsvc.NewTenantDirectoryService(db)}
	input := cap.CoreCapabilityInvokeInput{CapabilityID: "com.corex.iam.authorization.check", TenantUUID: directoryTestTenant, Method: "INVOKE", Endpoint: iamsvc.AuthorizationCheckEndpoint, Body: map[string]any{"operation": "check", "request": payload}, Payload: map[string]any{"body": map[string]any{}}}
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(t.Context(), directoryTestTenant), &reqctx.CoreXClaims{TenantUUID: directoryTestTenant, IsRoot: true})
	_, err := invoker.InvokeCoreCapability(ctx, input)
	require.Equal(t, 401, dto.StatusCode(err))
	ctx = reqctx.WithClaims(ctx, ctxClaims)
	input.Payload["endpoint"] = "https://example.invalid"
	_, err = invoker.InvokeCoreCapability(ctx, input)
	require.Equal(t, 400, dto.StatusCode(err))
	delete(input.Payload, "endpoint")
	input.Body["request"] = payload
	// 查询本身的依赖故障必须返回 503，不能误报 allowed=false。
	require.NoError(t, db.Migrator().DropTable(&model.Permission{}))
	resp = directoryRouteRequest(router, http.MethodPost, "/tenant/iam/authorization:check", payload, directoryTestPlugin)
	require.Equal(t, 503, resp.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Equal(t, "IAM_UPSTREAM_DEPENDENCY", body["reason_code"])
}
