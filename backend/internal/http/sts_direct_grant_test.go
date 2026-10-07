package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSTSDirectGateRevokesExistingTokenBeforeHandler(t *testing.T) {
	for _, tc := range []struct{ path, capability, prefix string }{
		{"/api/v1/ai/llm/invoke", "com.corex.ai.llm.invoke", "CAPABILITY"},
		{"/api/v1/tenant/agent/sessions", "com.corex.agent.session.manage", "AGENT_SESSION"},
	} {
		t.Run(tc.capability, func(t *testing.T) {
			old := coremodel.PowerXSchema
			coremodel.PowerXSchema = "main"
			t.Cleanup(func() { coremodel.PowerXSchema = old })
			db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&tenantmodel.Tenant{}, &capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}))
			tenant := tenantmodel.Tenant{Key: "test", Name: "test", Status: tenantmodel.TenantStatusActive}
			require.NoError(t, db.Create(&tenant).Error)
			require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: tc.capability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
			require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant.UUID.String(), CapabilityID: tc.capability, Status: "published", ContractRef: "test", Version: 1}).Error)
			payload, _ := json.Marshal(map[string]any{"client_id": "test-client", "allowed_capabilities": []string{tc.capability}})
			credential := setting.PluginInstanceConfig{TenantUUID: tenant.UUID.String(), PluginID: "com.powerx.plugins.test", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(payload)}
			require.NoError(t, db.Create(&credential).Error)
			secret := []byte("direct-gate-test-secret-not-a-runtime-key")
			claims := reqctx.CoreXClaims{TenantUUID: tenant.UUID.String(), PluginID: credential.PluginID, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:test-client", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
			require.NoError(t, err)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			called := 0
			router.Use(middleware.JwtMiddleware(secret, "powerx-sts", []string{"powerx:api"}, nil, buildJWTSubjectValidationCallback(db)))
			router.POST(tc.path, func(c *gin.Context) { called++; c.Status(204) })
			request := func() *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", tc.path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w
			}
			w := request()
			require.Equal(t, 204, w.Code, w.Body.String())
			require.Equal(t, 1, called)
			require.NoError(t, db.Model(&credential).Update("value_json", datatypes.JSON([]byte(`{"client_id":"test-client","allowed_capabilities":[]}`))).Error)
			w = request()
			require.Equal(t, 403, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.prefix+"_FORBIDDEN")
			require.Equal(t, 1, called)
			require.NoError(t, db.Model(&credential).Update("value_json", datatypes.JSON(payload)).Error)
			// A historical published registration cannot override a withdrawn latest version.
			require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant.UUID.String(), CapabilityID: tc.capability, Status: "disabled", ContractRef: "test", Version: 2}).Error)
			w = request()
			require.Equal(t, 403, w.Code, w.Body.String())
			require.Equal(t, 1, called)
			require.NoError(t, sqlDB.Close())
			w = request()
			require.Equal(t, 503, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.prefix+"_UPSTREAM_DEPENDENCY")
			require.Equal(t, 1, called)
		})
	}
}
