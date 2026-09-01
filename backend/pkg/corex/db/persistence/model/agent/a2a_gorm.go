package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

const (
	DispatchModeSerial   = "serial"
	DispatchModeParallel = "parallel"
	DispatchModeMixed    = "mixed"

	FailurePolicyFailFast  = "fail-fast"
	FailurePolicyContinue  = "continue"
	FailurePolicyRetryOnce = "retry-once"

	TeamStatusActive   = "active"
	TeamStatusDisabled = "disabled"
)

const (
	TeamRolePlanner   = "planner"
	TeamRoleRetriever = "retriever"
	TeamRoleExecutor  = "executor"
	TeamRoleReviewer  = "reviewer"
)

type TeamRoleOption struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CanBeTL     bool   `json:"can_be_tl"`
	CanBeChild  bool   `json:"can_be_child"`
	Priority    int    `json:"priority"`
}

func TeamRoleOptions() []TeamRoleOption {
	return []TeamRoleOption{
		{Code: TeamRolePlanner, Name: "任务规划", Description: "TL 主智能体角色，负责拆解任务、规划协作并汇总结果。", CanBeTL: true, CanBeChild: false, Priority: 10},
		{Code: TeamRoleRetriever, Name: "资料检索", Description: "子智能体角色，负责检索、整理事实、返回证据摘要。", CanBeTL: false, CanBeChild: true, Priority: 20},
		{Code: TeamRoleExecutor, Name: "任务执行", Description: "子智能体角色，负责执行具体步骤、调用能力并返回执行结果。", CanBeTL: false, CanBeChild: true, Priority: 30},
		{Code: TeamRoleReviewer, Name: "结果复核", Description: "子智能体角色，负责复核结果一致性、风险等级与遗漏项。", CanBeTL: false, CanBeChild: true, Priority: 40},
	}
}

func TeamChildRoleOptions() []TeamRoleOption {
	all := TeamRoleOptions()
	out := make([]TeamRoleOption, 0, len(all))
	for _, item := range all {
		if item.CanBeChild {
			out = append(out, item)
		}
	}
	return out
}

func DefaultChildTeamRole() string {
	return TeamRoleExecutor
}

func IsChildTeamRole(role string) bool {
	normalized := strings.ToLower(strings.TrimSpace(role))
	for _, item := range TeamChildRoleOptions() {
		if item.Code == normalized {
			return true
		}
	}
	return false
}

const (
	TaskStatusQueued    = "queued"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusTimedOut  = "timed_out"
	TaskStatusCancelled = "cancelled"
)

// AgentTeam defines parent/child agent collaboration scope under one tenant.
type AgentTeam struct {
	coremodel.PowerUUIDModel

	TenantUUID    string `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_agent_team_tenant;uniqueIndex:idx_agent_team_tenant_key,priority:1" json:"tenant_uuid"`
	ParentAgentID uint64 `gorm:"column:parent_agent_id;not null;index:idx_agent_team_parent" json:"parent_agent_id"`
	// TeamKey is a stable machine-semantic identifier. It is never rendered as
	// the business name and is unique only within a tenant.
	TeamKey              string         `gorm:"column:team_key;type:varchar(128);not null;uniqueIndex:idx_agent_team_tenant_key,priority:2" json:"team_key"`
	DisplayNameI18n      datatypes.JSON `gorm:"column:display_name_i18n;type:jsonb;not null;default:'{}'" json:"display_name_i18n"`
	DispatchMode         string         `gorm:"column:dispatch_mode;type:varchar(16);not null;default:'parallel'" json:"dispatch_mode"`
	DefaultFailurePolicy string         `gorm:"column:default_failure_policy;type:varchar(16);not null;default:'continue'" json:"default_failure_policy"`
	Status               string         `gorm:"column:status;type:varchar(16);not null;default:'active';index:idx_agent_team_status" json:"status"`
	CreatedBy            string         `gorm:"column:created_by;type:varchar(64)" json:"created_by"`
	// OrchestrationSpec is the authoritative, versioned team task graph. Runtime
	// must compile this data and must not branch on a mutable team name.
	OrchestrationSpec datatypes.JSON `gorm:"column:orchestration_spec;type:jsonb;not null;default:'{}'" json:"orchestration_spec"`
}

