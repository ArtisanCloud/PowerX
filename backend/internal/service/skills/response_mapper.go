package skills

import (
	"net/http"
	"strings"
)

const (
	ErrorCodeSkillNotFound    = "skill.not_found"
	ErrorCodeVersionNotFound  = "skill.version_not_found"
	ErrorCodePermissionDenied = "skill.permission_denied"
	ErrorCodeExecutionFailed  = "skill.execution_failed"
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
