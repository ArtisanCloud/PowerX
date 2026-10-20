package system_recovery

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Runs against the real owned PG/Core/Web/media fixture. Both the ordinary Core
// and Web are stopped before the administrator opens the independent UI.
func exerciseOfflineNativePortal(t *testing.T, ctx context.Context, c *Coordinator, n *Native, d Deployment, snapshotID string) snapshots.ManagedJob {
	t.Helper()
	require.NoError(t, n.Stop(ctx, d))
	_, err := c.EnsureRecoveryAccess()
	require.NoError(t, err)
	key, err := c.readRecoveryKey()
	require.NoError(t, err)
	keyDigest := sha256.Sum256([]byte(key))
	server := httptest.NewServer(http.HandlerFunc(c.gateway))
	defer server.Close()
	newClient := func() *http.Client {
		jar, err := cookiejar.New(nil)
		require.NoError(t, err)
		return &http.Client{Jar: jar, Timeout: 10 * time.Second}
	}
	client := newClient()
	request := func(client *http.Client, method, path, csrf string, body any) *http.Response {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, err = json.Marshal(body)
			require.NoError(t, err)
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Accept", "text/html")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			req.Header.Set("X-PowerX-Recovery-CSRF", csrf)
		}
		req.Header.Set("Idempotency-Key", uuid.NewString())
		response, err := client.Do(req)
		require.NoError(t, err)
		return response
	}
	page := request(client, "GET", "/ops/backup", "", nil)
	body, err := io.ReadAll(page.Body)
	page.Body.Close()
	require.NoError(t, err)
	require.Equal(t, 200, page.StatusCode)
	require.Contains(t, string(body), "恢复控制台")
	login := func(client *http.Client) string {
		t.Helper()
		response := request(client, "POST", recoveryEntry+"session", "", map[string]string{"key": key})
		defer response.Body.Close()
		require.Equal(t, 200, response.StatusCode)
		var data struct {
			CSRF string `json:"csrf"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&data))
		require.NotEmpty(t, data.CSRF)
		return data.CSRF
	}
	csrf := login(client)
	active, err := c.Active()
	require.NoError(t, err)
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "heartbeat.json"), map[string]any{"at": time.Now()}))
	response := request(client, "POST", recoveryEntry+"restore", csrf, map[string]any{"snapshot_id": snapshotID, "confirmation": snapshotID, "generation": active.Generation})
	var receipt struct {
		Recovery snapshots.ManagedJob `json:"recovery"`
		Ticket   string               `json:"progress_token"`
	}
	require.Equal(t, 202, response.StatusCode)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
	response.Body.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	prior := c.Checkpoint
	c.Checkpoint = func(phase string) error {
		if prior != nil {
			if err := prior(phase); err != nil {
				return err
			}
		}
		if phase == "maintenance" {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	defer func() { c.Checkpoint = prior }()
	done := make(chan error, 1)
	go func() { done <- c.RunOnce(ctx) }()
	select {
	case <-entered:
	case err := <-done:
		require.NoError(t, err)
		t.Fatal("restore did not reach maintenance")
	case <-time.After(3 * time.Minute):
		t.Fatal("maintenance checkpoint timed out")
	}
	// Refresh the ordinary URL, then discard the old browser session and sign in
	// again using the key. This must not call the stopped Core for authentication.
	page = request(client, "GET", "/ops/backup", "", nil)
	body, err = io.ReadAll(page.Body)
	page.Body.Close()
	require.NoError(t, err)
	require.Equal(t, 200, page.StatusCode)
	require.Contains(t, string(body), "恢复控制台")
	client = newClient()
	login(client)
	response = request(client, "GET", recoveryEntry+"catalog", "", nil)
	require.Equal(t, 200, response.StatusCode)
	var catalog struct {
		Maintenance bool              `json:"maintenance"`
		Tokens      map[string]string `json:"progress_tokens"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&catalog))
	response.Body.Close()
	require.True(t, catalog.Maintenance)
	require.NotEmpty(t, catalog.Tokens[receipt.Recovery.ID])
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+recoveryEntry+"jobs/"+receipt.Recovery.ID+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+catalog.Tokens[receipt.Recovery.ID])
	response, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	scanner := bufio.NewScanner(response.Body)
	found := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data:") {
			found = true
			require.Contains(t, scanner.Text(), "maintenance")
			break
		}
	}
	response.Body.Close()
	require.True(t, found)
	once.Do(func() { close(release) })
	require.NoError(t, <-done)
	job, err := c.Store.Get(receipt.Recovery.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", job.Status, job.Error)
	currentKey, err := c.readRecoveryKey()
	require.NoError(t, err)
	require.True(t, keyDigest == sha256.Sum256([]byte(currentKey)), "recovery credential must survive application replacement")
	return job
}
