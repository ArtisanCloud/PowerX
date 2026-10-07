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
	customer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Opt-in against a running Core. Only generated fixture records/grants are changed.
// Credentials remain in memory; logs contain server trace IDs, never credentials.
func TestLiveCustomerProfileUpdateAPIKeyAndSTS(t *testing.T) {
	path, base, stsAddress := os.Getenv("POWERX_CUSTOMER_TEST_CONFIG"), os.Getenv("POWERX_LIVE_TEST_URL"), os.Getenv("POWERX_LIVE_TEST_STS")
	if path == "" || base == "" || stsAddress == "" {
		t.Skip("live_customer_acceptance_not_configured")
	}
	require.True(t, strings.HasPrefix(base, "http://127.0.0.1:"))
	require.True(t, strings.HasPrefix(stsAddress, "127.0.0.1:"))
	cfg, err := config.Load(path)
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
	const manage = "com.corex.customer.accounts.service_manage"
	const read = "com.corex.customer.accounts.service_read"
	const status = "com.corex.capabilities.grant_status.read"
	const identityRead = "com.corex.customer.external_identities.service_read"
	const identityManage = "com.corex.customer.external_identities.service_manage"
	pluginID := "com.powerx.customer.acceptance." + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM plugin_instance_configs WHERE plugin_id = ?", pluginID).Error)
	})
	conn, err := grpc.NewClient(stsAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	pluginService := pluginsvc.NewTenantPluginInstanceService(db)
	issue := func(tenant string) string {
		_, id, secret, err := pluginService.Enable(ctx, tenant, plugin_mgr.Plugin{ID: pluginID, Version: "acceptance", RequiredCapabilities: []string{manage, read, status, identityRead, identityManage}}, nil)
		require.NoError(t, err)
		token, err := stsv1.NewSTSServiceClient(conn).Exchange(ctx, &stsv1.ExchangeRequest{ClientId: id, ClientSecret: secret, Audience: "powerx:api", Scope: "access", TtlSeconds: 300})
		require.NoError(t, err)
		require.NotEmpty(t, token.GetData().GetAccessToken())
		return "Bearer " + token.GetData().GetAccessToken()
	}
	sts, crossSTS := issue(tenants[0]), issue(tenants[1])
	profile := iam.APIKeyProfile{TenantUUID: tenants[0], Key: pluginID, Name: "customer update acceptance", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	t.Cleanup(func() { require.NoError(t, db.Delete(&profile).Error) })
	rawKey := uuid.NewString() + uuid.NewString()
	digest := sha256.Sum256([]byte(rawKey))
	hash := hex.EncodeToString(digest[:])
	iamKey := iam.APIKey{TenantUUID: tenants[0], ProfileID: profile.ID, KeyHash: hash}
	require.NoError(t, db.Create(&iamKey).Error)
	t.Cleanup(func() { require.NoError(t, db.Delete(&iamKey).Error) })
	expires := time.Now().Add(5 * time.Minute)
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenants[0], ProfileID: profile.ID, Name: "customer update acceptance", KeyPrefix: "acceptance", KeyHash: hash, Status: "active", ExpiresAt: &expires}
	require.NoError(t, db.Create(&key).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(&key).Error) })
	grants := []gw.IntegrationGatewayAPIKeyPermission{
		{APIKeyUUID: key.UUID, Scope: "_scope.customer.accounts.service_manage", Action: "manage", ResourceType: "capability", ResourcePattern: "customer_accounts_service_manage", Effect: "allow"},
		{APIKeyUUID: key.UUID, Scope: "_scope.customer.accounts.service_read", Action: "read", ResourceType: "capability", ResourcePattern: "customer_accounts_service_read", Effect: "allow"},
		{APIKeyUUID: key.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"},
	}
	grants = append(grants,
		gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.customer.external_identities.service_read", Action: "read", ResourceType: "capability", ResourcePattern: "customer_external_identities_service_read", Effect: "allow", PluginID: pluginID},
		gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.customer.external_identities.service_manage", Action: "manage", ResourceType: "capability", ResourcePattern: "customer_external_identities_service_manage", Effect: "allow", PluginID: pluginID})
	require.NoError(t, db.Create(&grants).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Unscoped().Where("api_key_uuid = ?", key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	})
	client := http.Client{Timeout: 15 * time.Second}
	call := func(label, credential, path string, body any, want int) map[string]any {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set("Authorization", credential)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		var result map[string]any
		require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
		require.Equal(t, want, response.StatusCode, "%s: %v", label, result)
		data, _ := result["data"].(map[string]any)
		trace := response.Header.Get("X-Trace-ID")
		if data["trace_id"] != nil {
			trace, _ = data["trace_id"].(string)
		}
		t.Logf("%s http=%d trace_id=%s reason=%v", label, response.StatusCode, trace, result["reason_code"])
		return data
	}
	invoke := func(label, credential, cap string, body map[string]any, want int) map[string]any {
		endpoint := "core://customer/accounts"
		if cap == identityRead || cap == identityManage {
			endpoint = "core://customer/external-identities"
		}
		data := call(label, credential, "/api/v1/tenant/invocations", map[string]any{"capability_id": cap, "preferred_protocol": "core_internal", "payload": map[string]any{"method": "INVOKE", "endpoint": endpoint, "body": body}}, want)
		if want != 200 {
			return nil
		}
		require.NotEmpty(t, data["trace_id"])
		payload, ok := data["payload"].(map[string]any)
		require.True(t, ok)
		return payload
	}
	apiKey := "ApiKey " + rawKey
	grantStatus := func(credential, want string) {
		data := call("grant-status", credential, "/api/v1/tenant/capabilities:grant-status", map[string]any{"capability_ids": []string{manage}}, 200)
		items := data["items"].([]any)
		require.Len(t, items, 1)
		require.Equal(t, want, items[0].(map[string]any)["status"])
	}
	grantStatus(apiKey, "granted")
	grantStatus(sts, "granted")
	name := "Customer update acceptance " + uuid.NewString()
	created := invoke("api_key_create", apiKey, manage, map[string]any{"operation": "create", "type": "person", "display_name": name, "primary_email": "original@example.test", "primary_phone": "+8613800000000"}, 200)["item"].(map[string]any)
	id := created["uuid"].(string)
	t.Cleanup(func() {
		require.NoError(t, db.Unscoped().Where("customer_uuid = ?", id).Delete(&customer.AuthIdentity{}).Error)
		require.NoError(t, db.Unscoped().Where("customer_uuid = ?", id).Delete(&customer.Contact{}).Error)
		require.NoError(t, db.Unscoped().Where("customer_uuid = ?", id).Delete(&customer.TenantMembership{}).Error)
		require.NoError(t, db.Unscoped().Where("uuid = ?", id).Delete(&customer.Account{}).Error)
	})
	var before customer.Contact
	require.NoError(t, db.Where("uuid = ?", created["primary_contact_uuid"]).First(&before).Error)
	identity := customer.AuthIdentity{CustomerUUID: id, Provider: "acceptance", ProviderSubject: uuid.NewString(), Email: "login@example.test", Status: "active"}
	require.NoError(t, db.Create(&identity).Error)
	for _, mode := range []struct{ name, credential string }{{"api_key", apiKey}, {"sts", sts}} {
		invoke(mode.name+"_prepare_phone", mode.credential, manage, map[string]any{"operation": "update", "customer_uuid": id, "primary_phone": "+8613800000000"}, 200)
		updated := invoke(mode.name+"_update", mode.credential, manage, map[string]any{"operation": "update", "customer_uuid": id, "display_name": name + mode.name, "primary_phone": "", "nickname": mode.name}, 200)["item"].(map[string]any)
		require.Equal(t, created["type"], updated["type"])
		require.Equal(t, created["primary_contact_uuid"], updated["primary_contact_uuid"])
		require.Equal(t, "original@example.test", updated["primary_email"])
		require.Empty(t, updated["primary_phone"])
		listed := invoke(mode.name+"_reread", mode.credential, read, map[string]any{"operation": "list", "q": name, "page": 1, "page_size": 20}, 200)["items"].([]any)
		require.Len(t, listed, 1)
		require.Equal(t, updated, listed[0])
		for _, bad := range []map[string]any{
			{"operation": "update", "customer_uuid": "bad", "nickname": "x"},
			{"operation": "update", "customer_uuid": id, "status": "bogus"},
			{"operation": "update", "customer_uuid": id, "type": "company"},
			{"operation": "update", "customer_uuid": id, "primary_contact_uuid": uuid.NewString()},
			{"operation": "update", "customer_uuid": id, "tenant_uuid": tenants[1], "nickname": "x"},
		} {
			invoke(mode.name+"_invalid", mode.credential, manage, bad, 400)
		}
	}
	identitySubject := "shop:acceptance.myshopify.com:customer:" + uuid.NewString()
	for _, mode := range []struct{ name, credential string }{{"api_key", apiKey}, {"sts", sts}} {
		subject := identitySubject + mode.name
		missing := invoke(mode.name+"_identity_lookup_missing", mode.credential, identityRead, map[string]any{"operation": "lookup", "provider_subject": subject}, 200)
		require.Equal(t, false, missing["found"])
		bound := invoke(mode.name+"_identity_bind", mode.credential, identityManage, map[string]any{"operation": "bind", "customer_uuid": id, "provider_subject": subject}, 200)["item"]
		replay := invoke(mode.name+"_identity_bind_repeat", mode.credential, identityManage, map[string]any{"operation": "bind", "customer_uuid": id, "provider_subject": subject}, 200)["item"]
		require.Equal(t, bound, replay)
		found := invoke(mode.name+"_identity_lookup_found", mode.credential, identityRead, map[string]any{"operation": "lookup", "provider_subject": subject}, 200)
		require.Equal(t, true, found["found"])
		invoke(mode.name+"_identity_list", mode.credential, identityRead, map[string]any{"operation": "list_by_customer", "customer_uuid": id}, 200)
		original := invoke(mode.name+"_identity_create_existing", mode.credential, identityManage, map[string]any{"operation": "create_and_bind", "provider_subject": subject, "customer": map[string]any{"primary_email": "ignored@example.test"}}, 200)["item"].(map[string]any)
		require.Equal(t, id, original["customer_uuid"])
		fresh := invoke(mode.name+"_identity_create_new", mode.credential, identityManage, map[string]any{"operation": "create_and_bind", "provider_subject": subject + "fresh", "customer": map[string]any{"primary_email": "only@example.test"}}, 200)["item"].(map[string]any)
		freshID := fresh["customer_uuid"].(string)
		t.Cleanup(func() {
			require.NoError(t, db.Unscoped().Where("customer_uuid = ?", freshID).Delete(&customer.AuthIdentity{}).Error)
			require.NoError(t, db.Unscoped().Where("customer_uuid = ?", freshID).Delete(&customer.Contact{}).Error)
			require.NoError(t, db.Unscoped().Where("customer_uuid = ?", freshID).Delete(&customer.TenantMembership{}).Error)
			require.NoError(t, db.Unscoped().Where("uuid = ?", freshID).Delete(&customer.Account{}).Error)
		})
		require.NotEmpty(t, fresh["primary_contact_uuid"])
		again := invoke(mode.name+"_identity_create_new_repeat", mode.credential, identityManage, map[string]any{"operation": "create_and_bind", "provider_subject": subject + "fresh", "customer": map[string]any{"primary_email": "only@example.test"}}, 200)["item"]
		require.Equal(t, fresh, again)
		invoke(mode.name+"_identity_conflict", mode.credential, identityManage, map[string]any{"operation": "bind", "customer_uuid": uuid.NewString(), "provider_subject": subject}, 409)
	}
	invoke("identity_cross_tenant_bind", crossSTS, identityManage, map[string]any{"operation": "bind", "customer_uuid": id, "provider_subject": identitySubject + "foreign"}, 404)
	invoke("cross_tenant", crossSTS, manage, map[string]any{"operation": "update", "customer_uuid": id, "nickname": "forbidden"}, 404)
	var after customer.Contact
	require.NoError(t, db.Where("uuid = ?", before.UUID).First(&after).Error)
	require.Equal(t, before, after)
	var unchanged customer.AuthIdentity
	require.NoError(t, db.Where("uuid = ?", identity.UUID).First(&unchanged).Error)
	require.Equal(t, identity.Email, unchanged.Email)
	require.NoError(t, db.Unscoped().Delete(&grants[0]).Error)
	require.NoError(t, db.Unscoped().Delete(&grants[4]).Error)
	invoke("api_key_identity_no_grant", apiKey, identityManage, map[string]any{"operation": "bind", "customer_uuid": id, "provider_subject": identitySubject}, 403)
	grantStatus(apiKey, "not_granted")
	invoke("api_key_no_grant", apiKey, manage, map[string]any{"operation": "update", "customer_uuid": id, "nickname": "forbidden"}, 403)
	_, _, _, err = pluginService.Enable(ctx, tenants[0], plugin_mgr.Plugin{ID: pluginID, Version: "acceptance", RequiredCapabilities: []string{read, status}}, nil)
	require.NoError(t, err)
	grantStatus(sts, "not_granted")
	invoke("sts_identity_no_grant", sts, identityManage, map[string]any{"operation": "bind", "customer_uuid": id, "provider_subject": identitySubject}, 403)
	invoke("sts_no_grant", sts, manage, map[string]any{"operation": "update", "customer_uuid": id, "nickname": "forbidden"}, 403)
}
