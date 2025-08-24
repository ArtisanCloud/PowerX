package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
)

type MeContextHandler struct {
	S *authsvc.MeService
}

func NewMeContextHandler(dep *bootstrap.Deps) *MeContextHandler {
	return &MeContextHandler{
		S: dep.MeService,
	}
}

// GET /api/v1/auth/me/context
func (h *MeContextHandler) GetMeContext(c *gin.Context) {
	ctx := c.Request.Context()

	resp, err := h.S.GetMeContext(ctx)
	if err != nil {
		// 统一错误返回
		dto.ResponseError(c, http.StatusInternalServerError, "获取上下文失败", err)
		return
	}
	dto.ResponseSuccess(c, resp)
}
