package agent

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
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

	TenantUUID           string `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_agent_team_tenant" json:"tenant_uuid"`
	ParentAgentID        uint64 `gorm:"column:parent_agent_id;not null;index:idx_agent_team_parent" json:"parent_agent_id"`
	TeamName             string `gorm:"column:team_name;type:varchar(128);not null" json:"team_name"`
	DispatchMode         string `gorm:"column:dispatch_mode;type:varchar(16);not null;default:'parallel'" json:"dispatch_mode"`
	DefaultFailurePolicy string `gorm:"column:default_failure_policy;type:varchar(16);not null;default:'continue'" json:"default_failure_policy"`
	Status               string `gorm:"column:status;type:varchar(16);not null;default:'active';index:idx_agent_team_status" json:"status"`
	CreatedBy            string `gorm:"column:created_by;type:varchar(64)" json:"created_by"`
}

func (AgentTeam) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableAgentTeams
}

func (m *AgentTeam) Normalize() {
	m.TenantUUID = strings.ToLower(strings.TrimSpace(m.TenantUUID))
	m.TeamName = strings.TrimSpace(m.TeamName)
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
