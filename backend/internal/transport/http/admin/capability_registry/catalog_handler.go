package capability_registry

import (
	"errors"
	"strconv"
	"strings"

	capabilitycatalog "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// catalogHandler exposes read-only capability catalog endpoints for admin APIs.
type catalogHandler struct {
	svc *capabilitycatalog.RegistryService
}

func newCatalogHandler(svc *capabilitycatalog.RegistryService) *catalogHandler {
	if svc == nil {
		return nil
	}
	return &catalogHandler{svc: svc}
}

// ListSources handles GET /admin/capabilities/sources.
func (h *catalogHandler) ListSources(c *gin.Context) {
	if h == nil || h.svc == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"default": "all",
		"sources": []gin.H{
			{
				"id":          "all",
				"label":       "all",
				"description": "查询全部来源（不传 source 或 source=all）",
			},
			{
				"id":          capabilitycatalog.CapabilitySourceCoreX,
				"label":       "corex",
				"description": "PowerX 底座能力",
			},
			{
				"id":          capabilitycatalog.CapabilitySourcePlugin,
				"label":       "plugin",
				"description": "插件/租户注册能力",
			},
		},
		"aliases": gin.H{
			"all":      "all",
			"any":      "all",
			"platform": capabilitycatalog.CapabilitySourceCoreX,
		},
	})
}

// ListCapabilities handles GET /admin/capabilities.
func (h *catalogHandler) ListCapabilities(c *gin.Context) {
	if h == nil || h.svc == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}

	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "50"), 50)
	if page <= 0 {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("page must be positive"), nil)
		return
	}
	if pageSize <= 0 {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("page_size must be positive"), nil)
		return
	}

	includeWorkflows := parseBool(c.Query("include_workflows"))
	statusFilter := parseCSV(c.Query("status"))
	var sourceFilter string
	if sourceParam := strings.TrimSpace(c.Query("source")); sourceParam != "" {
		source, err := capabilitycatalog.NormalizeCapabilitySource(sourceParam)
		if err != nil {
			capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("source must be corex or plugin"), err)
			return
		}
		sourceFilter = source
	}

	opts := capabilitycatalog.CapabilityListOptions{
		PluginID:                 strings.TrimSpace(c.Query("plugin_id")),
		Intent:                   strings.TrimSpace(c.Query("intent")),
		Protocol:                 strings.TrimSpace(c.Query("protocol")),
		ToolScope:                strings.TrimSpace(c.Query("tool_scope")),
		Search:                   strings.TrimSpace(c.Query("search")),
		Source:                   sourceFilter,
		Limit:                    pageSize,
		Offset:                   (page - 1) * pageSize,
		IncludeWorkflowTemplates: includeWorkflows,
		IncludeTotal:             true,
		Status:                   statusFilter,
	}
	if tenant := strings.TrimSpace(c.Query("tenant_uuid")); tenant != "" {
		opts.TenantUUID = tenant
	}

	views, total, err := h.svc.ListCapabilities(c.Request.Context(), opts)
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
		return
	}

	items := make([]capability_registrydto.CapabilityRecordDTO, 0, len(views))
	for _, view := range views {
		items = append(items, capability_registrydto.CapabilityViewToDTO(view, includeWorkflows))
	}

	dto.ResponseList(c, items, &dto.PaginationResponse{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetCapability handles GET /admin/capabilities/{capabilityId}.
func (h *catalogHandler) GetCapability(c *gin.Context) {
	if h == nil || h.svc == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	capabilityID := strings.TrimSpace(c.Param("capabilityId"))
	if capabilityID == "" {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("capability_id is required"), nil)
		return
	}
	includeWorkflows := parseBool(c.Query("include_workflows"))
	view, err := h.svc.GetCapability(c.Request.Context(), capabilityID, includeWorkflows)
	if err != nil {
		template := capability_registrydto.ErrInternal
		if errors.Is(err, repo.ErrCapabilityRecordNotFound) {
			template = capability_registrydto.ErrNotFound
		}
		capability_registrydto.RespondError(c, template, err)
		return
	}
	dto.ResponseSuccess(c, capability_registrydto.CapabilityViewToDTO(view, includeWorkflows))
}

// ListSyncJobs handles GET /admin/capability-sync/jobs.
func (h *catalogHandler) ListSyncJobs(c *gin.Context) {
	if h == nil || h.svc == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "50"), 50)
	if page <= 0 || pageSize <= 0 {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("page and page_size must be positive"), nil)
		return
	}
	statuses := parseCSV(c.Query("status"))
	jobs, err := h.svc.ListSyncJobs(c.Request.Context(), capabilitycatalog.CapabilitySyncJobListOptions{
		PluginID:     strings.TrimSpace(c.Query("plugin_id")),
		CapabilityID: strings.TrimSpace(c.Query("capability_id")),
		Status:       statuses,
		Limit:        pageSize,
		Offset:       (page - 1) * pageSize,
	})
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
		return
	}

	items := make([]capability_registrydto.CapabilitySyncJobDTO, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, capability_registrydto.SyncJobToDTO(job))
	}

	dto.ResponseList(c, items, &dto.PaginationResponse{
		Total:    int64(len(items)),
		Page:     page,
		PageSize: pageSize,
	})
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func parseCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func parsePositiveInt(value string, fallback int) int {
	i, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || i <= 0 {
		return fallback
	}
	return i
}
