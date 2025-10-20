package eventfabric

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	authorizationService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthorizationHandlerOptions struct {
	Service   authorizationService.Service
	Templates authorizationService.TemplateService
	Reporting authorizationService.ReportingService
}

type AuthorizationHandler struct {
	service   authorizationService.Service
	templates authorizationService.TemplateService
	reporting authorizationService.ReportingService
}

func NewAuthorizationHandler(opts AuthorizationHandlerOptions) *AuthorizationHandler {
	return &AuthorizationHandler{
		service:   opts.Service,
		templates: opts.Templates,
		reporting: opts.Reporting,
	}
}

// POST /capabilities
func (h *AuthorizationHandler) CreateCapability(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}

	var req createCapabilityDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid capability payload", err))
		return
	}

	result, err := h.service.CreateCapability(c.Request.Context(), authorizationService.CapabilityCreateRequest{
		Namespace:        strings.TrimSpace(req.Namespace),
		Action:           strings.TrimSpace(req.Action),
		Description:      req.Description,
		RiskLevel:        req.RiskLevel,
		DefaultRateLimit: req.DefaultRateLimit,
		Metadata:         req.Metadata,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("create capability failed", err))
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusCreated, capabilityToDTO(result))
}

// PATCH /capabilities/:capabilityId
func (h *AuthorizationHandler) UpdateCapability(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}

	capabilityID, err := uuid.Parse(c.Param("capabilityId"))
	if err != nil || capabilityID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid capability id", err))
		return
	}

	var req updateCapabilityDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	result, err := h.service.UpdateCapability(c.Request.Context(), capabilityID, authorizationService.CapabilityUpdateRequest{
		Description:      req.Description,
		RiskLevel:        req.RiskLevel,
		DefaultRateLimit: req.DefaultRateLimit,
		Metadata:         req.Metadata,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("update capability failed", err))
		return
	}

	dto.ResponseSuccess(c, capabilityToDTO(result))
}

// POST /grants
func (h *AuthorizationHandler) CreateGrant(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}

	var req createGrantDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid grant payload", err))
		return
	}

	createReq, err := h.buildGrantCreateRequest(req)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("build grant request failed", err))
		return
	}

	result, err := h.service.CreateGrant(c.Request.Context(), *createReq)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("create grant failed", err))
		return
	}

	if result.Challenged && result.Ticket != nil {
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, map[string]any{
			"ticket_id":      result.Ticket.UUID.String(),
			"sla_expires_at": result.Ticket.SLAExpiresAt,
		})
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusCreated, buildGrantResponse(result.Grant, result.Capabilities, result.CapabilityMap, result.Conditions, result.Ticket))
}

// GET /grants/:grantId
func (h *AuthorizationHandler) GetGrant(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}
	grantID, err := uuid.Parse(c.Param("grantId"))
	if err != nil || grantID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid grant id", err))
		return
	}

	detail, err := h.service.GetGrant(c.Request.Context(), grantID, true)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("get grant failed", err))
		return
	}
	if detail == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("grant not found", nil))
		return
	}

	dto.ResponseSuccess(c, buildGrantResponse(detail.Grant, detail.Capabilities, detail.CapabilityMap, detail.Conditions, detail.Ticket))
}

// PATCH /grants/:grantId
func (h *AuthorizationHandler) UpdateGrant(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}
	grantID, err := uuid.Parse(c.Param("grantId"))
	uriErr := err
	if err != nil || grantID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid grant id", uriErr))
		return
	}

	var req updateGrantDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	updateReq, err := h.buildGrantUpdateRequest(req)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("build update payload failed", err))
		return
	}

	result, err := h.service.UpdateGrant(c.Request.Context(), grantID, *updateReq)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("update grant failed", err))
		return
	}

	status := http.StatusOK
	if result.Challenged && result.Ticket != nil {
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, map[string]any{
			"ticket_id":      result.Ticket.UUID.String(),
			"sla_expires_at": result.Ticket.SLAExpiresAt,
		})
		return
	}
	dto.ResponseSuccessWithStatus(c, status, buildGrantResponse(result.Grant, result.Capabilities, result.CapabilityMap, result.Conditions, result.Ticket))
}

