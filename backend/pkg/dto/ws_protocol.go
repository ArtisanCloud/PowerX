// dto/ws_protocol.go
package dto

import (
	"encoding/json"
	"time"
)

// ===== 1) 协议/事件/指令常量（与 SSE 对齐）=====

const ProtocolVersion = "px.ws.v1"

// 客户端 -> 服务端（Commands）
const (
	WSHello        = "hello"
	WSJoinSession  = "join_session"
	WSLeaveSession = "leave_session"
	WSChatSend     = "chat_send"
	WSTypingStart  = "typing_start"
	WSTypingStop   = "typing_stop"
	WSCancel       = "cancel"
	WSFetchHistory = "fetch_history"
	WSPing         = "ping"
	WSAckEvent     = "ack_event"
	WSActionResult = "action_result"
)

// 服务端 -> 客户端（业务事件补充）
const (
	EventMessageCreated = "message_created"
	EventExecStatus     = "execution_status"
	EventRateLimit      = "rate_limit"
)

// WS 事件名别名（仅为可读性，底层语义与 Event* 保持一致）
const (
	WSWelcome = "welcome"
	WSAck     = "ack"
	WSPong    = "pong"

	WSStart     = EventStart
	WSIntent    = EventIntent
	WSPlan      = EventPlan
	WSToken     = EventToken
	WSData      = EventData
	WSAction    = EventAction
	WSFinal     = EventFinal
	WSEnd       = EventEnd
	WSHeartbeat = EventHeartbeat
	WSError     = EventError

	WSMessageCreated = EventMessageCreated
	WSExecStatus     = EventExecStatus
	WSRateLimit      = EventRateLimit
)

// ===== 2) 统一信封（传输层）=====

type WSEnvelope struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp int64           `json:"timestamp"` // UTC 秒
}

func NewEnv(t string, payload any) (WSEnvelope, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return WSEnvelope{}, err
		}
		raw = b
	}
	return WSEnvelope{
		Type:      t,
		Data:      raw,
		Timestamp: time.Now().UTC().Unix(),
	}, nil
}

// ===== 3) 负载类型（强类型定义）=====

// —— Commands

