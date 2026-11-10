package plugin_marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
)

type InstanceStatus struct {
	State               string    `json:"state"`
	ID                  string    `json:"id,omitempty"`
	LastHeartbeat       time.Time `json:"last_heartbeat,omitempty"`
	PendingReports      int       `json:"pending_reports"`
	BlockedReports      int       `json:"blocked_reports"`
	ErrorCode           string    `json:"error_code,omitempty"`
	UnconfirmedInstalls int       `json:"unconfirmed_installs"`
}
type distribution struct {
	service      *Service
	store        *market.DistributionStore
	enrollment   string
	interval     time.Duration
	wake         chan struct{}
	mu           sync.Mutex
	connectMu    sync.Mutex
	flushMu      sync.Mutex
	resultMu     sync.Mutex
	results      map[string]*pendingResultWrite
	cancel       context.CancelFunc
	done         chan struct{}
	initError    string
	recovering   atomic.Bool
	authRejected atomic.Bool
}

// ConfigureDistribution 在 DI 装配时调用。失败只关闭市场写入口，不影响本地安装。
func (s *Service) ConfigureDistribution(store *market.DistributionStore, enrollment string, interval time.Duration, initErr error) {
	s.clearDeploymentProofs()
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	d := &distribution{service: s, store: store, enrollment: enrollment, interval: interval, wake: make(chan struct{}, 1), results: make(map[string]*pendingResultWrite)}
	s.distribution = d
	if initErr != nil || store == nil {
		d.initError = "MARKETPLACE_STATE_UNAVAILABLE"
		s.client.SetInstance("", "")
		return
	}
	state := store.Snapshot()
	for _, a := range state.Attempts {
		if a.InstanceID == state.InstanceID && a.Blocked == "" && (a.Phase == "created" || a.Phase == "requesting" || a.Phase == "download" || a.Phase == "install" || (a.Phase == "installed" && a.Enable && !a.NeedsReview)) {
			d.recovering.Store(true)
		}
	}
	if state.InstanceToken != "" && state.InstanceID != "" && !state.AuthRejected {
		s.client.SetInstance(state.InstanceToken, state.Fingerprint)
	} else {
		// 正常启动自动登记，不以旧共享下载令牌冒充独立实例。
		s.client.SetInstance("", "")
	}
}
func (d *distribution) registered() bool {
	if d.store == nil || d.authRejected.Load() {
		return false
	}
	state := d.store.Identity()
	return state.InstanceID != "" && state.InstanceToken != "" && !state.AuthRejected
}
func (d *distribution) available() bool {
	if d.initError != "" || d.store == nil || d.recovering.Load() {
		return false
	}
	state := d.store.Identity()
	return !state.AuthRejected && d.registered()
}
func (d *distribution) status() *InstanceStatus {
	out := &InstanceStatus{State: "not_registered", ErrorCode: d.initError}
	if d.store == nil {
		out.State = "unavailable"
		return out
	}
	d.resultMu.Lock()
	state := d.store.Snapshot()
	unconfirmed := make(map[string]bool)
	for id := range d.results {
		unconfirmed[id] = true
	}
	d.resultMu.Unlock()
	out.ID, out.LastHeartbeat = state.InstanceID, state.LastHeartbeat
	if d.registered() {
		out.State = "registered"
	}
	if state.RegistrationError != "" {
		out.State = "connecting"
		out.ErrorCode = state.RegistrationError
	}
	if state.AuthRejected || d.authRejected.Load() {
		out.State = "credential_rejected"
		out.ErrorCode = "MARKETPLACE_INSTANCE_REJECTED"
	}
	if d.recovering.Load() && d.registered() {
		out.ErrorCode = "MARKETPLACE_RECONCILING_INSTALLS"
	}
	for _, a := range state.Attempts {
		if a.NeedsReview {
			unconfirmed[a.ID] = true
		}
		for _, e := range a.Events {
			if !e.Sent {
				out.PendingReports++
				if a.Blocked != "" {
					out.BlockedReports++
				}
			}
		}
	}
	out.UnconfirmedInstalls = len(unconfirmed)
	if out.UnconfirmedInstalls > 0 && out.ErrorCode == "" {
		out.ErrorCode = "MARKETPLACE_INSTALL_RESULT_UNCONFIRMED"
	}
	return out
}
func (d *distribution) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// connectEnrolledInstance 仅兼容已有的管理员接入码配置，不是普通实例启动前提。
func (s *Service) connectEnrolledInstance(ctx context.Context) error {
	d := s.distribution
	if d == nil || d.store == nil {
		return nil
	}
	d.connectMu.Lock()
	defer d.connectMu.Unlock()
	state := d.store.Snapshot()
	if (state.InstanceID != "" && state.InstanceToken != "" && !state.AuthRejected) || d.enrollment == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(d.enrollment))
	enrollmentHash := hex.EncodeToString(sum[:])
	if state.EnrollmentHash == enrollmentHash {
		return marketStateError("MARKETPLACE_REGISTRATION_REQUIRED", "接入码已尝试使用；请由市场管理员核对并重新生成接入码")
	}
	if err := d.store.Update(func(st *market.DistributionState) error {
		st.EnrollmentHash = enrollmentHash
		st.RegistrationError = "MARKETPLACE_REGISTRATION_UNCONFIRMED"
		return nil
	}); err != nil {
		return err
	}
	result, err := s.client.Register(ctx, d.enrollment, state.Fingerprint, s.coreVersion)
	if err != nil {
		return err
	}
	if _, err := uuid.Parse(result.Instance.ID); err != nil || !strings.HasPrefix(result.InstanceToken, "pxm_instance_") {
		return marketStateError("MARKETPLACE_REGISTRATION_INVALID", "实例登记响应无效；请由市场管理员核对接入状态")
	}
	if err := d.store.Update(func(st *market.DistributionState) error {
		for _, a := range st.Attempts {
			if a.InstanceID != result.Instance.ID {
				a.Blocked = "MARKETPLACE_PREVIOUS_INSTANCE"
			}
		}
		st.InstanceID = result.Instance.ID
		st.InstanceToken = result.InstanceToken
		st.RegistrationError = ""
		st.AuthRejected = false
		return nil
	}); err != nil {
		return err
	}
	s.client.SetInstance(result.InstanceToken, state.Fingerprint)
	d.authRejected.Store(false)
	return nil
}
func marketStateError(code, message string) error {
	return &market.Error{Status: 503, Code: code, Message: message}
}

