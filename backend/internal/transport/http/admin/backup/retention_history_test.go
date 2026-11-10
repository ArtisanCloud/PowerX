package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func exerciseRetentionHistoryHTTP(t *testing.T, db *gorm.DB, jobs *backupops.JobService, base, token string, artifact m.BackupArtifact) {
	t.Helper()
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	view, e := jobs.InspectCleanupReview(ctx, artifact.ID, artifact.CleanupToken)
	require.NoError(t, e)
	in := backupops.CleanupRetainInput{RequestID: uuid.NewString(), ExpectedToken: artifact.CleanupToken, ReviewVersion: view.Item.ReviewVersion, Reason: "owned legacy history import", Confirm: true}
	raw, e := json.Marshal(struct {
		Artifact uint64
		Input    backupops.CleanupRetainInput
	}{artifact.ID, in})
	require.NoError(t, e)
	sum := sha256.Sum256(raw)
	row := m.BackupCleanupRetention{RequestID: uuid.MustParse(in.RequestID), ArtifactID: artifact.ID, JobID: artifact.JobID, CleanupToken: in.ExpectedToken, ActorID: 9, InputDigest: hex.EncodeToString(sum[:]), ReviewVersion: in.ReviewVersion, ScopeFingerprint: (gov.ControlScope{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http"}).ID(), Reason: in.Reason, State: "checking"}
	require.NoError(t, db.Create(&row).Error)
	if os.Getenv("POWERX_BACKUP_REVIEW_BROWSER_DIR") != "" {
		return
	}
	call := func(method, path, bearer string, input any) (int, string) {
		var raw []byte
		if input != nil {
			raw, _ = json.Marshal(input)
		}
		req, e := http.NewRequest(method, base+"/custom/v2/admin/ops/backup"+path, bytes.NewReader(raw))
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		res, e := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		require.NoError(t, e)
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		return res.StatusCode, string(b)
	}
	status, body := call("GET", "/cleanup-retention-history", token, nil)
	require.Equal(t, 200, status)
	var envelope struct {
		Data backupops.RetentionHistory `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &envelope))
	require.Greater(t, envelope.Data.Missing, 0)
	input := backupops.RetentionImportInput{RequestID: uuid.NewString(), ExpectedSnapshot: envelope.Data.Snapshot, Reason: "owned HTTP explicit import", Confirm: true}
	for _, path := range []string{"/cleanup-retention-history?scope=foreign", "/cleanup-retention-history?page=1&page=2", "/cleanup-retention-history?page_size=51", "/cleanup-retention-imports?verified=true"} {
		status, _ = call("GET", path, token, nil)
		require.Equal(t, 400, status)
	}
	status, _ = call("POST", "/cleanup-retention-imports", token, map[string]any{"request_id": input.RequestID, "expected_snapshot": input.ExpectedSnapshot, "reason": input.Reason, "confirm": true, "verified": true})
	require.Equal(t, 400, status)
	nonRoot, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{UserID: 9, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, "owned-monitoring", []string{"admin"}, time.Hour, []byte("owned-monitor-http-no-deployment-key"))
	require.NoError(t, e)
	for _, path := range []string{"/cleanup-retention-history", "/cleanup-retention-imports", "/cleanup-retention-imports/" + input.RequestID} {
		status, _ = call("GET", path, nonRoot, nil)
		require.Equal(t, 403, status)
	}
	status, _ = call("POST", "/cleanup-retention-imports", nonRoot, input)
	require.Equal(t, 403, status)
	status, body = call("POST", "/cleanup-retention-imports", token, input)
	require.Equal(t, 200, status)
	require.Contains(t, body, "event: accepted")
	require.Contains(t, body, "event: complete")
	require.Contains(t, body, `"complete":true`)
	status, body = call("POST", "/cleanup-retention-imports", token, input)
	require.Equal(t, 200, status)
	require.NotContains(t, body, "event: accepted")
	status, body = call("GET", "/cleanup-retention-imports/"+input.RequestID, token, nil)
	require.Equal(t, 200, status)
	require.Contains(t, body, `"complete":true`)
	require.Contains(t, body, `"unknown":1`)
	var after m.BackupCleanupRetention
	require.NoError(t, db.First(&after, row.ID).Error)
	require.Equal(t, uint64(9), after.ActorID)
	require.Equal(t, "checking", after.State)
}
