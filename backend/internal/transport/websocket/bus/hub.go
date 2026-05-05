package bus

import (
	"context"
	"sync"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Hub manages WS sessions and topic subscriptions.
type Hub struct {
	mu          sync.RWMutex
	sessions    map[string]*Client
	subscribers map[string]map[string]*Client
}

func NewHub() *Hub {
	return &Hub{
		sessions:    make(map[string]*Client),
		subscribers: make(map[string]map[string]*Client),
	}
}

var DefaultHub = NewHub()

func (h *Hub) Register(client *Client) {
	if client == nil {
		return
	}
	h.mu.Lock()
	h.sessions[client.ID] = client
	h.mu.Unlock()
}

func (h *Hub) Unregister(client *Client) {
	if client == nil {
		return
	}
	h.mu.Lock()
	delete(h.sessions, client.ID)
	for topic, subs := range h.subscribers {
		delete(subs, client.ID)
		if len(subs) == 0 {
			delete(h.subscribers, topic)
		}
	}
	h.mu.Unlock()
}

func (h *Hub) Subscribe(client *Client, topic string) {
	if client == nil || topic == "" {
		return
	}
	h.mu.Lock()
	if _, ok := h.subscribers[topic]; !ok {
		h.subscribers[topic] = make(map[string]*Client)
	}
	h.subscribers[topic][client.ID] = client
	h.mu.Unlock()
	client.addTopic(topic)
}

func (h *Hub) Unsubscribe(client *Client, topic string) {
	if client == nil || topic == "" {
		return
	}
	h.mu.Lock()
	if subs, ok := h.subscribers[topic]; ok {
		delete(subs, client.ID)
		if len(subs) == 0 {
			delete(h.subscribers, topic)
		}
	}
	h.mu.Unlock()
	client.removeTopic(topic)
}

func (h *Hub) Publish(tenantUUID, topic string, payload any, traceID string) {
	if topic == "" {
		return
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	env, err := dto.NewWSBusEnvelope(dto.WSBusTypeEvent, topic, payload, traceID)
	if err != nil {
		return
	}
	h.mu.RLock()
	subs := h.subscribers[topic]
	subscriberCount := len(subs)
	emittedCount := 0
	droppedTenantMismatch := 0
	for _, client := range subs {
		if client == nil {
			continue
		}
		if client.TenantUUID != tenantUUID {
			droppedTenantMismatch++
			logger.Info(logger.WithLogFields(client.ctx, map[string]interface{}{"module": "transport.wsbus"}), "wsbus_delivery",
				zap.String("stage", "drop_tenant_mismatch"),
				zap.String("topic", topic),
				zap.String("tenant_uuid", tenantUUID),
				zap.String("client_tenant_uuid", client.TenantUUID),
				zap.String("connection_id", client.ID),
				zap.String("trace_id", traceID),
			)
			continue
		}
		client.sendEnvelope(env)
		emittedCount++
	}
	h.mu.RUnlock()
	logger.Info(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "transport.wsbus"}), "wsbus_delivery",
		zap.String("stage", "emit"),
		zap.String("topic", topic),
		zap.String("tenant_uuid", tenantUUID),
		zap.String("trace_id", traceID),
		zap.Int("subscriber_count", subscriberCount),
		zap.Int("emitted_count", emittedCount),
		zap.Int("dropped_tenant_mismatch", droppedTenantMismatch),
	)
}
