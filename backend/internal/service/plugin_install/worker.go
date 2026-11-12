package plugininstall

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/datatypes"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_install"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) Run(ctx context.Context) error {
	if !s.Ready() {
		return problem(503, "PLUGIN_INSTALL_SCHEMA_NOT_READY")
	}
	if s.opts.Disabled || !s.running.CompareAndSwap(false, true) {
		return problem(503, "PLUGIN_INSTALL_WORKER_DISABLED")
	}
	defer s.running.Store(false)
	recoverCtx, stopRecovery := context.WithCancel(ctx)
	recovered := make(chan struct{})
	go func() {
		defer close(recovered)
		for {
			for recoverCtx.Err() == nil {
				if s.opts.Queue.Recover(recoverCtx, queueTenant, s.subscriber) == nil {
					break
				}
				select {
				case <-recoverCtx.Done():
					return
				case <-time.After(time.Second):
				}
			}
			select {
			case <-recoverCtx.Done():
				return
			case <-s.wake:
			}
		}
	}()
	defer func() { stopRecovery(); <-recovered }()
	worker := "plugin-install-" + uuid.NewString()
	for ctx.Err() == nil {
		items, e := s.opts.Queue.Dequeue(ctx, queueTenant, s.subscriber, "plugin-install-workers", worker, 1, 3*time.Second)
		if e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			select {
			case s.wake <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		for _, item := range items {
			if err := s.handleDelivery(ctx, item); err != nil && ctx.Err() == nil {
				retry, cancel := context.WithTimeout(ctx, 5*time.Second)
				row, statusErr := s.opts.Queue.Status(retry, item.Message.Metadata["taskbus_record_tenant"], s.subscriber, item.Message.Metadata["taskbus_record_id"])
				if statusErr == nil && row != nil && row.Attempt >= 9 {
					record, e := s.repo.Get(retry, row.TenantKey, row.TaskID)
					if e == nil {
						_ = s.finish(retry, item, record, "needs_attention", "PLUGIN_INSTALL_RETRY_LIMIT", nil)
					}
				} else {
					_ = s.opts.Queue.Reject(retry, item, "PLUGIN_INSTALL_EXECUTION_INTERRUPTED", false)
				}
				cancel()
			}
		}
	}
	return ctx.Err()
}

func (s *Service) handleDelivery(ctx context.Context, item event_bus.StreamTaskDelivery) error {
	var message struct {
		Tenant string `json:"tenant_uuid"`
		Task   string `json:"task_id"`
	}
	if json.Unmarshal(item.Message.Payload, &message) != nil || !canonicalID(message.Tenant) || !canonicalID(message.Task) || item.Message.SubscriberID != s.subscriber || item.Message.Metadata["taskbus_record_tenant"] != message.Tenant || item.Message.Metadata["taskbus_record_id"] != message.Task {
		return s.opts.Queue.Reject(ctx, item, "PLUGIN_INSTALL_TASK_INVALID", true)
	}
	row, e := s.repo.Get(ctx, message.Tenant, message.Task)
	if e != nil {
		return e
	}
	if row.InstanceID != s.scope {
		return s.opts.Queue.Reject(ctx, item, "PLUGIN_INSTALL_SCOPE_INVALID", true)
	}
	run, cancel := context.WithTimeout(reqctx.WithTenantUUID(ctx, row.TenantUUID), s.opts.Timeout)
	run = reqctx.WithUserID(run, row.ActorID)
	run = reqctx.WithIsRoot(run, true)
	run = reqctx.WithClaims(run, &reqctx.CoreXClaims{UserID: row.ActorID, UserUUID: row.ActorUUID, TenantUUID: row.TenantUUID, IsRoot: true})
	tenantUUID, actorID := row.TenantUUID, row.ActorID
	leaseDone := make(chan struct{})
	go func() {
		defer close(leaseDone)
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-tick.C:
				if s.opts.Authorize(run, tenantUUID, actorID) != nil || s.opts.Queue.Renew(run, item) != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-leaseDone }()
	if e = privateDirectory(s.opts.Root); e != nil {
		return e
	}
	unlock, e := fileLock(run, filepath.Join(s.opts.Root, "execution.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	row, e = s.repo.Get(run, message.Tenant, message.Task)
	if e != nil {
		return e
	}
	if terminal(row.Status) {
		if e := s.opts.Queue.Complete(run, item, func(*gorm.DB) error { return nil }); e != nil {
			return e
		}
		return s.opts.Queue.Ack(run, item)
	}
	if e = s.opts.Authorize(run, row.TenantUUID, row.ActorID); e != nil {
		return s.finish(run, item, row, "failed", "PLUGIN_INSTALL_ACTOR_REVOKED", nil)
	}
	input, e := s.input(row)
	if e != nil {
		return s.finish(run, item, row, "failed", "PLUGIN_INSTALL_INTENT_INVALID", nil)
	}
	var installed pm.Plugin
	resumed := false
	if row.Status == "running" {
		current, err := s.opts.Manager.Get(run, row.PluginID)
		if err != nil || current.Version != row.Version || (current.Revision != row.UUID.String() && (row.InstalledRevision == "" || current.Revision != row.InstalledRevision)) {
			return s.finish(run, item, row, "needs_attention", "PLUGIN_INSTALL_INTERRUPTED_REVIEW_REQUIRED", nil)
		}
		installed = current
		resumed = true
	} else if row.Status != "queued" {
		return s.finish(run, item, row, "failed", "PLUGIN_INSTALL_STATE_INVALID", nil)
	}
	checkpoint := func(run context.Context, phase string, p pm.Plugin) error {
		if e := s.opts.Authorize(run, row.TenantUUID, row.ActorID); e != nil {
			return e
		}
		e := s.opts.Queue.Apply(run, item, func(tx *gorm.DB) error {
			return s.repo.Update(run, tx, row.TenantUUID, row.UUID.String(), func(current *m.Installation) error {
				if terminal(current.Status) {
					return taskqueue.ErrTaskTerminal
				}
				current.Status = "running"
				current.Phase = phase
				current.Revision++
				if p.ID != "" {
					current.PluginID = p.ID
					current.Version = p.Version
				}
				if phase == "installed" {
					current.InstalledRevision = p.Revision
				}
				*row = *current
				return nil
			})
		})
		if e == nil {
			s.changed(run, row)
		}
		return e
	}
	if !resumed {
		if e = checkpoint(run, "validating", pm.Plugin{}); e != nil {
			return e
		}
		opts := pm.InstallOptions{RequireAbsent: input.RequireAbsent, AutoEnable: input.Enable, LocalUpload: input.SourceKind == "local", InstallationRevision: row.UUID.String(), Progress: checkpoint, ExpectedPluginID: input.PluginID, ExpectedVersion: input.Version, ExpectedRevision: input.ExpectedRevision, Force: input.Force, ConfirmLegacyProvider: input.ConfirmLegacyProvider, VerifyChecksum: input.SHA256 != "", Metadata: input.Metadata, Origin: pm.InstallOrigin{Type: "local", Channel: "local"}, HostConfigSeed: &pm.HostConfig{Values: map[string]string{"PLUGIN_IAM_TENANT_KEY": row.TenantUUID, "POWERX_GRPC_UPSTREAM_TENANT_UUID": row.TenantUUID, "POWERX_INSTALL_TENANT_UUID": row.TenantUUID}}}
		if input.SourceKind == "local" {
			source := filepath.Join(s.taskDirectory(row.UUID.String()), "source")
			digest, bytes, err := copySource(run, source, "")
			if err != nil || digest != row.SourceDigest || bytes != row.SourceBytes {
				return s.finish(run, item, row, "failed", "PLUGIN_INSTALL_SOURCE_CHANGED", nil)
			}
			installed, e = s.opts.Manager.InstallFromFile(pm.WithEventFabricProvisioning(run), source, opts)
		} else {
			opts.Origin = pm.InstallOrigin{Type: "local", Channel: "url"}
			installed, e = s.opts.Manager.InstallFromURL(pm.WithEventFabricProvisioning(run), input.URL, input.SHA256, "", opts)
		}
		if e != nil {
			if run.Err() != nil {
				return run.Err()
			}
			var pending *pm.InstallCompletionError
			if errors.As(e, &pending) {
				installed = pm.Plugin{ID: pending.PluginID, Version: pending.Version, Revision: pending.Revision, State: pending.State}
				return s.finish(run, item, row, "needs_attention", "PLUGIN_INSTALL_ACTIVATION_FAILED", &installed)
			}
			details := installationFailure(e)
			code, _ := details["code"].(string)
			return s.finish(run, item, row, "failed", code, nil, details)
		}
	}
	if e = checkpoint(run, "installed", installed); e != nil {
		return e
	}
	if input.Enable {
		if e = checkpoint(run, "activating", installed); e != nil {
			return e
		}
		enableCtx := pm.WithExpectedInstallation(pm.WithEventFabricProvisioning(run), installed.Version, installed.Revision)
		if e = s.opts.Manager.Enable(enableCtx, installed.ID); e != nil && !pm.IsCode(e, pm.CodeAlreadyEnabled) {
			if run.Err() != nil {
				return run.Err()
			}
			return s.finish(run, item, row, "needs_attention", "PLUGIN_INSTALL_ACTIVATION_FAILED", &installed)
		}
		expected := installed
		installed, e = s.opts.Manager.Get(run, installed.ID)
		if e != nil {
			return e
		}
		if installed.Version != expected.Version || installed.Revision != expected.Revision {
			return s.finish(run, item, row, "needs_attention", "PLUGIN_INSTALL_TARGET_CHANGED", nil)
		}
		if e = checkpoint(run, "tenant_activation", installed); e != nil {
			return e
		}
		if e = s.opts.Finalize(run, row.TenantUUID, installed); e != nil {
			if run.Err() != nil {
				return run.Err()
			}
			return s.finish(run, item, row, "needs_attention", "PLUGIN_INSTALL_TENANT_FINALIZATION_FAILED", &installed)
		}
	}
	if e = s.finish(run, item, row, "succeeded", "", &installed); e != nil {
		return e
	}
	return nil
}

func (s *Service) finish(ctx context.Context, item event_bus.StreamTaskDelivery, row *m.Installation, status, code string, p *pm.Plugin, details ...map[string]any) error {
	e := s.opts.Queue.Complete(ctx, item, func(tx *gorm.DB) error {
		return s.repo.Update(ctx, tx, row.TenantUUID, row.UUID.String(), func(current *m.Installation) error {
			if terminal(current.Status) {
				return nil
			}
			current.Status = status
			if status == "succeeded" {
				current.Phase = "completed"
			}
			current.ErrorCode = code
			if len(details) > 0 {
				raw, _ := json.Marshal(details[0])
				current.FailureDetails = datatypes.JSON(raw)
			}
			current.Revision++
			now := time.Now().UTC()
			current.CompletedAt = &now
			if p != nil {
				current.PluginID = p.ID
				current.Version = p.Version
				current.InstalledRevision = p.Revision
			}
			*row = *current
			return nil
		})
	})
	if e != nil {
		return e
	}
	s.changed(ctx, row)
	ackErr := s.opts.Queue.Ack(ctx, item)
	_ = removeOwnedSource(s.taskDirectory(row.UUID.String()))
	return ackErr
}

// Only allowlisted structural fields are public; never persist raw manager errors,
// download URLs, HostConfig, credentials or absolute server paths in a receipt.
func installationFailure(err error) map[string]any {
	code := dto.CodeOf(err)
	if code == "" {
		code = "PLUGIN_INSTALL_FAILED"
	}
	out := map[string]any{"code": code}
	var replacement *pm.ReplacementRequiredError
	if errors.As(err, &replacement) {
		preview := func(p pm.InstallPreview) map[string]any {
			return map[string]any{"version": p.Version, "revision": p.Revision, "provider": p.Provider}
		}
		return map[string]any{"code": "PLUGIN_REPLACEMENT_REQUIRED", "plugin_id": replacement.PluginID, "current": preview(replacement.Current), "target": preview(replacement.Target), "requires_provider_confirmation": replacement.RequiresProviderConfirmation}
	}
	var manager *pm.ManagerError
	if errors.As(err, &manager) {
		if strings.HasPrefix(manager.Op, "PLUGIN_") {
			out["code"] = manager.Op
		}
		if manager.Op == "install_file.platform_preflight" {
			out["code"] = "PLUGIN_PACKAGE_PLATFORM_INCOMPATIBLE"
		}
		if pm.IsCode(err, pm.CodeConflict) && strings.Contains(strings.ToLower(err.Error()), "tenant instances") {
			out["code"] = "PLUGIN_DRAIN_REQUIRED"
			out["requires_drain"] = true
		}
		out["plugin_id"] = manager.PluginID
		out["version"] = manager.Version
	}
	return out
}
