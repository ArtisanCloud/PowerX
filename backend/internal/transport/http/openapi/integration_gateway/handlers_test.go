package integration_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	capaccess "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	tenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const wireTenant = "a6dba953-bbc9-4592-a7fc-9495c52e8d2b"

type wireService struct {
	err    error
	result tenant.InvokeResult
	calls  int
	input  tenant.InvokeInput
}

func (s *wireService) ListRoutes(context.Context, string, string, string) ([]manager.Route, error) {
	s.calls++
	return []manager.Route{}, s.err
}
func (s *wireService) GetRoute(context.Context, string, string) (manager.Route, error) {
	s.calls++
	return manager.Route{}, s.err
}
func (s *wireService) Invoke(_ context.Context, in tenant.InvokeInput) (tenant.InvokeResult, error) {
	s.calls++
	s.input = in
	return s.result, s.err
}

func gatewayWireRequest(s *wireService, method, path, body, tenantUUID string, headers map[string]string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &tenantHandler{svc: s}
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), tenantUUID))
		c.Next()
	})
	r.GET("/routes", h.ListRoutes)
	r.GET("/routes/:route_slug", h.GetRoute)
	r.POST("/routes/:route_slug/invoke", h.InvokeRoute)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertGatewayError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Equal(t, code, envelope["error_code"])
	require.Equal(t, code, envelope["reason_code"])
	require.NotEmpty(t, envelope["message"])
	require.NotEqual(t, code, envelope["message"])
}

func TestGatewayHTTPErrorContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unauthorized", &capaccess.DirectGrantError{Status: 401}, 401, "CAPABILITY_UNAUTHORIZED"},
		{"no_grant", &capaccess.DirectGrantError{Status: 403}, 403, "CAPABILITY_FORBIDDEN"},
		{"invisible", tenant.ErrRouteNotAccessible{}, 404, "CAPABILITY_ROUTE_NOT_FOUND"},
		{"disabled_channel", tenant.ErrChannelDisabled{}, 404, "CAPABILITY_ROUTE_NOT_FOUND"},
		{"tool_grant", tenant.ErrToolGrantDenied{}, 403, "CAPABILITY_FORBIDDEN"},
		{"dependency", errors.New("private_dependency_detail"), 503, "CAPABILITY_UPSTREAM_DEPENDENCY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/routes", "/routes/private", "/routes/private/invoke"} {
				method := http.MethodGet
				if strings.HasSuffix(path, "/invoke") {
					method = http.MethodPost
				}
				s := &wireService{err: tc.err}
				w := gatewayWireRequest(s, method, path, `{"payload":{}}`, wireTenant, nil)
				assertGatewayError(t, w, tc.status, tc.code)
				require.NotContains(t, w.Body.String(), "private_dependency_detail")
			}
		})
	}
}

func TestGatewayHTTPIdentityAndInput(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, tenant string
		headers                  map[string]string
		status                   int
		code                     string
	}{
		{"missing_context", "/routes/x/invoke", `{"payload":{}}`, "", nil, 401, "CAPABILITY_UNAUTHORIZED"},
		{"invalid_context", "/routes/x/invoke", `{"payload":{}}`, "123", nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"query_tenant", "/routes/x/invoke?tenant_uuid=", `{"payload":{}}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"query_alias", "/routes/x/invoke?tenantId=123", `{"payload":{}}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"header_tenant", "/routes/x/invoke", `{"payload":{}}`, wireTenant, map[string]string{"X-Tenant-UUID": ""}, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"body_tenant", "/routes/x/invoke", `{"payload":{},"tenant_uuid":""}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"null_payload", "/routes/x/invoke", `{"payload":null}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"trailing_json", "/routes/x/invoke", `{"payload":{}} {}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"duplicate_payload", "/routes/x/invoke", `{"payload":{},"payload":{}}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"case_alias", "/routes/x/invoke", `{"Payload":{}}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
		{"null_context", "/routes/x/invoke", `{"payload":{},"context":null}`, wireTenant, nil, 400, "CAPABILITY_INVALID_ARGUMENT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &wireService{}
			w := gatewayWireRequest(s, http.MethodPost, tc.path, tc.body, tc.tenant, tc.headers)
			assertGatewayError(t, w, tc.status, tc.code)
			require.Zero(t, s.calls)
		})
	}
}

func TestGatewayHTTPExecutionResults(t *testing.T) {
	for _, tc := range []struct {
		status tenant.InvokeStatus
		http   int
		code   string
	}{
		{tenant.InvokeStatusOK, 200, ""},
		{tenant.InvokeStatusAccepted, 202, ""},
		{tenant.InvokeStatusDenied, 403, "CAPABILITY_FORBIDDEN"},
		{tenant.InvokeStatusFailed, 503, "CAPABILITY_UPSTREAM_DEPENDENCY"},
		{"", 503, "CAPABILITY_UPSTREAM_DEPENDENCY"},
	} {
		s := &wireService{result: tenant.InvokeResult{Status: tc.status, TraceID: "trace-test", ErrorMessage: "private_executor_detail"}}
		w := gatewayWireRequest(s, http.MethodPost, "/routes/x/invoke", `{"payload":{}}`, wireTenant, map[string]string{"Authorization": "Bearer secret-test"})
		require.Equal(t, tc.http, w.Code)
		require.Empty(t, s.input.Actor)
		require.Equal(t, wireTenant, s.input.TenantUUID)
		require.NotContains(t, w.Body.String(), "private_executor_detail")
		if tc.code != "" {
			assertGatewayError(t, w, tc.http, tc.code)
		}
	}
	s := &wireService{err: tenant.RateLimitError{Scope: "per_route_per_tenant"}}
	w := gatewayWireRequest(s, http.MethodPost, "/routes/x/invoke", `{"payload":{}}`, wireTenant, nil)
	assertGatewayError(t, w, 429, "CAPABILITY_RATE_LIMITED")
}
