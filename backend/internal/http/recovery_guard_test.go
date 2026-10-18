package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRecoveryMaintenanceBlocksWritesAndWebSocketBeforeHandler(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, upgrade string
		enabled                     bool
		allowed                     bool
	}{
		{"read", "GET", "/custom/admin/orders", "", true, true},
		{"head", "HEAD", "/custom/admin/orders", "", true, true},
		{"preflight", "OPTIONS", "/custom/admin/orders", "", true, true},
		{"login", "POST", "/custom/admin/user/auth/login", "", true, true},
		{"refresh", "POST", "/custom/admin/user/auth/refresh", "", true, true},
		{"write", "POST", "/custom/admin/orders", "", true, false},
		{"delete", "DELETE", "/custom/admin/orders/1", "", true, false},
		{"prefix_not_matched", "POST", "/api/v1/admin/user/auth/login", "", true, false},
		{"login_prefix_escape", "POST", "/custom/admin/user/auth/login/execute", "", true, false},
		{"websocket_command_channel", "GET", "/custom/admin/agents/stream/ws", "WebSocket", true, false},
		{"plugin_websocket", "GET", "/_p/fixture/ws", "websocket", true, false},
		{"normal_write", "POST", "/custom/admin/orders", "", false, true},
		{"normal_websocket", "GET", "/custom/admin/agents/stream/ws", "websocket", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RecoveryMutationGuard(tc.enabled, "/custom/"))
			called := false
			router.NoRoute(func(c *gin.Context) { called = true; c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.upgrade != "" {
				request.Header.Set("Connection", "Upgrade")
				request.Header.Set("Upgrade", tc.upgrade)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.allowed, called)
			if tc.allowed {
				require.Equal(t, http.StatusNoContent, response.Code)
			} else {
				require.Equal(t, http.StatusServiceUnavailable, response.Code)
				require.Equal(t, "5", response.Header().Get("Retry-After"))
				require.Contains(t, response.Body.String(), "RECOVERY_READ_ONLY")
			}
		})
	}
}

func TestRecoveryTypedIdentityReadDoesNotPermitArbitraryInvocation(t *testing.T) {
	for _, tc := range []struct {
		body    string
		allowed bool
	}{
		{`{"capability_id":"com.corex.iam.identity_context.service_read","preferred_protocol":"core_internal","payload":{"body":{"operation":"resolve_request_identity","request":{"context_ticket":"opaque"}}}}`, true},
		{`{"capability_id":"com.corex.iam.identity_context.service_read","preferred_protocol":"rest","payload":{"body":{"operation":"resolve_request_identity"}}}`, false},
		{`{"capability_id":"com.corex.notifications.create","preferred_protocol":"core_internal","payload":{"body":{"operation":"create"}}}`, false},
		{`{"capability_id":"com.corex.iam.identity_context.service_read","preferred_protocol":"core_internal","payload":{"endpoint":"/write","body":{"operation":"resolve_request_identity"}}}`, false},
		{`{"capability_id":"com.corex.iam.identity_context.service_read","preferred_protocol":"core_internal","intent":"write","payload":{"body":{"operation":"resolve_request_identity"}}}`, false},
		{`{} {}`, false},
		{strings.Repeat(" ", 65<<10), false},
	} {
		router := gin.New()
		router.Use(RecoveryMutationGuard(true, "/custom"))
		var received string
		router.POST("/custom/tenant/invocations", func(c *gin.Context) { raw, _ := c.GetRawData(); received = string(raw); c.Status(204) })
		request := httptest.NewRequest("POST", "/custom/tenant/invocations", strings.NewReader(tc.body))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if tc.allowed {
			require.Equal(t, 204, response.Code)
			require.Equal(t, tc.body, received)
		} else {
			require.Equal(t, 503, response.Code)
		}
	}
}
