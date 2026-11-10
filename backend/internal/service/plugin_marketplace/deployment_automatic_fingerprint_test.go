package plugin_marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/internal/productinfo"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 可由 Marketplace 隔离测试提供真实 handler；默认使用本测试私有 HTTP 服务。
func TestDevelopmentAutomaticFingerprintSurvivesWorkerRestart(t *testing.T) {
	info := productinfo.LoadExecutable()
	require.Equal(t, "development", info.State)
	require.NotNil(t, info.BuildInfo)
	root := os.Getenv("POWERX_DEPLOYMENT_FINGERPRINT_TEST_URL")
	var registrations atomic.Int32
	if root == "" {
		id := uuid.NewString()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			var data any
			switch r.URL.Path {
			case "/api/v1/host/instances/discovery":
				data = map[string]any{"service": "powerx-marketplace", "protocol": market.AutomaticRegistrationProtocol, "features": []string{market.DeploymentFingerprintFeature}}
			case "/api/v1/host/instances/register", "/api/v1/host/instances/heartbeat":
				var body struct {
					BuildInfo *productinfo.BuildInfo `json:"build_info"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.BuildInfo == nil || *body.BuildInfo != *info.BuildInfo {
					t.Error("automatic registration and heartbeat must include the actual development fingerprint")
					w.WriteHeader(400)
					return
				}
				data = map[string]any{"server_time": "2026-10-23T00:00:00.000Z", "site_challenges": []any{}, "status": "recorded"}
				if r.URL.Path == "/api/v1/host/instances/register" {
					registrations.Add(1)
					data.(map[string]any)["protocol"] = market.AutomaticRegistrationProtocol
					data.(map[string]any)["instance"] = map[string]string{"id": id}
				}
			default:
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
		}))
		defer server.Close()
		root = server.URL + "/api/v1"
	}
	dir := t.TempDir()
	var original market.DistributionState
	for run := 0; run < 2; run++ {
		client := market.NewClient(root, "", nil)
		svc := NewService(client, "1.0.0", nil)
		require.NoError(t, svc.ConfigureDeployment(info, ""))
		store, err := market.OpenDistributionStore(dir, client.SourceID(), "automatic-fingerprint-environment")
		require.NoError(t, err)
		svc.ConfigureDistribution(store, "", time.Hour, nil)
		t.Cleanup(svc.CloseDistribution)
		svc.StartDistribution(context.Background())
		require.Eventually(t, func() bool { return store.Identity().LastHeartbeat.After(original.LastHeartbeat) }, 8*time.Second, 10*time.Millisecond)
		current := store.Identity()
		if run == 0 {
			original = current
		} else {
			require.Equal(t, original.InstanceID, current.InstanceID)
			require.Equal(t, original.InstanceToken, current.InstanceToken)
		}
		svc.CloseDistribution()
	}
	if os.Getenv("POWERX_DEPLOYMENT_FINGERPRINT_TEST_URL") == "" {
		require.EqualValues(t, 1, registrations.Load())
	}
	t.Logf("automatic development fingerprint reported on startup and restart: component=%s release=%s build_id=%s", info.BuildInfo.Component, info.BuildInfo.ReleaseID, info.BuildInfo.BuildID)
}
