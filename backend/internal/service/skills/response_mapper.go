package skills

import (
	"net/http"
	"strings"
)

const (
	ErrorCodeSkillNotFound             = "skill.not_found"
	ErrorCodeVersionNotFound           = "skill.version_not_found"
	ErrorCodePermissionDenied          = "skill.permission_denied"
	ErrorCodeExecutionFailed           = "skill.execution_failed"
	ErrorCodePluginNotInstalled        = "skill.plugin_not_installed"
	ErrorCodePluginExecutorUnavailable = "skill.plugin_executor_unavailable"
	ErrorCodePluginContextMissing      = "skill.plugin_context_missing"
	ErrorCodePluginCapabilityMismatch  = "skill.plugin_capability_mismatch"
)

type ErrorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func MapInvokeError(err error) (int, ErrorEnvelope) {
	if err == nil {
		return http.StatusOK, ErrorEnvelope{}
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, ErrorCodePluginNotInstalled), strings.Contains(lower, "plugin not installed"):
		return http.StatusNotFound, ErrorEnvelope{Code: ErrorCodePluginNotInstalled, Message: msg}
	case strings.Contains(lower, ErrorCodePluginExecutorUnavailable), strings.Contains(lower, "plugin executor unavailable"):
		return http.StatusServiceUnavailable, ErrorEnvelope{Code: ErrorCodePluginExecutorUnavailable, Message: msg}
	case strings.Contains(lower, ErrorCodePluginContextMissing), strings.Contains(lower, "plugin context missing"):
		return http.StatusBadRequest, ErrorEnvelope{Code: ErrorCodePluginContextMissing, Message: msg}
	case strings.Contains(lower, ErrorCodePluginCapabilityMismatch), strings.Contains(lower, "capability mismatch"):
		return http.StatusBadRequest, ErrorEnvelope{Code: ErrorCodePluginCapabilityMismatch, Message: msg}
	case strings.Contains(lower, "not found"):
		return http.StatusNotFound, ErrorEnvelope{Code: ErrorCodeSkillNotFound, Message: msg}
	case strings.Contains(lower, "permission"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "unauthorized"):
		return http.StatusForbidden, ErrorEnvelope{Code: ErrorCodePermissionDenied, Message: msg}
	case strings.Contains(lower, "version"):
		return http.StatusNotFound, ErrorEnvelope{Code: ErrorCodeVersionNotFound, Message: msg}
	default:
		return http.StatusBadRequest, ErrorEnvelope{Code: ErrorCodeExecutionFailed, Message: msg}
	}
}
