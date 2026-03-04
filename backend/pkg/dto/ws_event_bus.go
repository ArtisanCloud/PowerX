package dto

import (
	"encoding/json"
	"time"
)

// Event bus protocol constants (WS bus).
const (
	WSBusCmdSubscribe   = "subscribe"
	WSBusCmdUnsubscribe = "unsubscribe"
	WSBusCmdPing        = "ping"

	WSBusTypeWelcome = "welcome"
	WSBusTypeAck     = "ack"
	WSBusTypeError   = "error"
	WSBusTypeEvent   = "event"
)

// WSBusEnvelope is the unified message envelope for the WS event bus.
type WSBusEnvelope struct {
	Topic     string          `json:"topic,omitempty"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"ts"`
	TraceID   string          `json:"trace_id,omitempty"`
}

func NewWSBusEnvelope(t, topic string, payload any, traceID string) (WSBusEnvelope, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return WSBusEnvelope{}, err
		}
		raw = b
	}
	return WSBusEnvelope{
		Topic:     topic,
		Type:      t,
		Payload:   raw,
		Timestamp: time.Now().UTC().UnixMilli(),
		TraceID:   traceID,
	}, nil
}

// WSBusCommand defines client commands to the WS bus.
type WSBusCommand struct {
	Type   string   `json:"type"`
	Topic  string   `json:"topic,omitempty"`
	Topics []string `json:"topics,omitempty"`
	ReqID  string   `json:"req_id,omitempty"`
}

type WSBusWelcomePayload struct {
	Protocol     string `json:"protocol"`
	Server       string `json:"server"`
	HeartbeatSec int    `json:"heartbeat_sec,omitempty"`
}

type WSBusAckPayload struct {
	ReqID   string   `json:"req_id,omitempty"`
	OK      bool     `json:"ok"`
	Message string   `json:"message,omitempty"`
	Topics  []string `json:"topics,omitempty"`
}

type WSBusErrorPayload struct {
	ReqID   string `json:"req_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}
