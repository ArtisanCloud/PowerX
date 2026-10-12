package plugingrants_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	metadatahttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	metadata "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Actual registered routes, API-key/JWT middleware, database grants and services.
// The STS token is signed locally here; live acceptance uses STS Exchange.
func TestTenantTagUpdateAPIKeyAndSTS(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&metadata.Tag{}, &capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &iam.APIKeyProfile{}, &iam.APIKey{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}))
	tenant, other, plugin := uuid.NewString(), uuid.NewString(), "com.powerx.tags.test"
	for _, cap := range []string{"com.corex.metadata.tag.manage", "com.corex.metadata.tag.read"} {
		require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: cap, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
		for _, tid := range []string{tenant, other} {
			require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tid, CapabilityID: cap, ContractRef: "test", Status: "published", Version: 1}).Error)
		}
	}
	raw := uuid.NewString()
	digest := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(digest[:])
	profile := iam.APIKeyProfile{TenantUUID: tenant, Key: plugin, Name: "test", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Create(&iam.APIKey{TenantUUID: tenant, ProfileID: profile.ID, KeyHash: hash}).Error)
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: profile.ID, Name: "test", KeyPrefix: "test", KeyHash: hash, Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	for _, action := range []string{"read", "manage"} {
		require.NoError(t, db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.metadata.tag." + action, Action: action, ResourceType: "api", ResourcePattern: "tag", Effect: "allow"}).Error)
	}
	secret := []byte("tag-route-test-not-a-live-secret")
	issue := func(tid string) string {
		require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tid, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"allowed_capabilities":["com.corex.metadata.tag.read","com.corex.metadata.tag.manage"]}`)}).Error)
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &reqctx.CoreXClaims{TenantUUID: tid, PluginID: plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}).SignedString(secret)
		require.NoError(t, err)
		return "Bearer " + token
	}
	sts, cross := issue(tenant), issue(other)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1", middleware.APIKeyOrJwtMiddleware(db, secret, "powerx-sts", []string{"powerx:api"}, nil, nil))
	metadatahttp.RegisterTenantHostRoutes(group, &shared.Deps{DB: db})
	row := metadata.Tag{TenantUUID: tenant, Namespace: "corex.test", ResourceType: "test.item", Code: "original", LabelI18n: datatypes.JSON(`{"zh-CN":"原始"}`), Color: "#000000"}
	require.NoError(t, db.Create(&row).Error)
	path := "/api/v1/tenant/metadata/tags/" + row.UUID.String()
	call := func(method, path, credential, body string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", credential)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, want, w.Code, w.Body.String())
		var out map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out
	}
	for _, credential := range []string{"ApiKey " + raw, sts} {
		out := call("PATCH", path, credential, `{"label_i18n":{"zh-CN":"更新","en":"Updated"},"description_i18n":{"zh-CN":"说明","en":"Description"},"color":"#abcdef","status":"disabled"}`, 200)
		updated := out["data"].(map[string]any)["payload"]
		listed := call("GET", "/api/v1/tenant/metadata/tags?status=disabled", credential, "", 200)["data"].(map[string]any)["payload"].(map[string]any)["items"].([]any)
		require.Len(t, listed, 1)
		require.Equal(t, updated, listed[0])
		out = call("PATCH", path, credential, `{"color":""}`, 200)
		item := out["data"].(map[string]any)["payload"].(map[string]any)
		require.Empty(t, item["color"])
		require.Equal(t, "Updated", item["label_i18n"].(map[string]any)["en"])
		require.Equal(t, "original", item["code"])
		for _, body := range []string{`{"status":"invalid"}`, `{"label_i18n":{"en":"only"}}`, `{"tenant_uuid":"` + other + `","color":"red"}`} {
			call("PATCH", path, credential, body, 400)
		}
		call("PATCH", "/api/v1/tenant/metadata/tags/invalid", credential, `{"color":"red"}`, 400)
	}
	call("PATCH", path, cross, `{"color":"red"}`, 404)
	foreign := metadata.Tag{TenantUUID: other, Namespace: "corex.test", ResourceType: "test.item", Code: "foreign", LabelI18n: datatypes.JSON(`{"zh-CN":"其他"}`)}
	require.NoError(t, db.Create(&foreign).Error)
	call("PATCH", "/api/v1/tenant/metadata/tags/"+foreign.UUID.String(), "ApiKey "+raw, `{"color":"red"}`, 404)
	require.NoError(t, db.Where("api_key_uuid = ? AND action = ?", key.UUID, "manage").Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	call("PATCH", path, "ApiKey "+raw, `{"color":"red"}`, 403)
	require.NoError(t, db.WithContext(context.Background()).Model(&setting.PluginInstanceConfig{}).Where("plugin_id = ?", plugin).Update("value_json", datatypes.JSON(`{"allowed_capabilities":["com.corex.metadata.tag.read"]}`)).Error)
	call("PATCH", path, sts, `{"color":"red"}`, 403)
	call("PATCH", path, "", `{"color":"red"}`, 401)
	var unchanged metadata.Tag
	require.NoError(t, db.First(&unchanged, "uuid = ?", row.UUID).Error)
	require.Empty(t, unchanged.Color)
}
