package iam

import (
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// AuthorizationTracking 是已声明插件业务权限的 Core 授权追踪；首次快照不冒充历史授权。
type AuthorizationTracking struct {
	model.PowerUUIDModel
	TenantUUID              string `gorm:"type:char(36);not null;uniqueIndex:uk_auth_tracking,priority:1"`
	PluginID                string `gorm:"type:varchar(128);not null;uniqueIndex:uk_auth_tracking,priority:2"`
	PermissionCode          string `gorm:"type:varchar(255);not null;uniqueIndex:uk_auth_tracking,priority:3"`
	Status                  string `gorm:"type:varchar(24);not null"`
	KnownSince              *time.Time
	InitializationGuardUUID string `gorm:"type:char(36);not null;default:''"` // Core 自动初始化收据或旧运维门禁收据。
	InitializedBy           string `gorm:"type:varchar(128);not null;default:''"`
}

func (*AuthorizationTracking) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMAuthorizationTracking
}

// AuthorizationStream 保留单调事实修订，撤权不能删除该行。
type AuthorizationStream struct {
	model.PowerUUIDModel
	TenantUUID       string    `gorm:"type:char(36);not null;uniqueIndex:uk_auth_stream,priority:1"`
	PluginID         string    `gorm:"type:varchar(128);not null;uniqueIndex:uk_auth_stream,priority:2"`
	MemberUUID       string    `gorm:"type:char(36);not null;uniqueIndex:uk_auth_stream,priority:3"`
	PermissionCode   string    `gorm:"type:varchar(255);not null;uniqueIndex:uk_auth_stream,priority:4"`
	ScopeFingerprint string    `gorm:"type:varchar(80);not null;uniqueIndex:uk_auth_stream,priority:5"`
	FactRevision     uint64    `gorm:"not null"`
	FactsHash        string    `gorm:"type:varchar(64);not null"`
	KnownSince       time.Time `gorm:"not null"`
}

func (*AuthorizationStream) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMAuthorizationStream
}

// AuthorizationSegment 将决策边界与连续授权 epoch 分开。时间使用 UTC 微秒。
type AuthorizationSegment struct {
	model.PowerUUIDModel
	TenantUUID     string    `gorm:"type:char(36);not null;index:idx_auth_segment,priority:1"`
	StreamUUID     string    `gorm:"type:char(36);not null;index:idx_auth_segment,priority:2"`
	EffectiveFrom  time.Time `gorm:"not null;index:idx_auth_segment,priority:3"`
	EffectiveUntil *time.Time
	State          string `gorm:"type:varchar(64);not null"`
	SourcesHash    string `gorm:"type:varchar(64);not null"`
	EpochUUID      string `gorm:"type:char(36);not null;default:''"`
	BaselineKind   string `gorm:"type:varchar(24);not null;default:''"`
	GrantedAt      *time.Time
	VisibleSince   *time.Time
	ValidUntil     *time.Time
}

func (*AuthorizationSegment) TableName() string {
	return model.PowerXSchema + "." + model.TableIAMAuthorizationSegment
}
