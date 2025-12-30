package manager

import (
	"time"

	"github.com/google/uuid"
)

const (
	LifecyclePending   = "pending"
	LifecycleActive    = "active"
	LifecycleSuspended = "suspended"
	LifecycleRetired   = "retired"

	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
)

// RateLimitPolicy 描述限流策略。
type RateLimitPolicy struct {
	Limit         uint64 `json:"limit"`
	Burst         uint64 `json:"burst"`
	WindowSeconds int    `json:"window_seconds"`
	Scope         string `json:"scope"`
}

// EventTopics 描述事件主题配置。
type EventTopics struct {
	Created             string `json:"created,omitempty"`
	Updated             string `json:"updated,omitempty"`
	InvocationSucceeded string `json:"invocation_succeeded,omitempty"`
	InvocationFailed    string `json:"invocation_failed,omitempty"`
}

// Route 表示对外返回的集成入口。
type Route struct {
	RouteID         uuid.UUID       `json:"route_id"`
	TenantUUID      string          `json:"tenant_uuid"`
	RouteSlug       string          `json:"route_slug"`
	CapabilityID    string          `json:"capability_id"`
	ToolGrantIDs    []string        `json:"tool_grant_ids"`
	Channels        []string        `json:"channels"`
	RateLimit       RateLimitPolicy `json:"rate_limit"`
	EventTopics     EventTopics     `json:"event_topics"`
	LifecycleState  string          `json:"lifecycle_state"`
	Status          string          `json:"status"`
	CurrentVersion  uint32          `json:"current_version"`
	Description     string          `json:"description,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	LastActivityAt  *time.Time      `json:"last_activity_at,omitempty"`
	LastPublishedAt *time.Time      `json:"last_published_at,omitempty"`
}

// RouteVersion 描述历史快照。
type RouteVersion struct {
	Version       uint32    `json:"version"`
	ChangeType    string    `json:"change_type"`
	ChangeSummary string    `json:"change_summary,omitempty"`
	ChangedBy     string    `json:"changed_by,omitempty"`
	TraceID       string    `json:"trace_id,omitempty"`
	ChangedAt     time.Time `json:"changed_at"`
	Snapshot      Route     `json:"snapshot"`
}

// EventPayload 在事件发布时使用。
type EventPayload struct {
	Route   Route  `json:"route"`
	Actor   string `json:"actor"`
	Change  string `json:"change"`
	TraceID string `json:"trace_id"`
}
