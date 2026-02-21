package eventfabric

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	directory "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	admin "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/event_fabric"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestACLAdminRESTContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	topicStore := newMemoryTopicStore()

	// Seed topic for ACL operations
	topic, err := topicStore.Create(context.Background(), &model.TopicDefinition{
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
	sharedTopic, err := topicStore.Create(context.Background(), &model.TopicDefinition{
		TenantKey:     "global",
		Namespace:     "knowledge.space.feedback",
		Name:          "reprocess",
		Lifecycle:     model.TopicLifecycleActive,
		PayloadFormat: "json",
		MaxRetry:      5,
		AckTimeoutSec: 30,
	})
	if err != nil {
		t.Fatalf("seed shared topic failed: %v", err)
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

	handler := admin.NewAdminACLHandler(admin.AdminACLHandlerOptions{
		Service:   svc,
		Directory: directorySvc,
	})

	router := gin.New()
	group := router.Group("/event-fabric")
	attachTenantContext(group, "tenant-corex")
	group.POST("/acl", handler.UpsertBindings)
	group.GET("/acl", handler.ListBindings)
	group.GET("/acl/topic-matrix", handler.ListTopicRoleMatrix)
	group.GET("/acl/principal-matrix", handler.ListPrincipalTopicMatrix)

	grantReq := map[string]interface{}{
		"topic_full_name": "tenant-corex.corex.workflow.approved",
		"grants": []map[string]interface{}{
			{
				"principal_type": "service",
				"principal_id":   "svc-publisher",
				"action":         "publish",
			},
		},
	}

	resp := httpRequest(t, router, http.MethodPost, "/event-fabric/acl", grantReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.StatusCode)
	}

	listResp := httpRequest(t, router, http.MethodGet, "/event-fabric/acl?topic_uuid="+topic.UUID.String(), nil)
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
		"topic_full_name": "tenant-corex.corex.workflow.approved",
		"revokes": []map[string]interface{}{
			{
				"principal_type": "service",
				"principal_id":   "svc-publisher",
				"action":         "publish",
			},
		},
	}
	revokeResp := httpRequest(t, router, http.MethodPost, "/event-fabric/acl", revokeReq)
	if revokeResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke expected 200 got %d", revokeResp.StatusCode)
	}

	listResp = httpRequest(t, router, http.MethodGet, "/event-fabric/acl?topic_uuid="+topic.UUID.String(), nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list expected 200 got %d", listResp.StatusCode)
	}
	decodeJSON(t, listResp.Body, &listPayload)
	items = listPayload["data"].(map[string]interface{})["items"].([]interface{})
	if len(items) != 0 {
		t.Fatalf("expected 0 bindings after revoke, got %d", len(items))
	}

	sharedGrantReq := map[string]interface{}{
		"topic_full_name": "global.knowledge.space.feedback.reprocess",
		"grants": []map[string]interface{}{
			{
				"principal_type": "role",
				"principal_id":   "role:role_admin",
				"action":         "replay",
			},
		},
	}
	sharedResp := httpRequest(t, router, http.MethodPost, "/event-fabric/acl", sharedGrantReq)
	if sharedResp.StatusCode != http.StatusOK {
		t.Fatalf("shared grant expected 200 got %d", sharedResp.StatusCode)
	}

	sharedListResp := httpRequest(t, router, http.MethodGet, "/event-fabric/acl?topic_uuid="+sharedTopic.UUID.String(), nil)
	if sharedListResp.StatusCode != http.StatusOK {
		t.Fatalf("shared list expected 200 got %d", sharedListResp.StatusCode)
	}
	decodeJSON(t, sharedListResp.Body, &listPayload)
	items = listPayload["data"].(map[string]interface{})["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 binding on shared topic, got %d", len(items))
	}

	topicMatrixResp := httpRequest(t, router, http.MethodGet, "/event-fabric/acl/topic-matrix?namespace=knowledge.space.feedback&name=reprocess", nil)
	if topicMatrixResp.StatusCode != http.StatusOK {
		t.Fatalf("topic matrix expected 200 got %d", topicMatrixResp.StatusCode)
	}

	principalMatrixResp := httpRequest(t, router, http.MethodGet, "/event-fabric/acl/principal-matrix?principal_id=role:role_admin&namespace=knowledge.space.feedback&name=reprocess", nil)
	if principalMatrixResp.StatusCode != http.StatusOK {
		t.Fatalf("principal matrix expected 200 got %d", principalMatrixResp.StatusCode)
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
