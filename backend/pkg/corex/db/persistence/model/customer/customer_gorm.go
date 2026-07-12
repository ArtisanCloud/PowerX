package customer

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

const (
	StatusActive    = "active"
	StatusPending   = "pending"
	StatusSuspended = "suspended"
	StatusDisabled  = "disabled"
	StatusExpired   = "expired"
	StatusDeleted   = "deleted"
)

type Account struct {
	coremodel.PowerUUIDModel

	Status       string         `gorm:"column:status;type:varchar(32);not null;default:'active';index" json:"status"`
	PrimaryEmail string         `gorm:"column:primary_email;type:varchar(255);index" json:"primary_email,omitempty"`
	PrimaryPhone string         `gorm:"column:primary_phone;type:varchar(32);index" json:"primary_phone,omitempty"`
	DisplayName  string         `gorm:"column:display_name;type:varchar(128)" json:"display_name,omitempty"`
	Nickname     string         `gorm:"column:nickname;type:varchar(128)" json:"nickname,omitempty"`
	GivenName    string         `gorm:"column:given_name;type:varchar(128)" json:"given_name,omitempty"`
	FamilyName   string         `gorm:"column:family_name;type:varchar(128)" json:"family_name,omitempty"`
	AvatarURL    string         `gorm:"column:avatar_url;type:text" json:"avatar_url,omitempty"`
	Locale       string         `gorm:"column:locale;type:varchar(32)" json:"locale,omitempty"`
	Timezone     string         `gorm:"column:timezone;type:varchar(64)" json:"timezone,omitempty"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'::jsonb" json:"metadata,omitempty"`
}

func (Account) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerAccounts
}

type AuthIdentity struct {
	coremodel.PowerUUIDModel

	CustomerUUID    string         `gorm:"column:customer_uuid;type:uuid;not null;index" json:"customer_uuid"`
	Provider        string         `gorm:"column:provider;type:varchar(32);not null;uniqueIndex:uk_customer_identity_subject,priority:1;index:idx_customer_identity_provider" json:"provider"`
	ProviderSubject string         `gorm:"column:provider_subject;type:varchar(255);uniqueIndex:uk_customer_identity_subject,priority:2" json:"provider_subject,omitempty"`
	Email           string         `gorm:"column:email;type:varchar(255);index" json:"email,omitempty"`
	Phone           string         `gorm:"column:phone;type:varchar(32);index" json:"phone,omitempty"`
	PasswordHash    string         `gorm:"column:password_hash;type:text" json:"-"`
	Status          string         `gorm:"column:status;type:varchar(32);not null;default:'active';index" json:"status"`
	VerifiedAt      *time.Time     `gorm:"column:verified_at" json:"verified_at,omitempty"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'::jsonb" json:"metadata,omitempty"`
}

func (AuthIdentity) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerAuthIdentities
}

type TenantMembership struct {
	coremodel.PowerUUIDModel

	TenantUUID   string         `gorm:"column:tenant_uuid;type:uuid;not null;uniqueIndex:uk_customer_tenant_membership,priority:1;index:idx_customer_membership_tenant_status,priority:1" json:"tenant_uuid"`
	CustomerUUID string         `gorm:"column:customer_uuid;type:uuid;not null;uniqueIndex:uk_customer_tenant_membership,priority:2;index" json:"customer_uuid"`
	Status       string         `gorm:"column:status;type:varchar(32);not null;default:'active';index:idx_customer_membership_tenant_status,priority:2" json:"status"`
	Roles        datatypes.JSON `gorm:"column:roles;type:jsonb;default:'[]'::jsonb" json:"roles,omitempty"`
	Scopes       datatypes.JSON `gorm:"column:scopes;type:jsonb;default:'[]'::jsonb" json:"scopes,omitempty"`
	Source       string         `gorm:"column:source;type:varchar(32);not null;default:'platform';index" json:"source"`
	ExpiresAt    *time.Time     `gorm:"column:expires_at;index" json:"expires_at,omitempty"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'::jsonb" json:"metadata,omitempty"`
}

func (TenantMembership) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerTenantMemberships
}

