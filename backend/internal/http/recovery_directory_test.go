package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecoveryDirectoryReadsRetainBodyAndDownstreamAuthorization(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"member_uuids": []string{uuid.NewString()}})
	r := gin.New()
	r.Use(RecoveryMutationGuard(true, "/api/v1"))
	calls := 0
	r.POST("/api/v1/tenant/iam/members:batch-resolve", func(c *gin.Context) {
		calls++
		raw, _ := io.ReadAll(c.Request.Body)
		require.JSONEq(t, string(body), string(raw))
		c.Status(http.StatusForbidden)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/tenant/iam/members:batch-resolve", strings.NewReader(string(body))))
	require.Equal(t, 1, calls)
	require.Equal(t, http.StatusForbidden, w.Code)
	for _, payload := range []string{`{"member_uuids":[]}`, `{"member_uuids":["invalid"]}`, `{"member_uuids":["00000000-0000-0000-0000-000000000000"]}`, `{"member_uuids":["` + uuid.NewString() + `"],"tenant_uuid":"override"}`} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/tenant/iam/members:batch-resolve", strings.NewReader(payload)))
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	}
	require.Equal(t, 1, calls)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/iam/members:create", strings.NewReader(string(body)))
	require.False(t, recoveryReadOnlyPost(req, "/api/v1"))
}
