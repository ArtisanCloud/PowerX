package backup

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSystemRecoveryHTTPRootAndTypedInputs(t *testing.T) {
	_, router, root, member := snapshotHTTPFixture(t)
	store, e := snapshots.NewCoordinatorStore(t.TempDir(), []snapshots.RecoveryTarget{{ID: "local", Mode: "isolated", Environment: "dev"}, {ID: "formal", Mode: "formal", Environment: "prod", AllowOverwrite: true}})
	require.NoError(t, e)
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(store.Directory, "heartbeat.json"), struct {
		At time.Time `json:"at"`
	}{time.Now().UTC()}))
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(store.Directory, "active.json"), map[string]any{"generation": 1, "maintenance": false}))
	h := &handler{coordinator: store}
	base := "/custom/v2/admin/ops/backup/system-snapshots"
	group := router.Group(base, RequireRestoreRoot())
	group.GET("/recovery", h.RecoveryOverview)
	group.POST("/:snapshot_id/recovery", h.PrepareSystemRecovery)
	group.POST("/recovery/:recovery_id/cutover", h.CutoverSystemRecovery)
	group.POST("/recovery/:recovery_id/rollback", h.RollbackSystemRecovery)
	group.POST("/recovery/:recovery_id/cleanup", h.CleanupSystemRecovery)
	call := func(path, token, body, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Idempotency-Key", key)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, request)
		return out
	}
	id := uuid.NewString()
	path := base + "/" + id + "/recovery"
	payload := `{"confirmation":"` + id + `","generation":1}`
	require.Equal(t, 401, call(path, "", payload, uuid.NewString()).Code)
	require.Equal(t, 403, call(path, member, payload, uuid.NewString()).Code)
	require.Equal(t, 400, call(path, root, `{"confirmation":"`+id+`","target_id":"formal","dsn":"postgres://forbidden"}`, uuid.NewString()).Code)
	require.Equal(t, 400, call(path, root, `{"confirmation":"`+id+`","target_id":"unregistered"}`, uuid.NewString()).Code)
	require.Equal(t, 400, call(path, root, `{"confirmation":"`+id+`","target_id":"formal","restore_mode":"overwrite_current"}`, uuid.NewString()).Code)
	require.Equal(t, 400, call(path, root, `{"confirmation":"`+id+`","target_id":"formal","restore_mode":"unsafe"}`, uuid.NewString()).Code)
	key := uuid.NewString()
	prepared := 0
	store.PrepareSession = func(w http.ResponseWriter, r *http.Request, operator string) error {
		prepared++
		http.SetCookie(w, &http.Cookie{Name: "powerx_recovery_admin", Value: "fixture", HttpOnly: true})
		return nil
	}
	first := call(path, root, payload, key)
	require.Equal(t, 202, first.Code, first.Body.String())
	require.Equal(t, 1, prepared)
	require.Contains(t, first.Header().Get("Set-Cookie"), "HttpOnly")
	var receipt struct {
		Data struct {
			Recovery snapshots.ManagedJob `json:"recovery"`
			Token    string               `json:"progress_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &receipt))
	job := receipt.Data.Recovery
	require.Equal(t, "queued", job.Status)
	require.False(t, job.BusinessVerified)
	require.True(t, store.VerifyProgressToken(job.ID, receipt.Data.Token))
	replay := call(path, root, payload, key)
	var second struct {
		Data struct {
			Recovery snapshots.ManagedJob `json:"recovery"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(replay.Body.Bytes(), &second))
	require.Equal(t, job.ID, second.Data.Recovery.ID)
	for _, action := range []string{"rollback", "cleanup", "cutover"} {
		operation := base + "/recovery/" + job.ID + "/" + action
		require.Equal(t, 401, call(operation, "", `{}`, uuid.NewString()).Code)
		require.Equal(t, 403, call(operation, member, `{}`, uuid.NewString()).Code)
		require.Equal(t, 410, call(operation, root, `{"confirmation":"`+job.ID+`","generation":1}`, uuid.NewString()).Code)
	}
	require.Equal(t, "restore", job.Kind)
	require.Equal(t, "replace_instance", job.RestoreMode)
	require.EqualValues(t, 1, job.ExpectedGeneration)
}
