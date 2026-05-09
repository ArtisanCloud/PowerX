package bus

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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
	sessionCount := len(h.sessions)
	h.mu.Unlock()
	logger.Info(logger.WithLogFields(client.ctx, map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
		zap.String("stage", "hub_register"),
		zap.String("connection_id", client.ID),
		zap.String("tenant_uuid", strings.TrimSpace(client.TenantUUID)),
		zap.Int("session_count", sessionCount),
		zap.Int("pid", os.Getpid()),
	)
}

func (h *Hub) Unregister(client *Client) {
	if client == nil {
		return
	}
	h.mu.Lock()
	removedTopics := make([]string, 0, 8)
	delete(h.sessions, client.ID)
	for topic, subs := range h.subscribers {
		if _, exists := subs[client.ID]; exists {
			delete(subs, client.ID)
			removedTopics = append(removedTopics, topic)
		}
		if len(subs) == 0 {
			delete(h.subscribers, topic)
		}
	}
	sessionCount := len(h.sessions)
	h.mu.Unlock()
	logger.Info(logger.WithLogFields(client.ctx, map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
		zap.String("stage", "hub_unregister"),
		zap.String("connection_id", client.ID),
		zap.String("tenant_uuid", strings.TrimSpace(client.TenantUUID)),
		zap.Int("session_count", sessionCount),
		zap.Int("pid", os.Getpid()),
		zap.Strings("removed_topics", removedTopics),
	)
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
	topicSubscriberCount := len(h.subscribers[topic])
	h.mu.Unlock()
	client.addTopic(topic)
	logger.Info(logger.WithLogFields(client.ctx, map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
		zap.String("stage", "hub_subscribe"),
		zap.String("connection_id", client.ID),
		zap.String("tenant_uuid", strings.TrimSpace(client.TenantUUID)),
		zap.String("topic", strings.TrimSpace(topic)),
		zap.Int("topic_subscribers", topicSubscriberCount),
		zap.Int("pid", os.Getpid()),
	)
}

func (h *Hub) Unsubscribe(client *Client, topic string) {
	if client == nil || topic == "" {
		return
	}
	h.mu.Lock()
	topicSubscriberCount := 0
	if subs, ok := h.subscribers[topic]; ok {
		delete(subs, client.ID)
		topicSubscriberCount = len(subs)
		if len(subs) == 0 {
			delete(h.subscribers, topic)
		}
	}
	h.mu.Unlock()
	client.removeTopic(topic)
	logger.Info(logger.WithLogFields(client.ctx, map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
		zap.String("stage", "hub_unsubscribe"),
		zap.String("connection_id", client.ID),
		zap.String("tenant_uuid", strings.TrimSpace(client.TenantUUID)),
		zap.String("topic", strings.TrimSpace(topic)),
		zap.Int("topic_subscribers", topicSubscriberCount),
		zap.Int("pid", os.Getpid()),
	)
}

func (h *Hub) Publish(tenantUUID, topic string, payload any, traceID string) {
	h.PublishWithContext(context.Background(), tenantUUID, topic, payload, traceID)
}

func (h *Hub) PublishWithContext(ctx context.Context, tenantUUID, topic string, payload any, traceID string) {
	if topic == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestID := requestIDFromContext(ctx)
	if strings.TrimSpace(traceID) == "" {
		traceID = reqctx.GetTraceID(ctx)
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
	topicKeys := make([]string, 0, len(h.subscribers))
	for topicKey := range h.subscribers {
		topicKeys = append(topicKeys, topicKey)
	}
	logger.Info(logger.WithLogFields(ctx, map[string]interface{}{"module": "transport.wsbus"}), "wsbus_delivery",
		zap.String("stage", "emit_start"),
		zap.String("topic", topic),
		zap.String("tenant_uuid", tenantUUID),
		zap.String("trace_id", traceID),
		zap.Int("subscriber_count", subscriberCount),
		zap.Int("pid", os.Getpid()),
		zap.Strings("topics", topicKeys),
	)
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
				zap.String("request_id", requestID),
				zap.String("trace_id", traceID),
			)
			continue
		}
		client.sendEnvelope(env)
		emittedCount++
	}
	h.mu.RUnlock()
	logger.Info(logger.WithLogFields(ctx, map[string]interface{}{"module": "transport.wsbus"}), "wsbus_delivery",
		zap.String("stage", "emit"),
		zap.String("topic", topic),
		zap.String("tenant_uuid", tenantUUID),
		zap.String("request_id", requestID),
		zap.String("trace_id", traceID),
		zap.Int("subscriber_count", subscriberCount),
		zap.Int("emitted_count", emittedCount),
		zap.Int("dropped_tenant_mismatch", droppedTenantMismatch),
	)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value("request_id").(string); ok {
		return strings.TrimSpace(v)
	}
	if v, ok := ctx.Value("powerx.request_id").(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
