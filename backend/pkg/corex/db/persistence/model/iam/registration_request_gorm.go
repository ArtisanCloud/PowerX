package iam

import (
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

const (
	RegistrationRequestModeWaitlist         = "waitlist"
	RegistrationRequestModeApprovalRequired = "approval_required"

	RegistrationRequestStatusSubmitted = "submitted"
	RegistrationRequestStatusApproved  = "approved"
	RegistrationRequestStatusRejected  = "rejected"
	RegistrationRequestStatusConverted = "converted"
	RegistrationRequestStatusCancelled = "cancelled"
)

type RegistrationRequest struct {
	model.PowerUUIDModel

	Mode                string         `gorm:"column:mode;type:varchar(32);not null;index" json:"mode"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;default:submitted;index" json:"status"`
	TenantName          string         `gorm:"column:tenant_name;type:varchar(128);not null" json:"tenant_name"`
	TenantKey           string         `gorm:"column:tenant_key;type:varchar(64);index" json:"tenant_key,omitempty"`
	OwnerEmail          string         `gorm:"column:owner_email;type:varchar(128);index" json:"owner_email,omitempty"`
	OwnerPhone          string         `gorm:"column:owner_phone;type:varchar(32);index" json:"owner_phone,omitempty"`
	OwnerDisplayName    string         `gorm:"column:owner_display_name;type:varchar(128)" json:"owner_display_name,omitempty"`
	Plan                string         `gorm:"column:plan;type:varchar(64)" json:"plan,omitempty"`
	Channel             string         `gorm:"column:channel;type:varchar(64);index" json:"channel,omitempty"`
	Campaign            string         `gorm:"column:campaign;type:varchar(128);index" json:"campaign,omitempty"`
	InviteCodeUUID      string         `gorm:"column:invite_code_uuid;type:char(36);index" json:"invite_code_uuid,omitempty"`
	PolicyUUID          string         `gorm:"column:policy_uuid;type:char(36);not null;index" json:"policy_uuid"`
	PolicyVersion       int            `gorm:"column:policy_version;not null;index" json:"policy_version"`
	ReviewedByUserUUID  string         `gorm:"column:reviewed_by_user_uuid;type:char(36);index" json:"reviewed_by_user_uuid,omitempty"`
	ReviewedAt          *time.Time     `gorm:"column:reviewed_at;index" json:"reviewed_at,omitempty"`
	RejectReasonCode    string         `gorm:"column:reject_reason_code;type:varchar(64);index" json:"reject_reason_code,omitempty"`
	CreatedTenantUUID   string         `gorm:"column:created_tenant_uuid;type:char(36);index" json:"created_tenant_uuid,omitempty"`
	ApprovalPayloadJSON datatypes.JSON `gorm:"column:approval_payload_json;type:jsonb;not null;default:'{}'" json:"-"`
}

func (mdl *RegistrationRequest) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRegistrationRequest
}

func (mdl *RegistrationRequest) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRegistrationRequest
}
