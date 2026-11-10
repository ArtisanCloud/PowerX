package plugin_marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/internal/productinfo"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDeploymentServiceProtocolLifecycle(t *testing.T) {
	var mu sync.Mutex
	support, offline, rejected, empty := true, false, false, false
	id := uuid.NewString()
	site := "https://admin.example.com/console/"
	instance := uuid.NewString()
	var registeredToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		reply := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": v}) }
		if strings.HasSuffix(r.URL.Path, "discovery") {
			features := []string{}
			if support {
				features = append(features, market.DeploymentFingerprintFeature)
			}
			reply(map[string]any{"service": "powerx-marketplace", "protocol": market.AutomaticRegistrationProtocol, "features": features})
			return
		}
		if offline {
			w.WriteHeader(503)
			return
		}
		if rejected {
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, extended := body["features"]
		if !support && extended {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_input"}`))
			return
		}
		result := map[string]any{"status": "recorded"}
		if strings.HasSuffix(r.URL.Path, "register") {
			registeredToken = r.Header.Get("Authorization")
			result = map[string]any{"protocol": market.AutomaticRegistrationProtocol, "instance": map[string]string{"id": instance}}
		} else if r.Header.Get("Authorization") != registeredToken {
			t.Error("instance credential changed")
		}
		if extended {
			result["server_time"] = "2026-01-01T00:00:00Z"
			challenges := []market.SiteChallenge{}
			if body["public_site_url"] == site && !empty {
				challenges = append(challenges, market.SiteChallenge{Schema: "powerx.instance-proof/v1", ChallengeID: id, SiteURL: site, Nonce: strings.Repeat("A", 43), ExpiresAt: "2026-01-01T00:10:00Z", ExpiresInMS: 600000})
			}
			result["site_challenges"] = challenges
		}
		reply(result)
	}))
	defer server.Close()
	client := market.NewClient(server.URL, "", server.Client())
	svc := NewService(client, "1.0.0", nil)
	require.NoError(t, svc.ConfigureDeployment(productinfo.Information{Product: "powerx", State: "development"}, site))
	store, err := market.OpenDistributionStore(t.TempDir(), client.SourceID(), "environment-one")
	require.NoError(t, err)
	svc.ConfigureDistribution(store, "", time.Minute, nil)
	defer svc.CloseDistribution()
	require.NoError(t, svc.ConnectInstance(context.Background()))
	proof, ok := svc.MarketplaceProof(id)
	require.True(t, ok)
	raw, err := json.Marshal(proof)
	require.NoError(t, err)
	require.NotContains(t, string(raw), instance)
	require.NotContains(t, string(raw), "site_url")
	mu.Lock()
	offline = true
	mu.Unlock()
	require.Error(t, svc.HeartbeatInstance(context.Background()))
	_, ok = svc.MarketplaceProof(id)
	require.True(t, ok, "network failure must retain live proof")
	mu.Lock()
	offline = false
	support = false
	mu.Unlock()
	require.NoError(t, svc.HeartbeatInstance(context.Background()))
	_, ok = svc.MarketplaceProof(id)
	require.False(t, ok)
	mu.Lock()
	support = true
	mu.Unlock()
	require.NoError(t, client.RefreshDeploymentCapabilities(context.Background()))
	require.NoError(t, svc.HeartbeatInstance(context.Background()))
	_, ok = svc.MarketplaceProof(id)
	require.True(t, ok)
	mu.Lock()
	empty = true
	mu.Unlock()
	require.NoError(t, svc.HeartbeatInstance(context.Background()))
	_, ok = svc.MarketplaceProof(id)
	require.False(t, ok)
	mu.Lock()
	empty = false
	mu.Unlock()
	require.NoError(t, svc.HeartbeatInstance(context.Background()))
	require.NoError(t, svc.ConfigureDeployment(svc.ProductInformation(), "https://other.example/"))
	_, ok = svc.MarketplaceProof(id)
	require.False(t, ok)
	require.NoError(t, svc.ConfigureDeployment(svc.ProductInformation(), site))
	require.NoError(t, svc.HeartbeatInstance(context.Background()))
	mu.Lock()
	rejected = true
	mu.Unlock()
	require.Error(t, svc.HeartbeatInstance(context.Background()))
	_, ok = svc.MarketplaceProof(id)
	require.False(t, ok)
	require.Equal(t, instance, store.Identity().InstanceID)
}

