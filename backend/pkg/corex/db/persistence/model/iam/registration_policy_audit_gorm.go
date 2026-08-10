package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

const (
	RegistrationPolicyAuditEventPolicyChanged   = "policy_changed"
	RegistrationPolicyAuditEventEvaluateAllowed = "evaluate_allowed"
	RegistrationPolicyAuditEventEvaluateDenied  = "evaluate_denied"
	RegistrationPolicyAuditEventEvaluatePending = "evaluate_pending"
	RegistrationPolicyAuditEventInviteConsumed  = "invite_consumed"
	RegistrationPolicyAuditEventRequestApproved = "request_approved"
	RegistrationPolicyAuditEventRequestRejected = "request_rejected"

	RegistrationPolicyAuditDecisionAllow   = "allow"
	RegistrationPolicyAuditDecisionDeny    = "deny"
	RegistrationPolicyAuditDecisionPending = "pending"
)

type RegistrationPolicyAuditEvent struct {
	model.PowerUUIDModel

	EventType      string         `gorm:"column:event_type;type:varchar(64);not null;index" json:"event_type"`
	PolicyUUID     string         `gorm:"column:policy_uuid;type:char(36);not null;index" json:"policy_uuid"`
	PolicyVersion  int            `gorm:"column:policy_version;not null;index" json:"policy_version"`
	ContactHash    string         `gorm:"column:contact_hash;type:char(64);index" json:"contact_hash,omitempty"`
	TenantUUID     string         `gorm:"column:tenant_uuid;type:char(36);index" json:"tenant_uuid,omitempty"`
	RequestUUID    string         `gorm:"column:request_uuid;type:char(36);index" json:"request_uuid,omitempty"`
	InviteCodeUUID string         `gorm:"column:invite_code_uuid;type:char(36);index" json:"invite_code_uuid,omitempty"`
	Decision       string         `gorm:"column:decision;type:varchar(32);not null;index" json:"decision"`
	ReasonCode     string         `gorm:"column:reason_code;type:varchar(64);index" json:"reason_code,omitempty"`
	MatchedRules   datatypes.JSON `gorm:"column:matched_rules;type:jsonb;not null;default:'[]'" json:"matched_rules,omitempty"`
	IP             string         `gorm:"column:ip;type:varchar(64)" json:"ip,omitempty"`
	UserAgent      string         `gorm:"column:user_agent;type:varchar(512)" json:"user_agent,omitempty"`
	TraceID        string         `gorm:"column:trace_id;type:varchar(128);index" json:"trace_id,omitempty"`
}

func (mdl *RegistrationPolicyAuditEvent) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRegistrationPolicyAudit
}

func (mdl *RegistrationPolicyAuditEvent) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRegistrationPolicyAudit
}
