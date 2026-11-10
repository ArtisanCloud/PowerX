package plugin_marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type automaticFixture struct {
	mu                                     sync.Mutex
	server                                 *httptest.Server
	identities                             map[string]string
	journal                                *market.DistributionStore
	loseResponse, revoked, offline         bool
	discoveries, registrations, heartbeats int
}

func newAutomaticFixture(t *testing.T) *automaticFixture {
	t.Helper()
	f := &automaticFixture{identities: map[string]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		respond := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": v}) }
		if f.offline {
			w.WriteHeader(503)
			return
		}
		switch r.URL.Path {
		case "/api/v1/host/instances/discovery":
			f.discoveries++
			if r.Method != "GET" || r.Header.Get("Authorization") != "" {
				t.Error("public discovery must not send credentials")
			}
			respond(map[string]string{"service": "powerx-marketplace", "protocol": "powerx.instance-registration/v1"})
		case "/api/v1/host/instances/register":
			f.registrations++
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(token, "pxm_instance_") || len(token) != len("pxm_instance_")+43 {
				t.Error("automatic registration missing random instance credential")
			}
			if f.journal != nil && f.journal.Identity().PendingInstanceToken != token {
				t.Error("credential was not persisted before registration")
			}
			if f.revoked {
				w.WriteHeader(401)
				return
			}
			id := f.identities[token]
			if id == "" {
				id = uuid.NewString()
				f.identities[token] = id
			}
			if f.loseResponse {
				f.loseResponse = false
				_, _ = w.Write([]byte("lost response"))
				return
			}
			respond(map[string]any{"instance": map[string]string{"id": id}, "protocol": "powerx.instance-registration/v1"})
		case "/api/v1/host/instances/heartbeat":
			if f.revoked {
				w.WriteHeader(401)
				return
			}
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if f.identities[token] == "" || len(r.Header.Get("X-PowerX-Fingerprint")) != 64 {
				t.Error("heartbeat missing identity")
			}
			f.heartbeats++
			respond(map[string]string{"status": "recorded"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func automaticService(t *testing.T, root, dir, env string) *Service {
	t.Helper()
	client := market.NewClient(root+"/api/v1", "legacy-must-not-be-used", nil)
	store, err := market.OpenDistributionStore(dir, client.SourceID(), env)
	require.NoError(t, err)
	s := NewService(client, "v1.0.0", nil)
	s.ConfigureDistribution(store, "", time.Minute, nil)
	t.Cleanup(s.CloseDistribution)
	return s
}
func TestAutomaticRegistrationPersistsBeforeRequestAndLostResponseRetry(t *testing.T) {
	f := newAutomaticFixture(t)
	dir := t.TempDir()
	s := automaticService(t, f.server.URL, dir, "environment-a")
	f.journal = s.distribution.store
	f.loseResponse = true
	require.Error(t, s.ConnectInstance(context.Background()))
	pending := s.distribution.store.Identity().PendingInstanceToken
	require.NotEmpty(t, pending)
	s.CloseDistribution()
	s = automaticService(t, f.server.URL, dir, "environment-a")
	f.journal = s.distribution.store
	require.NoError(t, s.ConnectInstance(context.Background()))
	identity := s.Status().Instance.ID
	require.Equal(t, pending, s.distribution.store.Identity().InstanceToken)
	require.Empty(t, s.distribution.store.Identity().PendingInstanceToken)
	require.Len(t, f.identities, 1)
	require.Equal(t, 2, f.registrations)
	require.NoError(t, s.HeartbeatInstance(context.Background()))
	s.CloseDistribution()
	s = automaticService(t, f.server.URL, dir, "environment-a")
	f.journal = s.distribution.store
	require.NoError(t, s.ConnectInstance(context.Background()))
	require.NoError(t, s.HeartbeatInstance(context.Background()))
	require.Equal(t, identity, s.Status().Instance.ID)
	require.Equal(t, 2, f.registrations)
	raw, err := json.Marshal(s.Status())
	require.NoError(t, err)
	require.NotContains(t, string(raw), pending)
}
func TestAutomaticRegistrationWorkerRetriesAndRevocationKeepsIdentity(t *testing.T) {
	f := newAutomaticFixture(t)
	s := automaticService(t, f.server.URL, t.TempDir(), "environment-worker")
	f.offline = true
	started := time.Now()
	s.StartDistribution(context.Background())
	require.Less(t, time.Since(started), time.Second)
	require.Eventually(t, func() bool { return s.Status().Instance.ErrorCode != "" }, time.Second, 10*time.Millisecond)
	f.mu.Lock()
	f.offline = false
	f.mu.Unlock()
	require.Eventually(t, func() bool { return !s.distribution.store.Identity().LastHeartbeat.IsZero() }, 8*time.Second, 20*time.Millisecond)
	token := s.distribution.store.Identity().InstanceToken
	f.mu.Lock()
	f.revoked = true
	registrations := f.registrations
	f.mu.Unlock()
	require.Error(t, s.HeartbeatInstance(context.Background()))
	require.Error(t, s.ConnectInstance(context.Background()))
	require.Equal(t, "credential_rejected", s.Status().Instance.State)
	require.Equal(t, token, s.distribution.store.Identity().InstanceToken)
	f.mu.Lock()
	require.Equal(t, registrations, f.registrations)
	f.mu.Unlock()
}

// Opt-in real contract acceptance runs from the Marketplace handler test fixture.
// There is no admin provisioning, shared token, plugin install or production state.
func TestAutomaticRegistrationContract(t *testing.T) {
	root := os.Getenv("POWERX_AUTOMATIC_MARKET_TEST_URL")
	if root == "" {
		t.Skip("requires isolated actual Marketplace HTTP fixture")
	}
	directory := t.TempDir()
	a := automaticService(t, root, directory, "real-contract-a")
	b := automaticService(t, root, t.TempDir(), "real-contract-b")
	a.StartDistribution(context.Background())
	b.StartDistribution(context.Background())
	for _, s := range []*Service{a, b} {
		require.Eventually(t, func() bool { return !s.distribution.store.Identity().LastHeartbeat.IsZero() }, 10*time.Second, 20*time.Millisecond)
	}
	require.NotEqual(t, a.Status().Instance.ID, b.Status().Instance.ID)
	identity := a.distribution.store.Identity()
	a.CloseDistribution()
	a = automaticService(t, root, directory, "real-contract-a")
	a.StartDistribution(context.Background())
	require.Eventually(t, func() bool { return a.distribution.store.Identity().LastHeartbeat.After(identity.LastHeartbeat) }, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, a.ConnectInstance(context.Background()))
	require.Equal(t, identity.InstanceID, a.Status().Instance.ID)
	require.NoError(t, a.HeartbeatInstance(context.Background()))
	t.Log("Two independently persisted PowerX instances automatically registered and reported heartbeats against actual Marketplace handlers")
}
