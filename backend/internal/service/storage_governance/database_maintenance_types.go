package storagegov

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
)

// DatabaseMaintenanceBackend 隔离 PostgreSQL 自动提交操作，测试不使用日常数据库。
type DatabaseMaintenanceBackend interface {
	Inspect(context.Context) (repo.MaintenanceSnapshot, error)
	CheckIncremental(context.Context, repo.MaintenanceSnapshot, string) error
	Execute(context.Context, string, string, repo.MaintenanceSnapshot, []string, int64, func(string, repo.MaintenanceSession) error, func(string) error, func(context.Context) error, func(repo.MaintenanceIncrementalProgress) error) error
	SessionActive(context.Context, repo.MaintenanceSession, bool) (bool, error)
	RootActive(context.Context, uint64) error
}
type maintenanceTaskBus interface {
	Register(context.Context, *gorm.DB, event_bus.TaskMessage) error
	Enqueue(context.Context, event_bus.TaskMessage) error
	Recover(context.Context, string, string) error
	Dequeue(context.Context, string, string, string, string, int64, time.Duration) ([]event_bus.StreamTaskDelivery, error)
	Apply(context.Context, event_bus.StreamTaskDelivery, func(*gorm.DB) error) error
	Complete(context.Context, event_bus.StreamTaskDelivery, func(*gorm.DB) error) error
	Renew(context.Context, event_bus.StreamTaskDelivery) error
	Ack(context.Context, event_bus.StreamTaskDelivery) error
	Reject(context.Context, event_bus.StreamTaskDelivery, string, bool) error
}
type databaseMaintenance struct {
	backend DatabaseMaintenanceBackend
	queue   maintenanceTaskBus
	once    sync.Once
	started atomic.Bool
	done    chan struct{}
	wake    chan struct{}
}
type DatabaseMaintenanceStep struct {
	TableID     string                               `json:"table_id"`
	State       string                               `json:"state"`
	StartedAt   time.Time                            `json:"started_at"`
	FinishedAt  *time.Time                           `json:"finished_at,omitempty"`
	Incremental *repo.MaintenanceIncrementalProgress `json:"incremental,omitempty"`
}
type DatabaseMaintenanceRun struct {
	ID                        string                    `json:"id"`
	Version                   int64                     `json:"version"`
	ScopeID                   string                    `json:"scope_id"`
	State                     string                    `json:"state"`
	Mode                      string                    `json:"mode"`
	Actor                     uint64                    `json:"actor"`
	At                        time.Time                 `json:"at"`
	UpdatedAt                 time.Time                 `json:"updated_at"`
	ExpiresAt                 *time.Time                `json:"expires_at,omitempty"`
	PreviewID                 string                    `json:"preview_id,omitempty"`
	Tables                    []string                  `json:"tables"`
	Before                    repo.MaintenanceSnapshot  `json:"before"`
	After                     *repo.MaintenanceSnapshot `json:"after,omitempty"`
	PeakBytes                 int64                     `json:"peak_bytes"`
	ReservationID             string                    `json:"reservation_id,omitempty"`
	Session                   *repo.MaintenanceSession  `json:"session,omitempty"`
	Steps                     []DatabaseMaintenanceStep `json:"steps"`
	Problem                   string                    `json:"problem,omitempty"`
	WALStop                   *repo.IncrementalWALStop  `json:"wal_stop,omitempty"`
	CancelledBy               uint64                    `json:"cancelled_by,omitempty"`
	ReviewedBy                uint64                    `json:"reviewed_by,omitempty"`
	FilesystemAvailableBefore *int64                    `json:"filesystem_available_before,omitempty"`
	FilesystemAvailableAfter  *int64                    `json:"filesystem_available_after,omitempty"`
}
type DatabaseMaintenanceStatus struct {
	ScopeID       string                    `json:"scope_id"`
	Ready         bool                      `json:"ready"`
	WorkerEnabled bool                      `json:"worker_enabled"`
	WorkerRunning bool                      `json:"worker_running"`
	Problem       string                    `json:"problem,omitempty"`
	Snapshot      *repo.MaintenanceSnapshot `json:"snapshot,omitempty"`
	Active        *DatabaseMaintenanceRun   `json:"active,omitempty"`
	Runs          []DatabaseMaintenanceRun  `json:"runs"`
}
type DatabaseMaintenancePreview struct {
	Mode   string   `json:"mode"`
	Tables []string `json:"tables"`
}
type DatabaseMaintenanceRequest struct {
	RequestID              string `json:"request_id"`
	PreviewID              string `json:"preview_id"`
	Confirm                bool   `json:"confirm"`
	ExclusiveLockConfirmed bool   `json:"exclusive_lock_confirmed"`
}
