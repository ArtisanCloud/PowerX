package backup

import (
	"encoding/json"
	"fmt"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Runs inside the existing real PG/JWT/SSE fixture, using only its files.
func exerciseCleanupVerificationHTTP(t *testing.T, db *gorm.DB, base, token, path string, a modelops.BackupArtifact, policy uint64, content []byte) {
	t.Helper()
	raw, e := json.Marshal(map[string]any{"contract": "powerx.database-backup/v1", "source_database": "owned", "job_id": a.JobID, "policy_id": policy, "file": filepath.Base(path), "created_at": time.Now().UTC(), "size_bytes": a.SizeBytes, "sha256": a.Checksum, "format": "postgresql-custom"})
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path+".json", raw, 0600))
	require.NoError(t, os.WriteFile(path+".sha256", []byte(a.Checksum+"  "+filepath.Base(path)+"\n"), 0600))
	url := fmt.Sprintf("%s/custom/v2/admin/ops/backup/cleanup-reviews/%d/verification?expected_token=%s", base, a.ID, a.CleanupToken)
	read := func() string {
		req, e := http.NewRequest("GET", url, nil)
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+token)
		res, e := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		require.NoError(t, e)
		defer res.Body.Close()
		require.Equal(t, 200, res.StatusCode)
		require.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
		body, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		require.NotContains(t, string(body), path)
		return string(body)
	}
	body := read()
	require.Contains(t, body, "event: progress")
	require.Contains(t, body, "event: complete")
	require.Contains(t, body, `"state":"verified"`)
	require.Contains(t, body, `"content_verified":true`)
	require.Contains(t, body, `"cleanup_unchanged":true`)
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", len(content))), 0600))
	body = read()
	require.Contains(t, body, `"state":"checksum_mismatch"`)
	require.NotContains(t, body, `"content_verified":true`)
	require.NoError(t, os.WriteFile(path, content, 0600))
	var current modelops.BackupArtifact
	require.NoError(t, db.First(&current, a.ID).Error)
	require.Equal(t, a.CleanupState, current.CleanupState)
	require.Equal(t, a.CleanupToken, current.CleanupToken)
}