type MiniAppEntry struct {
	coremodel.PowerUUIDModel

	TenantUUID string         `gorm:"column:tenant_uuid;type:uuid;not null;index:idx_mini_app_entry_tenant_status,priority:1" json:"tenant_uuid"`
	EntryCode  string         `gorm:"column:entry_code;type:varchar(128);not null;uniqueIndex" json:"entry_code"`
	EntryType  string         `gorm:"column:entry_type;type:varchar(32);not null;index" json:"entry_type"`
	AppKey     string         `gorm:"column:app_key;type:varchar(128);index" json:"app_key,omitempty"`
	AppID      string         `gorm:"column:appid;type:varchar(128);index" json:"appid,omitempty"`
	Channel    string         `gorm:"column:channel;type:varchar(64);index" json:"channel,omitempty"`
	Campaign   string         `gorm:"column:campaign;type:varchar(128);index" json:"campaign,omitempty"`
	BrandName  string         `gorm:"column:brand_name;type:varchar(128)" json:"brand_name,omitempty"`
	OrgName    string         `gorm:"column:org_name;type:varchar(128)" json:"org_name,omitempty"`
	Theme      datatypes.JSON `gorm:"column:theme;type:jsonb;default:'{}'::jsonb" json:"theme,omitempty"`
	Features   datatypes.JSON `gorm:"column:features;type:jsonb;default:'{}'::jsonb" json:"features,omitempty"`
	Status     string         `gorm:"column:status;type:varchar(32);not null;default:'active';index:idx_mini_app_entry_tenant_status,priority:2" json:"status"`
	ExpiresAt  *time.Time     `gorm:"column:expires_at;index" json:"expires_at,omitempty"`
	Metadata   datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'::jsonb" json:"metadata,omitempty"`
}

func (MiniAppEntry) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableMiniAppEntries
}

type Session struct {
	coremodel.PowerUUIDModel

	CustomerUUID     string         `gorm:"column:customer_uuid;type:uuid;not null;index" json:"customer_uuid"`
	TenantUUID       string         `gorm:"column:tenant_uuid;type:uuid;index" json:"tenant_uuid,omitempty"`
	MembershipUUID   string         `gorm:"column:membership_uuid;type:uuid;index" json:"membership_uuid,omitempty"`
	RefreshTokenHash string         `gorm:"column:refresh_token_hash;type:text;index" json:"-"`
	Source           string         `gorm:"column:source;type:varchar(32);not null;default:'platform';index" json:"source"`
	IssuedAt         time.Time      `gorm:"column:issued_at;not null;index" json:"issued_at"`
	ExpiresAt        time.Time      `gorm:"column:expires_at;not null;index" json:"expires_at"`
	RevokedAt        *time.Time     `gorm:"column:revoked_at;index" json:"revoked_at,omitempty"`
	Metadata         datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'::jsonb" json:"metadata,omitempty"`
}

func (Session) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerSessions
}

type LoginEvent struct {
	coremodel.PowerModel

	TenantUUID       string         `gorm:"column:tenant_uuid;type:uuid;index" json:"tenant_uuid,omitempty"`
	CustomerUUID     string         `gorm:"column:customer_uuid;type:uuid;index" json:"customer_uuid,omitempty"`
	IdentityProvider string         `gorm:"column:identity_provider;type:varchar(32);index" json:"identity_provider,omitempty"`
	EventType        string         `gorm:"column:event_type;type:varchar(32);not null;index" json:"event_type"`
	OK               bool           `gorm:"column:ok;not null;default:false;index" json:"ok"`
	ErrorCode        string         `gorm:"column:error_code;type:varchar(64);index" json:"error_code,omitempty"`
	IP               string         `gorm:"column:ip;type:varchar(64)" json:"ip,omitempty"`
	UserAgent        string         `gorm:"column:user_agent;type:text" json:"user_agent,omitempty"`
	TraceID          string         `gorm:"column:trace_id;type:varchar(128);index" json:"trace_id,omitempty"`
	Metadata         datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'::jsonb" json:"metadata,omitempty"`
}

func (LoginEvent) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCustomerLoginEvents
}
