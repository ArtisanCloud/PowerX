package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const fixtureCredential = "pxm_instance_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const fixtureInstance = "b09b9208-157d-4ad3-b951-5bfa2c13ab5c"
const fixtureFingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func writeDeploymentFixture(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
}
func fixtureDiscovery(support bool) map[string]any {
	data := map[string]any{"service": "powerx-marketplace", "protocol": AutomaticRegistrationProtocol}
	if support {
		data["features"] = []string{DeploymentFingerprintFeature}
	}
	return data
}
func fixtureSuccess(register, extended, missing bool) map[string]any {
	data := map[string]any{"status": "recorded"}
	if register {
		data = map[string]any{"instance": map[string]any{"id": fixtureInstance}, "protocol": AutomaticRegistrationProtocol}
	}
	if extended && !missing {
		data["server_time"] = "2026-10-23T00:00:00.000Z"
		data["site_challenges"] = []any{}
	}
	return data
}

func TestDeploymentSharedHTTPCompatibilityFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "contracts", "deployment-fingerprint", "v1", "compatibility-cases.json"))
	require.NoError(t, err)
	var data struct {
		Cases []struct {
			Name             string
			Initial          bool `json:"initial_support"`
			After            bool `json:"support_after_error"`
			Status           int  `json:"post_status"`
			DiscoveryFailure bool `json:"rediscovery_failure"`
			Missing          bool `json:"missing_response_fields"`
			Error            bool `json:"expect_error"`
			Posts            int  `json:"expected_posts"`
			Extended         bool `json:"expected_extended_result"`
			RetryStatus      int  `json:"retry_status"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &data))
	for _, row := range data.Cases {
		for _, register := range []bool{false, true} {
			t.Run(row.Name+map[bool]string{false: "/heartbeat", true: "/register"}[register], func(t *testing.T) {
				var mu sync.Mutex
				discoveries := 0
				var bodies []map[string]any
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					if r.Method == http.MethodGet {
						discoveries++
						if discoveries > 1 && row.DiscoveryFailure {
							w.WriteHeader(503)
							return
						}
						support := row.Initial
						if discoveries > 1 {
							support = row.After
						}
						writeDeploymentFixture(w, fixtureDiscovery(support))
						return
					}
					require.Equal(t, "Bearer "+fixtureCredential, r.Header.Get("Authorization"))
					require.Equal(t, fixtureFingerprint, r.Header.Get("X-PowerX-Fingerprint"))
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					bodies = append(bodies, body)
					_, extended := body["features"]
					if !row.Initial || row.Name == "online-rollback" {
						encoded, _ := json.Marshal(body)
						decoder := json.NewDecoder(bytes.NewReader(encoded))
						decoder.DisallowUnknownFields()
						var old struct {
							PowerXVersion string `json:"powerx_version"`
							Fingerprint   string `json:"fingerprint_sha256"`
							OS            string `json:"os"`
							Arch          string `json:"arch"`
							Name          string `json:"name"`
						}
						if err := decoder.Decode(&old); err != nil {
							w.WriteHeader(400)
							_, _ = w.Write([]byte(`{"error":"invalid_input"}`))
							return
						}
					}
					if len(bodies) == 1 && row.Status != 200 {
						w.WriteHeader(row.Status)
						_ = json.NewEncoder(w).Encode(map[string]any{"code": row.Status, "error": "invalid_input", "message": "private body must not escape"})
						return
					}
					if len(bodies) > 1 && row.RetryStatus != 0 {
						w.WriteHeader(row.RetryStatus)
						_, _ = w.Write([]byte(`{"error":"invalid_input"}`))
						return
					}
					writeDeploymentFixture(w, fixtureSuccess(register, extended, row.Missing))
				}))
				defer server.Close()
				client := NewClient(server.URL+"/api/v1", "", server.Client())
				client.SetInstance(fixtureCredential, fixtureFingerprint)
				require.NoError(t, client.RefreshDeploymentCapabilities(context.Background()))
				site := "https://admin.example.com/console/"
				decl := &DeploymentDeclaration{PublicSiteURL: &site, BuildInfo: &BuildDeclaration{Schema: "powerx.build-fingerprint/v1", Component: "backend", ReleaseID: "1.0.0", SourceCommit: "fixture", BuildID: strings.Repeat("a", 64)}}
				var extended bool
				if register {
					out, err := client.RegisterAutomaticWithDeployment(context.Background(), fixtureCredential, fixtureFingerprint, "v1.0.0", "PowerX fixture", decl)
					extended = out.Extended
					if row.Error {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
						require.Equal(t, fixtureInstance, out.Instance.ID)
					}
				} else {
					out, err := client.HeartbeatWithDeployment(context.Background(), "v1.0.0", decl)
					extended = out.Extended
					if row.Error {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				}
				require.Equal(t, row.Extended, extended)
				mu.Lock()
				defer mu.Unlock()
				require.Len(t, bodies, row.Posts)
				if row.Name == "online-rollback" {
					require.Equal(t, 2, discoveries)
					for _, key := range []string{"features", "build_info", "public_site_url"} {
						require.NotContains(t, bodies[1], key)
					}
					base := bodies[0]
					delete(base, "features")
					delete(base, "build_info")
					delete(base, "public_site_url")
					require.Equal(t, base, bodies[1], "回退保留原身份和基础字段")
				}
				if row.Status != 400 && row.Status != 200 {
					require.Equal(t, 1, discoveries)
				}
			})
		}
	}
}

func TestDeploymentConcurrentRollbackSharesOneRediscovery(t *testing.T) {
	const n = 12
	var discoveries, extendedPosts, legacyPosts atomic.Int32
	arrived, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			count := discoveries.Add(1)
			writeDeploymentFixture(w, fixtureDiscovery(count == 1))
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["features"]; ok {
			if extendedPosts.Add(1) == n {
				close(arrived)
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_input"}`))
			return
		}
		legacyPosts.Add(1)
		writeDeploymentFixture(w, fixtureSuccess(false, false, false))
	}))
	t.Cleanup(func() { once.Do(func() { close(release) }); server.Close() })
	client := NewClient(server.URL, "", server.Client())
	client.SetInstance(fixtureCredential, fixtureFingerprint)
	require.NoError(t, client.RefreshDeploymentCapabilities(context.Background()))
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := client.HeartbeatWithDeployment(ctx, "1.0.0", &DeploymentDeclaration{})
			results <- err
		}()
	}
	select {
	case <-arrived:
	case <-time.After(3 * time.Second):
		once.Do(func() { close(release) })
		t.Fatal("concurrent requests did not arrive")
	}
	once.Do(func() { close(release) })
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, discoveries.Load())
	require.EqualValues(t, n, legacyPosts.Load())
}