// POST /grants/:grantId/revoke
func (h *AuthorizationHandler) RevokeGrant(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}
	grantID, err := uuid.Parse(c.Param("grantId"))
	if err != nil || grantID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid grant id", err))
		return
	}

	var req revokeGrantDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	if err := h.service.RevokeGrant(c.Request.Context(), grantID, uuid.Nil, strings.TrimSpace(req.Reason)); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("revoke grant failed", err))
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /grants/cache:invalidate
func (h *AuthorizationHandler) InvalidateGrantCache(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}

	var req invalidateCacheDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	key, err := buildCacheKey(req)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid cache key", err))
		return
	}

	if err := h.service.InvalidateGrantCache(c.Request.Context(), key); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalidate cache failed", err))
		return
	}

	requestID := uuid.NewString()
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, map[string]string{
		"status":     "accepted",
		"request_id": requestID,
	})
}

// GET /audit/authorization
func (h *AuthorizationHandler) ListAuthorizationAudit(c *gin.Context) {
	if h.reporting == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization reporting unavailable", nil))
		return
	}

	var req auditQueryDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid query parameters", err))
		return
	}

	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid tenant id", err))
		return
	}

	from, err := time.Parse(time.RFC3339, req.From)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid from timestamp", err))
		return
	}
	to, err := time.Parse(time.RFC3339, req.To)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid to timestamp", err))
		return
	}

	if header := strings.TrimSpace(c.GetHeader("X-PowerX-Tenant-UUID")); header != "" && !strings.EqualFold(header, tenantID.String()) {
		dto.RespondErrorFrom(c, dto.NewForbidden("tenant scope mismatch", nil))
		return
	}

	filter := authorizationService.ReportingFilter{
		TenantID: tenantID,
		From:     from,
		To:       to,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	if strings.TrimSpace(req.SubjectID) != "" {
		subjectID, err := uuid.Parse(req.SubjectID)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid subject id", err))
			return
		}
		filter.SubjectID = &subjectID
	}
	if req.SubjectType != "" {
		filter.SubjectType = strings.ToLower(req.SubjectType)
	}
	if req.Capability != "" {
		filter.Capability = strings.ToLower(strings.TrimSpace(req.Capability))
	}
	if req.Decision != "" {
		filter.Decision = strings.ToLower(req.Decision)
	}

	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "json"
	}
	if format == "csv" {
		filter.NoLimit = true
	}

	result, err := h.reporting.Query(c.Request.Context(), filter)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("query authorization audit failed", err))
		return
	}

	if format == "csv" {
		data, err := buildAuthorizationAuditCSV(result.Items)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewInternal("build csv failed", err))
			return
		}
		filename := fmt.Sprintf("authorization-audit-%s.csv", time.Now().UTC().Format("20060102T150405Z"))
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		c.String(http.StatusOK, data)
		return
	}

	pagination := map[string]any{
		"page":     result.Page,
		"pageSize": result.PageSize,
		"total":    result.Total,
		"pages":    result.TotalPage,
	}
	if filter.PageSize == 0 {
		pagination = nil
	}

	response := gin.H{
		"items": result.Items,
	}
	if pagination != nil {
		response["pagination"] = pagination
	}
	dto.ResponseSuccess(c, response)
}

// POST /challenges/:ticketId/decision
func (h *AuthorizationHandler) DecideChallenge(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}

	ticketID, err := uuid.Parse(c.Param("ticketId"))
	if err != nil || ticketID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid ticket id", err))
		return
	}

	var req challengeDecisionDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	result, err := h.service.DecideChallenge(c.Request.Context(), ticketID, authorizationService.ChallengeDecisionInput{
		Decision: strings.ToLower(req.Decision),
		Reason:   req.DecisionReason,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("challenge decision failed", err))
		return
	}

	dto.ResponseSuccess(c, map[string]any{
		"ticket_id":    result.Ticket.UUID.String(),
		"status":       result.Ticket.Status,
		"processed_at": result.Ticket.DecisionAt,
	})
}

// Templates ------------------------------------------------------------------

// GET /grant-templates
func (h *AuthorizationHandler) ListTemplates(c *gin.Context) {
	if h.templates == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("template service unavailable", nil))
		return
	}

	var tenantID *uuid.UUID
	if id := strings.TrimSpace(c.Query("tenant_id")); id != "" {
		parsed, err := uuid.Parse(id)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid tenant id", err))
			return
		}
		tenantID = &parsed
	}
	includeGlobal := c.DefaultQuery("include_global", "true") != "false"
	sources := c.QueryArray("source")

	opts := authorizationService.TemplateListOptions{
		TenantID:      tenantID,
		Sources:       sources,
		Search:        c.Query("search"),
		IncludeGlobal: includeGlobal,
	}

	templates, total, err := h.templates.List(c.Request.Context(), opts)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("list templates failed", err))
		return
	}

	items := make([]map[string]any, 0, len(templates))
	for _, tmpl := range templates {
		items = append(items, templateToDTO(tmpl))
	}

	dto.ResponseSuccess(c, map[string]any{
		"items": items,
		"total": total,
	})
}

