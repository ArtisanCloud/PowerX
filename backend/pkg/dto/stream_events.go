// dto/stream_events.go
package dto

// 传输无关（SSE/WS 共用的事件名）
const (
	EventStart     = "start"
	EventMeta      = "meta"
	EventIntent    = "intent"
	EventPlan      = "plan"
	EventToken     = "token"
	EventData      = "data"
	EventAction    = "action"
	EventFinal     = "final"
	EventEnd       = "end"
	EventError     = "error"
	EventHeartbeat = "heartbeat"
)
