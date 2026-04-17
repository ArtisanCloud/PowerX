package backup_ops

import (
	"errors"
	"net/http"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

var (
	ErrInvalidBackupPolicy        = errors.New("invalid backup policy")
	ErrInvalidBackupRequest       = errors.New("invalid backup request")
	ErrInvalidBackupTarget        = errors.New("invalid backup target")
	ErrBackupTargetConnectFailed  = errors.New("backup target connect failed")
	ErrBackupPolicyNotFound       = errors.New("backup policy not found")
	ErrBackupJobNotFound          = errors.New("backup job not found")
	ErrBackupAlertNotFound        = errors.New("backup alert not found")
	ErrInvalidRestoreDrillRequest = errors.New("invalid restore drill request")
	ErrRestoreDrillNotFound       = errors.New("restore drill not found")
	ErrBackupJobAlreadyRunning    = errors.New("backup job already running")

	ErrInvalidStateTransition = errors.New("invalid job state transition")
	ErrInvalidJobState        = errors.New("invalid job state")
)

const (
	ErrorCodeInvalidPolicy       = "backup.invalid_policy"
	ErrorCodeInvalidRequest      = "backup.invalid_request"
	ErrorCodeInvalidTarget       = "backup.invalid_target"
	ErrorCodeTargetConnectFailed = "backup.target_connect_failed"
	ErrorCodePolicyNotFound      = "backup.policy_not_found"
	ErrorCodeJobNotFound         = "backup.job_not_found"
	ErrorCodeAlertNotFound       = "backup.alert_not_found"
	ErrorCodeInvalidDrillRequest = "backup.invalid_restore_drill_request"
	ErrorCodeDrillNotFound       = "backup.restore_drill_not_found"
	ErrorCodePolicyBusy          = "backup.policy_busy"
	ErrorCodeInvalidState        = "backup.invalid_state"
)

func ToAppError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidBackupPolicy):
		return dto.WithCode(dto.NewBadRequest("备份策略参数不合法", err), ErrorCodeInvalidPolicy)
	case errors.Is(err, ErrInvalidBackupRequest):
		return dto.WithCode(dto.NewBadRequest("备份请求参数不合法", err), ErrorCodeInvalidRequest)
	case errors.Is(err, ErrInvalidBackupTarget):
		return dto.WithCode(dto.NewBadRequest("目标连接参数不合法", err), ErrorCodeInvalidTarget)
	case errors.Is(err, ErrBackupTargetConnectFailed):
		return dto.WithCode(dto.NewBadRequest("目标连接失败，请检查地址和凭据", err), ErrorCodeTargetConnectFailed)
	case errors.Is(err, ErrInvalidRestoreDrillRequest):
		return dto.WithCode(dto.NewBadRequest("恢复演练请求参数不合法", err), ErrorCodeInvalidDrillRequest)
	case errors.Is(err, ErrRestoreDrillNotFound):
		return dto.WithCode(dto.NewNotFound("恢复演练记录不存在", err), ErrorCodeDrillNotFound)
	case errors.Is(err, ErrBackupPolicyNotFound):
		return dto.WithCode(dto.NewNotFound("备份策略不存在", err), ErrorCodePolicyNotFound)
	case errors.Is(err, ErrBackupJobNotFound):
		return dto.WithCode(dto.NewNotFound("备份任务不存在", err), ErrorCodeJobNotFound)
	case errors.Is(err, ErrBackupAlertNotFound):
		return dto.WithCode(dto.NewNotFound("备份告警不存在", err), ErrorCodeAlertNotFound)
	case errors.Is(err, ErrBackupJobAlreadyRunning):
		return dto.WithCode(dto.NewConflict("当前策略已有运行中的备份任务", err), ErrorCodePolicyBusy)
	case errors.Is(err, ErrInvalidStateTransition), errors.Is(err, ErrInvalidJobState):
		return dto.WithCode(dto.NewBadRequest("备份任务状态流转非法", err), ErrorCodeInvalidState)
	default:
		return dto.NewError(http.StatusInternalServerError, "备份操作失败", err)
	}
}
