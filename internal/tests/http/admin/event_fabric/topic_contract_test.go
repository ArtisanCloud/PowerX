package eventfabric

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	admin "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/event_fabric"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestEventFabricAdminTopics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	bus := &recordingBus{}
	store := newMemoryTopicStore()
	svc := directory.NewDirectoryService(directory.Options{
		Store:             store,
		EventBus:          bus,
		Clock:             func() time.Time { return time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC) },
		ActorResolver:     func(ctx context.Context) string { return "tester" },
		DefaultMaxRetry:   5,
		DefaultAckTimeout: 30 * time.Second,
	})

	handler := admin.NewAdminDirectoryHandler(admin.AdminDirectoryHandlerOptions{Service: svc})
	router := gin.New()
	group := router.Group("/event-fabric")
	group.POST("/topics", handler.CreateTopic)
	group.GET("/topics", handler.ListTopics)
	group.PATCH("/topics/:topic_id/lifecycle", handler.UpdateLifecycle)

	server := httptest.NewServer(router)
	defer server.Close()

	createBody := map[string]interface{}{
		"tenant_id":      "tenant-corex",
		"namespace":      "corex.workflow",
		"name":           "approved",
		"payload_format": "json",
		"max_retry":      7,
	}
	resp := doRequest(t, server, http.MethodPost, "/event-fabric/topics", createBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.StatusCode)
	}
	var created map[string]interface{}
	decodeBody(t, resp.Body, &created)
	topicData := created["data"].(map[string]interface{})
	topicID := topicData["id"].(string)
	if topicData["full_topic"].(string) != "tenant-corex.corex.workflow.approved" {
		t.Fatalf("unexpected full topic: %s", topicData["full_topic"])
	}

	listResp := doRequest(t, server, http.MethodGet, "/event-fabric/topics", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list expected 200 got %d", listResp.StatusCode)
	}
	var list map[string]interface{}
	decodeBody(t, listResp.Body, &list)
	items := list["data"].(map[string]interface{})["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 topic got %d", len(items))
	}

	patchBody := map[string]interface{}{
		"target_state":  "deprecated",
		"change_reason": "sunset",
	}
	updatePath := "/event-fabric/topics/" + topicID + "/lifecycle"
	updateResp := doRequest(t, server, http.MethodPatch, updatePath, patchBody)
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update expected 200 got %d", updateResp.StatusCode)
	}
	var updated map[string]interface{}
	decodeBody(t, updateResp.Body, &updated)
	lifecycle := updated["data"].(map[string]interface{})["lifecycle"].(string)
	if lifecycle != "deprecated" {
		t.Fatalf("expected lifecycle deprecated got %s", lifecycle)
	}
	if len(bus.events) == 0 {
		t.Fatalf("expected lifecycle event published")
	}
}

func doRequest(t *testing.T, server *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body error: %v", err)
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

func decodeBody(t *testing.T, body io.ReadCloser, out interface{}) {
	t.Helper()
	defer body.Close()
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(out); err != nil {
		t.Fatalf("decode body error: %v", err)
	}
}

type recordingBus struct {
	mu     sync.Mutex
	events []string
}

func (b *recordingBus) Subscribe(eventType string, handler event_bus.Handler) func() {
	return func() {}
}

func (b *recordingBus) Publish(eventType string, payload interface{}, ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, eventType)
}

func (b *recordingBus) Close() error { return nil }

type memoryTopicStore struct {
	mu     sync.RWMutex
	topics map[uuid.UUID]*model.TopicDefinition
	index  map[string]uuid.UUID
	seq    uint64
}

func newMemoryTopicStore() *memoryTopicStore {
	return &memoryTopicStore{
		topics: make(map[uuid.UUID]*model.TopicDefinition),
		index:  make(map[string]uuid.UUID),
	}
}

func (m *memoryTopicStore) Create(ctx context.Context, topic *model.TopicDefinition) (*model.TopicDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := topic.BeforeCreate(nil); err != nil {
		return nil, err
	}
	key := compositeKey(topic.TenantKey, topic.Namespace, topic.Name)
	if _, ok := m.index[key]; ok {
		return nil, fmt.Errorf("duplicate topic")
	}
	m.seq++
	topic.ID = m.seq
	now := time.Now().UTC()
	topic.CreatedAt = now
	topic.UpdatedAt = now
	clone := *topic
	m.topics[clone.UUID] = &clone
	m.index[key] = clone.UUID
	return &clone, nil
}

func (m *memoryTopicStore) Update(ctx context.Context, topic *model.TopicDefinition) (*model.TopicDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.topics[topic.UUID]; ok {
		topic.UpdatedAt = time.Now().UTC()
		clone := *topic
		m.topics[clone.UUID] = &clone
		key := compositeKey(clone.TenantKey, clone.Namespace, clone.Name)
		m.index[key] = clone.UUID
		return &clone, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *memoryTopicStore) FindByUUID(ctx context.Context, id uuid.UUID) (*model.TopicDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if record, ok := m.topics[id]; ok {
		clone := *record
		return &clone, nil
	}
	return nil, nil
}

func (m *memoryTopicStore) FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*model.TopicDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := compositeKey(tenantKey, namespace, name)
	if id, ok := m.index[key]; ok {
		clone := *m.topics[id]
		return &clone, nil
	}
	return nil, nil
}

func (m *memoryTopicStore) List(ctx context.Context, query repository.QueryContext) ([]*model.TopicDefinition, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var rows []*model.TopicDefinition
	for _, item := range m.topics {
		if query.Filter.TenantID != "" && item.TenantKey != query.Filter.TenantID {
			continue
		}
		if query.Filter.Namespace != "" && item.Namespace != query.Filter.Namespace {
			continue
		}
		if len(query.Filter.Lifecycle) > 0 && !containsLifecycle(query.Filter.Lifecycle, item.Lifecycle) {
			continue
		}
		clone := *item
		rows = append(rows, &clone)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	total := int64(len(rows))
	offset := query.Page.Offset
	if offset >= len(rows) {
		return []*model.TopicDefinition{}, total, nil
	}
	limit := query.Page.Limit
	if limit <= 0 || offset+limit > len(rows) {
		limit = len(rows) - offset
	}
	return rows[offset : offset+limit], total, nil
}

func compositeKey(tenant, namespace, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "|" + strings.ToLower(strings.TrimSpace(namespace)) + "|" + strings.ToLower(strings.TrimSpace(name))
}

func containsLifecycle(list []model.TopicLifecycle, value model.TopicLifecycle) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
