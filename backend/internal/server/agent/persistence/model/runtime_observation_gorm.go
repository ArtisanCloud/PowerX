package model

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AgentRunSnapshot freezes only authorized descriptor metadata. It never stores
// raw resource content or a credential.
type AgentRunSnapshot struct {
	coremodel.PowerModel
	UUID                     uuid.UUID      `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"snapshot_uuid"`
	Env                      string         `gorm:"size:32;not null;index:agent_run_snapshot_scope,priority:1" json:"-"`
	TenantUUID               string         `gorm:"column:tenant_uuid;not null;index:agent_run_snapshot_scope,priority:2" json:"tenant_uuid"`
	RunUUID                  uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:agent_run_snapshot_run" json:"run_uuid"`
	AgentUUID                uuid.UUID      `gorm:"type:uuid;not null;index" json:"agent_uuid"`
	SubjectUUID              string         `gorm:"size:64;not null;index" json:"subject_uuid"`
	PolicyVersion            string         `gorm:"size:128;not null" json:"policy_version"`
	AuthorizationFingerprint string         `gorm:"size:128;not null" json:"authorization_fingerprint"`
	Descriptors              datatypes.JSON `gorm:"type:jsonb;not null" json:"descriptors"`
	ExpiresAt                time.Time      `gorm:"not null;index" json:"expires_at"`
}

func (m *AgentRunSnapshot) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentRunSnapshot
}
func (m *AgentRunSnapshot) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// AgentRunObservation stores a redacted summary or artifact reference produced
// through a registered read capability; raw provider payloads stay at source.
type AgentRunObservation struct {
	coremodel.PowerModel
	UUID         uuid.UUID      `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"observation_uuid"`
	Env          string         `gorm:"size:32;not null;index:agent_run_observation_scope,priority:1" json:"-"`
	TenantUUID   string         `gorm:"column:tenant_uuid;not null;index:agent_run_observation_scope,priority:2" json:"tenant_uuid"`
	RunUUID      uuid.UUID      `gorm:"type:uuid;not null;index:agent_run_observation_scope,priority:3" json:"run_uuid"`
	SnapshotUUID uuid.UUID      `gorm:"type:uuid;not null;index" json:"snapshot_uuid"`
	ResourceUUID uuid.UUID      `gorm:"type:uuid;not null;index" json:"resource_uuid"`
	Purpose      string         `gorm:"type:text;not null" json:"purpose"`
	Summary      string         `gorm:"type:text" json:"summary,omitempty"`
	ArtifactRef  string         `gorm:"type:text" json:"artifact_ref,omitempty"`
	Directive    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"directive"`
	Digest       string         `gorm:"size:128;not null;index" json:"digest"`
}

func (m *AgentRunObservation) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentRunObservation
}
func (m *AgentRunObservation) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// AgentPlanRevision is an immutable, replayable plan decision. Task IDs are
// local to the serialized plan; Run/Snapshot/Revision UUIDs are the only
// cross-record identities.
type AgentPlanRevision struct {
	coremodel.PowerModel
	UUID                    uuid.UUID      `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"revision_uuid"`
	Env                     string         `gorm:"size:32;not null;index:agent_plan_revision_scope,priority:1" json:"-"`
	TenantUUID              string         `gorm:"column:tenant_uuid;not null;index:agent_plan_revision_scope,priority:2" json:"tenant_uuid"`
	RunUUID                 uuid.UUID      `gorm:"type:uuid;not null;index:agent_plan_revision_scope,priority:3" json:"run_uuid"`
	SnapshotUUID            uuid.UUID      `gorm:"type:uuid;not null;index" json:"snapshot_uuid"`
	ParentRevisionUUID      *uuid.UUID     `gorm:"type:uuid;index" json:"parent_revision_uuid,omitempty"`
	ReasonCode              string         `gorm:"size:128;not null;index" json:"reason_code"`
	Plan                    datatypes.JSON `gorm:"type:jsonb;not null" json:"plan"`
	TriggerObservationUUIDs datatypes.JSON `gorm:"type:jsonb;not null" json:"trigger_observation_uuids"`
	SupersededTaskRefs      datatypes.JSON `gorm:"type:jsonb;not null" json:"superseded_task_refs"`
	NewTaskRefs             datatypes.JSON `gorm:"type:jsonb;not null" json:"new_task_refs"`
	PlanRevisionsConsumed   int            `gorm:"not null" json:"plan_revisions_consumed"`
	ObservationsConsumed    int            `gorm:"not null" json:"observations_consumed"`
}

func (m *AgentPlanRevision) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentPlanRevision
}
func (m *AgentPlanRevision) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// AgentVerificationEvidence records deterministic runtime verification without
// persisting raw provider results or user-visible free text.
type AgentVerificationEvidence struct {
	coremodel.PowerModel
	UUID                    uuid.UUID      `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"evidence_uuid"`
	Env                     string         `gorm:"size:32;not null;index:agent_verification_evidence_scope,priority:1" json:"-"`
	TenantUUID              string         `gorm:"column:tenant_uuid;not null;index:agent_verification_evidence_scope,priority:2" json:"tenant_uuid"`
	RunUUID                 uuid.UUID      `gorm:"type:uuid;not null;index:agent_verification_evidence_scope,priority:3" json:"run_uuid"`
	SnapshotUUID            uuid.UUID      `gorm:"type:uuid;not null;index" json:"snapshot_uuid"`
	TaskRefs                datatypes.JSON `gorm:"type:jsonb;not null" json:"task_refs"`
	VerdictClass            string         `gorm:"size:32;not null;index" json:"verdict_class"`
	ReasonCode              string         `gorm:"size:128;not null;index" json:"reason_code"`
	UserAction              string         `gorm:"size:128" json:"user_action,omitempty"`
	RequiredInputFields     datatypes.JSON `gorm:"type:jsonb;not null" json:"required_input_fields"`
	RequiredPermissionCodes datatypes.JSON `gorm:"type:jsonb;not null" json:"required_permission_codes"`
	ArtifactRef             string         `gorm:"type:text" json:"artifact_ref,omitempty"`
}

const (
	AgentCapabilityApprovalPending  = "pending"
	AgentCapabilityApprovalApproved = "approved"
	AgentCapabilityApprovalRejected = "rejected"
)

// AgentCapabilityApproval is a session-scoped human decision for exactly one
// frozen capability. Its UUID is the only value a caller may submit to resume.
// The server still resolves every scope field before injecting runtime evidence.
type AgentCapabilityApproval struct {
	coremodel.PowerModel
	UUID                uuid.UUID  `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"approval_uuid"`
	Env                 string     `gorm:"size:32;not null;index:agent_capability_approval_scope,priority:1" json:"-"`
	TenantUUID          string     `gorm:"column:tenant_uuid;not null;index:agent_capability_approval_scope,priority:2" json:"tenant_uuid"`
	RunUUID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"run_uuid"`
	SnapshotUUID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"snapshot_uuid"`
	PlanRevisionUUID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"plan_revision_uuid"`
	AgentUUID           uuid.UUID  `gorm:"type:uuid;not null;index:agent_capability_approval_scope,priority:3" json:"agent_uuid"`
	SessionUUID         uuid.UUID  `gorm:"type:uuid;not null;index:agent_capability_approval_scope,priority:4" json:"session_uuid"`
	MessageUUID         uuid.UUID  `gorm:"type:uuid;not null;index:agent_capability_approval_scope,priority:5" json:"message_uuid"`
	CapabilityUUID      uuid.UUID  `gorm:"type:uuid;not null;index:agent_capability_approval_scope,priority:6" json:"capability_uuid"`
	RequestedByUserUUID string     `gorm:"size:64;not null;index" json:"requested_by_user_uuid"`
	ApprovedByUserUUID  *string    `gorm:"size:64;index" json:"approved_by_user_uuid,omitempty"`
	Status              string     `gorm:"size:16;not null;index:agent_capability_approval_scope,priority:7" json:"status"`
	EvidenceRef         string     `gorm:"type:text;not null" json:"evidence_ref"`
	ApprovedAt          *time.Time `gorm:"index" json:"approved_at,omitempty"`
	RejectedAt          *time.Time `gorm:"index" json:"rejected_at,omitempty"`
}

