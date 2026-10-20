// Package snapshot_ops 编排系统恢复点；清单独立于业务库，敏感配置仅以密文保存。
package snapshot_ops

import (
	"context"
	"io"
	"time"
)

const Contract = "powerx.system-snapshot/v1"

// Config 由宿主注入，HTTP 调用方不能指定来源路径或数据库连接。
type Config struct {
	Directory       string
	DirectorySource string
	Instance        string
	Version         string
	Platform        string
	MediaRoot       string
	DatabaseDSN     string
	EffectiveConfig []byte
	ConfigFiles     map[string]string
	Artifacts       map[string]string
	LifecycleRoots  []string
	SourceProblem   string
}

// Policy 是统一采集策略，不允许分别指定数据库和媒体周期。
type Policy struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
	RetentionDays   int  `json:"retention_days"`
	RetentionCount  int  `json:"retention_count"`
}

// NamedPolicy 是可维护的策略；只有默认策略可以启用自动调度。
type NamedPolicy struct {
	Policy
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Default   bool       `json:"is_default"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
}

// Entry 是恢复包内的普通文件；不接受绝对路径、软链接或任意执行命令。
type Entry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}

// Manifest 在完整性检查通过后封存，不包含密码、连接串或配置正文。
type Manifest struct {
	Contract         string      `json:"contract"`
	ID               string      `json:"id"`
	Instance         string      `json:"instance"`
	Version          string      `json:"version"`
	Platform         string      `json:"platform"`
	CapturedAt       time.Time   `json:"captured_at"`
	DatabaseSnapshot string      `json:"database_snapshot"`
	Files            []Entry     `json:"files"`
	Components       []Component `json:"components"`
}

type Component struct {
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	FileCount int    `json:"file_count"`
	Size      int64  `json:"size_bytes"`
}

// Snapshot 的采集状态与业务恢复验收状态分别记录。
type Snapshot struct {
	PolicyID          string      `json:"policy_id,omitempty"`
	PolicyName        string      `json:"policy_name,omitempty"`
	Attempt           int         `json:"attempt,omitempty"`
	RetryAt           *time.Time  `json:"retry_at,omitempty"`
	RecoverySupported bool        `json:"recovery_supported"`
	ID                string      `json:"id"`
	Status            string      `json:"status"`
	Phase             string      `json:"phase"`
	CreatedAt         time.Time   `json:"created_at"`
	CapturedAt        *time.Time  `json:"captured_at,omitempty"`
	UpdatedAt         time.Time   `json:"updated_at"`
	Trigger           string      `json:"trigger"`
	Instance          string      `json:"instance"`
	Version           string      `json:"version"`
	Components        []Component `json:"components"`
	Protected         bool        `json:"protected"`
	Imported          bool        `json:"imported"`
	BusinessVerified  bool        `json:"business_verified"`
	ErrorCode         string      `json:"error_code,omitempty"`
	Error             string      `json:"error,omitempty"`
	TraceID           string      `json:"trace_id,omitempty"`
	Operator          string      `json:"operator"`
	RequestHash       string      `json:"-"`
}

type Restore struct {
	ID               string      `json:"id"`
	SnapshotID       string      `json:"snapshot_id"`
	Status           string      `json:"status"`
	Phase            string      `json:"phase"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	Directory        string      `json:"directory"`
	DatabaseCopyID   string      `json:"database_copy_id,omitempty"`
	Database         string      `json:"database,omitempty"`
	Components       []Component `json:"components"`
	BusinessVerified bool        `json:"business_verified"`
	CutoverAvailable bool        `json:"cutover_available"`
	Warnings         []string    `json:"warnings"`
	Error            string      `json:"error,omitempty"`
	TraceID          string      `json:"trace_id,omitempty"`
	Operator         string      `json:"operator"`
	RequestHash      string      `json:"-"`
}

type Overview struct {
	Directory        string        `json:"directory"`
	DirectorySource  string        `json:"directory_source"`
	Ready            bool          `json:"ready"`
	Problems         []string      `json:"problems"`
	Instance         string        `json:"instance"`
	Policy           Policy        `json:"policy"`
	Policies         []NamedPolicy `json:"policies"`
	DefaultPolicyID  string        `json:"default_policy_id"`
	NextRunAt        *time.Time    `json:"next_run_at,omitempty"`
	SchedulerError   string        `json:"scheduler_error,omitempty"`
	Snapshots        []Snapshot    `json:"snapshots"`
	Restores         []Restore     `json:"restores"`
	CutoverAvailable bool          `json:"cutover_available"`
}

// Backend 将 PostgreSQL 与文件采集细节放在适配层，方便真实集成测试。
type Backend interface {
	Capture(context.Context, string) (string, error)
	RestoreDatabase(context.Context, string) (string, string, error)
}

type AuditEvent struct{ Action, ID, Operator, TraceID, Outcome string }
type Options struct {
	Backend Backend
	Sources func(context.Context) (Config, error)
	Audit   func(context.Context, AuditEvent) error
	Now     func() time.Time
	// DispatchCapture 将采集交给独立协调器；Core 停止后任务继续执行。
	DispatchCapture func(context.Context, string) error
	Coordinator     bool
	// RecoveryStore 由宿主延迟解析当前恢复账本；删除与恢复入队共用锁。
	RecoveryStore func() *CoordinatorStore
}

// SnapshotDeletion 仅在记录和包文件均移除后返回成功。
type SnapshotDeletion struct {
	ID            string `json:"id"`
	RecordDeleted bool   `json:"record_deleted"`
	FilesDeleted  bool   `json:"files_deleted"`
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.source.Read(p)
}
