package eventfabric

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	RiskLevelLow      = "low"
	RiskLevelMedium   = "medium"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"
)

const (
	GrantStatusPending = "pending"
	GrantStatusActive  = "active"
	GrantStatusRevoked = "revoked"
	GrantStatusExpired = "expired"

	GrantSourceSystemTemplate = "system_template"
	GrantSourceTenantTemplate = "tenant_template"
	GrantSourceSessionTemp    = "session_temp"
)

const (
	GrantConditionTypeResource   = "resource"
	GrantConditionTypeContextTag = "context_tag"
	GrantConditionTypeTimeWindow = "time_window"
)

const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
	ApprovalStatusExpired  = "expired"
)

// AuthorizationCapability 描述可授权的能力定义。
type AuthorizationCapability struct {
	coremodel.PowerUUIDModel

	Namespace        string         `gorm:"column:namespace;type:varchar(128);not null;uniqueIndex:uk_event_auth_capability,priority:1" json:"namespace"`
	Action           string         `gorm:"column:action;type:varchar(128);not null;uniqueIndex:uk_event_auth_capability,priority:2" json:"action"`
	Description      string         `gorm:"column:description;type:text" json:"description,omitempty"`
	RiskLevel        string         `gorm:"column:risk_level;type:varchar(32);not null;default:'low';index:idx_event_auth_capabilities_risk" json:"risk_level"`
	DefaultRateLimit datatypes.JSON `gorm:"column:default_rate_limit;type:jsonb;default:'{}'" json:"default_rate_limit,omitempty"`
	Metadata         datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata,omitempty"`
}

func (m *AuthorizationCapability) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventAuthCapabilities
}

func (m *AuthorizationCapability) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	if m.RiskLevel == "" {
		m.RiskLevel = RiskLevelLow
	}
	return nil
}

// AuthorizationGrant 描述主体在租户下的授权生命周期。
type AuthorizationGrant struct {
	coremodel.PowerUUIDModel

	TenantID        uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null;index:idx_event_auth_grants_tenant" json:"tenant_id"`
	SubjectType     string         `gorm:"column:subject_type;type:varchar(32);not null;index:idx_event_auth_grants_subject,priority:1" json:"subject_type"`
	SubjectID       uuid.UUID      `gorm:"column:subject_id;type:uuid;not null;index:idx_event_auth_grants_subject,priority:2" json:"subject_id"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_event_auth_grants_status" json:"status"`
	Source          string         `gorm:"column:source;type:varchar(32);not null;default:'system_template';index:idx_event_auth_grants_source" json:"source"`
	TemplateID      *uuid.UUID     `gorm:"column:template_id;type:uuid" json:"template_id,omitempty"`
	TTLExpiresAt    *time.Time     `gorm:"column:ttl_expires_at" json:"ttl_expires_at,omitempty"`
	CreatedBy       *uuid.UUID     `gorm:"column:created_by;type:uuid" json:"created_by,omitempty"`
	RevokedBy       *uuid.UUID     `gorm:"column:revoked_by;type:uuid" json:"revoked_by,omitempty"`
	RevokedAt       *time.Time     `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	RevokedReason   string         `gorm:"column:revoked_reason;type:text" json:"revoked_reason,omitempty"`
	Notes           datatypes.JSON `gorm:"column:notes;type:jsonb;default:'{}'" json:"notes,omitempty"`
	Version         uint64         `gorm:"column:version;type:bigint;not null;default:1;index:idx_event_auth_grants_version" json:"version"`
	LastEvaluatedAt *time.Time     `gorm:"column:last_evaluated_at" json:"last_evaluated_at,omitempty"`
}

func (m *AuthorizationGrant) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventAuthGrants
}

func (m *AuthorizationGrant) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	if m.Status == "" {
		m.Status = GrantStatusPending
	}
	if m.Source == "" {
		m.Source = GrantSourceSystemTemplate
	}
	if m.Version == 0 {
		m.Version = 1
	}
	return nil
}

// AuthorizationGrantCapability 关联 Grant 与 Capability，并允许自定义速率。
type AuthorizationGrantCapability struct {
	coremodel.PowerUUIDModel

	GrantID         uuid.UUID      `gorm:"column:grant_id;type:uuid;not null;uniqueIndex:uk_event_auth_grant_capability,priority:1;index:idx_event_auth_grant_capability_grant" json:"grant_id"`
	CapabilityID    uuid.UUID      `gorm:"column:capability_id;type:uuid;not null;uniqueIndex:uk_event_auth_grant_capability,priority:2;index:idx_event_auth_grant_capability_cap" json:"capability_id"`
	CustomRateLimit datatypes.JSON `gorm:"column:custom_rate_limit;type:jsonb" json:"custom_rate_limit,omitempty"`
}

func (m *AuthorizationGrantCapability) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventAuthGrantCapabilities
}

func (m *AuthorizationGrantCapability) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// AuthorizationGrantCondition 存储 Grant 关联的附加条件。
type AuthorizationGrantCondition struct {
	coremodel.PowerUUIDModel

	GrantID    uuid.UUID      `gorm:"column:grant_id;type:uuid;not null;uniqueIndex:uk_event_auth_grant_condition,priority:1;index:idx_event_auth_grant_conditions_grant" json:"grant_id"`
	Type       string         `gorm:"column:type;type:varchar(32);not null;uniqueIndex:uk_event_auth_grant_condition,priority:2;index:idx_event_auth_grant_conditions_type" json:"type"`
	Expression datatypes.JSON `gorm:"column:expression;type:jsonb;not null;uniqueIndex:uk_event_auth_grant_condition,priority:3" json:"expression"`
}

func (m *AuthorizationGrantCondition) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventAuthGrantConditions
}

func (m *AuthorizationGrantCondition) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

// AuthorizationApprovalTicket 跟踪 Challenge 审批生命周期。
type AuthorizationApprovalTicket struct {
	coremodel.PowerUUIDModel

	TenantID           uuid.UUID      `gorm:"column:tenant_id;type:uuid;not null;index:idx_event_auth_tickets_tenant" json:"tenant_id"`
	GrantID            *uuid.UUID     `gorm:"column:grant_id;type:uuid" json:"grant_id,omitempty"`
	RequestFingerprint uuid.UUID      `gorm:"column:request_fingerprint;type:uuid;not null;uniqueIndex:uk_event_auth_ticket_request" json:"request_fingerprint"`
	Status             string         `gorm:"column:status;type:varchar(32);not null;default:'pending';index:idx_event_auth_tickets_status" json:"status"`
	AssignedTeam       string         `gorm:"column:assigned_team;type:varchar(64);not null;default:'secops'" json:"assigned_team"`
	SLAExpiresAt       time.Time      `gorm:"column:sla_expires_at;not null;index:idx_event_auth_tickets_sla" json:"sla_expires_at"`
	DecisionBy         *uuid.UUID     `gorm:"column:decision_by;type:uuid" json:"decision_by,omitempty"`
	DecisionAt         *time.Time     `gorm:"column:decision_at" json:"decision_at,omitempty"`
	DecisionReason     string         `gorm:"column:decision_reason;type:text" json:"decision_reason,omitempty"`
	PayloadSnapshot    datatypes.JSON `gorm:"column:payload_snapshot;type:jsonb;default:'{}'" json:"payload_snapshot,omitempty"`
}

func (m *AuthorizationApprovalTicket) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableEventAuthApprovalTickets
}

func (m *AuthorizationApprovalTicket) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	if m.Status == "" {
		m.Status = ApprovalStatusPending
	}
	if m.AssignedTeam == "" {
		m.AssignedTeam = "secops"
	}
	return nil
}