// POST /grant-templates
func (h *AuthorizationHandler) CreateTemplate(c *gin.Context) {
	if h.templates == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("template service unavailable", nil))
		return
	}

	var req createTemplateDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	var tenantID *uuid.UUID
	if strings.TrimSpace(req.TenantID) != "" {
		id, err := uuid.Parse(req.TenantID)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid tenant id", err))
			return
		}
		tenantID = &id
	}

	template, err := h.templates.Create(c.Request.Context(), authorizationService.TemplateCreateRequest{
		Name:         req.Name,
		Description:  req.Description,
		Source:       req.Source,
		TenantID:     tenantID,
		Capabilities: req.Capabilities,
		Conditions:   toGrantConditions(req.Conditions),
		TTLSeconds:   req.TTLSeconds,
		Metadata:     req.Metadata,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("create template failed", err))
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusCreated, templateToDTO(template))
}

// PATCH /grant-templates/:templateId
func (h *AuthorizationHandler) UpdateTemplate(c *gin.Context) {
	if h.templates == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("template service unavailable", nil))
		return
	}
	templateID, err := uuid.Parse(c.Param("templateId"))
	if err != nil || templateID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid template id", err))
		return
	}

	var req updateTemplateDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	template, err := h.templates.Update(c.Request.Context(), templateID, authorizationService.TemplateUpdateRequest{
		Description:  req.Description,
		Capabilities: req.Capabilities,
		Conditions:   convertOptionalConditions(req.Conditions),
		TTLSeconds:   req.TTLSeconds,
		Metadata:     req.Metadata,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("update template failed", err))
		return
	}

	dto.ResponseSuccess(c, templateToDTO(template))
}

// DELETE /grant-templates/:templateId
func (h *AuthorizationHandler) DeleteTemplate(c *gin.Context) {
	if h.templates == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("template service unavailable", nil))
		return
	}
	templateID, err := uuid.Parse(c.Param("templateId"))
	if err != nil || templateID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid template id", err))
		return
	}

	if err := h.templates.Delete(c.Request.Context(), templateID); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("delete template failed", err))
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /grant-templates/:templateId/apply
func (h *AuthorizationHandler) ApplyTemplate(c *gin.Context) {
	if h.templates == nil || h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("authorization service unavailable", nil))
		return
	}
	templateID, err := uuid.Parse(c.Param("templateId"))
	if err != nil || templateID == uuid.Nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid template id", err))
		return
	}

	var req applyTemplateDTO
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid tenant id", err))
		return
	}
	subjectID, err := uuid.Parse(req.Subject.ID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid subject id", err))
		return
	}

	applyReq := authorizationService.TemplateApplyRequest{
		TemplateID:           templateID,
		TenantID:             tenantID,
		SubjectType:          req.Subject.Type,
		SubjectID:            subjectID,
		TTLOverride:          req.TTLSeconds,
		ConditionsOverride:   convertOptionalConditions(req.Conditions),
		CapabilitiesOverride: req.Capabilities,
		Notes:                req.Notes,
	}

	createReq, err := h.templates.Apply(c.Request.Context(), applyReq)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("apply template failed", err))
		return
	}

	result, err := h.service.CreateGrant(c.Request.Context(), *createReq)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("create grant failed", err))
		return
	}

	if result.Challenged && result.Ticket != nil {
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, map[string]any{
			"ticket_id":      result.Ticket.UUID.String(),
			"sla_expires_at": result.Ticket.SLAExpiresAt,
		})
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, buildGrantResponse(result.Grant, result.Capabilities, result.CapabilityMap, result.Conditions, result.Ticket))
}

// Helpers --------------------------------------------------------------------

