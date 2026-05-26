package auth

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"net/http"

	"github.com/gin-gonic/gin"

	dto "github.com/ArtisanCloud/PowerX/pkg/dto"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
)

type MeContextHandler struct {
	S *authsvc.MeService
}

func NewMeContextHandler(dep *shared.Deps) *MeContextHandler {
	return &MeContextHandler{
		S: dep.MeService,
	}
}

// GET /api/v1/admin/user/auth/me/context
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