func TestDeploymentParameterFailuresBackOffWithoutHidingErrors(t *testing.T) {
	var discoveries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			discoveries.Add(1)
			writeDeploymentFixture(w, fixtureDiscovery(true))
			return
		}
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_input","message":"https://private.example/credential=secret"}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "", server.Client())
	for i := 0; i < 4; i++ {
		_, err := client.HeartbeatWithDeployment(context.Background(), "1.0.0", &DeploymentDeclaration{})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private.example")
		require.NotContains(t, err.Error(), "secret")
	}
	require.EqualValues(t, 2, discoveries.Load(), "重复参数失败不得形成重新发现热循环")
}

func TestDeploymentURLClearingAndCapabilityRecovery(t *testing.T) {
	var supported atomic.Bool
	supported.Store(false)
	var mu sync.Mutex
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeDeploymentFixture(w, fixtureDiscovery(supported.Load()))
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		_, extended := body["features"]
		writeDeploymentFixture(w, fixtureSuccess(false, extended, false))
	}))
	defer server.Close()
	client := NewClient(server.URL, "", server.Client())
	client.SetInstance(fixtureCredential, fixtureFingerprint)
	_, err := client.HeartbeatWithDeployment(context.Background(), "1.0.0", &DeploymentDeclaration{})
	require.NoError(t, err)
	supported.Store(true)
	require.NoError(t, client.RefreshDeploymentCapabilities(context.Background()))
	cleared := ""
	out, err := client.HeartbeatWithDeployment(context.Background(), "1.0.0", &DeploymentDeclaration{PublicSiteURL: &cleared})
	require.NoError(t, err)
	require.True(t, out.Extended)
	_, err = client.HeartbeatWithDeployment(context.Background(), "1.0.0", &DeploymentDeclaration{})
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.NotContains(t, bodies[0], "features")
	require.Equal(t, "", bodies[1]["public_site_url"])
	require.NotContains(t, bodies[2], "public_site_url")
}

