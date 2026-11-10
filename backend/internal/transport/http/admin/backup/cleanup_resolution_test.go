package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func exerciseCleanupResolutionHTTP(t *testing.T, db *gorm.DB, jobs *backupops.JobService, base, token string, artifact m.BackupArtifact) {
	t.Helper()
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	require.NoError(t, database.MigrateBackupCleanupResolutionModels(db))
	history, e := jobs.RetentionHistory(ctx, 1, 20, "all")
	require.NoError(t, e)
	if !history.Complete {
		_, e = jobs.ImportRetentionHistory(ctx, backupops.RetentionImportInput{RequestID: uuid.NewString(), ExpectedSnapshot: history.Snapshot, Reason: "owned resolution fixture legacy history coverage", Confirm: true}, func(string, backupops.RetentionImportReceipt) error { return nil })
		require.NoError(t, e)
	}
	if os.Getenv("POWERX_BACKUP_REVIEW_BROWSER_DIR") != "" {
		return
	}
	call := func(method, path, bearer string, body any) (int, string) {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req, e := http.NewRequest(method, base+"/custom/v2/admin/ops/backup"+path, bytes.NewReader(raw))
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		res, e := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		require.NoError(t, e)
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		return res.StatusCode, string(b)
	}
	path := fmt.Sprintf("/cleanup-reviews/%d", artifact.ID)
	status, body := call("GET", path+"/resolution-preview?expected_token="+artifact.CleanupToken, token, nil)
	require.Equal(t, 200, status)
	var env struct {
		Data backupops.CleanupResolutionPreview `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	require.True(t, env.Data.CanResolve, body)
	in := backupops.CleanupResolutionInput{RequestID: uuid.NewString(), ExpectedToken: artifact.CleanupToken, ReviewVersion: env.Data.ReviewVersion, HistoryDigest: env.Data.HistoryDigest, Reason: "owned HTTP non-deleting disposition", Confirm: true}
	status, _ = call("POST", path+"/resolutions", token, map[string]any{"request_id": in.RequestID, "expected_token": in.ExpectedToken, "review_version": in.ReviewVersion, "history_digest": in.HistoryDigest, "reason": in.Reason, "confirm": true, "verified": true})
	require.Equal(t, 400, status)
	for _, claims := range []reqctx.CoreXClaims{{UserID: 2, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, {UserID: 2, IsRoot: true, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString(), Platforms: []string{"api_key"}}} {
		bearer, e := auth.GenerateAccessJWT(claims, "owned-monitoring", []string{"admin"}, time.Hour, []byte("owned-monitor-http-no-deployment-key"))
		require.NoError(t, e)
		status, _ = call("GET", path+"/resolution-preview?expected_token="+artifact.CleanupToken, bearer, nil)
		require.Equal(t, 403, status)
		status, _ = call("POST", path+"/resolutions", bearer, in)
		require.Equal(t, 403, status)
	}
	var before []m.BackupCleanupRetention
	require.NoError(t, db.Order("id ASC").Find(&before).Error)
	status, body = call("POST", path+"/resolutions", token, in)
	require.Equal(t, 200, status)
	require.Contains(t, body, "event: accepted")
	require.Contains(t, body, "event: complete")
	require.Contains(t, body, `"independent_complete":true`)
	status, body = call("POST", path+"/resolutions", token, in)
	require.Equal(t, 200, status)
	require.NotContains(t, body, "event: accepted")
	status, body = call("GET", "/cleanup-resolutions/"+in.RequestID, token, nil)
	require.Equal(t, 200, status)
	require.Contains(t, body, `"state":"retained"`)
	status, body = call("GET", path+"/resolutions?expected_token="+artifact.CleanupToken, token, nil)
	require.Equal(t, 200, status)
	require.Contains(t, body, in.RequestID)
	var after []m.BackupCleanupRetention
	require.NoError(t, db.Order("id ASC").Find(&after).Error)
	require.Equal(t, before, after)
	var a m.BackupArtifact
	require.NoError(t, db.First(&a, artifact.ID).Error)
	require.Equal(t, "retained", a.CleanupState)
	require.Equal(t, artifact.CleanupToken, a.CleanupToken)
}