// BeginInstall 先记安装尝试，再允许申请下载；旧服务令牌兼容模式不伪造实例回报。
func (s *Service) BeginInstall(plugin, version, tenant string, enable bool, priorRevision string) (string, error) {
	d := s.distribution
	if d == nil {
		return "", nil
	}
	if !d.available() {
		return "", marketStateError("MARKETPLACE_INSTANCE_NOT_READY", "市场实例尚未接入或凭证已失效，请检查服务端接入状态")
	}
	if !d.registered() {
		return "", nil
	}
	id := uuid.NewString()
	err := d.store.Update(func(st *market.DistributionState) error {
		st.Attempts[id] = &market.InstallAttempt{ID: id, InstanceID: st.InstanceID, PluginID: plugin, Version: version, Tenant: tenant, Enable: enable, PriorRevision: priorRevision, CreatedAt: time.Now().UTC(), Phase: "created"}
		return nil
	})
	return id, err
}
func (d *distribution) prepareDownload(id, plugin, version, tenant string, target market.Target) error {
	return d.store.Update(func(st *market.DistributionState) error {
		a := st.Attempts[id]
		if a == nil || a.PluginID != plugin || a.Version != version || a.Tenant != tenant || a.Phase != "created" {
			return invalid("安装尝试与下载选择不一致")
		}
		a.Target = target
		a.Phase = "requesting"
		return nil
	})
}
func (d *distribution) saveTicket(id string, ticket market.Download) error {
	if _, err := uuid.Parse(ticket.DownloadID); err != nil || ticket.TrackingMode != "registered_instance" || ticket.Status != "url_issued" {
		return marketStateError("MARKETPLACE_TRACKING_REQUIRED", "市场未返回有效的实例下载记录")
	}
	return d.store.Update(func(st *market.DistributionState) error {
		a := st.Attempts[id]
		if a == nil {
			return invalid("安装尝试不存在")
		}
		a.DownloadID = ticket.DownloadID
		a.Phase = "download"
		return nil
	})
}
func appendReport(a *market.InstallAttempt, event market.TransferReport) {
	now := time.Now().UTC()
	event.EventID = uuid.NewString()
	event.ReportedAt = &now
	a.Events = append(a.Events, market.PendingReport{Payload: event})
	a.NextAttempt = time.Time{}
}
func (s *Service) RecordDownloaded(id string, verification plugin_mgr.DownloadVerification) error {
	if id == "" || s.distribution == nil {
		return nil
	}
	d := s.distribution
	err := d.store.Update(func(st *market.DistributionState) error {
		a := st.Attempts[id]
		if a == nil || a.Phase != "download" || a.DownloadID == "" {
			return invalid("下载记录未持久化")
		}
		if !strings.EqualFold(verification.SHA256, a.Target.SHA256) || verification.Size != a.Target.Size {
			return invalid("实际下载校验与下载记录不一致")
		}
		appendReport(a, market.TransferReport{Status: "downloaded", SHA256: verification.SHA256, Size: &verification.Size})
		a.Phase = "install"
		return nil
	})
	if err == nil {
		d.signal()
	}
	return err
}
func (s *Service) RecordInstalled(id string, p plugin_mgr.Plugin) error {
	if id == "" || s.distribution == nil {
		return nil
	}
	return s.distribution.recordResult(id, "installed", p, "")
}
func (s *Service) RecordEnabled(id string) error {
	if id == "" || s.distribution == nil {
		return nil
	}
	return s.distribution.recordResult(id, "enabled", plugin_mgr.Plugin{}, "")
}