func TestDeploymentMalformedSuccessDoesNotReplayPOST(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeDeploymentFixture(w, fixtureDiscovery(true))
			return
		}
		posts.Add(1)
		writeDeploymentFixture(w, map[string]any{"status": "recorded", "server_time": 123, "site_challenges": []any{}})
	}))
	defer server.Close()
	client := NewClient(server.URL, "", server.Client())
	_, err := client.HeartbeatWithDeployment(context.Background(), "1.0.0", &DeploymentDeclaration{})
	require.Error(t, err)
	require.EqualValues(t, 1, posts.Load())
	client.deployment.mu.Lock()
	defer client.deployment.mu.Unlock()
	require.False(t, client.deployment.known)
	require.False(t, client.deployment.supported)
}

func TestDeploymentResponseTimeAndIdentityValidation(t *testing.T) {
	site := "https://admin.example.com/console/"
	serverTime := "2026-10-23T00:00:00.000Z"
	challenge := SiteChallenge{Schema: "powerx.instance-proof/v1", ChallengeID: fixtureInstance, SiteURL: site, Nonce: strings.Repeat("A", 43), ExpiresAt: "2026-10-23T00:10:00.000Z", ExpiresInMS: 600000}
	set := []SiteChallenge{challenge}
	reply := DeploymentResponse{ServerTime: &serverTime, SiteChallenges: &set}
	require.NoError(t, validateDeploymentResponse(reply, &DeploymentDeclaration{PublicSiteURL: &site}))
	set[0].ExpiresInMS = 599999
	require.Error(t, validateDeploymentResponse(reply, &DeploymentDeclaration{PublicSiteURL: &site}))
	set[0] = challenge
	other := "https://other.example/"
	require.Error(t, validateDeploymentResponse(reply, &DeploymentDeclaration{PublicSiteURL: &other}))
	serverTime = "2026-10-23T00:00:00Z"
	set[0].ExpiresAt = "2026-10-23T00:10:00Z"
	require.NoError(t, validateDeploymentResponse(reply, &DeploymentDeclaration{PublicSiteURL: &site}))
	serverTime = "2026-10-23T00:00:00+00:00"
	require.Error(t, validateDeploymentResponse(reply, &DeploymentDeclaration{PublicSiteURL: &site}))
}

// TestDeploymentOldMarketContract 由 Marketplace 隔离 HTTP fixture 显式调用。
func TestDeploymentOldMarketContract(t *testing.T) {
	root := os.Getenv("POWERX_DEPLOYMENT_OLD_MARKET_TEST_URL")
	if root == "" {
		t.Skip("requires isolated actual Marketplace handler")
	}
	client := NewClient(root+"/api/v1", "", &http.Client{Timeout: 5 * time.Second})
	site := "https://admin.example.com/console/"
	declaration := &DeploymentDeclaration{PublicSiteURL: &site}
	registered, err := client.RegisterAutomaticWithDeployment(context.Background(), fixtureCredential, fixtureFingerprint, "1.0.0", "M0 compatibility", declaration)
	require.NoError(t, err)
	require.NotEmpty(t, registered.Instance.ID)
	require.False(t, registered.Extended)
	client.SetInstance(fixtureCredential, fixtureFingerprint)
	heartbeat, err := client.HeartbeatWithDeployment(context.Background(), "1.0.0", declaration)
	require.NoError(t, err)
	require.False(t, heartbeat.Extended)
	// 真实旧 handler 严格拒绝扩展，不依靠新增专用错误码。
	err = client.request(context.Background(), http.MethodPost, "/host/instances/heartbeat", nil, map[string]any{"powerx_version": "1.0.0", "features": []string{DeploymentFingerprintFeature}}, nil)
	var request *RequestError
	require.ErrorAs(t, err, &request)
	require.Equal(t, 400, request.HTTPStatus)
	require.Equal(t, "invalid_input", request.UpstreamCode)
	_, err = client.HeartbeatWithDeployment(context.Background(), "1.0.0", declaration)
	require.NoError(t, err)
	require.Equal(t, fixtureCredential, client.token)
}
