package system_recovery

import (
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestRecoveryProgressTicketIsReadOnlyAndJobScoped(t *testing.T) {
	store, e := snapshots.NewCoordinatorStore(t.TempDir(), nil)
	require.NoError(t, e)
	id := uuid.NewString()
	require.NoError(t, store.Save(snapshots.ManagedJob{ID: id, Kind: "cutover", Status: "running", Phase: "maintenance"}))
	token, e := store.ProgressToken(id)
	require.NoError(t, e)
	server := (&Coordinator{Store: store}).StatusHandler()
	call := func(method, path, authorization string) int {
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response.Code
	}
	path := "/__powerx_recovery/jobs/" + id
	require.Equal(t, 401, call("GET", path, ""))
	require.Equal(t, 200, call("GET", path, "Bearer "+token))
	require.Equal(t, 401, call("GET", "/__powerx_recovery/jobs/"+uuid.NewString(), "Bearer "+token))
	require.Equal(t, 401, call("GET", path+"?token="+token, "Bearer "+token))
	require.Equal(t, 405, call("POST", path, "Bearer "+token))
	require.Equal(t, 404, call("POST", "/__powerx_recovery/cutover", "Bearer "+token))
}
