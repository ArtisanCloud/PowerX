package agentmodelhubintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuditLatencyPublishAndEnforce(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	env := testenv.New(t)
	repo := newMemoryAuditRepo()
	audit := auditsvc.NewService(auditsvc.ServiceOptions{
		Repository: repo,
		Config: auditsvc.AuditOptions{
			BatchSize: 1,
			BatchWait: 10 * time.Millisecond,
		},
	})
	defer audit.Close()

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	deps := &appshared.Deps{DB: env.DB, AuditSvc: audit}
	agentmodelhubhttp.RegisterAPIRoutes(public, protected, deps)

	registerPayload := map[string]any{
		"env":              "default",
		"name":             "audit-provider",
		"capabilities":     []string{"llm"},
		"primary_endpoint": "https://example.invalid",
		"regions":          []string{"us-east-1"},
		"tenantWhitelist": []map[string]string{
			{"tenantId": "demo", "environment": "staging"},
		},
		"credentials": map[string]string{
			"api_key": "sk-audit-test",
		},
	}
	resp := doJSONRequest(t, engine, http.MethodPost, "/api/internal/providers/register", registerPayload)
	require.Equal(t, http.StatusAccepted, resp.Code)
	var registerResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &registerResp))
	provider := registerResp.Data["provider"].(map[string]interface{})
	providerID := provider["provider_id"].(string)

	validatePayload := map[string]any{
		"report": map[string]any{
			"providerId":  providerID,
			"suite":       "full",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"stats": map[string]any{
				"total":  1,
				"passed": 1,
				"failed": 0,
			},
			"results": []map[string]any{
				{"name": "llm ping", "modality": "llm", "success": true},
			},
		},
	}
	resp = doJSONRequest(t, engine, http.MethodPost, "/api/internal/providers/"+providerID+"/validate?suite=full", validatePayload)
	require.Equal(t, http.StatusAccepted, resp.Code)

	publishStart := time.Now()
	resp = doJSONRequest(t, engine, http.MethodPost, "/api/internal/providers/"+providerID+"/publish", map[string]any{
		"rolloutStrategy": "full",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	publishEvent, ok := repo.waitForOperation("provider.published", 3*time.Second)
	require.True(t, ok, "expected provider.published audit event")
	require.Equal(t, providerID, publishEvent.ResourceID)
	require.Less(t, time.Since(publishStart), time.Second, "audit write should complete quickly")

	enforceStart := time.Now()
	resp = doJSONRequest(t, engine, http.MethodPost, "/api/internal/provider-quotas/enforce", map[string]any{
		"env":         "default",
		"tenantId":    "demo-tenant-audit",
		"action":      "throttle",
		"reason":      "audit latency test",
		"ticketId":    "AUDIT-TEST",
		"requestedBy": "audit-test",
	})
	require.Equal(t, http.StatusOK, resp.Code)
	enforceEvent, ok := repo.waitForOperation("cost_quota.enforcement", 3*time.Second)
	require.True(t, ok, "expected cost_quota.enforcement audit event")
	require.Contains(t, string(enforceEvent.Meta), "audit latency test")
	require.Less(t, time.Since(enforceStart), time.Second, "enforcement audit should be queryable quickly")
}

func doJSONRequest(t *testing.T, engine *gin.Engine, method, path string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	return rr
}

type memoryAuditRepo struct {
	mu      sync.Mutex
	events  []dbmaudit.AuditEvent
	waiters map[string][]chan dbmaudit.AuditEvent
}

func newMemoryAuditRepo() *memoryAuditRepo {
	return &memoryAuditRepo{
		waiters: map[string][]chan dbmaudit.AuditEvent{},
	}
}

func (m *memoryAuditRepo) InsertBatch(_ context.Context, rows []dbmaudit.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, evt := range rows {
		m.events = append(m.events, evt)
		if chans := m.waiters[evt.Operation]; len(chans) > 0 {
			delete(m.waiters, evt.Operation)
			for _, ch := range chans {
				ch <- evt
				close(ch)
			}
		}
	}
	return nil
}

func (m *memoryAuditRepo) waitForOperation(operation string, timeout time.Duration) (dbmaudit.AuditEvent, bool) {
	ch := make(chan dbmaudit.AuditEvent, 1)
	m.mu.Lock()
	for _, evt := range m.events {
		if evt.Operation == operation {
			m.mu.Unlock()
			return evt, true
		}
	}
	m.waiters[operation] = append(m.waiters[operation], ch)
	m.mu.Unlock()

	select {
	case evt := <-ch:
		return evt, true
	case <-time.After(timeout):
		m.mu.Lock()
		waiters := m.waiters[operation]
		for i, candidate := range waiters {
			if candidate == ch {
				m.waiters[operation] = append(waiters[:i], waiters[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
		return dbmaudit.AuditEvent{}, false
	}
}
