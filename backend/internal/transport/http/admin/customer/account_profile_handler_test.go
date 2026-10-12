package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCustomerProfileOpenAPI(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	loader := openapi3.NewLoader()
	contract, err := loader.LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../docs/contracts/customer-admin-profile.openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, contract.Validate(context.Background()))
	require.NotNil(t, contract.Paths.Find("/admin/customers/accounts/{customer_uuid}").Patch)
}

func TestCustomerProfileRejectsAuthorityAndUnsupportedFields(t *testing.T) {
	for _, field := range []string{"tenant_uuid", "actor", "password", "type", "status", "primary_contact_uuid", "metadata", "given_name"} {
		router := gin.New()
		router.PATCH("/test", func(c *gin.Context) {
			var request updateAccountProfileRequest
			if bindCustomerAdminJSON(c, &request, 4096, "CUSTOMER_PROFILE_REQUEST_INVALID") {
				c.Status(200)
			}
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("PATCH", "/test", strings.NewReader(`{"`+field+`":"override"}`)))
		require.Equal(t, 400, response.Code, field)
	}
}

// 独立 schema 中验收真实 Handler/权限/事务；清理只删除本测试创建的 schema。
func TestCustomerProfileHTTPPostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_CUSTOMER_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("set POWERX_CUSTOMER_ADMIN_TEST_DSN for native PostgreSQL profile acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	beforeSchema := model.PowerXSchema
	schema := "customer_profile_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		model.PowerXSchema = beforeSchema
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&cm.Account{}, &cm.TenantMembership{}, &cm.Contact{}, &cm.AuthIdentity{}, &audit.AuditEvent{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}))
	tenant, member := uuid.NewString(), uuid.NewString()
	root := reqctx.WithTraceID(reqctx.WithUserID(reqctx.WithIsRoot(reqctx.WithTenantUUID(context.Background(), tenant), true), 42), "profile-http-trace")
	service := customersvc.NewAccountService(db)
	created, err := service.Create(root, customersvc.CreateAccountInput{TenantUUID: tenant, Type: "person", DisplayName: "Original", PrimaryEmail: "original@example.test"})
	require.NoError(t, err)
	var contactBefore cm.Contact
	require.NoError(t, db.Where("uuid = ?", created.PrimaryContactUUID).First(&contactBefore).Error)
	identity := cm.AuthIdentity{CustomerUUID: created.UUID, Provider: "email", ProviderSubject: "login-original", Email: "login@example.test", PasswordHash: "test-hash-preserved", Status: "active"}
	require.NoError(t, db.Create(&identity).Error)
	role := im.Role{TenantUUID: tenant, Scope: "tenant", Code: "role_user", Name: "User"}
	require.NoError(t, db.Create(&role).Error)
	binding := im.RoleBinding{TenantUUID: tenant, RoleUUID: role.UUID.String(), RoleID: role.ID, SubjectType: im.SubMember, SubjectUUID: member, SubjectID: 7}
	require.NoError(t, db.Create(&binding).Error)
	permission := im.Permission{Module: "customer", Resource: "admin_customers_accounts_customer_uuid", Action: "update", Source: "platform_capability", Status: "active", Effect: "allow"}
	require.NoError(t, db.Create(&permission).Error)
	h := &Handler{svc: service}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := root
		if c.GetHeader("X-Test-Actor") == "member" {
			ctx = reqctx.WithMemberUUID(reqctx.WithMemberID(reqctx.WithIsRoot(ctx, false), 7), member)
		}
		if v := c.GetHeader("X-Test-Tenant"); v != "" {
			ctx = reqctx.WithTenantUUID(ctx, v)
		}
		if v := c.GetHeader("X-Test-Plane"); v != "" {
			claims := &reqctx.CoreXClaims{}
			if v == "sts" {
				claims.Issuer = "powerx-sts"
			} else {
				claims.Platforms = []string{"api_key"}
			}
			ctx = reqctx.WithClaims(ctx, claims)
		}
		c.Request = c.Request.WithContext(context.WithValue(ctx, "request_id", "profile-http-request"))
	})
	router.PATCH("/accounts/:customer_uuid", h.UpdateAccountProfile)
	router.GET("/accounts/:customer_uuid", h.GetAccount)
	call := func(method, suffix string, value any, headers map[string]string) (int, []byte) {
		body, _ := json.Marshal(value)
		request := httptest.NewRequest(method, "/accounts/"+created.UUID+suffix, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code, response.Body.Bytes()
	}
	read := func() repo.AccountRow {
		result, err := service.Get(root, tenant, created.UUID)
		require.NoError(t, err)
		return result
	}
	patch := func(name string) map[string]any {
		return map[string]any{"expected_profile_version": read().ProfileVersion, "display_name": name}
	}
	status, _ := call("PATCH", "", patch("Denied"), map[string]string{"X-Test-Actor": "member"})
	require.Equal(t, 403, status)
	for _, triple := range [][3]string{{"menu", "customer", "view"}, {"customer", "admin_customers_accounts", "create"}} {
		p := im.Permission{Module: triple[0], Resource: triple[1], Action: triple[2], Source: "platform_capability"}
		require.NoError(t, db.Create(&p).Error)
		require.NoError(t, db.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: p.ID, PermissionUUID: p.PermissionUUID}).Error)
	}
	status, _ = call("PATCH", "", patch("Still denied"), map[string]string{"X-Test-Actor": "member"})
	require.Equal(t, 403, status)
	grant := im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID}
	require.NoError(t, db.Create(&grant).Error)
	for _, plane := range []string{"sts", "api_key"} {
		status, _ = call("PATCH", "", patch("Denied plane"), map[string]string{"X-Test-Plane": plane})
		require.Equal(t, 403, status)
	}
	status, _ = call("PATCH", "", patch("Wrong tenant"), map[string]string{"X-Test-Tenant": uuid.NewString()})
	require.Equal(t, 404, status)
	status, _ = call("PATCH", "?tenant_uuid=override", patch("Query"), nil)
	require.Equal(t, 400, status)
	for _, p := range []map[string]any{{"display_name": "No version"}, {"expected_profile_version": read().ProfileVersion, "display_name": ""}, {"expected_profile_version": read().ProfileVersion, "primary_email": "invalid"}, {"expected_profile_version": read().ProfileVersion, "primary_phone": "+-"}, {"expected_profile_version": read().ProfileVersion, "status": "disabled"}} {
		status, _ = call("PATCH", "", p, nil)
		require.Equal(t, 400, status)
	}
	old := read().ProfileVersion
	status, body := call("PATCH", "", map[string]any{"expected_profile_version": old, "display_name": "Updated", "nickname": "Nick", "primary_email": "updated@example.test", "primary_phone": "+86 13800000000"}, map[string]string{"X-Test-Actor": "member"})
	require.Equal(t, 200, status, string(body))
	require.Equal(t, "Updated", read().DisplayName)
	require.NotEqual(t, old, read().ProfileVersion)
	var contactAfter cm.Contact
	require.NoError(t, db.First(&contactAfter, contactBefore.ID).Error)
	require.Equal(t, contactBefore, contactAfter)
	var identityAfter cm.AuthIdentity
	require.NoError(t, db.First(&identityAfter, identity.ID).Error)
	require.Equal(t, identity, identityAfter)
	status, _ = call("PATCH", "", map[string]any{"expected_profile_version": old, "display_name": "Stale"}, nil)
	require.Equal(t, 409, status)
	require.Equal(t, "Updated", read().DisplayName)
	for _, change := range []map[string]any{{"effect": "deny"}, {"effect": "allow", "status": "deprecated"}} {
		require.NoError(t, db.Model(&permission).Updates(change).Error)
		status, _ = call("PATCH", "", patch("Revoked"), map[string]string{"X-Test-Actor": "member"})
		require.Equal(t, 403, status)
	}
	require.NoError(t, db.Model(&permission).Updates(map[string]any{"effect": "allow", "status": "active"}).Error)
	end := time.Now().Add(-time.Minute).UnixMilli()
	require.NoError(t, db.Model(&binding).Update("end_at", end).Error)
	status, _ = call("PATCH", "", patch("Expired"), map[string]string{"X-Test-Actor": "member"})
	require.Equal(t, 403, status)
	require.NoError(t, db.Model(&binding).Update("end_at", nil).Error)
	require.NoError(t, db.Delete(&grant).Error)
	status, _ = call("PATCH", "", patch("Removed grant"), map[string]string{"X-Test-Actor": "member"})
	require.Equal(t, 403, status)
	version := read().ProfileVersion
	statuses := [2]int{}
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], _ = call("PATCH", "", map[string]any{"expected_profile_version": version, "display_name": []string{"Writer A", "Writer B"}[i]}, nil)
		}(i)
	}
	wg.Wait()
	require.ElementsMatch(t, []int{200, 409}, statuses[:])
	require.NoError(t, db.Exec(`CREATE FUNCTION "`+schema+`".reject_profile_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected profile audit failure'; END $$`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_profile_audit BEFORE INSERT ON "`+schema+`".audit_event FOR EACH ROW EXECUTE FUNCTION "`+schema+`".reject_profile_audit()`).Error)
	before := read()
	status, _ = call("PATCH", "", patch("Must roll back"), nil)
	require.Equal(t, 503, status)
	require.Equal(t, before, read())
	require.NoError(t, db.Exec(`DROP TRIGGER reject_profile_audit ON "`+schema+`".audit_event`).Error)
	status, _ = call("PATCH", "", map[string]any{"expected_profile_version": read().ProfileVersion, "nickname": "", "primary_email": "", "primary_phone": ""}, nil)
	require.Equal(t, 200, status)
	require.Empty(t, read().PrimaryEmail)
	require.Empty(t, read().Nickname)
	var events []audit.AuditEvent
	require.NoError(t, db.Where("operation = ?", "customer.account.profile_updated").Find(&events).Error)
	require.Len(t, events, 3)
	for _, event := range events {
		require.Equal(t, created.UUID, event.ResourceID)
		require.Equal(t, "profile-http-trace", event.CorrelationID)
		require.Equal(t, int64(42), *event.ActorUserID)
		require.Contains(t, string(event.ChangesAfter), "profile-http-request")
		require.NotContains(t, string(event.ChangesAfter), "updated@example.test")
	}
	// 可选真实浏览器验收，提供本测试的隔离 HTTP 夹具；结束文件触发正常清理。
	if out := os.Getenv("POWERX_CUSTOMER_PROFILE_BROWSER_OUT"); out != "" {
		require.NoError(t, db.Create(&grant).Error)
		router.GET("/accounts", h.ListAccounts)
		router.GET("/iam/me/check", func(c *gin.Context) {
			allowed := false
			if c.Query("permission") == "customer.admin_customers_accounts_customer_uuid:update" {
				ctx := c.Request.Context()
				if reqctx.IsRoot(ctx) {
					allowed = true
				} else {
					allowed, _ = iamsvc.NewRBACService(db).Enforce(ctx, iamsvc.ActorContext{TenantUUID: reqctx.GetTenantUUID(ctx)}, reqctx.GetTenantUUID(ctx), reqctx.GetMemberID(ctx), "customer", "admin_customers_accounts_customer_uuid", "update")
				}
			}
			c.JSON(200, gin.H{"code": 200, "data": gin.H{"ok": allowed, "has_permission": allowed}})
		})
		router.POST("/__test/grant", func(c *gin.Context) {
			var request struct {
				Enabled bool `json:"enabled"`
			}
			require.NoError(t, c.ShouldBindJSON(&request))
			if request.Enabled {
				require.NoError(t, db.Create(&grant).Error)
			} else {
				require.NoError(t, db.Delete(&grant).Error)
			}
			c.Status(204)
		})
		server := httptest.NewServer(router)
		t.Cleanup(server.Close)
		metadata, _ := json.Marshal(map[string]any{"url": server.URL, "pid": os.Getpid(), "schema": schema, "customer_uuid": created.UUID})
		require.NoError(t, os.WriteFile(out, metadata, 0600))
		t.Cleanup(func() { os.Remove(out); os.Remove(out + ".done") })
		deadline := time.Now().Add(3 * time.Minute)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(out + ".done"); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("browser acceptance timed out")
	}
}
