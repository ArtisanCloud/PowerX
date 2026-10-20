package system_recovery

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIndependentProgressPushesWithoutCoreAndBindsTicketToOneJob(t *testing.T) {
	store, e := snapshots.NewCoordinatorStore(t.TempDir(), nil)
	require.NoError(t, e)
	id := uuid.NewString()
	job := snapshots.ManagedJob{ID: id, Kind: "cutover", Status: "running", Phase: "maintenance", RequestHash: "private-request-hash"}
	require.NoError(t, store.Save(job))
	token, e := store.ProgressToken(id)
	require.NoError(t, e)
	handler := (&Coordinator{Store: store}).StatusHandler()
	base := "/__powerx_recovery/jobs/" + id + "/events"
	for _, tc := range []struct {
		path, authorization string
		status              int
	}{{base, "", 401}, {base + "?token=" + token, "Bearer " + token, 401}, {"/__powerx_recovery/jobs/" + uuid.NewString() + "/events", "Bearer " + token, 401}} {
		request := httptest.NewRequest("GET", tc.path, nil)
		request.Header.Set("Authorization", tc.authorization)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, tc.status, response.Code)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	t.Logf("owned coordinator test server pid=%d address=%s; closed by test", os.Getpid(), server.Listener.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, e := http.NewRequestWithContext(ctx, "GET", server.URL+base, nil)
	require.NoError(t, e)
	request.Header.Set("Authorization", "Bearer "+token)
	response, e := server.Client().Do(request)
	require.NoError(t, e)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
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
	require.Contains(t, initial, "event: progress")
	require.Contains(t, initial, "maintenance")
	require.NotContains(t, initial, "private-request-hash")
	job.Phase = "writers_stopped"
	require.NoError(t, store.Save(job))
	update := read()
	require.Contains(t, update, "writers_stopped")
	require.NotContains(t, update, "private-request-hash")
	cancel()
	response.Body.Close()
}
