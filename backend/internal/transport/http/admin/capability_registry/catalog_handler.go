package capability_registry

import (
	"errors"
	"strconv"
	"strings"

	caperrdto "github.com/ArtisanCloud/PowerX/internal/dto/capability_registry"
	capabilitycatalog "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capdto "github.com/ArtisanCloud/PowerX/internal/transport/http/capability_registrydto"
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

// ListCapabilities handles GET /admin/capabilities.
func (h *catalogHandler) ListCapabilities(c *gin.Context) {
	if h == nil || h.svc == nil {
		caperrdto.RespondError(c, caperrdto.ErrUnavailable, nil)
		return
	}

	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "50"), 50)
	if page <= 0 {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest.WithHint("page must be positive"), nil)
		return
	}
	if pageSize <= 0 {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest.WithHint("page_size must be positive"), nil)
		return
	}

	includeWorkflows := parseBool(c.Query("include_workflows"))
	statusFilter := parseCSV(c.Query("status"))

	opts := capabilitycatalog.CapabilityListOptions{
		PluginID:                 strings.TrimSpace(c.Query("plugin_id")),
		Intent:                   strings.TrimSpace(c.Query("intent")),
		Protocol:                 strings.TrimSpace(c.Query("protocol")),
		ToolScope:                strings.TrimSpace(c.Query("tool_scope")),
		Search:                   strings.TrimSpace(c.Query("search")),
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
		caperrdto.RespondError(c, caperrdto.ErrInternal, err)
		return
	}

	items := make([]capdto.CapabilityRecordDTO, 0, len(views))
	for _, view := range views {
		items = append(items, capdto.CapabilityViewToDTO(view, includeWorkflows))
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
		caperrdto.RespondError(c, caperrdto.ErrUnavailable, nil)
		return
	}
	capabilityID := strings.TrimSpace(c.Param("capabilityId"))
	if capabilityID == "" {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest.WithHint("capability_id is required"), nil)
		return
	}
	includeWorkflows := parseBool(c.Query("include_workflows"))
	view, err := h.svc.GetCapability(c.Request.Context(), capabilityID, includeWorkflows)
	if err != nil {
		template := caperrdto.ErrInternal
		if errors.Is(err, repo.ErrCapabilityRecordNotFound) {
			template = caperrdto.ErrNotFound
		}
		caperrdto.RespondError(c, template, err)
		return
	}
	dto.ResponseSuccess(c, capdto.CapabilityViewToDTO(view, includeWorkflows))
}

// ListSyncJobs handles GET /admin/capability-sync/jobs.
func (h *catalogHandler) ListSyncJobs(c *gin.Context) {
	if h == nil || h.svc == nil {
		caperrdto.RespondError(c, caperrdto.ErrUnavailable, nil)
		return
	}
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "50"), 50)
	if page <= 0 || pageSize <= 0 {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest.WithHint("page and page_size must be positive"), nil)
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
		caperrdto.RespondError(c, caperrdto.ErrInternal, err)
		return
	}

	items := make([]capdto.CapabilitySyncJobDTO, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, capdto.SyncJobToDTO(job))
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
