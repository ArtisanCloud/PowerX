package bus

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second
	readLimit = 1024 * 1024
)

// Client represents a single WS connection and its subscriptions.
type Client struct {
	ID         string
	TenantUUID string
	MemberID   uint64
	UserID     uint64
	IsRoot     bool

	ctx        context.Context
	conn       *websocket.Conn
	hub        *Hub
	authorizer Authorizer
	send       chan dto.WSBusEnvelope

	mu     sync.RWMutex
	topics map[string]struct{}
}

func NewClient(ctx context.Context, conn *websocket.Conn, hub *Hub, authorizer Authorizer) *Client {
	return &Client{
		ID:         uuid.NewString(),
		ctx:        ctx,
		conn:       conn,
		hub:        hub,
		authorizer: authorizer,
		send:       make(chan dto.WSBusEnvelope, 16),
		topics:     make(map[string]struct{}),
	}
}

func (c *Client) Run() {
	if c.conn == nil || c.hub == nil {
		return
	}
	c.conn.SetReadLimit(readLimit)
	go c.writeLoop()
	c.readLoop()
}

func (c *Client) Close() {
	if c.hub != nil {
		c.hub.Unregister(c)
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.mu.Lock()
	if c.send != nil {
		close(c.send)
		c.send = nil
	}
	c.mu.Unlock()
}

func (c *Client) readLoop() {
	defer c.Close()
	for {
		var cmd dto.WSBusCommand
		if err := c.conn.ReadJSON(&cmd); err != nil {
			return
		}
		switch strings.TrimSpace(cmd.Type) {
		case dto.WSBusCmdSubscribe:
			c.handleSubscribe(cmd)
		case dto.WSBusCmdUnsubscribe:
			c.handleUnsubscribe(cmd)
		case dto.WSBusCmdPing:
			c.sendAck(cmd.ReqID, "pong", nil)
		default:
			c.sendError(cmd.ReqID, "unsupported_command", "unsupported command", "")
		}
	}
}

func (c *Client) writeLoop() {
	for env := range c.send {
		if c.conn == nil {
			return
		}
		_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteJSON(env); err != nil {
			return
		}
	}
}

func (c *Client) handleSubscribe(cmd dto.WSBusCommand) {
	topics := normalizeTopics(cmd)
	if len(topics) == 0 {
		c.sendError(cmd.ReqID, "bad_request", "topics required", "")
		return
	}
	allowed := make([]string, 0, len(topics))
	for _, topic := range topics {
		if c.authorizer != nil {
			if err := c.authorizer.Authorize(c.ctx, c, topic); err != nil {
				c.sendError(cmd.ReqID, "permission_denied", "subscription rejected", err.Error())
				continue
			}
		}
		c.hub.Subscribe(c, topic)
		allowed = append(allowed, topic)
	}
	if len(allowed) == 0 {
		return
	}
	c.sendAck(cmd.ReqID, "subscribed", allowed)
}

func (c *Client) handleUnsubscribe(cmd dto.WSBusCommand) {
	topics := normalizeTopics(cmd)
	if len(topics) == 0 {
		c.sendError(cmd.ReqID, "bad_request", "topics required", "")
		return
	}
	for _, topic := range topics {
		c.hub.Unsubscribe(c, topic)
	}
	c.sendAck(cmd.ReqID, "unsubscribed", topics)
}

func (c *Client) sendAck(reqID, message string, topics []string) {
	env, err := dto.NewWSBusEnvelope(dto.WSBusTypeAck, "", dto.WSBusAckPayload{
		ReqID:   reqID,
		OK:      true,
		Message: message,
		Topics:  topics,
	}, "")
	if err != nil {
		return
	}
	c.sendEnvelope(env)
}

func (c *Client) sendError(reqID, code, message, detail string) {
	env, err := dto.NewWSBusEnvelope(dto.WSBusTypeError, "", dto.WSBusErrorPayload{
		ReqID:   reqID,
		Code:    code,
		Message: message,
		Detail:  detail,
	}, "")
	if err != nil {
		return
	}
	c.sendEnvelope(env)
}

func (c *Client) sendEnvelope(env dto.WSBusEnvelope) {
	c.mu.RLock()
	ch := c.send
	c.mu.RUnlock()
	if ch == nil {
		return
	}
	select {
	case ch <- env:
	default:
		logger.DebugF(c.ctx, "[ws-bus] drop message topic=%s client=%s", env.Topic, c.ID)
	}
}

func (c *Client) addTopic(topic string) {
	c.mu.Lock()
	c.topics[topic] = struct{}{}
	c.mu.Unlock()
}

func (c *Client) removeTopic(topic string) {
	c.mu.Lock()
	delete(c.topics, topic)
	c.mu.Unlock()
}

func normalizeTopics(cmd dto.WSBusCommand) []string {
	topics := cmd.Topics
	if cmd.Topic != "" {
		topics = append(topics, cmd.Topic)
	}
	out := make([]string, 0, len(topics))
	seen := map[string]struct{}{}
	for _, t := range topics {
		trimmed := strings.TrimSpace(t)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
