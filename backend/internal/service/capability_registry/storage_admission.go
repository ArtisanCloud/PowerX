package capability_registry

import (
	"errors"
	"regexp"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

var safeStorageCause = regexp.MustCompile(`^(STORAGE_CONTROL_|DATABASE_RETENTION_)[A-Z0-9_]{1,80}$`)

// PublicStorageAdmissionError 保留分类及恢复机器码，丢弃原始 SQL、凭据和错误详情。
func PublicStorageAdmissionError(err error) error {
	code, hint, ok := StorageAdmissionFailure(err)
	if !ok {
		return err
	}
	out := dto.NewErrorWithCode(503, code, hint, nil)
	if cause, ok := dto.DetailsOf(err)["cause_code"].(string); ok && safeStorageCause.MatchString(cause) {
		out.Details = map[string]interface{}{"cause_code": cause}
	}
	return out
}

// StorageAdmissionFailure 为 HTTP/gRPC 提供同一受控错误分类，不返回底层原文。
func StorageAdmissionFailure(err error) (code, hint string, ok bool) {
	switch dto.CodeOf(err) {
	case "STORAGE_CONTROL_SCHEMA_NOT_READY":
		return "STORAGE_CONTROL_SCHEMA_NOT_READY", "存储治理结构尚未就绪，请由 Core 管理员完成数据库迁移后重试", true
	case "STORAGE_GOVERNANCE_UNAVAILABLE":
		return "STORAGE_GOVERNANCE_UNAVAILABLE", "存储治理服务暂不可用，请稍后重试；持续失败时请联系 Core 管理员", true
	case "STORAGE_CAPACITY_EXHAUSTED":
		return "STORAGE_CAPACITY_EXHAUSTED", "存储容量保护已阻止新的调用，请处理容量后重试", true
	}
	if errors.Is(err, ErrInvocationStorageCapacity) {
		return "STORAGE_CAPACITY_EXHAUSTED", "存储容量保护已阻止新的调用，请处理容量后重试", true
	}
	return "", "", false
}