func (h *AuthorizationHandler) buildGrantCreateRequest(req createGrantDTO) (*authorizationService.GrantCreateRequest, error) {
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return nil, err
	}
	subjectID, err := uuid.Parse(req.Subject.ID)
	if err != nil {
		return nil, err
	}

	capabilities := make([]authorizationService.GrantCapabilityInput, 0, len(req.Capabilities))
	for _, cap := range req.Capabilities {
		ns, action, err := parseCapabilityKey(cap)
		if err != nil {
			return nil, err
		}
		capabilities = append(capabilities, authorizationService.GrantCapabilityInput{
			Namespace: ns,
			Action:    action,
		})
	}

	var templateID *uuid.UUID
	if strings.TrimSpace(req.TemplateID) != "" {
		id, err := uuid.Parse(req.TemplateID)
		if err != nil {
			return nil, err
		}
		templateID = &id
	}

	return &authorizationService.GrantCreateRequest{
		TenantID:     tenantID,
		SubjectType:  req.Subject.Type,
		SubjectID:    subjectID,
		Source:       req.Source,
		TemplateID:   templateID,
		TTLSeconds:   req.TTLSeconds,
		Capabilities: capabilities,
		Conditions:   toGrantConditions(req.Conditions),
		Notes:        req.Notes,
	}, nil
}

func (h *AuthorizationHandler) buildGrantUpdateRequest(req updateGrantDTO) (*authorizationService.GrantUpdateRequest, error) {
	update := &authorizationService.GrantUpdateRequest{
		Reason: req.Reason,
		Notes:  req.Notes,
	}
	if req.TTLSeconds != nil {
		update.TTLSeconds = req.TTLSeconds
	}
	if req.Capabilities != nil {
		inputs := make([]authorizationService.GrantCapabilityInput, 0, len(*req.Capabilities))
		for _, cap := range *req.Capabilities {
			ns, action, err := parseCapabilityKey(cap.Capability)
			if err != nil {
				return nil, err
			}
			inputs = append(inputs, authorizationService.GrantCapabilityInput{
				Namespace:       ns,
				Action:          action,
				CustomRateLimit: cap.RateLimit,
			})
		}
		update.Capabilities = &inputs
	}
	if req.Conditions != nil {
		conds := toGrantConditions(req.Conditions)
		update.Conditions = &conds
	}
	return update, nil
}

func toGrantConditions(input *grantConditionsDTO) authorizationService.GrantConditionsInput {
	if input == nil {
		return authorizationService.GrantConditionsInput{}
	}
	out := authorizationService.GrantConditionsInput{
		Resources:   input.Resources,
		ContextTags: input.ContextTags,
	}
	if input.TimeWindow != nil {
		out.TimeWindow = &authorizationService.GrantTimeWindow{
			Start: input.TimeWindow.Start,
			End:   input.TimeWindow.End,
		}
	}
	return out
}

func convertOptionalConditions(input *grantConditionsDTO) *authorizationService.GrantConditionsInput {
	if input == nil {
		return nil
	}
	conds := toGrantConditions(input)
	return &conds
}

func buildGrantResponse(grant *eventfabricmodel.AuthorizationGrant, capabilities []*eventfabricmodel.AuthorizationGrantCapability, capMap map[uuid.UUID]*eventfabricmodel.AuthorizationCapability, conditions []*eventfabricmodel.AuthorizationGrantCondition, ticket *eventfabricmodel.AuthorizationApprovalTicket) map[string]any {
	payload := map[string]any{
		"id":        grant.UUID.String(),
		"tenant_id": grant.TenantID.String(),
		"subject": map[string]any{
			"type": grant.SubjectType,
			"id":   grant.SubjectID.String(),
		},
		"status":         grant.Status,
		"version":        grant.Version,
		"source":         grant.Source,
		"ttl_expires_at": grant.TTLExpiresAt,
	}

	items := make([]map[string]any, 0, len(capabilities))
	for _, record := range capabilities {
		capability := capMap[record.CapabilityID]
		entry := map[string]any{
			"capability": "",
			"rate_limit": nil,
		}
		if capability != nil {
			entry["capability"] = capability.Namespace + "." + capability.Action
			if len(capability.DefaultRateLimit) > 0 {
				var defaults map[string]any
				if err := json.Unmarshal(capability.DefaultRateLimit, &defaults); err == nil {
					entry["default_rate_limit"] = defaults
				}
			}
		}
		if len(record.CustomRateLimit) > 0 {
			var custom map[string]any
			if err := json.Unmarshal(record.CustomRateLimit, &custom); err == nil {
				entry["rate_limit"] = custom
			}
		}
		items = append(items, entry)
	}
	payload["capabilities"] = items

	if len(conditions) > 0 {
		conditionPayload := map[string]any{}
		for _, cond := range conditions {
			switch cond.Type {
			case eventfabricmodel.GrantConditionTypeResource:
				var data map[string]any
				if err := json.Unmarshal(cond.Expression, &data); err == nil {
					conditionPayload["resources"] = data["resources"]
				}
			case eventfabricmodel.GrantConditionTypeContextTag:
				var data map[string]any
				if err := json.Unmarshal(cond.Expression, &data); err == nil {
					conditionPayload["context_tags"] = data["context_tags"]
				}
			case eventfabricmodel.GrantConditionTypeTimeWindow:
				var data map[string]any
				if err := json.Unmarshal(cond.Expression, &data); err == nil {
					conditionPayload["time_window"] = data
				}
			}
		}
		payload["conditions"] = conditionPayload
	}

	if ticket != nil {
		payload["challenge_ticket"] = map[string]any{
			"id":             ticket.UUID.String(),
			"status":         ticket.Status,
			"sla_expires_at": ticket.SLAExpiresAt,
		}
	}

	return payload
}

