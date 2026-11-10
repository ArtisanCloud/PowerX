package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	observer "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
	notifications "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"golang.org/x/sys/unix"
)

func configureStorageRuntime(cfg *config.Config, deps *shared.Deps, opts *gov.Options, logs *observer.ApplicationLogs) error {
	budget.SetDefault(nil)
	opts.LogDelivery = logger.LokiDeliveryStatus
	c := cfg.StorageGovernance
	opts.DatabaseRetentionAllowed = c.LifecycleEnabled && c.AllowEnforce
	if c.AllowEnforce && !c.ManageNativeRetention {
		return errors.New("storage enforce requires explicit native retention handover")
	}
	if !c.LifecycleEnabled && !c.AdmissionEnabled {
		return nil
	}
	coordinator, err := budget.New(budget.Config{Directory: bindStorageSourcePath(c.RuntimeDirectory), FilesystemPath: opts.FilesystemPath, HostID: c.HostID, FilesystemID: c.FilesystemID, InstanceID: cfg.EnvironmentID, MinFreeBytes: c.MinFreeBytes, CriticalFreeBytes: c.CriticalFreeBytes, MaxUsedBytes: c.HostBudgetBytes, MaxConcurrent: c.MaxConcurrent})
	if err != nil {
		return fmt.Errorf("storage runtime directory or filesystem identity unavailable: %w", err)
	}
	opts.Budget = coordinator
	opts.AdmissionEnabled = c.AdmissionEnabled
	if c.AdmissionEnabled {
		budget.SetDefault(coordinator)
	}
	opts.LifecycleWorkerEnabled = !config.BackgroundWorkersDisabled(cfg)
	if c.NotificationTenantUUID != "" && c.NotificationMemberUUID != "" && deps.Notifications != nil {
		opts.Notify = func(ctx context.Context, alert gov.CapacityNotification) error {
			kind, title := "warning", "主机存储需要处理"
			if alert.Recovered {
				kind, title = "success", "主机存储已恢复"
			}
			item, err := deps.Notifications.CreateStorageAlertOnce(ctx, notifications.CreateInput{TenantUUID: c.NotificationTenantUUID, MemberUUID: c.NotificationMemberUUID, Title: title, Content: "请前往监控中心的存储治理查看容量、来源和保护状态。", Type: kind, Category: "system", IsImportant: !alert.Recovered, RelatedID: alert.ID, RelatedType: "storage_capacity"})
			if err != nil {
				return err
			}
			if !item.DeletedAt.Valid {
				wsbus.DefaultHub.PublishWithContext(ctx, c.NotificationTenantUUID, eventbus.TopicSystemNotification, notifications.CreatedEvent(item), "")
			}
			return nil
		}
	}
	if !c.LifecycleEnabled {
		return nil
	}
	sources := []lifecycle.Source{}
	for _, r := range opts.Resources {
		s := lifecycle.Source{ID: r.ID, Name: r.Name, Kind: r.Kind, Scope: r.Scope, InstanceID: r.InstanceID, Directory: r.Path, RequireArchive: r.Scope == "prod" || r.Scope == "staging"}
		var root unix.Stat_t
		if unix.Stat(r.Path, &root) == nil {
			s.OwnerUID = root.Uid
		}
		if ordinary, ok := logs.Sources[r.ID]; ok {
			s.Format = "lumberjack"
			s.ActiveFiles = ordinary.ActiveFiles
		}
		if r.ID == "core-audit-files" {
			s.Kind = "audit_logs"
			s.Format = "dated"
			prefix := cfg.Audit.File.FilePrefix
			if prefix == "" {
				prefix = "audit_event"
			}
			s.ActiveFiles = []string{prefix + "-"}
		}
		for _, extra := range c.Sources {
			if extra.ID != r.ID {
				continue
			}
			s.Format = extra.RotationFormat
			s.ActiveFiles = extra.ActiveFiles
			s.CurrentLogfiles = extra.CurrentLogfiles
			if extra.FileOwnerUID != nil {
				s.OwnerUID = *extra.FileOwnerUID
			}
			s.RequireArchive = s.RequireArchive || extra.RequireVerifiedArchive
		}
		if s.Kind == "audit_logs" || s.Kind == "postgresql_logs" || s.Kind == "journal" {
			s.RequireArchive = true
		}
		sources = append(sources, s)
	}
	engine, err := lifecycle.New(lifecycle.Options{Coordinator: coordinator, InstanceID: cfg.EnvironmentID, Sources: sources, AllowEnforce: c.AllowEnforce,
		LegacySourceOwned: func(ctx context.Context, s lifecycle.Source) bool {
			if cfg.LogConfig.Retention.Enabled {
				return false
			}
			if old := monitorlogs.GetRetentionService(); old != nil && old.Config().Enabled && !old.NativeRetentionDelegated() {
				return false
			}
			return deps.Archives != nil && deps.Archives.ManagedLegacySource(s.Directory)
		},
		LegacyBlocked: func(ctx context.Context) (bool, error) {
			if old := monitorlogs.GetRetentionService(); old != nil && old.Config().Enabled {
				return true, nil
			}
			if cfg.LogConfig.Retention.Enabled {
				return true, nil
			}
			if runtime.GOOS != "linux" {
				return false, nil
			}
			run, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			for _, unit := range []string{"powerx-space-guard.timer", "powerx-space-guard.service"} {
				out, err := exec.CommandContext(run, "systemctl", "is-active", unit).Output()
				state := strings.TrimSpace(string(out))
				if state == "active" || state == "activating" || state == "reloading" {
					return true, nil
				}
				if state == "inactive" || state == "failed" || state == "unknown" {
					continue
				}
				if err != nil {
					return true, errors.New("legacy host guard ownership unknown")
				}
				return true, errors.New("legacy host guard state unsupported")
			}
			return false, nil
		},
		VerifyArchive: func(ctx context.Context, s lifecycle.Source, c lifecycle.Candidate) (bool, error) {
			if deps.Archives == nil {
				return false, nil
			}
			return deps.Archives.VerifyLifecycleFile(ctx, s.Directory, c.Name, c.Bytes, time.Unix(0, c.ModifiedNS))
		},
		ArchiveReady: func(ctx context.Context, s lifecycle.Source) (bool, error) {
			if deps.Archives == nil {
				return false, nil
			}
			return deps.Archives.LifecycleArchiveReady(ctx, s.Directory)
		},
		Publish: func() {
			if deps.StorageGovernance != nil {
				deps.StorageGovernance.PublishChange("lifecycle_changed")
			}
		},
	})
	if err != nil {
		return err
	}
	opts.Lifecycle = engine
	return nil
}
