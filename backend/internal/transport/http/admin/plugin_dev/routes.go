package plugin_dev

import (
	"encoding/json"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pluginbootstrap "github.com/ArtisanCloud/PowerX/internal/service/plugin_bootstrap"
	pluginimport "github.com/ArtisanCloud/PowerX/internal/service/plugin_import"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterAPIRoutes exposes plugin bootstrap + import endpoints under /internal/plugins/*.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	_ = public
	if protected == nil {
		return
	}
	if deps.PluginBootstrapService == nil && deps.PluginImportService == nil {
		return
	}

	handler := &handler{
		bootstrap: deps.PluginBootstrapService,
		importSvc: deps.PluginImportService,
	}

	group := protected.Group("/internal/plugins")
	group.Use(middleware.AdminOnlyMiddleware())
	group.GET("/templates", handler.listTemplates)
	group.POST("/bootstrap/validate", handler.validateBootstrap)
	group.POST("/environments/check", handler.checkEnvironment)
	group.POST("/import", handler.submitImport)
	group.GET("/import/:id", handler.getImport)
}

type handler struct {
	bootstrap *pluginbootstrap.Service
	importSvc *pluginimport.Service
}

func (h *handler) listTemplates(c *gin.Context) {
	if h.bootstrap == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin bootstrap service disabled"})
		return
	}
	templates := h.bootstrap.ListTemplates(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"data": templates})
}

func (h *handler) validateBootstrap(c *gin.Context) {
	if h.bootstrap == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin bootstrap service disabled"})
		return
	}
	var payload pluginbootstrap.BootstrapValidateInput
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.bootstrap.ValidateBootstrap(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *handler) checkEnvironment(c *gin.Context) {
	if h.bootstrap == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin bootstrap service disabled"})
		return
	}
	var payload pluginbootstrap.EnvironmentCheckInput
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.bootstrap.CheckEnvironment(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if !result.Passed {
		status = http.StatusAccepted
	}
	c.JSON(status, gin.H{"data": result})
}

func (h *handler) submitImport(c *gin.Context) {
	if h.importSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin import service disabled"})
		return
	}
	var payload pluginimport.ImportRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.importSvc.Submit(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *handler) getImport(c *gin.Context) {
	if h.importSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin import service disabled"})
		return
	}
	run, err := h.importSvc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	reportRef := ""
	if run.ReportReference != uuid.Nil {
		reportRef = run.ReportReference.String()
	}
	record := pluginimport.ImportRecord{
		ID:        run.UUID.String(),
		Status:    run.Status,
		RiskLevel: run.RiskLevel,
		Package:   run.PackageName,
		Vendor:    run.Vendor,
		TenantID:  run.TenantID,
		Submitted: run.CreatedAt,
		Completed: run.CompletedAt,
		Findings:  unmarshalMap(run.Findings),
		Notes:     run.ApprovalNote,
		ReportRef: reportRef,
	}
	c.JSON(http.StatusOK, gin.H{"data": record})
}

func unmarshalMap(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{}
	}
	return m
}