func capabilityToDTO(cap *eventfabricmodel.AuthorizationCapability) map[string]any {
	result := map[string]any{
		"id":          cap.UUID.String(),
		"namespace":   cap.Namespace,
		"action":      cap.Action,
		"description": cap.Description,
		"risk_level":  cap.RiskLevel,
	}
	if len(cap.DefaultRateLimit) > 0 {
		var limit map[string]any
		if err := json.Unmarshal(cap.DefaultRateLimit, &limit); err == nil {
			result["default_rate_limit"] = limit
		}
	}
	if len(cap.Metadata) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(cap.Metadata, &meta); err == nil {
			result["metadata"] = meta
		}
	}
	return result
}

func templateToDTO(tmpl *eventfabricmodel.AuthorizationGrantTemplate) map[string]any {
	payload := map[string]any{
		"id":          tmpl.UUID.String(),
		"name":        tmpl.Name,
		"description": tmpl.Description,
		"source":      tmpl.Source,
		"ttl_seconds": tmpl.TTLSeconds,
	}
	if tmpl.TenantID != nil {
		payload["tenant_id"] = tmpl.TenantID.String()
	}
	if len(tmpl.Capabilities) > 0 {
		var caps []string
		if err := json.Unmarshal(tmpl.Capabilities, &caps); err == nil {
			payload["capabilities"] = caps
		}
	}
	if len(tmpl.Conditions) > 0 {
		var conds map[string]any
		if err := json.Unmarshal(tmpl.Conditions, &conds); err == nil {
			payload["conditions"] = conds
		}
	}
	if len(tmpl.Metadata) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(tmpl.Metadata, &meta); err == nil {
			payload["metadata"] = meta
		}
	}
	return payload
}

func buildCacheKey(req invalidateCacheDTO) (authorizationService.GrantCacheKey, error) {
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return authorizationService.GrantCacheKey{}, err
	}
	subjectID, err := uuid.Parse(req.Subject.ID)
	if err != nil {
		return authorizationService.GrantCacheKey{}, err
	}
	return authorizationService.GrantCacheKey{
		TenantID:    tenantID.String(),
		SubjectType: req.Subject.Type,
		SubjectID:   subjectID.String(),
	}, nil
}

func parseCapabilityKey(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid capability %q", value)
	}
	action := strings.TrimSpace(parts[len(parts)-1])
	namespace := strings.TrimSpace(strings.Join(parts[:len(parts)-1], "."))
	if namespace == "" || action == "" {
		return "", "", fmt.Errorf("invalid capability %q", value)
	}
	return namespace, action, nil
}

func buildAuthorizationAuditCSV(items []authorizationService.ReportingEvent) (string, error) {
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	header := []string{
		"occurred_at",
		"category",
		"operation",
		"decision",
		"outcome",
		"tenant_id",
		"subject_type",
		"subject_id",
		"capability",
		"reason",
		"actor",
	}
	if err := writer.Write(header); err != nil {
		return "", err
	}
	for _, item := range items {
		record := []string{
			item.OccurredAt.UTC().Format(time.RFC3339),
			item.Category,
			item.Operation,
			item.Decision,
			item.Outcome,
			item.TenantID,
			item.SubjectType,
			item.SubjectID,
			item.Capability,
			item.Reason,
			item.Actor,
		}
		if err := writer.Write(record); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return buf.String(), nil
}
