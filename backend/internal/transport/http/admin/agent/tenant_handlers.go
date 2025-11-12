package agent

import (
	"errors"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// TenantAgentFormHandler 处理租户自助表单。
type TenantAgentFormHandler struct {
	service *agent_lifecycle.Service
}

// NewTenantAgentFormHandler 构造 Handler。
func NewTenantAgentFormHandler(deps *shared.Deps) *TenantAgentFormHandler {
	if deps == nil || deps.AgentLifecycle == nil {
		return &TenantAgentFormHandler{}
	}
	return &TenantAgentFormHandler{service: deps.AgentLifecycle.Service}
}

// SubmitTenantForm 处理租户表单提交。
func (h *TenantAgentFormHandler) SubmitTenantForm(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	var req tenantFormRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	form, err := h.service.SubmitTenantForm(c.Request.Context(), toTenantFormInput(req))
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, fromTenantFormView(form))
}

// ListTenantForms 返回租户表单列表。
func (h *TenantAgentFormHandler) ListTenantForms(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		dto.ResponseValidationError(c, errors.New("tenant_id is required"))
		return
	}
	status := c.Query("status")
	var statuses []string
	if status != "" {
		statuses = []string{status}
	}
	forms, err := h.service.ListTenantForms(c.Request.Context(), tenantID, statuses)
	if err != nil {
		h.handleError(c, err)
		return
	}
	resp := make([]tenantFormResponse, 0, len(forms))
	for i := range forms {
		resp = append(resp, fromTenantFormView(&forms[i]))
	}
	dto.ResponseSuccess(c, resp)
}

// GetTenantForm 返回单个表单。
func (h *TenantAgentFormHandler) GetTenantForm(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	formID, err := parseTenantFormID(c.Param("form_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid form_id", err)
		return
	}
	form, err := h.service.GetTenantForm(c.Request.Context(), formID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromTenantFormView(form))
}

// ApproveTenantForm 审批通过。
func (h *TenantAgentFormHandler) ApproveTenantForm(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	formID, err := parseTenantFormID(c.Param("form_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid form_id", err)
		return
	}
	var req tenantFormActionRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	form, err := h.service.ApproveTenantForm(c.Request.Context(), formID, req.Operator)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromTenantFormView(form))
}

// RejectTenantForm 审批拒绝。
func (h *TenantAgentFormHandler) RejectTenantForm(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	formID, err := parseTenantFormID(c.Param("form_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid form_id", err)
		return
	}
	var req tenantFormActionRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	form, err := h.service.RejectTenantForm(c.Request.Context(), formID, req.Operator, req.Reason)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromTenantFormView(form))
}

func (h *TenantAgentFormHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent_lifecycle.ErrPolicyConflict):
		var conflictErr *agent_lifecycle.PolicyConflictError
		if errors.As(err, &conflictErr) {
			dto.ResponseError(c, http.StatusBadRequest, "policy conflict", conflictErr)
			return
		}
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, agent_lifecycle.ErrAliasConflict):
		dto.ResponseError(c, http.StatusConflict, err.Error(), err)
	case errors.Is(err, agent_lifecycle.ErrTenantFormNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, agent_lifecycle.ErrTenantFormInvalidStatus):
		dto.ResponseError(c, http.StatusConflict, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "internal error", err)
	}
}
