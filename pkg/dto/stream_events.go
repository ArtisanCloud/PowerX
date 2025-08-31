package dto

// 事件名常量（SSE 的 event / WS 的 WSMessage.Type）
const (
	EventStart     = "start"
	EventIntent    = "intent"
	EventToken     = "token"
	EventAction    = "action"
	EventData      = "data"
	EventFinal     = "final"
	EventEnd       = "end"
	EventError     = "error"
	EventHeartbeat = "heartbeat"
)