func TestDeploymentMonotonicDeadlineAndStaleConfiguration(t *testing.T) {
	svc := NewService(market.NewClient("", "", nil), "1.0.0", nil)
	site := "https://admin.example/"
	require.NoError(t, svc.ConfigureDeployment(productinfo.Information{Product: "powerx", State: "development"}, site))
	_, generation := svc.deploymentDeclaration()
	now := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC) // 与市场墙上时间故意不同。
	svc.deployment.now = func() time.Time { return now }
	id := uuid.NewString()
	set := []market.SiteChallenge{{Schema: "powerx.instance-proof/v1", ChallengeID: id, SiteURL: site, Nonce: strings.Repeat("A", 43), ExpiresAt: "2026-01-01T00:10:00Z", ExpiresInMS: 10000}}
	response := market.DeploymentResponse{Extended: true, SiteChallenges: &set, RoundTrip: time.Second, ReceivedAt: now}
	svc.acceptDeployment(response, generation, nil)
	original := svc.deployment.proofs[id].deadline
	require.Equal(t, now.Add(8*time.Second), original)
	now = now.Add(5 * time.Second)
	response.ReceivedAt = now
	svc.acceptDeployment(response, generation, nil)
	require.Equal(t, original, svc.deployment.proofs[id].deadline, "repeated delivery must not extend")
	now = now.Add(10 * time.Second)
	response.ReceivedAt = now
	svc.acceptDeployment(response, generation, nil)
	require.Equal(t, original, svc.deployment.proofs[id].deadline, "expired deadline must not resurrect")
	require.NoError(t, svc.ConfigureDeployment(svc.ProductInformation(), ""))
	svc.acceptDeployment(response, generation, nil)
	require.Empty(t, svc.deployment.proofs, "in-flight old response must not restore cleared site")
	require.Error(t, svc.ConfigureDeployment(svc.ProductInformation(), "not a url"))
	declaration, _ := svc.deploymentDeclaration()
	require.Nil(t, declaration)
	require.NoError(t, svc.ConfigureDeployment(svc.ProductInformation(), "http://public.example/console"))
	declaration, _ = svc.deploymentDeclaration()
	require.Equal(t, "http://public.example/console/", *declaration.PublicSiteURL, "HTTP can be declared; only HTTPS can receive verification challenges")
}

func TestDeploymentEnabledWorkerOldMarketAndStableRestart(t *testing.T) {
	f := newAutomaticFixture(t)
	dir := t.TempDir()
	svc := automaticService(t, f.server.URL, dir, "stable-environment")
	require.NoError(t, svc.ConfigureDeployment(productinfo.Information{Product: "powerx", State: "development"}, ""))
	svc.StartDistribution(context.Background())
	require.Eventually(t, func() bool { return !svc.distribution.store.Identity().LastHeartbeat.IsZero() }, 3*time.Second, 10*time.Millisecond)
	identity := svc.distribution.store.Identity()
	svc.CloseDistribution()
	second := automaticService(t, f.server.URL, dir, "stable-environment")
	require.NoError(t, second.ConfigureDeployment(productinfo.Information{Product: "powerx", State: "development"}, ""))
	second.StartDistribution(context.Background())
	require.Eventually(t, func() bool { return second.distribution.store.Identity().LastHeartbeat.After(identity.LastHeartbeat) }, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, identity.InstanceID, second.distribution.store.Identity().InstanceID)
	require.Equal(t, identity.InstanceToken, second.distribution.store.Identity().InstanceToken)
	second.CloseDistribution()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Equal(t, 1, f.registrations)
}