// RecordPluginEnableResult 关联后续手动启用，只接纳同一市场安装修订号及业务租户。
func (s *Service) RecordPluginEnableResult(p plugin_mgr.Plugin, tenant string, success bool) error {
	d := s.distribution
	if d == nil || strings.TrimSpace(tenant) == "" || !d.registered() || p.InstallOrigin.Type != "marketplace" || p.InstallOrigin.SourceID != s.client.SourceID() || p.Revision == "" {
		return nil
	}
	state := d.store.Snapshot()
	for id := range state.Attempts {
		a := d.resultView(id)
		if a == nil || a.Phase != "installed" || a.PluginID != p.ID || a.Version != p.Version || a.Revision != p.Revision || a.InstanceID != state.InstanceID || a.Tenant != tenant {
			continue
		}
		if success {
			return s.RecordEnabled(a.ID)
		}
		return s.RecordFailed(a.ID, "POWERX_ENABLE_FAILED")
	}
	return nil
}
func (s *Service) RecordFailed(id, code string) error {
	if id == "" || s.distribution == nil {
		return nil
	}
	return s.distribution.recordResult(id, "failed", plugin_mgr.Plugin{}, code)
}

// RecordOperationFailed 仅回报受控错误类别，不发送底层错误中的签名 URL、路径或凭证。
func (s *Service) RecordOperationFailed(id string, operationErr error, fallback string) error {
	var managed *plugin_mgr.ManagerError
	if errors.As(operationErr, &managed) {
		switch managed.Code {
		case plugin_mgr.CodeChecksumMismatch, plugin_mgr.CodeInvalidManifest, plugin_mgr.CodeMigrationFailed, plugin_mgr.CodeProcessStartFailed, plugin_mgr.CodeHealthcheckFailed, plugin_mgr.CodeConflict, plugin_mgr.CodeIOError:
			return s.RecordFailed(id, "POWERX_"+strings.ToUpper(string(managed.Code)))
		}
	}
	return s.RecordFailed(id, fallback)
}
func (d *distribution) authenticationError(err error) {
	if !market.AuthenticationRejected(err) || !d.registered() {
		return
	}
	d.service.clearDeploymentProofs()
	d.authRejected.Store(true)
	if saveErr := d.store.Update(func(st *market.DistributionState) error { st.AuthRejected = true; return nil }); saveErr != nil {
		logger.WarnF(context.Background(), "Marketplace 凭证拒绝状态保存失败")
	}
	d.service.client.SetInstance("", "")
}

