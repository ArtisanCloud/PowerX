package eventfabric

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	directory "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestACLAdminRESTContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	topicStore := newMemoryTopicStore()

	// Seed topic for ACL operations
	topic, err := topicStore.Create(context.Background(), &model.TopicDefinition{
		TenantID:      1,
		TenantKey:     "tenant-corex",
		Namespace:     "corex.workflow",
		Name:          "approved",
		Lifecycle:     model.TopicLifecycleActive,
		PayloadFormat: "json",
		MaxRetry:      5,
		AckTimeoutSec: 30,
	})
	if err != nil {
		t.Fatalf("seed topic failed: %v", err)
	}

	aclStore := newMemoryAclStore()
	svc := acl.NewACLService(acl.Options{
		Store:      aclStore,
		TopicStore: topicStore,
		Clock:      func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
	})

	directorySvc := directory.NewDirectoryService(directory.Options{
		Store:             topicStore,
		Clock:             func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) },
		DefaultMaxRetry:   5,
		DefaultAckTimeout: 30 * time.Second,
	})

	handler := NewAdminACLHandler(AdminACLHandlerOptions{
		Service:   svc,
		Directory: directorySvc,
	})

	router := gin.New()
	group := router.Group("/event-fabric")
	group.POST("/acl", handler.UpsertBindings)
	group.GET("/acl", handler.ListBindings)

	server := httptest.NewServer(router)
	defer server.Close()

	grantReq := map[string]interface{}{
		"tenant_id":       "tenant-corex",
		"topic_full_name": "tenant-corex.corex.workflow.approved",
		"grants": []map[string]interface{}{
			{
				"principal_type": "service",
				"principal_id":   "svc-publisher",
				"action":         "publish",
			},
		},
	}

	resp := httpRequest(t, server, http.MethodPost, "/event-fabric/acl", grantReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.StatusCode)
	}

	listResp := httpRequest(t, server, http.MethodGet, "/event-fabric/acl?tenant_id=tenant-corex&topic_uuid="+topic.UUID.String(), nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list expected 200 got %d", listResp.StatusCode)
	}
	var listPayload map[string]interface{}
	decodeJSON(t, listResp.Body, &listPayload)
	items := listPayload["data"].(map[string]interface{})["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 binding got %d", len(items))
	}

	revokeReq := map[string]interface{}{
		"tenant_id":       "tenant-corex",
		"topic_full_name": "tenant-corex.corex.workflow.approved",
		"revokes": []map[string]interface{}{
			{
				"principal_type": "service",
				"principal_id":   "svc-publisher",
				"action":         "publish",
			},
		},
	}
	revokeResp := httpRequest(t, server, http.MethodPost, "/event-fabric/acl", revokeReq)
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke expected 200 got %d", revokeResp.StatusCode)
	}

	listResp = httpRequest(t, server, http.MethodGet, "/event-fabric/acl?tenant_id=tenant-corex&topic_uuid="+topic.UUID.String(), nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list expected 200 got %d", listResp.StatusCode)
	}
	decodeJSON(t, listResp.Body, &listPayload)
	items = listPayload["data"].(map[string]interface{})["items"].([]interface{})
	if len(items) != 0 {
		t.Fatalf("expected 0 bindings after revoke, got %d", len(items))
	}
}

func httpRequest(t *testing.T, server *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("new request error: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, body io.ReadCloser, out interface{}) {
	t.Helper()
	defer body.Close()
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(out); err != nil {
		t.Fatalf("decode body error: %v", err)
	}
}

type memoryAclStore struct {
	mu    sync.RWMutex
	items map[string]*model.AclBinding
}

func newMemoryAclStore() *memoryAclStore {
	return &memoryAclStore{items: make(map[string]*model.AclBinding)}
}

func (m *memoryAclStore) UpsertBindings(ctx context.Context, bindings []*model.AclBinding) ([]*model.AclBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range bindings {
		if b.UUID == uuid.Nil {
			b.UUID = uuid.New()
		}
		key := aclKey(b.TenantKey, b.TopicUUID, b.PrincipalID, b.Action)
		clone := *b
		if clone.CreatedAt.IsZero() {
			clone.CreatedAt = time.Now().UTC()
		}
		clone.UpdatedAt = time.Now().UTC()
		m.items[key] = &clone
	}
	return bindings, nil
}

func (m *memoryAclStore) RemoveBindings(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, actions []string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var removed int64
	if len(actions) == 0 {
		actions = []string{"publish", "subscribe", "replay"}
	}
	for _, action := range actions {
		key := aclKey(tenantKey, topic, principalID, action)
		if _, ok := m.items[key]; ok {
			delete(m.items, key)
			removed++
		}
	}
	return removed, nil
}

func (m *memoryAclStore) ListByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) ([]*model.AclBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var rows []*model.AclBinding
	for _, item := range m.items {
		if item.TenantKey == tenantKey && item.TopicUUID == topic {
			clone := *item
			rows = append(rows, &clone)
		}
	}
	return rows, nil
}

func (m *memoryAclStore) HasPermission(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action string, now time.Time) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := aclKey(tenantKey, topic, principalID, action)
	binding, ok := m.items[key]
	if !ok {
		return false, nil
	}
	if binding.ExpiresAt != nil && !binding.ExpiresAt.After(now) {
		return false, nil
	}
	return true, nil
}

func aclKey(tenant string, topic uuid.UUID, principal, action string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "|" + topic.String() + "|" + strings.ToLower(strings.TrimSpace(principal)) + "|" + strings.ToLower(strings.TrimSpace(action))
}
