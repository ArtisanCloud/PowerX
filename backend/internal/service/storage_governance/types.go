// Package storagegov 提供存储观测、规划草稿和应用日志的 Root 手动清理；不启用统一自动治理。
package storagegov

import (
	"context"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/writer"
	"gorm.io/gorm"
	"time"
)

// Identity 复用部署环境与安装身份；空主机身份不能冒充宿主机视图。
type Identity struct {
	HostID       string `json:"host_id,omitempty"`
	InstanceID   string `json:"instance_id,omitempty"`
	Environment  string `json:"environment,omitempty"`
	FilesystemID string `json:"filesystem_id,omitempty"` // 部署登记的稳定 ID，区别于本次内核 fsid。
}

// Resource 是部署层登记的来源，路径不向 API 调用方开放。
type Resource struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Scope          string   `json:"scope"`
	InstanceID     string   `json:"instance_id,omitempty"`
	Path           string   `json:"-"`
	TenantUUID     string   `json:"tenant_uuid,omitempty"` // 仅显式登记；空值表示未细分，不代表所有租户。
	LogActiveFiles []string `json:"-"`                     // 自动日志来源只计量这些活动文件及其正式轮转文件。
}

// Filesystem 区分普通可用与文件系统预留；未知值用 null，不用零代替。
type Filesystem struct {
	ID             string `json:"id,omitempty"`
	View           string `json:"view"`
	State          string `json:"state"`
	TotalBytes     *int64 `json:"total_bytes"`
	UsedBytes      *int64 `json:"used_bytes"`
	AvailableBytes *int64 `json:"available_bytes"`
	ReservedBytes  *int64 `json:"reserved_bytes"`
	Problem        string `json:"problem,omitempty"`
}

// Usage 是一次有限扫描的结果；不完整扫描仅报告 partial_bytes。
type Usage struct {
	Resource
	FilesystemID    string `json:"filesystem_id,omitempty"`
	State           string `json:"state"`
	PhysicalBytes   *int64 `json:"physical_bytes"`
	LogicalBytes    *int64 `json:"logical_bytes"`
	AttributedBytes *int64 `json:"attributed_bytes"`
	PartialBytes    int64  `json:"partial_bytes,omitempty"`
	Files           int64  `json:"files"`
	Skipped         int64  `json:"skipped"`
	Problem         string `json:"problem,omitempty"`
}

// Snapshot 不包含文件名、凭据或推断的历史覆盖。
type Snapshot struct {
	Filesystem Filesystem `json:"filesystem"`
	Resources  []Usage    `json:"resources"`
}

// Collector 隔离文件系统实现，允许真实系统和测试来源使用同一业务服务。
type Collector interface {
	Collect(context.Context, string, []Resource) Snapshot
}

// Options 由 shared/bootstrap 构造；没有运行时调用者可控的路径。
type Options struct {
	Identity                 Identity
	FilesystemPath           string
	Resources                []Resource
	Collector                Collector
	CacheTTL                 time.Duration
	Now                      func() time.Time
	DB                       *gorm.DB
	DatabaseCollector        DatabaseCollector
	SamplingConfigured       bool
	SamplingWorkerEnabled    bool
	LogLifecycle             LogLifecycle
	Inventory                []CoverageItem
	Budget                   *budget.Coordinator
	Lifecycle                *lifecycle.Engine
	LifecycleWorkerEnabled   bool
	DatabaseRetentionAllowed bool
	AdmissionEnabled         bool
	LogDelivery              func() *writer.LokiDeliveryStats
	Notify                   func(context.Context, CapacityNotification) error
}

// Overview 明确公布当前只读阶段和未完成的观测范围。
type Overview struct {
	Mode              string          `json:"mode"`
	CleanupEnabled    bool            `json:"cleanup_enabled"`
	Identity          Identity        `json:"identity"`
	ObservedAt        time.Time       `json:"observed_at"`
	ExpiresAt         time.Time       `json:"expires_at"`
	Filesystem        Filesystem      `json:"filesystem"`
	Resources         []Usage         `json:"resources"`
	Coverage          string          `json:"coverage"`
	UnattributedBytes *int64          `json:"unattributed_bytes"`
	Limitations       []string        `json:"limitations"`
	Database          *DatabaseUsage  `json:"database,omitempty"`
	Alerts            []CapacityAlert `json:"alerts"`
	Sampling          SamplingStatus  `json:"sampling"`
	Inventory         []CoverageItem  `json:"inventory"`
	Runtime           *RuntimeStatus  `json:"runtime,omitempty"`
}

// CoverageItem 显式列出已接入、尚未接入和当前平台不适用的资源类型。
type CoverageItem struct {
	Kind  string `json:"kind"`
	State string `json:"state"`
}

// EstimateInput 的增量是显式人工假设，不伪装成服务器历史采样。
type EstimateInput struct {
	ResourceID          string `json:"resource_id,omitempty"`
	RetentionMode       string `json:"retention_mode,omitempty"`
	RetentionCount      int    `json:"retention_count,omitempty"`
	TargetDays          int    `json:"target_days"`
	MaxBytes            int64  `json:"max_bytes"`
	DailyIncrementBytes *int64 `json:"daily_increment_bytes"`
	ProtectedBytes      int64  `json:"protected_bytes"`
}

type Estimate struct {
	State          string       `json:"state"`
	Basis          string       `json:"basis"`
	RequiredBytes  *int64       `json:"required_bytes"`
	DeficitBytes   *int64       `json:"deficit_bytes"`
	SupportedDays  *float64     `json:"supported_days"`
	SafetyFactor   float64      `json:"safety_factor"`
	CanEnable      bool         `json:"can_enable"`
	Resource       *ResourceRef `json:"resource,omitempty"`
	RetentionMode  string       `json:"retention_mode,omitempty"`
	RetentionCount int          `json:"retention_count,omitempty"`
}
