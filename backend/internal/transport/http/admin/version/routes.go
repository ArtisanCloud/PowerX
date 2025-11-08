package version

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	compat "github.com/ArtisanCloud/PowerX/internal/service/plugin_compat"
	gov "github.com/ArtisanCloud/PowerX/internal/service/plugin_governance"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterAPIRoutes wires version governance endpoints.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	_ = public
	if protected == nil {
		return
	}
	h := &handler{gov: deps.PluginGovernance, compat: deps.PluginCompat}
	group := protected.Group("/internal/version")
	group.Use(middleware.AdminOnlyMiddleware())
	group.POST("/governance/scan", h.scan)
	group.GET("/governance/board", h.board)
	group.POST("/compat/check", h.check)
	group.POST("/compat/exception", h.createException)
	group.POST("/compat/approve", h.approveException)
}

type handler struct {
	gov    *gov.Service
	compat *compat.Service
}

func (h *handler) scan(c *gin.Context) {
	if h.gov == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "version governance disabled", nil)
		return
	}
	var req gov.ScanInput
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	report, err := h.gov.Scan(c.Request.Context(), req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, report)
}

func (h *handler) board(c *gin.Context) {
	if h.gov == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "version governance disabled", nil)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	filter := gov.BoardFilter{
		TenantID: strings.TrimSpace(c.Query("tenantId")),
		Limit:    limit,
	}
	summary, err := h.gov.Board(c.Request.Context(), filter)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, summary)
}

func (h *handler) check(c *gin.Context) {
	if h.compat == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "compatibility service disabled", nil)
		return
	}
	var req compat.CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	result, err := h.compat.Check(c.Request.Context(), req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *handler) createException(c *gin.Context) {
	if h.compat == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "compatibility service disabled", nil)
		return
	}
	var req compat.ExceptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	entity, err := h.compat.CreateException(c.Request.Context(), req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, entity)
}

func (h *handler) approveException(c *gin.Context) {
	if h.compat == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "compatibility service disabled", nil)
		return
	}
	var req compat.ApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	if req.ID == uuid.Nil {
		idParam := strings.TrimSpace(c.Query("id"))
		if idParam != "" {
			if parsed, err := uuid.Parse(idParam); err == nil {
				req.ID = parsed
			}
		}
	}
	entity, err := h.compat.Approve(c.Request.Context(), req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, entity)
}

