package backup

import (
	"encoding/json"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
	"time"
)

func exerciseQuarantineInventoryHTTP(t *testing.T, base, token, key string) {
	t.Helper()
	prefix := base + "/custom/v2/admin/ops/backup/cleanup-quarantine-inventory"
	call := func(path, bearer string) (int, string) {
		req, e := http.NewRequest("GET", prefix+path, nil)
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+bearer)
		res, e := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		require.NoError(t, e)
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		return res.StatusCode, string(b)
	}
	status, body := call("", token)
	require.Equal(t, 200, status)
	var list struct {
		Data backupops.QuarantineInventoryPage
	}
	require.NoError(t, json.Unmarshal([]byte(body), &list))
	require.Equal(t, 1, list.Data.Total)
	require.Equal(t, key, list.Data.Items[0].RequestID)
	status, body = call("/"+key, token)
	require.Equal(t, 200, status)
	var detail struct {
		Data backupops.QuarantineInventoryItem
	}
	require.NoError(t, json.Unmarshal([]byte(body), &detail))
	require.True(t, detail.Data.CanInspect)
	path := "/" + key + "/inspection?expected_snapshot=" + detail.Data.Snapshot
	status, body = call(path, token)
	require.Equal(t, 200, status)
	require.Contains(t, body, "event: complete")
	require.Contains(t, body, `"quarantine_unchanged":true`)
	require.Contains(t, body, `"valid_backup_proof":false`)
	require.Contains(t, body, `"currently_quarantined":true`)
	for _, query := range []string{"?page=0", "?page=01", "?page=1&page=2", "?page_size=101", "?current=other", "?sort=DROP", "?direction=sideways", "?free_path=/tmp/private", "/" + key + "?q=unknown", "/" + key + "/inspection?expected_snapshot=bad", path + "&extra=1"} {
		status, _ = call(query, token)
		require.Equal(t, 400, status, query)
	}
	status, body = call("?current=not_quarantined", token)
	require.Equal(t, 200, status)
	require.Contains(t, body, `"total":0`)
	for _, claims := range []reqctx.CoreXClaims{{UserID: 2}, {UserID: 2, IsRoot: true, Platforms: []string{"api_key"}}, {UserID: 2, IsRoot: true, Platforms: []string{"service_actor"}}} {
		claims.TenantUUID = uuid.NewString()
		claims.MemberUUID = uuid.NewString()
		bearer, e := auth.GenerateAccessJWT(claims, "owned-monitoring", []string{"admin"}, time.Hour, []byte("owned-monitor-http-no-deployment-key"))
		require.NoError(t, e)
		for _, p := range []string{"", "/" + key, path} {
			status, _ = call(p, bearer)
			require.Equal(t, 403, status)
		}
	}
}
