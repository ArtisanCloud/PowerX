package storagegovmodel

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// ControlConfiguration 是平台配置唯一权威；ScopeID 固定绑定主机、文件系统及安装。
// 本地发布快照和运行账本不接受第二条普通配置写入链路。
type ControlConfiguration struct {
	coremodel.PowerModel
	TenantID         uint64         `gorm:"not null;default:0;uniqueIndex:uk_ops_control_scope_domain"`
	ScopeID          string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_ops_control_scope_domain"`
	Domain           string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_ops_control_scope_domain"`
	HostID           string         `gorm:"type:varchar(128);not null"`
	FilesystemID     string         `gorm:"type:varchar(128);not null"`
	InstanceID       string         `gorm:"type:varchar(128);not null;index"`
	Revision         int64          `gorm:"not null"`
	AppliedRevision  int64          `gorm:"not null;default:0"`
	Document         datatypes.JSON `gorm:"type:jsonb;not null"`
	Digest           string         `gorm:"type:varchar(64);not null"`
	PublicationState string         `gorm:"type:varchar(32);not null;index"`
	Problem          string         `gorm:"type:varchar(128);not null;default:''"`
	ActorID          uint64         `gorm:"not null"`
	Status           int16          `gorm:"not null;default:1"`
}

func (*ControlConfiguration) TableName() string {
	return coremodel.PowerXSchema + "." + TableControlConfiguration
}

// ControlRevision 同时保存修改审计、请求幂等回执及发布意图；内容不可覆盖。
// TaskBus 登记与该记录及主配置 CAS 必须同事务提交。
type ControlRevision struct {
	coremodel.PowerModel
	TenantID      uint64         `gorm:"not null;default:0;uniqueIndex:uk_ops_control_request;uniqueIndex:uk_ops_control_revision"`
	ScopeID       string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_ops_control_request;uniqueIndex:uk_ops_control_revision"`
	Domain        string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_ops_control_revision"`
	Revision      int64          `gorm:"not null;uniqueIndex:uk_ops_control_revision"`
	RequestID     string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_ops_control_request"`
	RequestDigest string         `gorm:"type:varchar(64);not null"`
	Document      datatypes.JSON `gorm:"type:jsonb;not null"`
	Digest        string         `gorm:"type:varchar(64);not null"`
	ActorID       uint64         `gorm:"not null"`
	TraceID       string         `gorm:"type:varchar(128);not null;default:''"`
	TaskID        string         `gorm:"type:varchar(128);not null;uniqueIndex"`
	Status        int16          `gorm:"not null;default:1"`
}

func (*ControlRevision) TableName() string {
	return coremodel.PowerXSchema + "." + TableControlRevision
}
