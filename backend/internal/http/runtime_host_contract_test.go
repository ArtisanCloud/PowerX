package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	service "github.com/ArtisanCloud/PowerX/internal/service/runtime_host"
	fixture "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	host "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/runtime_host"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRuntimeHostSignedHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := fixture.Database(t)
	ctx, credential := fixture.Actor(t, db, uuid.NewString(), "plugin.http")
	other, _ := fixture.Actor(t, db, reqctx.GetTenantUUID(ctx), "plugin.other")
	cross, _ := fixture.Actor(t, db, uuid.NewString(), "plugin.http")
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { client.Close() })
	key := []byte("runtime-host-contract-test-signing-key")
	sign := func(claims *reqctx.CoreXClaims) string {
		token, e := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
		require.NoError(t, e)
		return token
	}
	token := sign(reqctx.GetClaims(ctx))
	otherToken := sign(reqctx.GetClaims(other))
	crossToken := sign(reqctx.GetClaims(cross))
	router := gin.New()
	g := router.Group("/api/v1", middleware.JwtMiddleware(key, "powerx-sts", []string{"powerx:api"}, []string{"access"}, buildJWTSubjectValidationCallback(db)))
	host.RegisterTenantRoutes(g, &shared.Deps{RuntimeHostSvc: service.NewService(db, client)})
	call := func(method, path, body, auth string, headers ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/tenant/runtime"+path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		for i := 0; i < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	data := func(out *httptest.ResponseRecorder) map[string]any {
		require.Equal(t, 200, out.Code, out.Body.String())
		var body struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(out.Body.Bytes(), &body))
		return body.Data
	}
	for _, path := range []string{"/cache/entries?namespace=n&key=k", "/tasks/" + uuid.NewString()} {
		r := call("GET", path, "", "")
		require.Equal(t, 401, r.Code)
		require.Contains(t, r.Body.String(), "_UNAUTHORIZED")
	}
	r := call("PUT", "/cache/entries", `{"namespace":"n","key":"k","value_base64":"","ttl_ms":1000}`, token)
	require.Equal(t, 200, r.Code, r.Body.String())
	require.Equal(t, true, data(call("GET", "/cache/entries?namespace=n&key=k", "", token))["found"])
	for _, auth := range []string{otherToken, crossToken} {
		require.Equal(t, false, data(call("GET", "/cache/entries?namespace=n&key=k", "", auth))["found"])
	}
	created := data(call("POST", "/tasks", `{"type":"export","idempotency_key":"one","payload":{}}`, token))
	id := created["task_uuid"].(string)
	for _, auth := range []string{otherToken, crossToken} {
		require.Equal(t, 404, call("GET", "/tasks/"+id, "", auth).Code)
		require.Equal(t, 404, call("PATCH", "/tasks/"+id, `{"expected_revision":1,"state":"running","progress":0}`, auth).Code)
	}
	updated := data(call("PATCH", "/tasks/"+id, `{"expected_revision":1,"state":"running","progress":20}`, token))
	require.EqualValues(t, 2, updated["revision"])
	require.Equal(t, 409, call("PATCH", "/tasks/"+id, `{"expected_revision":1,"state":"failed","progress":20}`, token).Code)
	for _, body := range []string{
		`null`, `{}`, `{"Type":"export","idempotency_key":"one","payload":{}}`,
		`{"type":"export","type":"other","idempotency_key":"one","payload":{}}`,
		`{"type":"export","idempotency_key":"one","payload":{},"tenant_uuid":"` + uuid.NewString() + `"}`,
		`{"type":"export","idempotency_key":"one","payload":{},"caller_subject_uuid":"` + uuid.NewString() + `"}`,
		`{"type":"export","idempotency_key":"one","payload":{"a":1,"a":2}}`,
		`{"type":"export","idempotency_key":"one","payload":{}} {}`,
	} {
		r = call("POST", "/tasks", body, token)
		require.Equal(t, 400, r.Code, r.Body.String())
		require.Contains(t, r.Body.String(), "TASKCENTER_INVALID_ARGUMENT")
	}
	for _, suffix := range []string{"?tenant_uuid=" + uuid.NewString(), "?unexpected=1"} {
		require.Equal(t, 400, call("GET", "/tasks/"+id+suffix, "", token).Code)
	}
	require.Equal(t, 400, call("GET", "/tasks/"+id, "", token, "X-Tenant-UUID", uuid.NewString()).Code)
	require.Equal(t, 400, call("GET", "/cache/entries?namespace=n&key=k&key=b", "", token).Code)
	require.Equal(t, 400, call("PATCH", "/tasks/"+id, `{"expected_revision":2,"state":"running"}`, token).Code)
	require.Equal(t, 400, call("PATCH", "/tasks/"+id, `{"expected_revision":2,"state":"running","progress":20,"metadata":{"tenantUuid":"bad"}}`, token).Code)
	expired := *reqctx.GetClaims(ctx)
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	require.Equal(t, 401, call("GET", "/tasks/"+id, "", sign(&expired)).Code)
	require.Equal(t, 401, call("GET", "/tasks/"+id, "", token+"invalid").Code)
	fixture.Grant(t, db, credential)
	for _, operation := range []struct{ method, path, body string }{{"GET", "/tasks/" + id, ""}, {"PATCH", "/tasks/" + id, `{"expected_revision":2,"state":"running","progress":20}`}, {"GET", "/cache/entries?namespace=n&key=k", ""}, {"DELETE", "/cache/entries?namespace=n&key=k", ""}} {
		r = call(operation.method, operation.path, operation.body, token)
		require.Equal(t, 403, r.Code, r.Body.String())
		require.Contains(t, r.Body.String(), "_FORBIDDEN")
	}
	fixture.Grant(t, db, credential, fixture.Capabilities...)
	redisServer.Close()
	r = call("GET", "/cache/entries?namespace=n&key=k", "", token)
	require.Equal(t, 503, r.Code)
	require.Contains(t, r.Body.String(), "CACHE_UPSTREAM_DEPENDENCY")
	require.NotContains(t, r.Body.String(), client.Options().Addr)
	// 正式目录只放开六项 method/path，未声明方法及 admin/internal 不开放。
	for _, method := range []string{"POST", "PATCH"} {
		requestCtx := reqctx.WithRequestMethod(reqctx.WithRequestPath(ctx, "/api/v1/tenant/runtime/cache/entries"), method)
		require.Error(t, validateSTSRouteOnly(requestCtx, reqctx.GetClaims(ctx)))
	}
	for _, path := range []string{"/api/v1/internal/tasks", "/api/v1/admin/runtime/tasks"} {
		requestCtx := reqctx.WithRequestMethod(reqctx.WithRequestPath(ctx, path), http.MethodGet)
		require.Error(t, validateSTSRouteOnly(requestCtx, reqctx.GetClaims(ctx)))
	}
	require.False(t, strings.Contains(r.Body.String(), "password"))
}
