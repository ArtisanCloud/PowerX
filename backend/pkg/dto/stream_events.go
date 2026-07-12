// dto/stream_events.go
package dto

// 传输无关（SSE/WS 共用的事件名）
const (
	EventStart     = "start"
	EventMeta      = "meta"
	EventIntent    = "intent"
	EventPlan      = "plan"
	EventNodeStart = "node_start"
	EventNodeEnd   = "node_end"
	EventToken     = "token"
	EventData      = "data"
	EventAction    = "action"
	EventFinal     = "final"
	EventEnd       = "end"
	EventError     = "error"
	EventHeartbeat = "heartbeat"
)

// Agent Run State Protocol events. These events are the public run/task state
// contract consumed by chat, trace and plugin debug UIs.
const (
	EventAgentRunStarted        = "agent_run.started"
	EventAgentRunResponsePlan   = "agent_run.response_plan"
	EventAgentRunIntentDetected = "agent_run.intent_detected"
	EventAgentRunPlanCreated    = "agent_run.plan_created"
	EventAgentRunTaskStatus     = "agent_run.task_status"
	EventAgentRunTaskStarted    = "agent_run.task_started"
	EventAgentRunAwaitingParams = "agent_run.awaiting_params"
	EventAgentRunTaskCompleted  = "agent_run.task_completed"
	EventAgentRunTaskFailed     = "agent_run.task_failed"
	EventAgentRunFinal          = "agent_run.final"
	EventAgentRunEnded          = "agent_run.ended"
)

const (
	AgentTaskStatusPending        = "pending"
	AgentTaskStatusAwaitingParams = "awaiting_params"
	AgentTaskStatusRunning        = "running"
	AgentTaskStatusCompleted      = "completed"
	AgentTaskStatusFailed         = "failed"
	AgentTaskStatusSkipped        = "skipped"
)

type AgentRunEvent struct {
	RunID     string `json:"run_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	Event     string `json:"event"`
	Payload   any    `json:"payload,omitempty"`
}

type AgentRunSummary struct {
	RunID          string `json:"run_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	Status         string `json:"status,omitempty"`
	TotalTasks     int    `json:"total_tasks,omitempty"`
	PendingTasks   int    `json:"pending_tasks,omitempty"`
	AwaitingTasks  int    `json:"awaiting_tasks,omitempty"`
	RunningTasks   int    `json:"running_tasks,omitempty"`
	CompletedTasks int    `json:"completed_tasks,omitempty"`
	FailedTasks    int    `json:"failed_tasks,omitempty"`
	SkippedTasks   int    `json:"skipped_tasks,omitempty"`
	CurrentStage   int    `json:"current_stage,omitempty"`
	TotalStages    int    `json:"total_stages,omitempty"`
	BlockedReason  string `json:"blocked_reason,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type AgentTaskState struct {
	RunID           string           `json:"run_id,omitempty"`
	SessionID       string           `json:"session_id,omitempty"`
	MessageID       string           `json:"message_id,omitempty"`
	TraceID         string           `json:"trace_id,omitempty"`
	TaskID          string           `json:"task_id,omitempty"`
	ParentTaskID    string           `json:"parent_task_id,omitempty"`
	DependsOn       []string         `json:"depends_on,omitempty"`
	Stage           int              `json:"stage,omitempty"`
	ParallelGroup   string           `json:"parallel_group,omitempty"`
	TeamID          string           `json:"team_id,omitempty"`
	AgentID         string           `json:"agent_id,omitempty"`
	AgentKey        string           `json:"agent_key,omitempty"`
	AgentName       string           `json:"agent_name,omitempty"`
	NodeKind        string           `json:"node_kind,omitempty"`
	NodeRef         string           `json:"node_ref,omitempty"`
	SkillID         string           `json:"skill_id,omitempty"`
	CapabilityID    string           `json:"capability_id,omitempty"`
	Action          string           `json:"action,omitempty"`
	FailurePolicy   string           `json:"failure_policy,omitempty"`
	Status          string           `json:"status"`
	Message         string           `json:"message,omitempty"`
	Summary         string           `json:"summary,omitempty"`
	CollectedParams map[string]any   `json:"collected_params,omitempty"`
	MissingFields   []string         `json:"missing_fields,omitempty"`
	Result          any              `json:"result,omitempty"`
	Links           []map[string]any `json:"links,omitempty"`
	Error           any              `json:"error,omitempty"`
	UpdatedAt       string           `json:"updated_at,omitempty"`
}

// Planner mode
const (
	PlannerModeUnified = "unified"
)

// Unified planner node kinds
const (
	NodeKindWorkflow = "workflow"
	NodeKindSkill    = "skill"
	NodeKindTooling  = "tooling"
	NodeKindLLM      = "llm"
	NodeKindHandoff  = "agent_handoff"
)