// AgentRunTaskState is the durable resume boundary. It stores no raw result.
type AgentRunTaskState struct {
	coremodel.PowerModel
	UUID             uuid.UUID `gorm:"type:uuid;column:uuid;uniqueIndex;index" json:"task_state_uuid"`
	Env              string    `gorm:"size:32;not null;index:agent_run_task_state_scope,priority:1" json:"-"`
	TenantUUID       string    `gorm:"column:tenant_uuid;not null;index:agent_run_task_state_scope,priority:2" json:"tenant_uuid"`
	RunUUID          uuid.UUID `gorm:"type:uuid;not null;index:agent_run_task_state_scope,priority:3" json:"run_uuid"`
	SnapshotUUID     uuid.UUID `gorm:"type:uuid;not null;index" json:"snapshot_uuid"`
	PlanRevisionUUID uuid.UUID `gorm:"type:uuid;not null;index:agent_run_task_state_scope,priority:4" json:"plan_revision_uuid"`
	TaskID           string    `gorm:"size:128;not null;index:agent_run_task_state_scope,priority:5" json:"task_id"`
	Status           string    `gorm:"size:32;not null;index" json:"status"`
	ArtifactRef      string    `gorm:"type:text" json:"artifact_ref,omitempty"`
}

func (m *AgentRunTaskState) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentRunTaskState
}
func (m *AgentRunTaskState) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *AgentCapabilityApproval) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentCapabilityApproval
}

func (m *AgentCapabilityApproval) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

func (m *AgentVerificationEvidence) TableName() string {
	return coremodel.PowerXSchema + "." + TableAgentVerificationEvidence
}
func (m *AgentVerificationEvidence) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}
