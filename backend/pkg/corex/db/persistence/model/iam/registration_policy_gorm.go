package iam

import (
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

const (
	RegistrationPolicyModeClosed             = "closed"
	RegistrationPolicyModeOpen               = "open"
	RegistrationPolicyModeInviteOnly         = "invite_only"
	RegistrationPolicyModeWaitlist           = "waitlist"
	RegistrationPolicyModeApprovalRequired   = "approval_required"
	RegistrationPolicyModeAllowlist          = "allowlist"
	RegistrationPolicyModeProgressiveRollout = "progressive_rollout"

	RegistrationPolicyStatusDraft    = "draft"
	RegistrationPolicyStatusActive   = "active"
	RegistrationPolicyStatusArchived = "archived"

	RegistrationPolicyRuleEmailDomainAllowlist = "email_domain_allowlist"
	RegistrationPolicyRuleContactAllowlist     = "contact_allowlist"
	RegistrationPolicyRuleInviteBatch          = "invite_batch"
	RegistrationPolicyRuleChannelAllowlist     = "channel_allowlist"
	RegistrationPolicyRulePercentage           = "percentage"
	RegistrationPolicyRuleTimeWindow           = "time_window"
	RegistrationPolicyRuleDailyQuota           = "daily_quota"
	RegistrationPolicyRuleTotalQuota           = "total_quota"
)

type RegistrationPolicy struct {
	model.PowerUUIDModel

	Version              int            `gorm:"column:version;not null;index" json:"version"`
	Mode                 string         `gorm:"column:mode;type:varchar(32);not null;index" json:"mode"`
	Status               string         `gorm:"column:status;type:varchar(32);not null;default:draft;index" json:"status"`
	RequiresVerification bool           `gorm:"column:requires_verification;not null;default:false" json:"requires_verification"`
	RequiresInviteCode   bool           `gorm:"column:requires_invite_code;not null;default:false" json:"requires_invite_code"`
	RequiresRootApproval bool           `gorm:"column:requires_root_approval;not null;default:false" json:"requires_root_approval"`
	DailyTenantQuota     *int           `gorm:"column:daily_tenant_quota" json:"daily_tenant_quota,omitempty"`
	TotalTenantQuota     *int           `gorm:"column:total_tenant_quota" json:"total_tenant_quota,omitempty"`
	StartAt              *time.Time     `gorm:"column:start_at;index" json:"start_at,omitempty"`
	EndAt                *time.Time     `gorm:"column:end_at;index" json:"end_at,omitempty"`
	Rules                datatypes.JSON `gorm:"column:rules;type:jsonb;not null;default:'[]'" json:"rules,omitempty"`
	ActivatedAt          *time.Time     `gorm:"column:activated_at;index" json:"activated_at,omitempty"`
	CreatedByUserUUID    string         `gorm:"column:created_by_user_uuid;type:char(36);index" json:"created_by_user_uuid,omitempty"`
	UpdatedByUserUUID    string         `gorm:"column:updated_by_user_uuid;type:char(36);index" json:"updated_by_user_uuid,omitempty"`
}

func (mdl *RegistrationPolicy) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRegistrationPolicy
}

func (mdl *RegistrationPolicy) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRegistrationPolicy
}
