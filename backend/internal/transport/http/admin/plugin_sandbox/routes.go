package plugin_sandbox

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	sandboxsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_sandbox"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterAPIRoutes wires sandbox orchestration endpoints.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	_ = public
	if protected == nil || deps == nil || deps.PluginSandbox == nil {
		return
	}
	handler := &handler{svc: deps.PluginSandbox}
	group := protected.Group("/internal/sandbox")
	group.Use(middleware.AdminOnlyMiddleware())
	group.POST("/deploy", handler.deploy)
	group.POST("/dataset/load", handler.loadDataset)
	group.POST("/test/run", handler.runTests)
}

type handler struct {
	svc *sandboxsvc.Service
}

func (h *handler) deploy(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "sandbox service unavailable", nil)
		return
	}
	var req sandboxsvc.DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	run, err := h.svc.Deploy(c.Request.Context(), req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, run)
}

func (h *handler) loadDataset(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "sandbox service unavailable", nil)
		return
	}
	var req sandboxsvc.DatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	if req.RunID == uuid.Nil {
		dto.ResponseError(c, http.StatusBadRequest, "runId is required", nil)
		return
	}
	if strings.TrimSpace(req.DatasetID) == "" {
		dto.ResponseError(c, http.StatusBadRequest, "datasetId is required", nil)
		return
	}
	if err := h.svc.LoadDataset(c.Request.Context(), req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"result": "dataset loaded"})
}

func (h *handler) runTests(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "sandbox service unavailable", nil)
		return
	}
	var req sandboxsvc.TestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	if req.RunID == uuid.Nil {
		dto.ResponseError(c, http.StatusBadRequest, "runId is required", nil)
		return
	}
	run, err := h.svc.RunTests(c.Request.Context(), req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, run)
}
