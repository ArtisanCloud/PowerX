package auth

import (
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

// CurrentSession 仅核验当前已认证的用户，不接受身份覆盖，不修改服务端会话。
func (h *AuthUserHandler) CurrentSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var body []byte
	var readErr error
	if c.Request.Body != nil {
		body, readErr = io.ReadAll(io.LimitReader(c.Request.Body, 1))
	}
	if c.Request.URL.RawQuery != "" || readErr != nil || len(body) > 0 {
		dto.RespondErrorFrom(c, &dto.AppError{HTTPCode: 400, Code: "AUTH_SESSION_ARGUMENT_INVALID", Message: "AUTH_SESSION_ARGUMENT_INVALID"})
		return
	}
	if h == nil || h.AuthService == nil {
		dto.RespondErrorFrom(c, &dto.AppError{HTTPCode: 503, Code: "AUTH_SESSION_UNAVAILABLE", Message: "AUTH_SESSION_UNAVAILABLE"})
		return
	}
	result, err := h.AuthService.CheckUserSession(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusOK, result)
}
