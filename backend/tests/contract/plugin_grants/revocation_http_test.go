package plugingrants_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capsvc "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	pluginsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	metadatahttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/metadata"
	caphttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	metamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The same signed token is sent through actual JWT middleware and registered
// Host handlers before and after manifest synchronization. No claims are
// injected by test middleware, and neither handler/service is replaced.
func TestManifestRevocationWithExistingSignedToken(t *testing.T) {
	const tenant = "b39ca82e-8bdd-4c3c-9af8-a58d39d7d9c0"
	const pluginID = "com.powerx.plugins.grant-test"
	const read = "com.corex.metadata.dictionary.read"
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &setting.PluginCapabilityApproval{}, &metamodel.DictionaryNamespace{}))
	for _, capability := range []string{read, capsvc.GrantStatusCapabilityID} {
		require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: capability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
		require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant, CapabilityID: capability, ContractRef: "test", Status: "published", Version: 1}).Error)
	}
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: pluginID, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON([]byte(`{"client_id":"test"}`))}).Error)
	sync := func(required []string) {
		require.NoError(t, pluginsvc.NewTenantPluginInstanceService(db).SyncManifestRequiredCapabilities(context.Background(), plugin_mgr.Manifest{ID: pluginID, Capabilities: plugin_mgr.HostCapabilitySpec{Required: required}}))
	}
	sync([]string{read, capsvc.GrantStatusCapabilityID})
	secret := []byte("contract-test-signing-key-not-a-runtime-credential")
	claims := &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: pluginID, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1", middleware.JwtMiddleware(secret, "powerx-sts", []string{"powerx:api"}, nil, nil))
	deps := &shared.Deps{DB: db, CapabilityCatalogSvc: &capsvc.RegistryService{}}
	metadatahttp.RegisterTenantHostRoutes(group, deps)
	caphttp.RegisterTenantRoutes(group, deps)
	request := func(method, path, body, authorization string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			req.Header.Set("Authorization", "Bearer "+authorization)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	checkStatus := func(want string) {
		response := request("POST", "/api/v1/tenant/capabilities:grant-status", `{"capability_ids":["`+read+`"]}`, token)
		require.Equal(t, 200, response.Code, response.Body.String())
		var envelope struct {
			Data struct {
				Items []capsvc.GrantStatusItem `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		require.Len(t, envelope.Data.Items, 1)
		require.Equal(t, want, envelope.Data.Items[0].Status)
	}
	checkStatus("granted")
	response := request("GET", "/api/v1/tenant/metadata/dictionaries", "", token)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	sync([]string{capsvc.GrantStatusCapabilityID})
	checkStatus("not_granted")
	response = request("GET", "/api/v1/tenant/metadata/dictionaries", "", token)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"reason_code":"METADATA_FORBIDDEN"`)
	sync(nil)
	response = request("POST", "/api/v1/tenant/capabilities:grant-status", `{"capability_ids":["`+read+`"]}`, token)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	for _, authz := range []string{"", "malformed"} {
		response = request("GET", "/api/v1/tenant/metadata/dictionaries", "", authz)
		require.Equal(t, http.StatusUnauthorized, response.Code)
		require.Contains(t, response.Body.String(), `"reason_code":"METADATA_UNAUTHORIZED"`)
	}
}
