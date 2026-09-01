package iam

import (
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

const (
	RegistrationInviteBatchStatusDraft   = "draft"
	RegistrationInviteBatchStatusActive  = "active"
	RegistrationInviteBatchStatusPaused  = "paused"
	RegistrationInviteBatchStatusExpired = "expired"
	RegistrationInviteBatchStatusRevoked = "revoked"

	RegistrationInviteCodeStatusActive   = "active"
	RegistrationInviteCodeStatusConsumed = "consumed"
	RegistrationInviteCodeStatusRevoked  = "revoked"
)

type RegistrationInviteBatch struct {
	model.PowerUUIDModel

	Name                string         `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;default:draft;index" json:"status"`
	MaxCodes            int            `gorm:"column:max_codes;not null" json:"max_codes"`
	MaxUsesPerCode      int            `gorm:"column:max_uses_per_code;not null;default:1" json:"max_uses_per_code"`
	AllowedPlan         string         `gorm:"column:allowed_plan;type:varchar(64)" json:"allowed_plan,omitempty"`
	AllowedEmailDomains datatypes.JSON `gorm:"column:allowed_email_domains;type:jsonb;not null;default:'[]'" json:"allowed_email_domains,omitempty"`
	AllowedChannels     datatypes.JSON `gorm:"column:allowed_channels;type:jsonb;not null;default:'[]'" json:"allowed_channels,omitempty"`
	StartsAt            *time.Time     `gorm:"column:starts_at;index" json:"starts_at,omitempty"`
	ExpiresAt           *time.Time     `gorm:"column:expires_at;index" json:"expires_at,omitempty"`
	CreatedByUserUUID   string         `gorm:"column:created_by_user_uuid;type:char(36);index" json:"created_by_user_uuid,omitempty"`
	UpdatedByUserUUID   string         `gorm:"column:updated_by_user_uuid;type:char(36);index" json:"updated_by_user_uuid,omitempty"`
}

func (mdl *RegistrationInviteBatch) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRegistrationInviteBatch
}

func (mdl *RegistrationInviteBatch) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRegistrationInviteBatch
}

type RegistrationInviteCode struct {
	model.PowerUUIDModel

	BatchUUID             string     `gorm:"column:batch_uuid;type:char(36);not null;index:idx_registration_invite_code_batch_status,priority:1" json:"batch_uuid"`
	PlainCode             string     `gorm:"column:plain_code;type:varchar(64);not null;default:''" json:"plain_code,omitempty"`
	CodeHash              string     `gorm:"column:code_hash;type:char(64);not null;uniqueIndex:uk_registration_invite_code_hash" json:"-"`
	Status                string     `gorm:"column:status;type:varchar(32);not null;default:active;index:idx_registration_invite_code_batch_status,priority:2" json:"status"`
	MaxUses               int        `gorm:"column:max_uses;not null;default:1" json:"max_uses"`
	UseCount              int        `gorm:"column:use_count;not null;default:0" json:"use_count"`
	LastUsedAt            *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	LastUsedByContactHash string     `gorm:"column:last_used_by_contact_hash;type:char(64);index" json:"-"`
	ConsumedTenantUUID    string     `gorm:"column:consumed_tenant_uuid;type:char(36);index" json:"consumed_tenant_uuid,omitempty"`
	RevokedAt             *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
}

func (mdl *RegistrationInviteCode) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMRegistrationInviteCode
}

func (mdl *RegistrationInviteCode) GetTableName(needFull bool) string {
	if needFull {
		return mdl.TableName()
	}
	return model.TableIAMRegistrationInviteCode
}