// RecoverDistribution 只核对中断结果并补发事件，不调用安装器或启用接口。
func (s *Service) RecoverDistribution(ctx context.Context) error {
	d := s.distribution
	if d == nil || !d.registered() {
		return nil
	}
	state := d.store.Snapshot()
	needsInventory := false
	for _, a := range state.Attempts {
		needsInventory = needsInventory || (a.InstanceID == state.InstanceID && a.Blocked == "" && a.Phase == "install")
	}
	if needsInventory && s.inventory == nil {
		return fmt.Errorf("中断安装需要可读取的宿主安装记录")
	}
	var inventory []plugin_mgr.Plugin
	if s.inventory != nil {
		var err error
		inventory, err = s.inventory(ctx)
		if err != nil {
			return fmt.Errorf("中断安装需要可读取的宿主安装记录")
		}
	}
	err := d.store.Update(func(st *market.DistributionState) error {
		for id, a := range st.Attempts {
			if a.InstanceID != st.InstanceID || a.Blocked != "" {
				continue
			}
			if a.Phase == "requesting" {
				a.Phase = "recover_download"
				continue
			}
			if a.Phase == "created" {
				a.Phase = "failed"
				continue
			}
			if a.Phase != "download" && a.Phase != "install" && !(a.Phase == "installed" && a.Enable) {
				continue
			}
			if a.Phase == "install" {
				for _, p := range inventory {
					if p.ID == a.PluginID && p.Version == a.Version && p.Revision != "" && p.Revision != a.PriorRevision && p.InstallOrigin.Type == "marketplace" && p.InstallOrigin.SourceID == state.SourceID && strings.EqualFold(p.InstallOrigin.SHA256, a.Target.SHA256) {
						a.Revision = p.Revision
						a.Phase = "installed"
						appendReport(a, market.TransferReport{Status: "installed"})
						break
					}
				}
			}
			if a.Phase == "installed" && !a.Enable {
				continue
			}
			if a.Phase == "installed" {
				// 无法仅凭运行时/注册表证明租户收尾已完成，保留已安装结果等待真实启用。
				a.NeedsReview = true
				continue
			}
			stage := "download"
			if a.Phase == "install" {
				stage = "install"
			}
			appendReport(a, market.TransferReport{Status: "failed", Stage: stage, ErrorCode: "POWERX_INSTALL_INTERRUPTED", Message: "PowerX 重启后已核对安装记录；此步骤结果未确认，请在已安装页面处理"})
			a.Phase = "failed"
			st.Attempts[id] = a
		}
		return nil
	})
	if err == nil {
		d.recovering.Store(false)
	}
	return err
}

