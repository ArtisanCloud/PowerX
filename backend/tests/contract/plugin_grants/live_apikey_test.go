package plugingrants_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLiveAPIKeyGrantStatusMatchesActualHost(t *testing.T) {
	dsn, base := os.Getenv("POWERX_LIVE_TEST_DSN"), os.Getenv("POWERX_LIVE_TEST_URL")
	if dsn == "" || base == "" {
		t.Skip("live_acceptance_not_configured")
	}
	require.True(t, strings.HasPrefix(base, "http://127.0.0.1:"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var tenant string
	require.NoError(t, db.Raw("SELECT uuid::text FROM iam_tenant WHERE status=1 AND deleted_at IS NULL ORDER BY uuid LIMIT 1").Scan(&tenant).Error)
	require.NotEmpty(t, tenant)
	profile := iam.APIKeyProfile{TenantUUID: tenant, Key: "acceptance-" + uuid.NewString(), Name: "acceptance", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	t.Cleanup(func() { require.NoError(t, db.Delete(&profile).Error) })
	rawKey := uuid.NewString() + uuid.NewString()
	digest := sha256.Sum256([]byte(rawKey))
	hash := hex.EncodeToString(digest[:])
	iamKey := iam.APIKey{TenantUUID: tenant, ProfileID: profile.ID, KeyHash: hash}
	require.NoError(t, db.Create(&iamKey).Error)
	t.Cleanup(func() { require.NoError(t, db.Delete(&iamKey).Error) })
	expires := time.Now().Add(5 * time.Minute)
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: profile.ID, Name: "acceptance", KeyPrefix: "acceptance", KeyHash: hash, Status: "active", ExpiresAt: &expires}
	require.NoError(t, db.Create(&key).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(&key).Error) })
	grant := gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"}
	directory := gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.iam.directory.catalog.read", Action: "read", ResourceType: "api", ResourcePattern: "directory-catalog", Effect: "allow"}
	require.NoError(t, db.Create(&grant).Error)
	require.NoError(t, db.Create(&directory).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Unscoped().Where("api_key_uuid = ?", key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	})
	client := http.Client{Timeout: 10 * time.Second}
	call := func(path string, body any, want int) map[string]any {
		method := "GET"
		var raw []byte
		if body != nil {
			method = "POST"
			raw, err = json.Marshal(body)
			require.NoError(t, err)
		}
		req, err := http.NewRequest(method, base+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set("Authorization", "ApiKey "+rawKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		var result map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		require.Equal(t, want, resp.StatusCode, "path=%s reason=%v", path, result["reason_code"])
		return result
	}
	query := map[string]any{"capability_ids": []string{"com.corex.iam.directory.read"}}
	status := func(want string) {
		result := call("/api/v1/tenant/capabilities:grant-status", query, 200)
		data := result["data"].(map[string]any)
		items := data["items"].([]any)
		require.Len(t, items, 1)
		require.Equal(t, want, items[0].(map[string]any)["status"])
	}
	status("granted")
	call("/api/v1/tenant/iam/tenant", nil, 200)
	require.NoError(t, db.Unscoped().Delete(&directory).Error)
	status("not_granted")
	denied := call("/api/v1/tenant/iam/tenant", nil, 403)
	require.Equal(t, "IAM_FORBIDDEN", denied["reason_code"])
	require.NoError(t, db.Unscoped().Delete(&grant).Error)
	call("/api/v1/tenant/capabilities:grant-status", query, 403)
	t.Log("live_api_key_exact_grant_and_revocation_match_host_passed")
}
