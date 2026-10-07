package knowledge_space

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	knowledgesvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
)

// Register mounts read-only knowledge space health endpoints.
func Register(public, protected *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.DB == nil {
		return
	}
	handler := &openapiHandler{deps: deps}
	qaHandler := newQABridgeHandler(deps)
	hostHandler := newHostContractHandler(deps)

	// Status is safe to expose as a read-only health snapshot. Keep QA endpoints protected.
	if public != nil {
		group := public.Group("/openapi/knowledge-spaces")
		group.GET("/status", handler.status)
	}
	if protected != nil && qaHandler != nil {
		group := protected.Group("/openapi/knowledge-spaces")
		group.POST("/qa/retrieval-plan", qaHandler.plan)
		group.POST("/qa/memory-snapshot", qaHandler.memorySnapshot)
	}
	if protected != nil && hostHandler != nil {
		group := protected.Group("/tenant/knowledge")
		group.GET("/spaces", hostHandler.listSpaces)
		group.GET("/catalog", hostHandler.catalog)
		group.POST("/spaces", hostHandler.createSpace)
		group.POST("/search", hostHandler.search)
		group.POST("/spaces/:space_uuid/documents", hostHandler.upsertDocument)
		group.DELETE("/spaces/:space_uuid/documents/:document_uuid", hostHandler.deleteDocument)
		// Gin parses ':' as a parameter delimiter. Keep the published action URL
		// behind a constrained dispatcher, as done for IAM batch actions.
		group.POST("/spaces/:space_uuid/indexes:operation", hostHandler.indexOperation)
		group.GET("/index-jobs/:job_uuid", hostHandler.getIndexJob)
	}
}

type openapiHandler struct {
	deps *shared.Deps
}

type statusView struct {
	PendingIAM int64 `json:"pendingIam"`
	Active     int64 `json:"active"`
	Retired    int64 `json:"retired"`
}

func (h *openapiHandler) status(c *gin.Context) {
	ctx := c.Request.Context()
	var pending, active, retired int64
	db := h.deps.DB.WithContext(ctx)
	db.Model(&models.KnowledgeSpace{}).Where("status = ?", models.KnowledgeSpaceStatusPending).Count(&pending)
	db.Model(&models.KnowledgeSpace{}).Where("status = ?", models.KnowledgeSpaceStatusActive).Count(&active)
	db.Model(&models.KnowledgeSpace{}).Where("status = ?", models.KnowledgeSpaceStatusRetired).Count(&retired)

	c.JSON(http.StatusOK, statusView{
		PendingIAM: pending,
		Active:     active,
		Retired:    retired,
	})
}

type hostContractHandler struct {
	provisioning *knowledgesvc.Service
	service      *knowledgesvc.HostContractService
	access       *knowledgesvc.HostContractAccess
}

func newHostContractHandler(deps *shared.Deps) *hostContractHandler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	var provisioning *knowledgesvc.Service
	if deps.KnowledgeSpace != nil {
		provisioning = deps.KnowledgeSpace.Service
	}
	return &hostContractHandler{provisioning: provisioning, service: knowledgesvc.NewHostContractService(deps.DB), access: knowledgesvc.NewHostContractAccess(deps.DB)}
}

type searchRequest struct {
	Query      string   `json:"query" binding:"required"`
	SpaceUUIDs []string `json:"space_uuids"`
	Limit      int      `json:"limit"`
	TenantUUID string   `json:"tenant_uuid"`
}
type documentRequest struct {
	Title       string   `json:"title" binding:"required"`
	URI         string   `json:"uri" binding:"required"`
	Content     string   `json:"content" binding:"required"`
	ContentType string   `json:"content_type" binding:"required"`
	Checksum    string   `json:"checksum" binding:"required"`
	Version     string   `json:"version" binding:"required"`
	Tags        []string `json:"tags"`
	TenantUUID  string   `json:"tenant_uuid"`
}
type tenantOverrideQuery struct {
	TenantUUID string `form:"tenant_uuid"`
}

