package customer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
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
)

func TestMiniAppCredentialMatchesPlatform(t *testing.T) {
	require.True(t, isMiniAppCredential("wechat_miniapp", "wechat_miniapp_code"))
	require.True(t, isMiniAppCredential("douyin_miniapp", "douyin_miniapp_code"))
	require.False(t, isMiniAppCredential("wechat_miniapp", "alipay_miniapp_code"))
	require.False(t, isMiniAppCredential("shopify_storefront", "shopify_customer_access_token"))
}

func TestPhoneVerifyRejectsInactiveCustomerWithValidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, state := range []string{"membership_suspended", "account_suspended", "membership_expired"} {
		t.Run(state, func(t *testing.T) {
			db := membershipTestDB(t)
			seedMembershipAuthorization(t, db, true)
			capability := customersvc.CustomerPhoneVerifyCapabilityID
			require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: capability, PluginID: "com.corex", PluginVersion: "1", Title: "phone", CapabilitiesHash: "phone", ProtocolHash: "phone", Status: "published"}).Error)
			require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: capability, TenantUUID: membershipTenant, ContractRef: "test", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
			require.NoError(t, db.Model(&settingmodel.PluginInstanceConfig{}).Where("tenant_uuid = ?", membershipTenant).Update("value_json", datatypes.JSON(`{"allowed_capabilities":["com.corex.customer.phone.verify"]}`)).Error)
			require.NoError(t, db.Create(&modelcustomer.Account{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(membershipCustomer)}, Status: modelcustomer.StatusActive}).Error)
			require.NoError(t, db.Create(&modelcustomer.TenantMembership{TenantUUID: membershipTenant, CustomerUUID: membershipCustomer, Status: modelcustomer.StatusActive, Roles: datatypes.JSON(`["customer"]`), Scopes: datatypes.JSON(`[]`)}).Error)
			tokens := customersvc.NewCustomerTokenService(db, []byte("customer-secret"), "powerx-auth", time.Minute)
			memberships := customersvc.NewMembershipService(db)
			m, err := memberships.ResolveCurrent(context.Background(), membershipTenant, membershipCustomer)
			require.NoError(t, err)
			pair, err := tokens.IssuePair(context.Background(), m)
			require.NoError(t, err)
			switch state {
			case "membership_suspended":
				require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("customer_uuid = ?", membershipCustomer).Update("status", "suspended").Error)
			case "account_suspended":
				require.NoError(t, db.Model(&modelcustomer.Account{}).Where("uuid = ?", membershipCustomer).Update("status", "suspended").Error)
			case "membership_expired":
				require.NoError(t, db.Model(&modelcustomer.TenantMembership{}).Where("customer_uuid = ?", membershipCustomer).Update("expires_at", time.Now().Add(-time.Minute)).Error)
			}
			h := &handler{memberships: memberships, tokens: tokens, auth: customersvc.NewCustomerAuthService(db, tokens)}
			claims := &reqctx.CoreXClaims{TenantUUID: membershipTenant, PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}}
			engine := gin.New()
			engine.POST("/api/v1/tenant/customer/phone:verify", func(c *gin.Context) {
				c.Request = c.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), membershipTenant), claims))
				h.verifyPhone(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/customer/phone:verify", strings.NewReader(`{"channel":"wechat_miniapp","app_id":"wx-test","credential":{"type":"wechat_miniapp_phone_code","value":"code"}}`))
			req.Header.Set(customerAuthorizationHeader, "Bearer "+pair.AccessToken)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.Contains(t, w.Body.String(), "CUSTOMER_MEMBERSHIP_INACTIVE")
		})
	}
}

func TestMiniAppErrorsHaveStableReasonCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		err    error
		status int
		reason string
	}{
		{customersvc.ErrCustomerClientAppNotFound, http.StatusBadRequest, "CUSTOMER_CLIENT_APP_NOT_FOUND"},
		{customersvc.ErrCustomerClientAppForbidden, http.StatusForbidden, "CUSTOMER_CLIENT_APP_FORBIDDEN"},
		{customersvc.ErrCustomerClientPlatformUnsupported, http.StatusBadRequest, "CUSTOMER_CLIENT_PLATFORM_UNSUPPORTED"},
		{customersvc.ErrCustomerClientAppNotConfigured, http.StatusServiceUnavailable, "CUSTOMER_CLIENT_APP_NOT_CONFIGURED"},
		{customersvc.ErrCustomerCredentialInvalid, http.StatusUnauthorized, "CUSTOMER_CREDENTIAL_INVALID"},
		{customersvc.ErrCustomerPhoneCodeInvalid, http.StatusUnauthorized, "CUSTOMER_PHONE_CODE_INVALID"},
		{customersvc.ErrCustomerPhoneConflict, http.StatusConflict, "CUSTOMER_PHONE_CONFLICT"},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/tenant/customer/auth/register", nil)
		respondAuthError(c, tt.err)
		require.Equal(t, tt.status, w.Code, tt.reason)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotEmpty(t, body)
		require.Contains(t, w.Body.String(), tt.reason)
	}
}

func TestPhoneVerifyRejectsUnauthenticatedAndBadInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &handler{}
	engine := gin.New()
	engine.POST("/api/v1/tenant/customer/phone:verify", h.verifyPhone)
	for _, body := range []string{`{}`, `not-json`} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/tenant/customer/phone:verify", strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, w.Code, body)
	}
	// 没有插件 STS 凭证时不得进入换号流程。
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/tenant/customer/phone:verify", strings.NewReader(`{"channel":"wechat_miniapp","app_id":"wx1","credential":{"type":"wechat_miniapp_phone_code","value":"c"}}`)))
	require.Equal(t, http.StatusUnauthorized, w.Code)
}
