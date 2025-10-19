package eventfabric

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/dlq"
	"github.com/gin-gonic/gin"
)

func TestDeliveryAdminPublishEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deliveryStub := newStubDeliveryService()
	router := gin.New()
	group := router.Group("/event-fabric")
	group.POST("/events:publish", NewAdminDeliveryHandler(AdminDeliveryHandlerOptions{Service: deliveryStub}).PublishEvent)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	payload := base64.StdEncoding.EncodeToString([]byte(`{"hello":"world"}`))
	resp := httpRequest(t, server, http.MethodPost, "/event-fabric/events:publish", map[string]interface{}{
		"tenant_id":  "tenant-corex",
		"topic":      "tenant-corex.corex.workflow.approved",
		"event_id":   "evt-001",
		"trace_id":   "trace-123",
		"version":    "v1",
		"payload":    payload,
		"attributes": map[string]string{"principal_id": "svc-publisher"},
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 got %d", resp.StatusCode)
	}

	deliveryStub.mu.Lock()
	if len(deliveryStub.publishRequests) != 1 {
		t.Fatalf("expected 1 publish call got %d", len(deliveryStub.publishRequests))
	}
	req := deliveryStub.publishRequests[0]
	deliveryStub.mu.Unlock()

	if req.TenantID != "tenant-corex" || req.Topic != "tenant-corex.corex.workflow.approved" || req.EventID != "evt-001" {
		t.Fatalf("unexpected publish payload: %#v", req)
	}
	if string(req.Payload) != `{"hello":"world"}` {
		t.Fatalf("unexpected payload bytes: %s", string(req.Payload))
	}

	badResp := httpRequest(t, server, http.MethodPost, "/event-fabric/events:publish", map[string]interface{}{
		"tenant_id": "tenant-corex",
		"topic":     "tenant-corex.corex.workflow.approved",
		"event_id":  "evt-002",
		"version":   "v1",
		"payload":   "not-base64",
	})
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid payload got %d", badResp.StatusCode)
	}
}

func TestDLQAdminEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &stubDLQService{
		listMessages: []*dlq.Message{
			{ID: "msg-1", TenantID: "tenant-corex", Topic: "tenant-corex.topic.a", EventID: "evt-001", RetryCount: 3, LastError: "timeout", CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "msg-2", TenantID: "tenant-corex", Topic: "tenant-corex.topic.b", EventID: "evt-002", RetryCount: 2, LastError: "nack", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		},
		listTotal:    2,
		replayResult: 2,
		purgeResult:  1,
	}

	router := gin.New()
	group := router.Group("/event-fabric")
	handler := NewAdminDLQHandler(AdminDLQHandlerOptions{Service: stub})
	group.GET("/dlq/messages", handler.ListMessages)
	group.POST("/dlq/messages:replay", handler.ReplayMessages)
	group.DELETE("/dlq/messages", handler.PurgeMessages)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	listResp := httpRequest(t, server, http.MethodGet, "/event-fabric/dlq/messages?tenant_id=tenant-corex&page=1&page_size=10", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", listResp.StatusCode)
	}
	var listPayload map[string]interface{}
	decodeJSON(t, listResp.Body, &listPayload)
	data := listPayload["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items got %d", len(items))
	}

	replayResp := httpRequest(t, server, http.MethodPost, "/event-fabric/dlq/messages:replay", map[string]interface{}{
		"message_ids": []string{"msg-1", "msg-2"},
		"operator_id": "ops-user",
	})
	if replayResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", replayResp.StatusCode)
	}
	var replayPayload map[string]interface{}
	decodeJSON(t, replayResp.Body, &replayPayload)
	replayData := replayPayload["data"].(map[string]interface{})
	if int(replayData["replayed"].(float64)) != 2 {
		t.Fatalf("expected replayed=2 got %v", replayData["replayed"])
	}

	purgeResp := httpRequest(t, server, http.MethodDelete, "/event-fabric/dlq/messages?tenant_id=tenant-corex&topic=tenant-corex.topic.a", nil)
	if purgeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", purgeResp.StatusCode)
	}
	var purgePayload map[string]interface{}
	decodeJSON(t, purgeResp.Body, &purgePayload)
	purgeData := purgePayload["data"].(map[string]interface{})
	if int(purgeData["removed"].(float64)) != 1 {
		t.Fatalf("expected removed=1 got %v", purgeData["removed"])
	}
}

type stubDLQService struct {
	listMessages []*dlq.Message
	listTotal    int64
	replayResult int
	purgeResult  int

	mu          sync.Mutex
	lastListReq dlq.ListRequest
	lastReplay  dlq.ReplayRequest
	lastPurge   struct {
		tenant string
		topic  string
	}
}

func (s *stubDLQService) List(ctx context.Context, req dlq.ListRequest) ([]*dlq.Message, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastListReq = req
	return s.listMessages, s.listTotal, nil
}

func (s *stubDLQService) Replay(ctx context.Context, req dlq.ReplayRequest) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastReplay = req
	return s.replayResult, nil
}

func (s *stubDLQService) Purge(ctx context.Context, tenantID string, topic string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastPurge.tenant = tenantID
	s.lastPurge.topic = topic
	return s.purgeResult, nil
}

type stubDeliveryService struct {
	mu              sync.Mutex
	publishRequests []delivery.PublishRequest
	publishErr      error
}

func newStubDeliveryService() *stubDeliveryService {
	return &stubDeliveryService{}
}

func (s *stubDeliveryService) Publish(ctx context.Context, req delivery.PublishRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishRequests = append(s.publishRequests, req)
	return s.publishErr
}

func (s *stubDeliveryService) Ack(context.Context, string, string) error {
	return nil
}

func (s *stubDeliveryService) Nack(context.Context, string, string, string) (delivery.RetryPlan, error) {
	return delivery.RetryPlan{}, nil
}

func (s *stubDeliveryService) PollRetry(context.Context, int) (map[string][]delivery.DeliveryAttempt, error) {
	return map[string][]delivery.DeliveryAttempt{}, nil
}
