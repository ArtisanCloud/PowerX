package plugin_marketplace

import (
	"context"
	"sort"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
)

// pendingResultWrite 只保存已经执行过的操作结果。文件暂时不可写时留在内存，
// 通过原事件键重试落盘；进程中断仍由已持久化安装意图和真实注册表核对。
type pendingResultWrite struct {
	InstanceID string
	Results    []installResult
	Failures   int
	NextRetry  time.Time
	LastError  error
}
type installResult struct {
	Before   string
	After    string
	Revision string
	Report   *market.TransferReport
}

func (d *distribution) resultView(id string) *market.InstallAttempt {
	d.resultMu.Lock()
	defer d.resultMu.Unlock()
	return d.resultViewLocked(id)
}
func (d *distribution) resultViewLocked(id string) *market.InstallAttempt {
	a := d.store.Attempt(id)
	if a == nil {
		return nil
	}
	if pending := d.results[id]; pending != nil {
		for _, result := range pending.Results {
			if err := applyInstallResult(a, result); err != nil {
				return nil
			}
		}
	}
	return a
}
func applyInstallResult(a *market.InstallAttempt, result installResult) error {
	if result.Report != nil {
		for _, event := range a.Events {
			if event.Payload.EventID == result.Report.EventID {
				return nil
			}
		}
	}
	if a.Phase != result.Before {
		return invalid("安装结果与当前安装尝试的阶段不一致")
	}
	a.Phase = result.After
	if result.Revision != "" {
		a.Revision = result.Revision
	}
	a.NeedsReview = false
	if result.Report != nil {
		a.Events = append(a.Events, market.PendingReport{Payload: *result.Report})
		a.NextAttempt = time.Time{}
	}
	return nil
}

func (d *distribution) recordResult(id, status string, p plugin_mgr.Plugin, code string) error {
	d.resultMu.Lock()
	defer d.resultMu.Unlock()
	a := d.resultViewLocked(id)
	if a == nil {
		if status == "failed" {
			return nil
		}
		return invalid("安装尝试不存在或结果阶段不一致")
	}
	pending := d.results[id]
	previousError := func() error {
		if pending != nil {
			return pending.LastError
		}
		return nil
	}
	result := installResult{Before: a.Phase, After: status}
	var report market.TransferReport
	switch status {
	case "installed":
		if a.PluginID != p.ID || a.Version != p.Version || p.Revision == "" {
			return invalid("安装结果与插件身份或修订号不一致")
		}
		if a.Phase == "installed" && a.Revision == p.Revision {
			return previousError()
		}
		if a.Phase != "install" {
			return invalid("尚未完成真实下载校验")
		}
		result.Revision = p.Revision
		report.Status = "installed"
	case "enabled":
		if a.Phase == "enabled" {
			return previousError()
		}
		if a.Phase != "installed" {
			return invalid("尚未完成安装")
		}
		report.Status = "enabled"
	case "failed":
		if a.Phase == "enabled" || a.Phase == "failed" || a.Phase == "recover_download" {
			return previousError()
		}
		if a.DownloadID == "" {
			if a.Phase == "requesting" {
				result.After = "recover_download"
			}
		} else {
			report = failureReport(a.Phase, code)
		}
	default:
		return invalid("未知的安装结果")
	}
	if report.Status != "" {
		now := time.Now().UTC()
		report.EventID = uuid.NewString()
		report.ReportedAt = &now
		result.Report = &report
	}
	if pending == nil {
		pending = &pendingResultWrite{InstanceID: a.InstanceID}
		d.results[id] = pending
	}
	pending.Results = append(pending.Results, result)
	err := d.persistResultWriteLocked(id, pending)
	// 失败同样唤醒 worker，让重试到期时间独立于心跳和全局恢复标记。
	d.signal()
	return err
}
func (d *distribution) persistResultWriteLocked(id string, pending *pendingResultWrite) error {
	if pending.NextRetry.After(time.Now()) {
		return pending.LastError
	}
	err := d.store.Update(func(st *market.DistributionState) error {
		a := st.Attempts[id]
		if a == nil || a.InstanceID != pending.InstanceID {
			return invalid("安装结果的实例归属不一致")
		}
		for _, result := range pending.Results {
			if err := applyInstallResult(a, result); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		pending.Failures++
		pending.NextRetry = time.Now().Add(deliveryBackoff(pending.Failures))
		pending.LastError = err
		return err
	}
	delete(d.results, id)
	return nil
}
func (d *distribution) retryResultWrites(ctx context.Context) error {
	d.resultMu.Lock()
	defer d.resultMu.Unlock()
	ids := make([]string, 0, len(d.results))
	for id := range d.results {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var firstError error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		pending := d.results[id]
		if pending.NextRetry.After(time.Now()) {
			continue
		}
		if err := d.persistResultWriteLocked(id, pending); err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}
func (d *distribution) resultWriteDeadline() time.Time {
	d.resultMu.Lock()
	defer d.resultMu.Unlock()
	var next time.Time
	for _, pending := range d.results {
		due := pending.NextRetry
		if due.IsZero() {
			due = time.Now()
		}
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return next
}
func failureReport(phase, code string) market.TransferReport {
	stage := "download"
	if phase == "install" {
		stage = "install"
	}
	if phase == "installed" {
		stage = "enable"
	}
	message := "PowerX 在 " + stage + " 阶段未完成本次安装，请查看宿主安装记录"
	switch code {
	case "POWERX_CHECKSUM_MISMATCH":
		message = "下载文件的 SHA256 或大小校验失败，未执行安装"
	case "POWERX_INVALID_MANIFEST":
		message = "安装包的插件声明或目标身份校验失败"
	case "POWERX_MIGRATION_FAILED":
		message = "插件数据库迁移失败，请检查宿主安装日志"
	case "POWERX_PROCESS_START_FAILED", "POWERX_HEALTHCHECK_FAILED":
		message = "插件启动或健康检查失败，请检查宿主运行日志"
	case "POWERX_CONFLICT":
		message = "当前安装状态或发布者身份与本次安装选择冲突"
	case "POWERX_IO_ERROR":
		message = "安装包下载、解包或文件操作失败，请检查宿主安装日志"
	}
	return market.TransferReport{Status: "failed", Stage: stage, ErrorCode: code, Message: message}
}
