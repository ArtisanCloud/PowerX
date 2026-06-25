package agent_trace

import (
	"fmt"
	"strings"
	"time"
)

const (
	ErrCodeContextMissing    = "AGENT_TRACE_CONTEXT_MISSING"
	ErrCodeSinkUnavailable   = "AGENT_TRACE_SINK_UNAVAILABLE"
	ErrCodeReportUnsupported = "AGENT_TRACE_REPORT_UNSUPPORTED"
)

const (
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

const (
	EventPhaseStart = "start"
	EventPhaseEnd   = "end"
	EventPhaseError = "error"
	EventPhaseDelta = "delta"
)

const (
	EventStatusRunning = "running"
	EventStatusSuccess = "success"
	EventStatusError   = "error"
	EventStatusSkipped = "skipped"
)

const (
	ArtifactPolicyRedacted = "redacted"
	ArtifactPolicySummary  = "summary"
	ArtifactPolicyRawRoot  = "raw_root_only"
)

const (
	DefaultLocalDir         = "backend/logs/agents"
	DefaultArtifactPolicy   = ArtifactPolicyRedacted
	DefaultMaxArtifactBytes = int64(1024 * 1024)
)

type TraceError struct {
	Code          string
	Message       string
	MissingFields []string
}

func (e *TraceError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.MissingFields) == 0 {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, strings.Join(e.MissingFields, ","))
}

type AgentRunMeta struct {
	TraceID             string         `json:"trace_id"`
	RunID               string         `json:"run_id"`
	TenantUUID          string         `json:"tenant_uuid"`
	UserUUID            string         `json:"user_uuid,omitempty"`
	AgentID             string         `json:"agent_id"`
	SessionID           string         `json:"session_id"`
	MessageID           string         `json:"message_id"`
	PlanID              string         `json:"plan_id,omitempty"`
	Channel             string         `json:"channel,omitempty"`
	UserMessageDigest   string         `json:"user_message_digest,omitempty"`
	FinalResponseDigest string         `json:"final_response_digest,omitempty"`
	Attributes          map[string]any `json:"attributes,omitempty"`
}

type AgentRunContext struct {
	Meta      AgentRunMeta  `json:"meta"`
	Trace     AgentRunTrace `json:"trace"`
	StartedAt time.Time     `json:"started_at"`
}