const TeamOrchestrationSchemaV1 = "powerx.agent.team-orchestration/v1"

type TeamOrchestrationSpec struct {
	Schema string                  `json:"schema"`
	Tasks  []TeamOrchestrationTask `json:"tasks"`
}

type TeamOrchestrationTask struct {
	TaskID        string   `json:"task_id"`
	NodeKind      string   `json:"node_kind"`
	AssigneeRole  string   `json:"assignee_role"`
	SkillID       string   `json:"skill_id"`
	Stage         int      `json:"stage"`
	DependsOn     []string `json:"depends_on,omitempty"`
	FailurePolicy string   `json:"failure_policy,omitempty"`
}

func ParseTeamOrchestrationSpec(raw []byte) (TeamOrchestrationSpec, error) {
	var spec TeamOrchestrationSpec
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "{}" {
		return spec, fmt.Errorf("team orchestration is required")
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return spec, fmt.Errorf("team orchestration is invalid: %w", err)
	}
	if spec.Schema != TeamOrchestrationSchemaV1 {
		return spec, fmt.Errorf("unsupported team orchestration schema: %s", strings.TrimSpace(spec.Schema))
	}
	if len(spec.Tasks) == 0 {
		return spec, fmt.Errorf("team orchestration requires tasks")
	}
	seen := map[string]struct{}{}
	for _, task := range spec.Tasks {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			return spec, fmt.Errorf("team orchestration task_id is required")
		}
		if _, ok := seen[taskID]; ok {
			return spec, fmt.Errorf("team orchestration task_id duplicated: %s", taskID)
		}
		seen[taskID] = struct{}{}
		if task.Stage <= 0 {
			return spec, fmt.Errorf("team orchestration task stage is required: %s", taskID)
		}
		if strings.TrimSpace(task.SkillID) == "" {
			return spec, fmt.Errorf("team orchestration task skill_id is required: %s", taskID)
		}
		switch strings.ToLower(strings.TrimSpace(task.NodeKind)) {
		case "agent_handoff":
			if !IsChildTeamRole(task.AssigneeRole) {
				return spec, fmt.Errorf("team orchestration handoff role is invalid: %s", taskID)
			}
		case "skill":
			if !strings.EqualFold(strings.TrimSpace(task.AssigneeRole), TeamRolePlanner) {
				return spec, fmt.Errorf("team orchestration skill task must be assigned to planner: %s", taskID)
			}
		default:
			return spec, fmt.Errorf("team orchestration node_kind is invalid: %s", taskID)
		}
		for _, dependency := range task.DependsOn {
			if strings.TrimSpace(dependency) == "" {
				return spec, fmt.Errorf("team orchestration dependency is empty: %s", taskID)
			}
		}
	}
	for _, task := range spec.Tasks {
		for _, dependency := range task.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if dependency == strings.TrimSpace(task.TaskID) {
				return spec, fmt.Errorf("team orchestration task cannot depend on itself: %s", task.TaskID)
			}
			if _, ok := seen[dependency]; !ok {
				return spec, fmt.Errorf("team orchestration dependency not found: %s", dependency)
			}
		}
	}
	// The stage is a scheduling hint, not a substitute for a DAG check. A
	// cyclic graph would otherwise be accepted and fail only after a run starts.
	visiting := make(map[string]bool, len(spec.Tasks))
	visited := make(map[string]bool, len(spec.Tasks))
	byID := make(map[string]TeamOrchestrationTask, len(spec.Tasks))
	for _, task := range spec.Tasks {
		byID[strings.TrimSpace(task.TaskID)] = task
	}
	var visit func(string) error
	visit = func(taskID string) error {
		if visiting[taskID] {
			return fmt.Errorf("team orchestration dependency cycle detected at: %s", taskID)
		}
		if visited[taskID] {
			return nil
		}
		visiting[taskID] = true
		for _, dependency := range byID[taskID].DependsOn {
			if err := visit(strings.TrimSpace(dependency)); err != nil {
				return err
			}
		}
		visiting[taskID] = false
		visited[taskID] = true
		return nil
	}
	for taskID := range byID {
		if err := visit(taskID); err != nil {
			return spec, err
		}
	}
	return spec, nil
}

