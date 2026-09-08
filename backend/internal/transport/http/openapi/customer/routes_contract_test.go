package customer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	pxauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const membershipTenant = "11111111-1111-1111-1111-111111111111"
const membershipCustomer = "22222222-2222-2222-2222-222222222222"

func TestCustomerMembershipContractRequiresBothCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := membershipTestDB(t)
	seedMembershipAuthorization(t, db, true)
	require.NoError(t, db.Create(&modelcustomer.Account{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(membershipCustomer)}, Status: modelcustomer.StatusActive}).Error)
	require.NoError(t, db.Create(&modelcustomer.TenantMembership{TenantUUID: membershipTenant, CustomerUUID: membershipCustomer, Status: modelcustomer.StatusActive, Roles: datatypes.JSON([]byte(`["customer"]`)), Scopes: datatypes.JSON([]byte(`[]`))}).Error)
	tokenSvc := customersvc.NewCustomerTokenService(db, []byte("customer-secret"), "powerx-auth", time.Minute)
	h := &handler{memberships: customersvc.NewMembershipService(db), tokens: tokenSvc}
	claims := &reqctx.CoreXClaims{TenantUUID: membershipTenant, PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}}
	membership, err := h.memberships.ResolveCurrent(context.Background(), membershipTenant, membershipCustomer)
	require.NoError(t, err)
	customerToken, err := tokenSvc.Issue(context.Background(), membership)
	require.NoError(t, err)

	for _, tt := range []struct {
		name, header string
		status       int
		reason       string
	}{
		{"success", "Bearer " + customerToken, http.StatusOK, ""},
		{"missing customer token", "", http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/api/v1/tenant/customer/memberships:resolve", func(c *gin.Context) {
				c.Request = c.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), membershipTenant), claims))
				h.resolve(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/customer/memberships:resolve", nil)
			if tt.header != "" {
				req.Header.Set(customerAuthorizationHeader, tt.header)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			require.Equal(t, tt.status, rec.Code)
			if tt.reason != "" {
				require.Contains(t, rec.Body.String(), tt.reason)
			}
		})
	}
}

func TestCustomerMembershipContractRejectsSTSWithoutCapabilityGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := membershipTestDB(t)
	seedMembershipAuthorization(t, db, false)
	h := &handler{memberships: customersvc.NewMembershipService(db), tokens: customersvc.NewCustomerTokenService(db, []byte("customer-secret"), "powerx-auth", time.Minute)}
	claims := &reqctx.CoreXClaims{TenantUUID: membershipTenant, PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}}
	token, err := pxauth.GenerateCustomerAccessJWT(membershipTenant, membershipCustomer, "powerx-auth", time.Minute, []byte("customer-secret"))
	require.NoError(t, err)
	engine := gin.New()
	engine.POST("/api/v1/tenant/customer/memberships:resolve", func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), membershipTenant), claims))
		h.resolve(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/customer/memberships:resolve", nil)
	req.Header.Set(customerAuthorizationHeader, "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "CUSTOMER_FORBIDDEN")
}

func membershipTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}, &settingmodel.PluginInstanceConfig{}))
	for _, statement := range []string{
		`CREATE TABLE main.customer_accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, status TEXT NOT NULL, primary_email TEXT, primary_phone TEXT, display_name TEXT, nickname TEXT, given_name TEXT, family_name TEXT, avatar_url TEXT, locale TEXT, timezone TEXT, metadata TEXT)`,
		`CREATE TABLE main.customer_tenant_memberships (id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, tenant_uuid TEXT NOT NULL, customer_uuid TEXT NOT NULL, status TEXT NOT NULL, roles TEXT, scopes TEXT, source TEXT NOT NULL, expires_at DATETIME, metadata TEXT, UNIQUE(tenant_uuid, customer_uuid))`,
		`CREATE TABLE main.customer_sessions (id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT UNIQUE, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, customer_uuid TEXT NOT NULL, tenant_uuid TEXT, membership_uuid TEXT, session_family_uuid TEXT NOT NULL, access_token_jti TEXT UNIQUE, refresh_token_hash TEXT, source TEXT NOT NULL, issued_at DATETIME NOT NULL, expires_at DATETIME NOT NULL, revoked_at DATETIME, metadata TEXT)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	return db
}
func seedMembershipAuthorization(t *testing.T, db *gorm.DB, granted bool) {
	t.Helper()
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: customersvc.CustomerMembershipsDelegatedReadCapabilityID, PluginID: "com.corex", PluginVersion: "1", Title: "membership", CapabilitiesHash: "membership", ProtocolHash: "membership", Status: "published"}).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: customersvc.CustomerMembershipsDelegatedReadCapabilityID, TenantUUID: membershipTenant, ContractRef: "test", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	allowed := `[]`
	if granted {
		allowed = `["com.corex.customer.memberships.delegated_read"]`
	}
	require.NoError(t, db.Create(&settingmodel.PluginInstanceConfig{TenantUUID: membershipTenant, PluginID: "com.powerx.plugins.scrm", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON([]byte(`{"allowed_capabilities":` + allowed + `}`))}).Error)
}
