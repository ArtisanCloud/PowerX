package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
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

func exerciseCleanupRetentionHTTP(t *testing.T, db *gorm.DB, svc *gov.Service, policies *backup.PolicyService, jobs *backup.JobService, base, token string, artifact m.BackupArtifact) {
	t.Helper()
	require.NoError(t, database.MigrateBackupCleanupRetentionModels(db))
	require.NoError(t, db.AutoMigrate(&m.RestoreDrillRecord{}))
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	policy := m.BackupPolicy{}
	backup.ApplySettings(&policy, backup.PolicySettings{Name: "Owned retention", Schedule: "1h", RetentionCount: 3, RetentionDays: 7, RetentionMode: "count", Timezone: "Asia/Shanghai", DrillIntervalDays: 7, TargetRef: "local_dump"})
	require.NoError(t, db.Create(&policy).Error)
	require.Equal(t, uint64(1), policy.ID)
	conf, e := policies.ControlConfiguration(ctx)
	require.NoError(t, e)
	_, e = policies.AdoptConfiguration(ctx, backup.PolicyAdoption{RequestID: uuid.NewString(), LegacyDigest: conf.LegacyDigest, Confirm: true})
	require.NoError(t, e)
	require.Eventually(t, func() bool { v, e := policies.ControlConfiguration(ctx); return e == nil && v.State == "applied" }, 5*time.Second, 25*time.Millisecond)
	_, e = svc.ChangeExecution(ctx, gov.ExecutionChange{RequestID: uuid.NewString(), ExpectedRevision: 0, Action: "pause", Reason: "owned retention acceptance", Confirm: true})
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		v, e := svc.ExecutionConfiguration(ctx)
		return e == nil && v.Confirmed && v.Mode == "paused"
	}, 5*time.Second, 25*time.Millisecond)
	if os.Getenv("POWERX_BACKUP_REVIEW_BROWSER_DIR") != "" {
		return
	}
	review, e := jobs.InspectCleanupReview(ctx, artifact.ID, artifact.CleanupToken)
	require.NoError(t, e)
	in := backup.CleanupRetainInput{RequestID: uuid.NewString(), ExpectedToken: artifact.CleanupToken, ReviewVersion: review.Item.ReviewVersion, Reason: "keep owned HTTP backup", Confirm: true}
	path := fmt.Sprintf("%s/custom/v2/admin/ops/backup/cleanup-reviews/%d/retentions", base, artifact.ID)
	call := func(method, url string, input any) (int, string) {
		var raw []byte
		if input != nil {
			raw, _ = json.Marshal(input)
		}
		req, e := http.NewRequest(method, url, bytes.NewReader(raw))
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, e := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		require.NoError(t, e)
		defer res.Body.Close()
		body, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		return res.StatusCode, string(body)
	}
	status, body := call("POST", path, in)
	require.Equal(t, 200, status)
	require.Contains(t, body, "event: accepted")
	require.Contains(t, body, "event: complete")
	require.Contains(t, body, `"state":"retained"`)
	require.Contains(t, body, `"cleanup_lock_preserved":true`)
	status, body = call("GET", base+"/custom/v2/admin/ops/backup/cleanup-retentions/"+in.RequestID, nil)
	require.Equal(t, 200, status)
	require.Contains(t, body, `"state":"retained"`)
	status, body = call("POST", path, in)
	require.Equal(t, 200, status)
	require.NotContains(t, body, "event: accepted")
	require.Contains(t, body, `"state":"retained"`)
	altered := in
	altered.Reason = "changed payload"
	status, _ = call("POST", path, altered)
	require.Equal(t, 409, status)
	status, _ = call("POST", path, map[string]any{"request_id": uuid.NewString(), "expected_token": in.ExpectedToken, "review_version": in.ReviewVersion, "reason": "owned", "confirm": true, "verified": true})
	require.Equal(t, 400, status)
	var current m.BackupArtifact
	require.NoError(t, db.First(&current, artifact.ID).Error)
	require.Equal(t, artifact.CleanupState, current.CleanupState)
	require.Equal(t, artifact.CleanupToken, current.CleanupToken)
	var count int64
	require.NoError(t, db.Model(&m.BackupCleanupRetention{}).Where("request_id = ?", in.RequestID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
