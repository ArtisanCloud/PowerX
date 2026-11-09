package customer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMiniAppCredentialMatchesPlatform(t *testing.T) {
	require.True(t, isMiniAppCredential("wechat_miniapp", "wechat_miniapp_code"))
	require.True(t, isMiniAppCredential("douyin_miniapp", "douyin_miniapp_code"))
	require.False(t, isMiniAppCredential("wechat_miniapp", "alipay_miniapp_code"))
	require.False(t, isMiniAppCredential("shopify_storefront", "shopify_customer_access_token"))
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
