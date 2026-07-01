package plugin_dev

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	pluginbootstrap "github.com/ArtisanCloud/PowerX/internal/service/plugin_bootstrap"
	plugindiag "github.com/ArtisanCloud/PowerX/internal/service/plugin_debug/diagnostics"
	plugindebughost "github.com/ArtisanCloud/PowerX/internal/service/plugin_debug/host"
	pluginimport "github.com/ArtisanCloud/PowerX/internal/service/plugin_import"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	adminauthz "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/authz"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterAPIRoutes exposes plugin bootstrap + import endpoints under /internal/plugins/*.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	_ = public
	if protected == nil || deps == nil {
		return
	}

	handler := &handler{
		bootstrap:      deps.PluginBootstrapService,
		importSvc:      deps.PluginImportService,
		host:           deps.PluginDebugHost,
		diagnostics:    deps.PluginDiagnostics,
		capabilitySync: deps.CapabilityRegistrySyncWorker,
		deps:           deps,
	}
	if deps.PluginReleaseService != nil {
		handler.local = deps.PluginReleaseService.LocalInstall()
	}

	group := protected.Group("/internal/plugins")
	group.Use(middleware.AdminOnlyMiddleware())
	group.GET("/templates", handler.listTemplates)
	group.POST("/bootstrap/validate", handler.validateBootstrap)
	group.POST("/environments/check", handler.checkEnvironment)
	group.POST("/import", handler.submitImport)
	group.GET("/import/:id", handler.getImport)
	group.POST("/host/mock", handler.startMockHost)
	group.POST("/local/install", handler.startLocalInstall)
	group.POST("/local/reload", handler.recordReload)
	group.POST("/debug/report", handler.createDiagnosticsReport)
	group.POST("/debug/logs/export", handler.exportLogs)

	capabilityGroup := protected.Group("/internal/plugins")
	capabilityGroup.Use(adminauthz.PluginRegistrySyncMiddleware(deps, adminauthz.ScopePluginCapabilityCatalogSync))
	capabilityGroup.POST("/capabilities/catalog", handler.syncCapabilityCatalog)
}

type handler struct {
	bootstrap      *pluginbootstrap.Service
	importSvc      *pluginimport.Service
	host           *plugindebughost.Service
	local          *local.InstallService
	diagnostics    *plugindiag.Service
	capabilitySync *capabilityregistry.SyncWorker
	deps           *shared.Deps
}

func (h *handler) listTemplates(c *gin.Context) {
	if h.bootstrap == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "plugin bootstrap service disabled", nil)
		return
	}
	dto.ResponseSuccess(c, h.bootstrap.ListTemplates(c.Request.Context()))
}

func (h *handler) validateBootstrap(c *gin.Context) {
	if h.bootstrap == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "plugin bootstrap service disabled", nil)
		return
	}
	var payload pluginbootstrap.BootstrapValidateInput
	if err := c.ShouldBindJSON(&payload); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	result, err := h.bootstrap.ValidateBootstrap(c.Request.Context(), payload)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *handler) checkEnvironment(c *gin.Context) {
	if h.bootstrap == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "plugin bootstrap service disabled", nil)
		return
	}
	var payload pluginbootstrap.EnvironmentCheckInput
	if err := c.ShouldBindJSON(&payload); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	result, err := h.bootstrap.CheckEnvironment(c.Request.Context(), payload)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	status := http.StatusOK
	if !result.Passed {
		status = http.StatusAccepted
	}
	dto.ResponseSuccessWithStatus(c, status, result)
}

func (h *handler) submitImport(c *gin.Context) {
	if h.importSvc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "plugin import service disabled", nil)
		return
	}
	var payload pluginimport.ImportRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	payload.TenantUUID = tenantUUID
	result, err := h.importSvc.Submit(c.Request.Context(), payload)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, result)
}

func (h *handler) getImport(c *gin.Context) {
	if h.importSvc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "plugin import service disabled", nil)
		return
	}
	run, err := h.importSvc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
		return
	}
	reportRef := ""
	if run.ReportReference != uuid.Nil {
		reportRef = run.ReportReference.String()
	}
	record := pluginimport.ImportRecord{
		ID:         run.UUID.String(),
		Status:     run.Status,
		RiskLevel:  run.RiskLevel,
		Package:    run.PackageName,
		Vendor:     run.Vendor,
		TenantUUID: run.TenantUUID,
		Submitted:  run.CreatedAt,
		Completed:  run.CompletedAt,
		Findings:   unmarshalMap(run.Findings),
		Notes:      run.ApprovalNote,
		ReportRef:  reportRef,
	}
	dto.ResponseSuccess(c, record)
}

func (h *handler) startMockHost(c *gin.Context) {
	if h.host == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "plugin debug host service disabled", nil)
		return
	}
	var req mockHostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	pluginID := strings.TrimSpace(req.PluginID)
	if pluginID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "pluginId is required", nil)
		return
	}
	if req.HTTPPort <= 0 {
		dto.ResponseError(c, http.StatusBadRequest, "httpPort is required", nil)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	mgr := pmimpl.GetPluginManager()
	if err := pmimpl.MountDebugHost(mgr, pluginID, req.HTTPPort); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "debug host mount failed", err)
		return
	}
	if h.deps == nil || h.deps.DB == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "database dependency missing", nil)
		return
	}
	if _, _, _, err := pluginservice.NewTenantPluginInstanceService(h.deps.DB).Enable(c.Request.Context(), tenantUUID, pm.Plugin{
		ID:      pluginID,
		Version: "local",
		State:   pm.StateEnabled,
		Name:    pluginID,
	}, nil); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "debug host tenant enable failed", err)
		return
	}
	session := h.host.RegisterMockHost(c.Request.Context(), pluginID, strings.TrimSpace(req.Environment), ttl, req.HTTPPort, req.GRPCPort, req.Capabilities)
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"hostId":       session.ID.String(),
		"pluginId":     session.PluginID,
		"httpPort":     session.HTTPPort,
		"grpcPort":     session.GRPCPort,
		"expiresAt":    session.ExpiresAt.Format(time.RFC3339),
		"environment":  session.Environment,
		"ttlSeconds":   int(session.TTL.Seconds()),
		"capabilities": session.Capabilities,
	})
}

func (h *handler) startLocalInstall(c *gin.Context) {
	if h.local == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "local install disabled", nil)
		return
	}
	var req localInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	start := time.Now()
	session, err := h.local.Start(c.Request.Context(), local.StartInput{
		TenantUUID:   tenantUUID,
		DeveloperID:  req.DeveloperID,
		ArtifactURI:  req.ArtifactURI,
		FeatureFlags: req.FeatureFlags,
		ResetCache:   req.ResetCache,
		Actor:        c.GetHeader("Authorization"),
	})
	if err != nil {
		h.writeLocalError(c, err)
		return
	}
	duration := time.Since(start)
	if h.host != nil {
		flag := ""
		if len(req.FeatureFlags) > 0 {
			flag = strings.Join(req.FeatureFlags, ",")
		}
		h.host.RecordInstall(c.Request.Context(), plugindebughost.InstallEvent{
			SessionID:   session.UUID,
			TenantUUID:  tenantUUID,
			DeveloperID: req.DeveloperID,
			ArtifactURI: req.ArtifactURI,
			Duration:    duration,
			FeatureFlag: flag,
		})
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, localInstallSession{
		SessionID:  session.UUID.String(),
		TenantUUID: strings.TrimSpace(session.TenantUUID),
		Status:     session.Status,
	})
}

func (h *handler) recordReload(c *gin.Context) {
	if h.host == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "plugin debug host service disabled", nil)
		return
	}
	var req localReloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	sessionID, err := uuid.Parse(strings.TrimSpace(req.SessionID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid sessionId", err)
		return
	}
	duration := time.Duration(req.DurationMs) * time.Millisecond
	versionMismatch := req.VersionMismatch
	if !versionMismatch && strings.Contains(strings.ToLower(req.Error), "version mismatch") {
		versionMismatch = true
	}
	h.host.RecordReload(c.Request.Context(), plugindebughost.ReloadEvent{
		SessionID:       sessionID,
		Duration:        duration,
		Success:         req.Success,
		Sequence:        req.Sequence,
		Error:           req.Error,
		VersionMismatch: versionMismatch,
	})
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"result": "ok"})
}

func (h *handler) createDiagnosticsReport(c *gin.Context) {
	if h.diagnostics == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "diagnostic service disabled", nil)
		return
	}
	var req plugindiag.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	req.TenantUUID = tenantUUID
	report, err := h.diagnostics.CreateReport(c.Request.Context(), req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, report)
}

func (h *handler) exportLogs(c *gin.Context) {
	if h.diagnostics == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "diagnostic service disabled", nil)
		return
	}
	var req exportLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	reportID, err := uuid.Parse(strings.TrimSpace(req.ReportID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid reportId", err)
		return
	}
	result, err := h.diagnostics.ExportLogs(c.Request.Context(), reportID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, result)
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

type mockHostRequest struct {
	PluginID     string   `json:"pluginId"`
	Environment  string   `json:"environment"`
	TTLSeconds   int      `json:"ttlSeconds"`
	HTTPPort     int      `json:"httpPort"`
	GRPCPort     int      `json:"grpcPort"`
	Capabilities []string `json:"capabilities"`
}

type localInstallRequest struct {
	DeveloperID  uint64   `json:"developerId"`
	ArtifactURI  string   `json:"artifactUri"`
	FeatureFlags []string `json:"featureFlags"`
	ResetCache   bool     `json:"resetCache"`
}

type localReloadRequest struct {
	SessionID       string `json:"sessionId"`
	DurationMs      int64  `json:"durationMs"`
	Sequence        int64  `json:"sequence"`
	Success         bool   `json:"success"`
	Error           string `json:"error"`
	VersionMismatch bool   `json:"versionMismatch"`
}

type localInstallSession struct {
	SessionID  string `json:"sessionId"`
	TenantUUID string `json:"tenant_uuid"`
	Status     string `json:"status"`
}

type exportLogsRequest struct {
	ReportID string `json:"reportId"`
}

func (h *handler) writeLocalError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, local.ErrFeatureDisabled):
		dto.ResponseError(c, http.StatusForbidden, "local install disabled", err)
	case errors.Is(err, local.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "invalid local install request", err)
	case errors.Is(err, local.ErrPermissionDenied):
		dto.ResponseError(c, http.StatusForbidden, "insufficient permission", err)
	case errors.Is(err, local.ErrSignatureInvalid):
		dto.ResponseError(c, http.StatusUnauthorized, "artifact signature invalid", err)
	case errors.Is(err, local.ErrActiveSession):
		dto.ResponseError(c, http.StatusConflict, "session already active", err)
	case errors.Is(err, local.ErrArtifactTooLarge):
		dto.ResponseError(c, http.StatusRequestEntityTooLarge, "artifact too large", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
	}
}