func ValidateTeamIdentity(teamKey string, displayNameI18n []byte) error {
	teamKey = strings.TrimSpace(teamKey)
	if teamKey == "" {
		return fmt.Errorf("team_key is required")
	}
	for _, char := range teamKey {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '.' && char != '_' && char != '-' {
			return fmt.Errorf("team_key contains unsupported character")
		}
	}
	var names map[string]string
	if err := json.Unmarshal(displayNameI18n, &names); err != nil {
		return fmt.Errorf("display_name_i18n is invalid: %w", err)
	}
	for _, locale := range []string{"zh-CN", "en-US", "ja", "ko"} {
		if strings.TrimSpace(names[locale]) == "" {
			return fmt.Errorf("display_name_i18n requires %s", locale)
		}
	}
	return nil
}

func (AgentTeam) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableAgentTeams
}

func (m *AgentTeam) Normalize() {
	m.TenantUUID = strings.ToLower(strings.TrimSpace(m.TenantUUID))
	m.TeamKey = strings.TrimSpace(m.TeamKey)
	m.DispatchMode = normalizeDispatchMode(m.DispatchMode)
	m.DefaultFailurePolicy = normalizeFailurePolicy(m.DefaultFailurePolicy)
	m.Status = normalizeTeamStatus(m.Status)
	m.CreatedBy = strings.TrimSpace(m.CreatedBy)
}

// AgentTeamMember stores child agent bind and role.
type AgentTeamMember struct {
	coremodel.PowerUUIDModel

	TeamID       uint64 `gorm:"column:team_id;not null;index:idx_agent_team_member_unique,unique,priority:1" json:"team_id"`
	TenantUUID   string `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_agent_team_member_tenant" json:"tenant_uuid"`
	ChildAgentID uint64 `gorm:"column:child_agent_id;not null;index:idx_agent_team_member_unique,unique,priority:2" json:"child_agent_id"`
	Role         string `gorm:"column:role;type:varchar(32);not null;default:'executor'" json:"role"`
	Priority     int    `gorm:"column:priority;not null;default:100" json:"priority"`
	Enabled      bool   `gorm:"column:enabled;not null;default:true;index:idx_agent_team_member_enabled" json:"enabled"`
}

func (AgentTeamMember) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableAgentTeamMembers
}

func (m *AgentTeamMember) Normalize() {
	m.TenantUUID = strings.ToLower(strings.TrimSpace(m.TenantUUID))
	m.Role = strings.ToLower(strings.TrimSpace(m.Role))
	if m.Role == "" {
		m.Role = DefaultChildTeamRole()
	}
	if m.Priority <= 0 {
		m.Priority = 100
	}
}

// AgentHandoffTask records one handoff execution record.
type AgentHandoffTask struct {
	coremodel.PowerUUIDModel

	TaskID         string     `gorm:"column:task_id;type:varchar(128);not null;uniqueIndex:uk_agent_handoff_task" json:"task_id"`
	TeamID         uint64     `gorm:"column:team_id;not null;index:idx_agent_handoff_team" json:"team_id"`
	TenantUUID     string     `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_agent_handoff_tenant" json:"tenant_uuid"`
	ParentAgentID  uint64     `gorm:"column:parent_agent_id;not null;index:idx_agent_handoff_parent" json:"parent_agent_id"`
	ChildAgentID   uint64     `gorm:"column:child_agent_id;not null;index:idx_agent_handoff_child" json:"child_agent_id"`
	SessionID      uint64     `gorm:"column:session_id;not null;default:0;index:idx_agent_handoff_session" json:"session_id"`
	PlanID         string     `gorm:"column:plan_id;type:varchar(128);index:idx_agent_handoff_plan" json:"plan_id"`
	NodeID         string     `gorm:"column:node_id;type:varchar(128);index:idx_agent_handoff_node" json:"node_id"`
	ContextRef     string     `gorm:"column:context_ref;type:varchar(128);index:idx_agent_handoff_ctx" json:"context_ref"`
	InputDigest    string     `gorm:"column:input_digest;type:varchar(128)" json:"input_digest"`
	OutputDigest   string     `gorm:"column:output_digest;type:varchar(128)" json:"output_digest"`
	FailurePolicy  string     `gorm:"column:failure_policy;type:varchar(16);not null;default:'continue'" json:"failure_policy"`
	Status         string     `gorm:"column:status;type:varchar(16);not null;default:'queued';index:idx_agent_handoff_status" json:"status"`
	ErrorCode      string     `gorm:"column:error_code;type:varchar(128)" json:"error_code"`
	ErrorSummary   string     `gorm:"column:error_summary;type:text" json:"error_summary"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"started_at"`
	EndedAt        *time.Time `gorm:"column:ended_at" json:"ended_at"`
	HandoffTraceID string     `gorm:"column:handoff_trace_id;type:varchar(128);index:idx_agent_handoff_trace" json:"handoff_trace_id"`
}

