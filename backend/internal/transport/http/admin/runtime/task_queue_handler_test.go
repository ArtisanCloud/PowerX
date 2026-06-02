package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
)

func TestTaskQueueEnqueueRejectsDrainingPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	driver := &taskDriverStub{}
	h := &taskQueueHandler{
		driver: driver,
		guard: func(_ context.Context, pluginID string) error {
			if pluginID != "com.powerx.plugins.base" {
				t.Fatalf("unexpected plugin id: %s", pluginID)
			}
			return dto.NewErrorWithCode(http.StatusConflict, "PLUGIN_DRAINING", "插件正在 drain 或已被平台禁用，禁止新增使用", errors.New("plugin is draining"))
		},
	}

	router := gin.New()
	router.POST("/enqueue", h.enqueue)
	body, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"id":             "task-1",
			"tenant_key":     "tenant-demo",
			"subscriber_id":  "worker-demo",
			"payload_base64": "e30=",
			"metadata": map[string]string{
				"plugin_id": "com.powerx.plugins.base",
			},
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/enqueue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if driver.enqueueCount != 0 {
		t.Fatalf("expected enqueue not called, count=%d", driver.enqueueCount)
	}
}

type taskDriverStub struct {
	enqueueCount int
}

func (s *taskDriverStub) Type() event_bus.QueueDriverType {
	return event_bus.QueueDriverMemory
}

func (s *taskDriverStub) Capability() event_bus.QueueDriverCapability {
	return event_bus.QueueDriverCapability{}
}

func (s *taskDriverStub) Enqueue(context.Context, event_bus.TaskMessage) error {
	s.enqueueCount++
	return nil
}

func (s *taskDriverStub) Dequeue(context.Context, event_bus.DequeueRequest) ([]event_bus.TaskMessage, error) {
	return nil, nil
}

func (s *taskDriverStub) Ack(context.Context, event_bus.AckRequest) error {
	return nil
}

func (s *taskDriverStub) Nack(context.Context, event_bus.NackRequest) error {
	return nil
}

func (s *taskDriverStub) Retry(context.Context, event_bus.RetryRequest) error {
	return nil
}
