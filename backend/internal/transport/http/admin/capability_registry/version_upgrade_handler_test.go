package capability_registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVersionUpgradeHTTPRejectsServiceCredentialsAndCallerActor(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		claims     *reqctx.CoreXClaims
		apiKey     bool
		status     int
	}{
		{name: "anonymous", body: `{}`, status: 401},
		{name: "member", body: `{}`, claims: &reqctx.CoreXClaims{UserID: 1}, status: 403},
		{name: "api_key", body: `{}`, claims: &reqctx.CoreXClaims{UserID: 1, IsRoot: true}, apiKey: true, status: 403},
		{name: "caller_actor", body: `{"actor":"root","capabilities_hash":"x","reason":"x"}`, claims: &reqctx.CoreXClaims{UserID: 1, IsRoot: true}, status: 400},
		{name: "trailing_json", body: `{} {}`, claims: &reqctx.CoreXClaims{UserID: 1, IsRoot: true}, status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				if tc.claims != nil {
					c.Request = c.Request.WithContext(reqctx.WithClaims(c.Request.Context(), tc.claims))
				}
				if tc.apiKey {
					c.Set("auth_source", "api_key")
				}
				c.Next()
			})
			handler := &versionUpgradeHandler{}
			engine.POST("/upgrade", middleware.AdminOnlyMiddleware(), handler.Confirm)
			request := httptest.NewRequest("POST", "/upgrade", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code)
			if tc.apiKey || tc.status == 400 {
				var body map[string]any
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				code := "capability.upgrade_invalid"
				if tc.apiKey {
					code = "capability.upgrade_forbidden"
				}
				require.Equal(t, code, body["error_code"])
				require.Equal(t, code, body["reason_code"])
				require.Equal(t, code, body["message"])
			}
		})
	}
}

func TestVersionUpgradeHTTPMachineReasonsMatchPublishedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../../../../../docs/contracts/fixtures/capability-version-upgrade-errors.v1.json")
	require.NoError(t, err)
	var fixture struct {
		Cases []struct {
			Name    string         `json:"name"`
			Status  int            `json:"status"`
			AuditID uint64         `json:"audit_id"`
			Body    map[string]any `json:"body"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	causes := map[string]error{
		"invalid": capservice.ErrVersionUpgradeInvalid, "forbidden": capservice.ErrVersionUpgradeForbidden,
		"not_found": capservice.ErrVersionUpgradeNotFound, "hash_mismatch": capservice.ErrVersionUpgradeHashMismatch,
		"binding_mismatch": capservice.ErrVersionUpgradeBindingMismatch, "unavailable": capservice.ErrVersionUpgradeUnavailable,
		"persistence_failed": capservice.ErrVersionUpgradePersistenceFailed, "runtime_apply_failed": fmt.Errorf("%w: %w", capservice.ErrVersionUpgradeRuntimeApplyFailed, gorm.ErrRecordNotFound),
		"audit_finalize_failed": capservice.ErrVersionUpgradeAuditFinalizeFailed,
		// 同文案的普通错误不是业务 sentinel，不能通过解析字符串伪造机器码。
		"unknown": errors.New("capability.upgrade_binding_mismatch"),
	}
	require.Len(t, fixture.Cases, len(causes))
	for _, sample := range fixture.Cases {
		t.Run(sample.Name, func(t *testing.T) {
			cause, ok := causes[sample.Name]
			require.True(t, ok)
			engine := gin.New()
			engine.POST("/upgrade", func(c *gin.Context) {
				c.Request = c.Request.WithContext(reqctx.WithTraceID(c.Request.Context(), "fixture.trace"))
				respondVersionUpgradeError(c, fmt.Errorf("private diagnostic: %w", cause), sample.AuditID, sample.Body["message"].(string))
			})
			req := httptest.NewRequest("POST", "/upgrade", nil)
			req.Header.Set("X-Request-ID", "fixture.request")
			out := httptest.NewRecorder()
			engine.ServeHTTP(out, req)
			require.Equal(t, sample.Status, out.Code)
			var actual map[string]any
			require.NoError(t, json.Unmarshal(out.Body.Bytes(), &actual))
			delete(actual, "timestamp")
			require.Equal(t, sample.Body, actual)
			require.NotContains(t, out.Body.String(), "private diagnostic")
		})
	}
}

func TestVersionUpgradeHTTPServiceFailuresPreserveLegacyMessage(t *testing.T) {
	engine := gin.New()
	handler := &versionUpgradeHandler{}
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithClaims(c.Request.Context(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}))
		c.Next()
	})
	engine.POST("/api/v1/admin/capability-registry/capabilities/:capabilityId/tenants/:tenant_uuid/upgrade", middleware.AdminOnlyMiddleware(), handler.Confirm)
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{name: "unavailable", body: `{"capabilities_hash":"` + strings.Repeat("a", 64) + `","reason":"fixture"}`, code: "capability.upgrade_unavailable", status: 503},
		{name: "invalid_service_input", body: `{}`, code: "capability.upgrade_invalid", status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/admin/capability-registry/capabilities/com.corex.ai.catalog.models.read/tenants/11111111-1111-4111-8111-111111111111/upgrade", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			engine.ServeHTTP(out, req)
			require.Equal(t, tc.status, out.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(out.Body.Bytes(), &body))
			require.Equal(t, tc.code, body["reason_code"])
			require.Equal(t, "capability.upgrade_failed", body["message"])
		})
	}
}