func (AgentHandoffTask) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableAgentHandoffTasks
}

func (m *AgentHandoffTask) Normalize() {
	m.TaskID = strings.TrimSpace(m.TaskID)
	m.TenantUUID = strings.ToLower(strings.TrimSpace(m.TenantUUID))
	m.PlanID = strings.TrimSpace(m.PlanID)
	m.NodeID = strings.TrimSpace(m.NodeID)
	m.ContextRef = strings.TrimSpace(m.ContextRef)
	m.InputDigest = strings.TrimSpace(m.InputDigest)
	m.OutputDigest = strings.TrimSpace(m.OutputDigest)
	m.FailurePolicy = normalizeFailurePolicy(m.FailurePolicy)
	m.Status = normalizeTaskStatus(m.Status)
	m.ErrorCode = strings.TrimSpace(m.ErrorCode)
	m.ErrorSummary = strings.TrimSpace(m.ErrorSummary)
	m.HandoffTraceID = strings.TrimSpace(m.HandoffTraceID)
}

// AgentSharedContextRef controls context sharing boundary between parent and children.
type AgentSharedContextRef struct {
	coremodel.PowerUUIDModel

	ContextRefID      string     `gorm:"column:context_ref_id;type:varchar(128);not null;uniqueIndex:uk_agent_ctx_ref" json:"context_ref_id"`
	TenantUUID        string     `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_agent_ctx_ref_tenant" json:"tenant_uuid"`
	SessionID         uint64     `gorm:"column:session_id;not null;default:0;index:idx_agent_ctx_ref_session" json:"session_id"`
	OwnerAgentID      uint64     `gorm:"column:owner_agent_id;not null;index:idx_agent_ctx_ref_owner" json:"owner_agent_id"`
	VisibleToAgentIDs string     `gorm:"column:visible_to_agent_ids;type:text;not null;default:''" json:"visible_to_agent_ids"`
	PayloadURI        string     `gorm:"column:payload_uri;type:text;not null" json:"payload_uri"`
	Checksum          string     `gorm:"column:checksum;type:varchar(128)" json:"checksum"`
	ExpiresAt         *time.Time `gorm:"column:expires_at;index:idx_agent_ctx_ref_expire" json:"expires_at"`
}

func (AgentSharedContextRef) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableAgentSharedContextRefs
}

func (m *AgentSharedContextRef) Normalize() {
	m.ContextRefID = strings.TrimSpace(m.ContextRefID)
	m.TenantUUID = strings.ToLower(strings.TrimSpace(m.TenantUUID))
	m.VisibleToAgentIDs = strings.TrimSpace(m.VisibleToAgentIDs)
	m.PayloadURI = strings.TrimSpace(m.PayloadURI)
	m.Checksum = strings.TrimSpace(m.Checksum)
}

func normalizeDispatchMode(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case DispatchModeSerial, DispatchModeParallel, DispatchModeMixed:
		return s
	default:
		return DispatchModeParallel
	}
}

func normalizeFailurePolicy(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case FailurePolicyFailFast, FailurePolicyContinue, FailurePolicyRetryOnce:
		return s
	default:
		return FailurePolicyContinue
	}
}

func normalizeTeamStatus(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case TeamStatusActive, TeamStatusDisabled:
		return s
	default:
		return TeamStatusActive
	}
}

func normalizeTaskStatus(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case TaskStatusQueued, TaskStatusRunning, TaskStatusCompleted, TaskStatusFailed, TaskStatusTimedOut, TaskStatusCancelled:
		return s
	default:
		return TaskStatusQueued
	}
}
