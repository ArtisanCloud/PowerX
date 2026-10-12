package plugingrants_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/config"
	pluginsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	metadata "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/datatypes"
)

// Opt-in against a running Core. Only generated fixture records/grants are changed.
// Credentials remain in memory; logs contain server trace IDs, never credentials.
func TestLiveMetadataTagUpdateAPIKeyAndSTS(t *testing.T) {
	configPath, base, stsAddress := os.Getenv("POWERX_METADATA_TEST_CONFIG"), os.Getenv("POWERX_LIVE_TEST_URL"), os.Getenv("POWERX_LIVE_TEST_STS")
	if configPath == "" || base == "" || stsAddress == "" {
		t.Skip("live_metadata_acceptance_not_configured")
	}
	require.True(t, strings.HasPrefix(base, "http://127.0.0.1:"))
	require.True(t, strings.HasPrefix(stsAddress, "127.0.0.1:"))
	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	db, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var tenants []string
	require.NoError(t, db.Raw("SELECT uuid::text FROM iam_tenant WHERE status=1 AND deleted_at IS NULL ORDER BY uuid LIMIT 2").Scan(&tenants).Error)
	require.Len(t, tenants, 2)
	const manage = "com.corex.metadata.tag.manage"
	const read = "com.corex.metadata.tag.read"
	const status = "com.corex.capabilities.grant_status.read"
	pluginID := "com.powerx.metadata.acceptance." + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM plugin_instance_configs WHERE plugin_id = ?", pluginID).Error)
	})
	conn, err := grpc.NewClient(stsAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	pluginService := pluginsvc.NewTenantPluginInstanceService(db)
	issue := func(tenant string) string {
		_, id, secret, err := pluginService.Enable(ctx, tenant, plugin_mgr.Plugin{ID: pluginID, Version: "acceptance", RequiredCapabilities: []string{manage, read, status}}, nil)
		require.NoError(t, err)
		token, err := stsv1.NewSTSServiceClient(conn).Exchange(ctx, &stsv1.ExchangeRequest{ClientId: id, ClientSecret: secret, Audience: "powerx:api", Scope: "access", TtlSeconds: 300})
		require.NoError(t, err)
		require.NotEmpty(t, token.GetData().GetAccessToken())
		return "Bearer " + token.GetData().GetAccessToken()
	}
	sts, crossSTS := issue(tenants[0]), issue(tenants[1])
	profile := iam.APIKeyProfile{TenantUUID: tenants[0], Key: pluginID, Name: "metadata tag acceptance", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	t.Cleanup(func() { require.NoError(t, db.Delete(&profile).Error) })
	rawKey := uuid.NewString() + uuid.NewString()
	digest := sha256.Sum256([]byte(rawKey))
	hash := hex.EncodeToString(digest[:])
	iamKey := iam.APIKey{TenantUUID: tenants[0], ProfileID: profile.ID, KeyHash: hash}
	require.NoError(t, db.Create(&iamKey).Error)
	t.Cleanup(func() { require.NoError(t, db.Delete(&iamKey).Error) })
	expires := time.Now().Add(5 * time.Minute)
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenants[0], ProfileID: profile.ID, Name: "metadata tag acceptance", KeyPrefix: "acceptance", KeyHash: hash, Status: "active", ExpiresAt: &expires}
	require.NoError(t, db.Create(&key).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(&key).Error) })
	grants := []gw.IntegrationGatewayAPIKeyPermission{
		{APIKeyUUID: key.UUID, Scope: "_scope.metadata.tag.manage", Action: "manage", ResourceType: "api", ResourcePattern: "tag", Effect: "allow"},
		{APIKeyUUID: key.UUID, Scope: "_scope.metadata.tag.read", Action: "read", ResourceType: "api", ResourcePattern: "tag", Effect: "allow"},
		{APIKeyUUID: key.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"},
	}
	require.NoError(t, db.Create(&grants).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Unscoped().Where("api_key_uuid = ?", key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	})

	client := http.Client{Timeout: 15 * time.Second}
	call := func(label, method, credential, path string, body any, want int) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		request, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(raw))
		require.NoError(t, err)
		request.Header.Set("Authorization", credential)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		var result map[string]any
		require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
		require.Equal(t, want, response.StatusCode, "%s: %v", label, result)
		trace := response.Header.Get("X-Trace-ID")
		require.NotEmpty(t, trace)
		t.Logf("%s http=%d trace_id=%s", label, response.StatusCode, trace)
		data, _ := result["data"].(map[string]any)
		return data
	}
	apiKey := "ApiKey " + rawKey
	for _, credential := range []string{apiKey, sts} {
		result := call("grant_status", "POST", credential, "/api/v1/tenant/capabilities:grant-status", map[string]any{"capability_ids": []string{manage}}, 200)
		require.Equal(t, "granted", result["items"].([]any)[0].(map[string]any)["status"])
	}
	row := metadata.Tag{TenantUUID: tenants[0], Namespace: "corex.acceptance", ResourceType: "acceptance.tag", Code: uuid.NewString(), LabelI18n: datatypes.JSON(`{"zh-CN":"验收"}`), Color: "#000000"}
	require.NoError(t, db.Create(&row).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(&row).Error) })
	foreign := metadata.Tag{TenantUUID: tenants[1], Namespace: row.Namespace, ResourceType: row.ResourceType, Code: row.Code, LabelI18n: row.LabelI18n}
	require.NoError(t, db.Create(&foreign).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(&foreign).Error) })
	path := "/api/v1/tenant/metadata/tags/" + row.UUID.String()
	for _, mode := range []struct{ name, credential string }{{"api_key", apiKey}, {"sts", sts}} {
		patch := map[string]any{"label_i18n": map[string]string{"zh-CN": "更新", "en": mode.name}, "description_i18n": map[string]string{"zh-CN": "描述", "en": "Description"}, "color": "#abcdef", "status": "disabled"}
		updated := call(mode.name+"_patch", "PATCH", mode.credential, path, patch, 200)["payload"]
		listed := call(mode.name+"_reread", "GET", mode.credential, "/api/v1/tenant/metadata/tags?q="+row.Code, nil, 200)["payload"].(map[string]any)["items"].([]any)
		require.Len(t, listed, 1)
		require.Equal(t, updated, listed[0])
		cleared := call(mode.name+"_clear", "PATCH", mode.credential, path, map[string]any{"color": ""}, 200)["payload"].(map[string]any)
		require.Empty(t, cleared["color"])
		require.Equal(t, patch["label_i18n"].(map[string]string)["en"], cleared["label_i18n"].(map[string]any)["en"])
		body := map[string]any{"operation": "update", "tag_uuid": row.UUID.String(), "color": "#112233"}
		invoke := map[string]any{"capability_id": manage, "preferred_protocol": "core_internal", "payload": map[string]any{"method": "INVOKE", "endpoint": "core://metadata/tags", "body": body}}
		item := call(mode.name+"_typed_update", "POST", mode.credential, "/api/v1/tenant/invocations", invoke, 200)["payload"].(map[string]any)["item"].(map[string]any)
		require.Equal(t, "#112233", item["color"])
		for _, bad := range []map[string]any{{"status": "invalid"}, {"tenant_uuid": tenants[1], "color": "red"}, {"label_i18n": map[string]string{"en": "only"}}} {
			call(mode.name+"_invalid", "PATCH", mode.credential, path, bad, 400)
		}
	}
	call("sts_cross_tenant", "PATCH", crossSTS, path, map[string]any{"color": "red"}, 404)
	call("api_key_cross_tenant", "PATCH", apiKey, "/api/v1/tenant/metadata/tags/"+foreign.UUID.String(), map[string]any{"color": "red"}, 404)
	require.NoError(t, db.Unscoped().Delete(&grants[0]).Error)
	call("api_key_revoked", "PATCH", apiKey, path, map[string]any{"color": "red"}, 403)
	_, _, _, err = pluginService.Enable(ctx, tenants[0], plugin_mgr.Plugin{ID: pluginID, Version: "acceptance", RequiredCapabilities: []string{read, status}}, nil)
	require.NoError(t, err)
	call("sts_revoked", "PATCH", sts, path, map[string]any{"color": "red"}, 403)
	var unchanged metadata.Tag
	require.NoError(t, db.First(&unchanged, "uuid = ?", row.UUID).Error)
	require.Equal(t, "#112233", unchanged.Color)
}