func (h *hostContractHandler) listSpaces(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	tenantUUID, err := h.authorizeDirectory(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	items, err := h.service.ListSpaces(c.Request.Context(), tenantUUID)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *hostContractHandler) search(c *gin.Context) {
	var req searchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || !rejectTenantOverride(c) {
		if strings.TrimSpace(req.TenantUUID) != "" {
			dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		}
		return
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	tenantUUID, err := h.authorizeSearch(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	items, err := h.service.Search(c.Request.Context(), tenantUUID, req.Query, req.SpaceUUIDs, req.Limit)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *hostContractHandler) upsertDocument(c *gin.Context) {
	var req documentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || !rejectTenantOverride(c) {
		if strings.TrimSpace(req.TenantUUID) != "" {
			dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		}
		return
	}
	spaceUUID, err := requiredUUID(c.Param("space_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.authorizeDocument(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	job, err := h.service.UpsertDocument(c.Request.Context(), tenantUUID, spaceUUID, knowledgesvc.HostDocumentInput{Title: req.Title, URI: req.URI, Content: req.Content, ContentType: req.ContentType, Checksum: req.Checksum, Version: req.Version, Tags: req.Tags})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, job)
}

func (h *hostContractHandler) deleteDocument(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	spaceUUID, err := requiredUUID(c.Param("space_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	documentUUID, err := requiredUUID(c.Param("document_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.authorizeDocument(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	job, err := h.service.DeleteDocument(c.Request.Context(), tenantUUID, spaceUUID, documentUUID)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, job)
}

func (h *hostContractHandler) indexOperation(c *gin.Context) {
	if strings.TrimPrefix(c.Param("operation"), ":") != "rebuild" {
		c.Status(http.StatusNotFound)
		return
	}
	if !rejectTenantOverride(c) {
		return
	}
	spaceUUID, err := requiredUUID(c.Param("space_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.authorizeDocument(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	job, err := h.service.RebuildIndex(c.Request.Context(), tenantUUID, spaceUUID)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, job)
}

func (h *hostContractHandler) getIndexJob(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	jobUUID, err := requiredUUID(c.Param("job_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.authorizeDocument(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	job, err := h.service.GetIndexJob(c.Request.Context(), tenantUUID, jobUUID)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, job)
}

func (h *hostContractHandler) authorizeDirectory(c *gin.Context) (string, error) {
	return h.access.AuthorizeDirectoryRead(c.Request.Context(), apiKeyHash(c))
}
func (h *hostContractHandler) authorizeSearch(c *gin.Context) (string, error) {
	return h.access.AuthorizeSearchRead(c.Request.Context(), apiKeyHash(c))
}
func (h *hostContractHandler) authorizeDocument(c *gin.Context) (string, error) {
	return h.access.AuthorizeDocumentManage(c.Request.Context(), apiKeyHash(c))
}
func apiKeyHash(c *gin.Context) string {
	raw, _ := c.Get("auth_api_key_hash")
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}
func requiredUUID(raw string) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
func rejectTenantOverride(c *gin.Context) bool {
	var query tenantOverrideQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return false
	}
	if strings.TrimSpace(query.TenantUUID) != "" {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		return false
	}
	if strings.TrimSpace(c.GetHeader("X-Tenant-UUID")) != "" {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(errors.New("tenant override header must not be supplied")))
		return false
	}
	return true
}

func (h *hostContractHandler) catalog(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	tenant, err := h.access.AuthorizeCatalogRead(c.Request.Context(), apiKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	catalog, err := h.provisioning.GetHostCatalog(c.Request.Context(), tenant)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"catalog": catalog})
}
func (h *hostContractHandler) createSpace(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	tenant, err := h.access.AuthorizeSpaceCreate(c.Request.Context(), apiKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var request knowledgesvc.HostCreateSpaceRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(err))
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		dto.RespondErrorFrom(c, knowledgesvc.KnowledgeInvalidArgumentError(errors.New("expected one JSON object")))
		return
	}
	item, err := h.provisioning.CreateHostSpace(c.Request.Context(), tenant, request)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"item": item})
}
