package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBackupEventStreamRequiresRootAndPushesCommittedChanges(t *testing.T) {
	svc, r, root, member := snapshotHTTPFixture(t)
	policy, e := svc.PutPolicy(reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true}), true), "", "Daily", snapshotops.Policy{Enabled: true, IntervalMinutes: 1440, RetentionDays: 7, RetentionCount: 7})
	require.NoError(t, e)
	base := "/custom/v2/admin/ops/backup"
	h := &handler{snapshots: svc}
	r.GET(base+"/events", RequireRestoreRoot(), h.BackupEvents)
	for _, tc := range []struct {
		token, query string
		status       int
	}{{"", "", 401}, {member, "", 403}, {root, "?path=/tmp", 400}} {
		request := httptest.NewRequest("GET", base+"/events"+tc.query, nil)
		request.Header.Set("Authorization", "Bearer "+tc.token)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, tc.status, response.Code)
	}
	server := httptest.NewServer(r)
	defer server.Close()
	t.Logf("owned test server pid=%d address=%s; closed by test", os.Getpid(), server.Listener.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, e := http.NewRequestWithContext(ctx, "GET", server.URL+base+"/events", nil)
	require.NoError(t, e)
	request.Header.Set("Authorization", "Bearer "+root)
	response, e := server.Client().Do(request)
	require.NoError(t, e)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	require.Contains(t, response.Header.Get("Content-Type"), "text/event-stream")
	require.Equal(t, "no", response.Header.Get("X-Accel-Buffering"))
	reader := bufio.NewReader(response.Body)
	read := func() string {
		var frame strings.Builder
		for {
			line, e := reader.ReadString('\n')
			require.NoError(t, e)
			frame.WriteString(line)
			if line == "\n" {
				return frame.String()
			}
		}
	}
	initial := read()
	require.Contains(t, initial, "event: invalidate")
	require.Contains(t, initial, "snapshots")
	require.NotContains(t, initial, "private-value")
	policyBody, e := json.Marshal(map[string]string{"policy_id": policy.ID})
	require.NoError(t, e)
	create, e := http.NewRequestWithContext(ctx, "POST", server.URL+base+"/system-snapshots", bytes.NewReader(policyBody))
	require.NoError(t, e)
	create.Header.Set("Authorization", "Bearer "+root)
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Idempotency-Key", uuid.NewString())
	created, e := server.Client().Do(create)
	require.NoError(t, e)
	defer created.Body.Close()
	require.Equal(t, 202, created.StatusCode)
	var receipt struct {
		Data struct {
			Snapshot struct {
				ID string `json:"id"`
			} `json:"snapshot"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(created.Body).Decode(&receipt))
	update := read()
	require.Contains(t, update, "snapshots")
	require.NotContains(t, update, "master.key")
	require.NotContains(t, update, "contents/")
	cancel()
	response.Body.Close()
}