type AgentRunTrace struct {
	TraceID             string     `json:"trace_id"`
	RunID               string     `json:"run_id"`
	TenantUUID          string     `json:"tenant_uuid"`
	UserUUID            string     `json:"user_uuid,omitempty"`
	AgentID             string     `json:"agent_id"`
	SessionID           string     `json:"session_id"`
	MessageID           string     `json:"message_id"`
	PlanID              string     `json:"plan_id,omitempty"`
	Channel             string     `json:"channel,omitempty"`
	Status              string     `json:"status"`
	NodeCount           int        `json:"node_count"`
	EventCount          int        `json:"event_count"`
	ErrorCount          int        `json:"error_count"`
	WarningCount        int        `json:"warning_count"`
	DurationMS          int64      `json:"duration_ms,omitempty"`
	UserMessageDigest   string     `json:"user_message_digest,omitempty"`
	FinalResponseDigest string     `json:"final_response_digest,omitempty"`
	ArtifactRoot        string     `json:"artifact_root,omitempty"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	EndedAt             *time.Time `json:"ended_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type AgentTraceEvent struct {
	EventID      string         `json:"event_id,omitempty"`
	TraceID      string         `json:"trace_id"`
	RunID        string         `json:"run_id"`
	TenantUUID   string         `json:"tenant_uuid"`
	UserUUID     string         `json:"user_uuid,omitempty"`
	AgentID      string         `json:"agent_id"`
	SessionID    string         `json:"session_id"`
	MessageID    string         `json:"message_id"`
	PlanID       string         `json:"plan_id,omitempty"`
	NodeID       string         `json:"node_id"`
	NodeSeq      int            `json:"node_seq"`
	NodeKind     string         `json:"node_kind"`
	NodeRef      string         `json:"node_ref,omitempty"`
	Phase        string         `json:"phase"`
	Status       string         `json:"status"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
	InputDigest  string         `json:"input_digest,omitempty"`
	OutputDigest string         `json:"output_digest,omitempty"`
	ArtifactRefs []string       `json:"artifact_refs,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorSummary string         `json:"error_summary,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type AgentTraceNode struct {
	AgentRunMeta
	NodeID       string         `json:"node_id"`
	NodeSeq      int            `json:"node_seq"`
	NodeKind     string         `json:"node_kind"`
	NodeRef      string         `json:"node_ref,omitempty"`
	InputDigest  string         `json:"input_digest,omitempty"`
	InputSummary map[string]any `json:"input_summary,omitempty"`
	ContextRef   string         `json:"context_ref,omitempty"`
	SkillID      string         `json:"skill_id,omitempty"`
	PluginID     string         `json:"plugin_id,omitempty"`
	CapabilityID string         `json:"capability_id,omitempty"`
	ExecutorPath string         `json:"executor_path,omitempty"`
	ArtifactRefs []string       `json:"artifact_refs,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	StartedAt    time.Time      `json:"started_at,omitempty"`
}

type AgentTraceNodeResult struct {
	AgentRunMeta
	NodeID           string           `json:"node_id"`
	NodeSeq          int              `json:"node_seq"`
	NodeKind         string           `json:"node_kind"`
	NodeRef          string           `json:"node_ref,omitempty"`
	OutputDigest     string           `json:"output_digest,omitempty"`
	OutputSummary    map[string]any   `json:"output_summary,omitempty"`
	PromptTokens     *int             `json:"prompt_tokens,omitempty"`
	CompletionTokens *int             `json:"completion_tokens,omitempty"`
	CachedTokens     *int             `json:"cached_tokens,omitempty"`
	TrimActions      []map[string]any `json:"trim_actions,omitempty"`
	ArtifactRefs     []string         `json:"artifact_refs,omitempty"`
	Attributes       map[string]any   `json:"attributes,omitempty"`
	StartedAt        time.Time        `json:"started_at,omitempty"`
	EndedAt          time.Time        `json:"ended_at,omitempty"`
}

type AgentTraceNodeFailure struct {
	AgentTraceNodeResult
	ErrorCode    string `json:"error_code"`
	ErrorSummary string `json:"error_summary"`
}

type AgentTraceNodeSnapshot struct {
	NodeID           string           `json:"node_id"`
	NodeSeq          int              `json:"node_seq"`
	NodeKind         string           `json:"node_kind"`
	NodeRef          string           `json:"node_ref,omitempty"`
	PhaseStatus      string           `json:"phase_status"`
	InputSummary     map[string]any   `json:"input_summary,omitempty"`
	OutputSummary    map[string]any   `json:"output_summary,omitempty"`
	ContextRef       string           `json:"context_ref,omitempty"`
	SkillID          string           `json:"skill_id,omitempty"`
	PluginID         string           `json:"plugin_id,omitempty"`
	CapabilityID     string           `json:"capability_id,omitempty"`
	ExecutorPath     string           `json:"executor_path,omitempty"`
	PromptTokens     *int             `json:"prompt_tokens,omitempty"`
	CompletionTokens *int             `json:"completion_tokens,omitempty"`
	CachedTokens     *int             `json:"cached_tokens,omitempty"`
	TrimActions      []map[string]any `json:"trim_actions,omitempty"`
	ErrorCode        string           `json:"error_code,omitempty"`
	ErrorSummary     string           `json:"error_summary,omitempty"`
	ArtifactRefs     []string         `json:"artifact_refs,omitempty"`
	Attributes       map[string]any   `json:"attributes,omitempty"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	EndedAt          *time.Time       `json:"ended_at,omitempty"`
}

type AgentTraceArtifact struct {
	ArtifactID      string    `json:"artifact_id"`
	RunID           string    `json:"run_id"`
	NodeID          string    `json:"node_id,omitempty"`
	ArtifactKind    string    `json:"artifact_kind"`
	URI             string    `json:"uri"`
	Checksum        string    `json:"checksum,omitempty"`
	RedactionPolicy string    `json:"redaction_policy"`
	SizeBytes       int64     `json:"size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
}

type AgentRunResult struct {
	AgentRunMeta
	Status              string         `json:"status"`
	NodeCount           int            `json:"node_count"`
	EventCount          int            `json:"event_count"`
	ErrorCount          int            `json:"error_count"`
	WarningCount        int            `json:"warning_count"`
	DurationMS          int64          `json:"duration_ms,omitempty"`
	FinalResponseDigest string         `json:"final_response_digest,omitempty"`
	ErrorCode           string         `json:"error_code,omitempty"`
	ErrorSummary        string         `json:"error_summary,omitempty"`
	Attributes          map[string]any `json:"attributes,omitempty"`
	StartedAt           time.Time      `json:"started_at,omitempty"`
	EndedAt             time.Time      `json:"ended_at,omitempty"`
}

type AgentReportQuery struct {
	TenantUUID string `json:"tenant_uuid,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	MessageID  string `json:"message_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	Source     string `json:"source,omitempty"`
	Format     string `json:"format,omitempty"`
}

type AgentRunReport struct {
	ReportScope  string                   `json:"report_scope"`
	Format       string                   `json:"format"`
	TenantUUID   string                   `json:"tenant_uuid"`
	SessionID    string                   `json:"session_id"`
	MessageID    string                   `json:"message_id,omitempty"`
	RunID        string                   `json:"run_id"`
	TraceID      string                   `json:"trace_id"`
	GeneratedBy  string                   `json:"generated_by,omitempty"`
	GeneratedAt  time.Time                `json:"generated_at"`
	Summary      map[string]any           `json:"summary,omitempty"`
	Timeline     []AgentTraceEvent        `json:"timeline,omitempty"`
	Nodes        []AgentTraceNodeSnapshot `json:"nodes,omitempty"`
	RunState     *AgentRunStateSnapshot   `json:"run_state,omitempty"`
	Errors       []map[string]any         `json:"errors,omitempty"`
	ArtifactRefs []string                 `json:"artifact_refs,omitempty"`
}

type AgentRunStateSnapshot struct {
	Run           map[string]any       `json:"run,omitempty"`
	Summary       map[string]any       `json:"summary,omitempty"`
	ResponsePlan  map[string]any       `json:"response_plan,omitempty"`
	Intent        map[string]any       `json:"intent,omitempty"`
	Plan          map[string]any       `json:"plan,omitempty"`
	Tasks         []AgentTaskStateItem `json:"tasks,omitempty"`
	PendingParams []AgentTaskStateItem `json:"pending_params,omitempty"`
	Results       []AgentTaskStateItem `json:"results,omitempty"`
	Errors        []AgentTaskStateItem `json:"errors,omitempty"`
	TraceLinks    []map[string]any     `json:"trace_links,omitempty"`
	Final         map[string]any       `json:"final,omitempty"`
	Ended         bool                 `json:"ended,omitempty"`
	UpdatedAt     time.Time            `json:"updated_at,omitempty"`
}

type AgentTaskStateItem struct {
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
	Status          string           `json:"status,omitempty"`
	Message         string           `json:"message,omitempty"`
	Summary         string           `json:"summary,omitempty"`
	CollectedParams map[string]any   `json:"collected_params,omitempty"`
	MissingFields   []string         `json:"missing_fields,omitempty"`
	Result          any              `json:"result,omitempty"`
	Links           []map[string]any `json:"links,omitempty"`
	Error           any              `json:"error,omitempty"`
	UpdatedAt       string           `json:"updated_at,omitempty"`
}

type AgentRunListQuery struct {
	TenantUUID string `json:"tenant_uuid,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Status     string `json:"status,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type AgentRunListItem struct {
	TenantUUID       string     `json:"tenant_uuid"`
	SessionID        string     `json:"session_id"`
	MessageID        string     `json:"message_id"`
	MessagePreview   string     `json:"message_preview,omitempty"`
	MessageRole      string     `json:"message_role,omitempty"`
	MessageCreatedAt *time.Time `json:"message_created_at,omitempty"`
	RunID            string     `json:"run_id"`
	TraceID          string     `json:"trace_id"`
	AgentID          string     `json:"agent_id"`
	Status           string     `json:"status"`
	NodeCount        int        `json:"node_count"`
	EventCount       int        `json:"event_count"`
	ErrorCount       int        `json:"error_count"`
	DurationMS       int64      `json:"duration_ms,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type AgentRunListResult struct {
	Items      []AgentRunListItem `json:"items"`
	TenantUUID string             `json:"tenant_uuid"`
	Total      int                `json:"total"`
	Offset     int                `json:"offset"`
	Limit      int                `json:"limit"`
}

type AgentSessionListItem struct {
	TenantUUID   string     `json:"tenant_uuid"`
	SessionID    string     `json:"session_id"`
	AgentID      string     `json:"agent_id"`
	Status       string     `json:"status"`
	MessageCount int        `json:"message_count"`
	NodeCount    int        `json:"node_count"`
	EventCount   int        `json:"event_count"`
	ErrorCount   int        `json:"error_count"`
	DurationMS   int64      `json:"duration_ms,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	LatestAt     time.Time  `json:"latest_at"`
}

type AgentSessionListResult struct {
	Items      []AgentSessionListItem `json:"items"`
	TenantUUID string                 `json:"tenant_uuid"`
	Total      int                    `json:"total"`
	Offset     int                    `json:"offset"`
	Limit      int                    `json:"limit"`
}
