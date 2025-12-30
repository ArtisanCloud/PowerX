package capability_registry

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	modelregistry "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type workflowHandler struct {
	service *capservice.WorkflowTemplateService
	catalog *capservice.WorkflowCatalog
}

func newWorkflowHandler(deps *shared.Deps) *workflowHandler {
	if deps == nil || deps.WorkflowTemplateSvc == nil || deps.WorkflowCatalog == nil {
		return nil
	}
	return &workflowHandler{
		service: deps.WorkflowTemplateSvc,
		catalog: deps.WorkflowCatalog,
	}
}

type templateUpgradeRequest struct {
	CapabilitiesHash string `json:"capabilities_hash" binding:"required"`
	Reason           string `json:"reason"`
}

// ApproveTemplateUpgrade 处理模板升级确认。
func (h *workflowHandler) ApproveTemplateUpgrade(c *gin.Context) {
	if h == nil || h.service == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	var req templateUpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest, err)
		return
	}
	templateID := strings.TrimSpace(c.Param("templateId"))
	if templateID == "" {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("templateId is required"), nil)
		return
	}
	operator := reqctx.GetSubject(c.Request.Context())
	if operator == "" {
		if memberID := reqctx.GetMemberID(c.Request.Context()); memberID > 0 {
			operator = "member:" + strconv.FormatUint(memberID, 10)
		}
	}
	input := capservice.TemplateUpgradeInput{
		TemplateID:       templateID,
		CapabilitiesHash: strings.TrimSpace(req.CapabilitiesHash),
		Reason:           strings.TrimSpace(req.Reason),
		Operator:         strings.TrimSpace(operator),
	}
	approval, err := h.service.ApproveUpgrade(c.Request.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, capservice.ErrWorkflowTemplateHashMismatch):
			capability_registrydto.RespondError(c, capability_registrydto.ErrWorkflowTemplateHashConflict, err)
		case errors.Is(err, repo.ErrWorkflowTemplateNotFound):
			capability_registrydto.RespondError(c, capability_registrydto.ErrNotFound, err)
		default:
			capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
		}
		return
	}
	dto.ResponseSuccess(c, capability_registrydto.WorkflowTemplateApprovalToDTO(approval))
}

// ListTemplates 返回 Workflow Catalog 模板及审批状态。
func (h *workflowHandler) ListTemplates(c *gin.Context) {
	if h == nil || h.catalog == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	snapshot, err := h.catalog.Snapshot(c.Request.Context())
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
		return
	}
	approvals := map[string]*modelregistry.WorkflowTemplateApproval{}
	if h.service != nil {
		if records, err := h.service.ListApprovals(c.Request.Context()); err == nil {
			approvals = records
		}
	}
	pluginFilter := strings.TrimSpace(c.Query("plugin_id"))
	capabilityFilter := strings.TrimSpace(c.Query("capability_id"))

	items := make([]capability_registrydto.WorkflowCatalogTemplateDTO, 0, len(snapshot.Templates))
	for _, tpl := range snapshot.Templates {
		if pluginFilter != "" && !strings.EqualFold(pluginFilter, tpl.PluginID) {
			continue
		}
		if capabilityFilter != "" && !strings.EqualFold(capabilityFilter, tpl.CapabilityID) {
			continue
		}
		dto := capability_registrydto.WorkflowCatalogTemplateToDTO(tpl, approvals[tpl.TemplateID])
		items = append(items, dto)
	}
	dto.ResponseSuccess(c, gin.H{
		"version":      snapshot.Version,
		"generated_at": snapshot.GeneratedAt,
		"items":        items,
	})
}