// FlushDistribution 仅发送持久化事件；同一下载严格按顺序，每次重试保持 payload 不变。
func (s *Service) FlushDistribution(ctx context.Context) error {
	d := s.distribution
	if d == nil || d.store == nil {
		return nil
	}
	d.flushMu.Lock()
	defer d.flushMu.Unlock()
	writeErr := d.retryResultWrites(ctx)
	if !d.registered() {
		return writeErr
	}
	state := d.store.Snapshot()
	ids := make([]string, 0, len(state.Attempts))
	for id := range state.Attempts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	processed := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		a := d.store.Attempt(id)
		if a == nil {
			continue
		}
		if a.InstanceID != state.InstanceID || a.Blocked != "" || a.NextAttempt.After(time.Now()) {
			continue
		}
		pending := a.Phase == "recover_download"
		for _, event := range a.Events {
			pending = pending || !event.Sent
		}
		if !pending {
			continue
		}
		processed++
		if processed > 64 {
			d.signal()
			return writeErr
		}
		if a.Phase == "recover_download" {
			ticket, err := s.client.DownloadWithKey(ctx, a.PluginID, a.Version, runtime.GOOS, runtime.GOARCH, a.Tenant, a.ID)
			if err == nil && (ticket.Target != a.Target) {
				err = invalid("恢复下载响应与原选择不一致")
			}
			if err == nil {
				err = d.saveTicket(id, ticket)
			}
			if err == nil {
				err = s.RecordFailed(id, "POWERX_INSTALL_INTERRUPTED")
			}
			if err != nil {
				if saveErr := d.failedDelivery(id, err); saveErr != nil {
					return saveErr
				}
				continue
			}
			a = d.store.Attempt(id)
		}
		for index, event := range a.Events {
			if err := ctx.Err(); err != nil {
				return err
			}
			if event.Sent {
				continue
			}
			if err := s.client.Report(ctx, a.DownloadID, event.Payload); err != nil {
				if saveErr := d.failedDelivery(id, err); saveErr != nil {
					return saveErr
				}
				break
			}
			if err := d.store.Update(func(st *market.DistributionState) error {
				current := st.Attempts[id]
				current.Events[index].Sent = true
				current.Failures = 0
				current.NextAttempt = time.Time{}
				return nil
			}); err != nil {
				return err
			}
		}
		if !d.registered() {
			break
		}
	}
	return writeErr
}
func (d *distribution) failedDelivery(id string, err error) error {
	d.authenticationError(err)
	return d.store.Update(func(st *market.DistributionState) error {
		a := st.Attempts[id]
		a.Failures++
		if market.Retryable(err) {
			a.NextAttempt = time.Now().Add(deliveryBackoff(a.Failures))
		} else {
			a.Blocked = "MARKETPLACE_REPORT_REJECTED"
		}
		return nil
	})
}
func deliveryBackoff(failures int) time.Duration {
	delay := 5 * time.Second
	for i := 1; i < failures && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
func (s *Service) HeartbeatInstance(ctx context.Context) error {
	d := s.distribution
	if d == nil || !d.registered() {
		return nil
	}
	declaration, generation := s.deploymentDeclaration()
	if declaration == nil {
		if err := s.client.Heartbeat(ctx, s.coreVersion); err != nil {
			d.authenticationError(err)
			return err
		}
	} else {
		result, err := s.client.HeartbeatWithDeployment(ctx, s.coreVersion, declaration)
		s.acceptDeployment(result.DeploymentResponse, generation, err)
		if err != nil {
			d.authenticationError(err)
			return err
		}
	}
	return d.store.Update(func(st *market.DistributionState) error { st.LastHeartbeat = time.Now().UTC(); return nil })
}

// StartDistribution 使用到期定时器和事件唤醒，不周期扫描私有任务队列。
func (s *Service) StartDistribution(parent context.Context) {
	d := s.distribution
	if d == nil || d.store == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	d.cancel = cancel
	d.done = make(chan struct{})
	go func() {
		defer close(d.done)
		defer s.clearDeploymentProofs()
		if declaration, _ := s.deploymentDeclaration(); declaration != nil {
			_ = s.client.RefreshDeploymentCapabilities(ctx)
		}
		nextConnection := time.Now()
		connectionFailures := 0
		nextHeartbeat := time.Now()
		nextRecovery := time.Now()
		recoveryFailures := 0
		for {
			if ctx.Err() != nil {
				return
			}
			if !d.registered() && !d.store.Identity().AuthRejected && !time.Now().Before(nextConnection) {
				if err := s.ConnectInstance(ctx); err != nil {
					connectionFailures++
					logger.WarnF(ctx, "Marketplace 自动接入未完成，将使用同一实例身份重试")
					nextConnection = time.Now().Add(deliveryBackoff(connectionFailures))
				} else {
					connectionFailures = 0
					nextConnection = time.Now().Add(d.interval)
					nextHeartbeat = time.Now()
				}
			}
			if !time.Now().Before(nextHeartbeat) {
				if err := s.HeartbeatInstance(ctx); err != nil {
					logger.WarnF(ctx, "Marketplace 心跳未完成")
				}
				nextHeartbeat = time.Now().Add(d.interval)
			}
			if d.registered() && d.recovering.Load() && !time.Now().Before(nextRecovery) {
				if err := s.RecoverDistribution(ctx); err != nil {
					recoveryFailures++
					nextRecovery = time.Now().Add(deliveryBackoff(recoveryFailures))
					logger.WarnF(ctx, "Marketplace 中断安装核对失败，已保留待处理记录")
				}
			}
			flushTimeout := d.interval / 2
			if flushTimeout > 30*time.Second {
				flushTimeout = 30 * time.Second
			}
			flushCtx, flushCancel := context.WithTimeout(ctx, flushTimeout)
			if err := s.FlushDistribution(flushCtx); err != nil {
				logger.WarnF(ctx, "Marketplace 本轮结果补报未完成，待发事件已保留")
			}
			flushCancel()
			next := nextHeartbeat
			if !d.registered() && !d.store.Identity().AuthRejected && nextConnection.Before(next) {
				next = nextConnection
			}
			if d.registered() && d.recovering.Load() && nextRecovery.Before(next) {
				next = nextRecovery
			}
			if due := d.resultWriteDeadline(); !due.IsZero() && due.Before(next) {
				next = due
			}
			if d.registered() {
				for _, a := range d.store.Snapshot().Attempts {
					if a.Blocked != "" {
						continue
					}
					pending := a.Phase == "recover_download"
					for _, e := range a.Events {
						pending = pending || !e.Sent
					}
					if pending {
						due := a.NextAttempt
						if due.IsZero() || due.Before(time.Now()) {
							due = time.Now().Add(time.Second)
						}
						if due.Before(next) {
							next = due
						}
					}
				}
			}
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-d.wake:
				timer.Stop()
			case <-timer.C:
			}
		}
	}()
}
func (s *Service) CloseDistribution() {
	s.clearDeploymentProofs()
	s.deployment.mu.Lock()
	s.deployment.enabled = false
	s.deployment.mu.Unlock()
	d := s.distribution
	if d == nil {
		return
	}
	d.mu.Lock()
	cancel, done := d.cancel, d.done
	d.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	if d.store != nil {
		_ = d.store.Close()
	}
}
