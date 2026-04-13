package monitor

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) GetLogConfig(c *gin.Context) {
	cfg, err := h.svc.GetConfig()
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "query monitor log config failed", err)
		return
	}
	dto.ResponseSuccess(c, cfg)
}