type HelloPayload struct {
	Token        string   `json:"token,omitempty"`
	ClientID     string   `json:"client_id,omitempty"`
	Protocol     string   `json:"protocol,omitempty"` // 建议 px.ws.v1
	Resume       bool     `json:"resume,omitempty"`
	LastEventSeq int64    `json:"last_event_seq,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type JoinSessionPayload struct {
	Env       string `json:"env"`
	AgentID   uint64 `json:"agent_id"`
	SessionID uint64 `json:"session_id,omitempty"`
	Singleton bool   `json:"singleton,omitempty"`
	ReqID     string `json:"req_id,omitempty"`
}

type LeaveSessionPayload struct {
	SessionID uint64 `json:"session_id"`
	ReqID     string `json:"req_id,omitempty"`
}

type ChatSendPayload struct {
	SessionID uint64                 `json:"session_id"`
	Message   string                 `json:"message"`
	Config    *ChatConfig            `json:"config,omitempty"` // 来自 dto/chat.go
	Route     *RouteOptions          `json:"route,omitempty"`
	Exec      *ExecOptions           `json:"exec,omitempty"`
	ReqID     string                 `json:"req_id,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

type TypingPayload struct {
	SessionID uint64 `json:"session_id"`
}

type CancelPayload struct {
	ExecutionID string `json:"execution_id"`
	Reason      string `json:"reason,omitempty"`
}

type FetchHistoryPayload struct {
	SessionID uint64 `json:"session_id"`
	Limit     int    `json:"limit,omitempty"`    // 默认 50
	AfterID   uint64 `json:"after_id,omitempty"` // 游标
	ReqID     string `json:"req_id,omitempty"`
}

type PingPayload struct {
	Seq int64 `json:"seq"`
}

type AckEventPayload struct {
	EventID string `json:"event_id,omitempty"`
	Seq     int64  `json:"seq,omitempty"`
}

type ActionResultPayload struct {
	ActionID    string                 `json:"action_id"`
	ExecutionID string                 `json:"execution_id,omitempty"`
	OK          bool                   `json:"ok"`
	Result      map[string]interface{} `json:"result,omitempty"`
}

// —— Events

type WelcomePayload struct {
	Protocol     string `json:"protocol"`
	Server       string `json:"server"`
	HeartbeatSec int    `json:"heartbeat_sec,omitempty"`
	Resumed      bool   `json:"resumed,omitempty"`
}

type AckPayload struct {
	ReqID       string `json:"req_id,omitempty"`
	OK          bool   `json:"ok"`
	ExecutionID string `json:"execution_id,omitempty"`
	SessionID   uint64 `json:"session_id,omitempty"`
	Message     string `json:"message,omitempty"`
}

type IntentPayload struct {
	Matched  bool    `json:"matched"`
	Strategy string  `json:"strategy,omitempty"`
	Reason   string  `json:"reason,omitempty"`
	Score    float64 `json:"score,omitempty"`
	AgentID  string  `json:"agent_id,omitempty"`
	FlowID   string  `json:"flow_id,omitempty"`
}

type PlanTask struct {
	TaskID      string            `json:"task_id"`
	FlowID      string            `json:"flow_id"`
	NodeKind    string            `json:"node_kind,omitempty"`
	NodeRef     string            `json:"node_ref,omitempty"`
	SourceScope string            `json:"source_scope,omitempty"`
	AgentID     string            `json:"agent_id,omitempty"`
	Params      map[string]any    `json:"params,omitempty"`
	ParamRefs   map[string]string `json:"param_refs,omitempty"`
	Stage       int               `json:"stage"`
	DependsOn   []string          `json:"depends_on,omitempty"`
}

type PlanPayload struct {
	PlanID string     `json:"plan_id"`
	Tasks  []PlanTask `json:"tasks"`
}

type StartPayload struct {
	FlowID      string `json:"flow_id"`
	ExecutionID string `json:"execution_id"`
	SessionID   uint64 `json:"session_id,omitempty"`
	Seq         int64  `json:"seq,omitempty"`
}

type TokenPayload struct {
	Delta     string `json:"delta"`
	StepID    string `json:"step_id,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Seq       int64  `json:"seq,omitempty"`
}

type DataPayload struct {
	Success   bool                   `json:"success"`
	StepID    string                 `json:"step_id,omitempty"`
	Timestamp int64                  `json:"timestamp,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Seq       int64                  `json:"seq,omitempty"`
}

type ActionPayload struct {
	Kind        string                 `json:"kind"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	ExecutionID string                 `json:"execution_id,omitempty"`
	ActionID    string                 `json:"action_id"`
}

type FinalPayload struct {
	Success   bool                   `json:"success"`
	Timestamp int64                  `json:"timestamp,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Seq       int64                  `json:"seq,omitempty"`
}

type EndPayload struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type MessageRef struct {
	ID        uint64    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type MessageCreatedPayload struct {
	SessionID uint64     `json:"session_id"`
	Message   MessageRef `json:"message"`
}

type ExecStatusPayload struct {
	ExecutionID string  `json:"execution_id"`
	Status      string  `json:"status"` // running|completed|failed|cancelled
	Progress    float64 `json:"progress,omitempty"`
	CurrentStep string  `json:"current_step,omitempty"`
}

type RateLimitPayload struct {
	LimitPerMin int `json:"limit_per_min"`
	ResetSec    int `json:"reset_sec"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	ReqID   string `json:"req_id,omitempty"`
}

type HeartbeatPayload struct {
	TS  int64 `json:"ts"`
	Seq int64 `json:"seq,omitempty"`
}

type PongPayload struct {
	Seq int64 `json:"seq"`
}

// ===== 4) 负载注册表（可扩展）=====

var wsPayloadRegistry = map[string]func() any{
	// commands
	WSHello:        func() any { return &HelloPayload{} },
	WSJoinSession:  func() any { return &JoinSessionPayload{} },
	WSLeaveSession: func() any { return &LeaveSessionPayload{} },
	WSChatSend:     func() any { return &ChatSendPayload{} },
	WSTypingStart:  func() any { return &TypingPayload{} },
	WSTypingStop:   func() any { return &TypingPayload{} },
	WSCancel:       func() any { return &CancelPayload{} },
	WSFetchHistory: func() any { return &FetchHistoryPayload{} },
	WSPing:         func() any { return &PingPayload{} },
	WSAckEvent:     func() any { return &AckEventPayload{} },
	WSActionResult: func() any { return &ActionResultPayload{} },

	// events
	WSWelcome:        func() any { return &WelcomePayload{} },
	WSAck:            func() any { return &AckPayload{} },
	WSIntent:         func() any { return &IntentPayload{} },
	WSPlan:           func() any { return &PlanPayload{} },
	WSStart:          func() any { return &StartPayload{} },
	WSToken:          func() any { return &TokenPayload{} },
	WSData:           func() any { return &DataPayload{} },
	WSAction:         func() any { return &ActionPayload{} },
	WSFinal:          func() any { return &FinalPayload{} },
	WSEnd:            func() any { return &EndPayload{} },
	WSMessageCreated: func() any { return &MessageCreatedPayload{} },
	WSExecStatus:     func() any { return &ExecStatusPayload{} },
	WSRateLimit:      func() any { return &RateLimitPayload{} },
	WSError:          func() any { return &ErrorPayload{} },
	WSHeartbeat:      func() any { return &HeartbeatPayload{} },
	WSPong:           func() any { return &PongPayload{} },
}

// DecodePayload：把 envelope.Data 解到 out
func DecodePayload(env WSEnvelope, out any) error {
	if len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// MustDecodeInto：动态解码，返回强类型 payload
func MustDecodeInto(env WSEnvelope) (any, bool, error) {
	newFn, ok := wsPayloadRegistry[env.Type]
	if !ok {
		return nil, false, nil
	}
	holder := newFn()
	if len(env.Data) != 0 {
		if err := json.Unmarshal(env.Data, holder); err != nil {
			return nil, false, err
		}
	}
	return holder, true, nil
}
