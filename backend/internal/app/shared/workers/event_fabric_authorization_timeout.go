package workers

import (
	"context"
	"time"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/google/uuid"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// TenantUUIDProvider 返回需要扫描 Challenge 超时的租户列表。
type TenantUUIDProvider func(ctx context.Context) ([]uuid.UUID, error)

// EventFabricAuthorizationTimeoutWorkerOptions 控制 Challenge 超时 worker。
type EventFabricAuthorizationTimeoutWorkerOptions struct {
	Service        authsvc.Service
	TenantProvider TenantUUIDProvider
	Interval       time.Duration
	Logger         *pxlog.Logger
	Clock          func() time.Time
}

// EventFabricAuthorizationTimeoutWorker 负责周期检查 Challenge 超时并触发处理。
type EventFabricAuthorizationTimeoutWorker struct {
	service        authsvc.Service
	tenantProvider TenantUUIDProvider
	interval       time.Duration
	logger         *pxlog.Logger
	clock          func() time.Time
}

// NewEventFabricAuthorizationTimeoutWorker 构建 Challenge 超时 worker。
func NewEventFabricAuthorizationTimeoutWorker(opts EventFabricAuthorizationTimeoutWorkerOptions) *EventFabricAuthorizationTimeoutWorker {
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &EventFabricAuthorizationTimeoutWorker{
		service:        opts.Service,
		tenantProvider: opts.TenantProvider,
		interval:       interval,
		logger:         logger,
		clock:          clock,
	}
}

// Run 启动 worker。
func (w *EventFabricAuthorizationTimeoutWorker) Run(ctx context.Context) {
	if w == nil || w.service == nil || w.tenantProvider == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			w.sweep(ctx)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *EventFabricAuthorizationTimeoutWorker) sweep(ctx context.Context) {
	tenantIDs, err := w.tenantProvider(ctx)
	if err != nil {
		w.logger.WarnF(ctx, "[authorization.timeout] tenant provider failed: %v", err)
		return
	}
	if len(tenantIDs) == 0 {
		return
	}

	now := w.clock().UTC()
	for _, tenantID := range tenantIDs {
		if tenantID == uuid.Nil {
			continue
		}
		count, err := w.service.ProcessExpiredChallenges(ctx, tenantID, now)
		if err != nil && err != authsvc.ErrOperationUnsupported {
			w.logger.WarnF(ctx, "[authorization.timeout] process tenant=%s err=%v", tenantID, err)
			continue
		}
		if count > 0 {
			w.logger.InfoF(ctx, "[authorization.timeout] processed %d tickets for tenant=%s", count, tenantID)
		}
	}
}
