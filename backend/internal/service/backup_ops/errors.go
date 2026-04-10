package backup_ops

import (
	"errors"
	"net/http"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

var (
	ErrInvalidBackupPolicy        = errors.New("invalid backup policy")
	ErrInvalidBackupRequest       = errors.New("invalid backup request")
	ErrBackupPolicyNotFound       = errors.New("backup policy not found")
	ErrInvalidRestoreDrillRequest = errors.New("invalid restore drill request")
	ErrBackupJobAlreadyRunning    = errors.New("backup job already running")

	ErrInvalidStateTransition = errors.New("invalid job state transition")
	ErrInvalidJobState        = errors.New("invalid job state")
)

const (
	ErrorCodeInvalidPolicy       = "backup.invalid_policy"
	ErrorCodeInvalidRequest      = "backup.invalid_request"
	ErrorCodePolicyNotFound      = "backup.policy_not_found"
	ErrorCodeInvalidDrillRequest = "backup.invalid_restore_drill_request"
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
	case errors.Is(err, ErrInvalidRestoreDrillRequest):
		return dto.WithCode(dto.NewBadRequest("恢复演练请求参数不合法", err), ErrorCodeInvalidDrillRequest)
	case errors.Is(err, ErrBackupPolicyNotFound):
		return dto.WithCode(dto.NewNotFound("备份策略不存在", err), ErrorCodePolicyNotFound)
	case errors.Is(err, ErrBackupJobAlreadyRunning):
		return dto.WithCode(dto.NewConflict("当前策略已有运行中的备份任务", err), ErrorCodePolicyBusy)
	case errors.Is(err, ErrInvalidStateTransition), errors.Is(err, ErrInvalidJobState):
		return dto.WithCode(dto.NewBadRequest("备份任务状态流转非法", err), ErrorCodeInvalidState)
	default:
		return dto.NewError(http.StatusInternalServerError, "备份操作失败", err)
	}
}
