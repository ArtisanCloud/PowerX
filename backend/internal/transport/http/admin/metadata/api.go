package metadata

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil {
		return
	}
	h := &handler{}
	if deps != nil && deps.DB != nil {
		svc, err := metasvc.NewService(metasvc.Deps{DB: deps.DB, ValidatorRegistry: deps.MetadataResourceValidatorRegistry})
		if err == nil {
			h.service = svc
		}
	}
	g := protected.Group("/admin/metadata")
	g.GET("/dictionaries", h.listDictionaryNamespaces)
	g.POST("/dictionaries", h.createDictionaryNamespace)
	g.PATCH("/dictionaries/:namespace_uuid", h.updateDictionaryNamespace)
	g.GET("/dictionaries/:namespace_uuid/items", h.listDictionaryItems)
	g.POST("/dictionaries/:namespace_uuid/items", h.createDictionaryItem)
	g.PATCH("/dictionary-items/:item_uuid", h.updateDictionaryItem)
	g.DELETE("/dictionary-items/:item_uuid", h.deleteDictionaryItem)
	g.GET("/taxonomies", h.listTaxonomies)
	g.POST("/taxonomies", h.createTaxonomy)
	g.PATCH("/taxonomies/:taxonomy_uuid", h.updateTaxonomy)
	g.GET("/taxonomies/:taxonomy_uuid/nodes", h.listTaxonomyNodes)
	g.POST("/taxonomies/:taxonomy_uuid/nodes", h.createTaxonomyNode)
	g.PATCH("/taxonomy-nodes/:node_uuid", h.updateTaxonomyNode)
	g.DELETE("/taxonomy-nodes/:node_uuid", h.deleteTaxonomyNode)
	g.POST("/taxonomy-nodes/:node_uuid/move", h.moveTaxonomyNode)
	g.GET("/tags", h.listTags)
	g.POST("/tags", h.createTag)
	g.PATCH("/tags/:tag_uuid", h.updateTag)
	g.DELETE("/tags/:tag_uuid", h.deleteTag)
	g.POST("/tags/merge", h.mergeTags)
	g.GET("/tag-bindings", h.listTagBindings)
	g.PUT("/tag-bindings", h.replaceTagBindings)
	g.GET("/resource-types", h.listResourceTypes)
	g.POST("/resource-types", h.registerResourceType)
	g.PATCH("/resource-types/:resource_type_uuid", h.updateResourceType)
}

type handler struct {
	service *metasvc.Service
}

func (h *handler) notImplemented(c *gin.Context) {
	respondError(c, errors.New(metasvc.CodeNotImplemented))
}

type listDictionaryNamespacesRequest struct {
	dto.PaginationRequest
	Module string `form:"module"`
	Status string `form:"status"`
	Q      string `form:"q"`
	Locale string `form:"locale"`
}

type createDictionaryNamespaceRequest struct {
	Namespace       string            `json:"namespace" validate:"required"`
	Module          string            `json:"module" validate:"required"`
	NameI18n        map[string]string `json:"name_i18n" validate:"required"`
	DescriptionI18n map[string]string `json:"description_i18n"`
}

type updateDictionaryNamespaceRequest struct {
	NameI18n        *map[string]string `json:"name_i18n"`
	DescriptionI18n *map[string]string `json:"description_i18n"`
	Status          *string            `json:"status"`
}

type createDictionaryItemRequest struct {
	Code            string            `json:"code" validate:"required"`
	LabelI18n       map[string]string `json:"label_i18n" validate:"required"`
	DescriptionI18n map[string]string `json:"description_i18n"`
	SortOrder       int               `json:"sort_order"`
	Metadata        map[string]any    `json:"metadata"`
}

type updateDictionaryItemRequest struct {
	LabelI18n       *map[string]string `json:"label_i18n"`
	DescriptionI18n *map[string]string `json:"description_i18n"`
	SortOrder       *int               `json:"sort_order"`
	Status          *string            `json:"status"`
	Metadata        *map[string]any    `json:"metadata"`
}

type listDictionaryItemsRequest struct {
	dto.PaginationRequest
	Status string `form:"status"`
	Q      string `form:"q"`
	Locale string `form:"locale"`
}

type listTaxonomiesRequest struct {
	dto.PaginationRequest
	Module string `form:"module"`
	Status string `form:"status"`
	Q      string `form:"q"`
	Locale string `form:"locale"`
}

type createTaxonomyRequest struct {
	Namespace       string            `json:"namespace" validate:"required"`
	Module          string            `json:"module" validate:"required"`
	NameI18n        map[string]string `json:"name_i18n" validate:"required"`
	DescriptionI18n map[string]string `json:"description_i18n"`
	MaxDepth        int               `json:"max_depth" validate:"required,min=1"`
}

type updateTaxonomyRequest struct {
	NameI18n        *map[string]string `json:"name_i18n"`
	DescriptionI18n *map[string]string `json:"description_i18n"`
	MaxDepth        *int               `json:"max_depth"`
	Status          *string            `json:"status"`
}

type listTaxonomyNodesRequest struct {
	dto.PaginationRequest
	Status string `form:"status"`
	Q      string `form:"q"`
	Locale string `form:"locale"`
}

type createTaxonomyNodeRequest struct {
	ParentUUID      *string           `json:"parent_uuid"`
	Code            string            `json:"code" validate:"required"`
	LabelI18n       map[string]string `json:"label_i18n" validate:"required"`
	DescriptionI18n map[string]string `json:"description_i18n"`
	SortOrder       int               `json:"sort_order"`
}

type updateTaxonomyNodeRequest struct {
	LabelI18n       *map[string]string `json:"label_i18n"`
	DescriptionI18n *map[string]string `json:"description_i18n"`
	SortOrder       *int               `json:"sort_order"`
	Status          *string            `json:"status"`
	Version         int64              `json:"version" validate:"required,min=1"`
}

type moveTaxonomyNodeRequest struct {
	TargetParentUUID *string `json:"target_parent_uuid"`
	SortOrder        *int    `json:"sort_order"`
	Version          int64   `json:"version" validate:"required,min=1"`
}

type listTagsRequest struct {
	dto.PaginationRequest
	Namespace    string `form:"namespace"`
	ResourceType string `form:"resource_type"`
	Status       string `form:"status"`
	Q            string `form:"q"`
	Locale       string `form:"locale"`
}

type createTagRequest struct {
	Namespace       string            `json:"namespace" validate:"required"`
	ResourceType    string            `json:"resource_type" validate:"required"`
	Code            string            `json:"code" validate:"required"`
	LabelI18n       map[string]string `json:"label_i18n" validate:"required"`
	DescriptionI18n map[string]string `json:"description_i18n"`
	Color           string            `json:"color"`
}

type updateTagRequest struct {
	LabelI18n       *map[string]string `json:"label_i18n"`
	DescriptionI18n *map[string]string `json:"description_i18n"`
	Color           *string            `json:"color"`
	Status          *string            `json:"status"`
}

type mergeTagsRequest struct {
	SourceTagUUID string `json:"source_tag_uuid" validate:"required"`
	TargetTagUUID string `json:"target_tag_uuid" validate:"required"`
}

type listTagBindingsRequest struct {
	ResourceType string `form:"resource_type" validate:"required"`
	ResourceUUID string `form:"resource_uuid" validate:"required"`
	Locale       string `form:"locale"`
}

type replaceTagBindingsRequest struct {
	ResourceType string   `json:"resource_type" validate:"required"`
	ResourceUUID string   `json:"resource_uuid" validate:"required"`
	TagUUIDs     []string `json:"tag_uuids"`
}

type listResourceTypesRequest struct {
	dto.PaginationRequest
	Module string `form:"module"`
	Status string `form:"status"`
	Q      string `form:"q"`
	Locale string `form:"locale"`
}

type registerResourceTypeRequest struct {
	ResourceType    string            `json:"resource_type" validate:"required"`
	Module          string            `json:"module" validate:"required"`
	NameI18n        map[string]string `json:"name_i18n" validate:"required"`
	DescriptionI18n map[string]string `json:"description_i18n"`
	ValidatorKey    string            `json:"validator_key"`
	BindingEnabled  bool              `json:"binding_enabled"`
}

type updateResourceTypeRequest struct {
	NameI18n        *map[string]string `json:"name_i18n"`
	DescriptionI18n *map[string]string `json:"description_i18n"`
	ValidatorKey    *string            `json:"validator_key"`
	BindingEnabled  *bool              `json:"binding_enabled"`
	Status          *string            `json:"status"`
}

func (h *handler) listDictionaryNamespaces(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listDictionaryNamespacesRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	page, err := h.service.ListDictionaryNamespaces(c.Request.Context(), metasvc.ListDictionaryNamespacesInput{
		TenantUUID: tenantUUID,
		Module:     req.Module,
		Status:     req.Status,
		Query:      req.Q,
		Locale:     req.Locale,
		Page:       req.Page,
		PageSize:   req.PageSize,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": page.Items, "pagination": gin.H{"total": page.Total, "page": page.Page, "page_size": page.PageSize}}})
}

func (h *handler) createDictionaryNamespace(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req createDictionaryNamespaceRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.CreateDictionaryNamespace(c.Request.Context(), metasvc.CreateDictionaryNamespaceInput{
		TenantUUID:      tenantUUID,
		Namespace:       req.Namespace,
		Module:          req.Module,
		NameI18n:        req.NameI18n,
		DescriptionI18n: req.DescriptionI18n,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": item})
}

func (h *handler) updateDictionaryNamespace(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req updateDictionaryNamespaceRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.UpdateDictionaryNamespace(c.Request.Context(), metasvc.UpdateDictionaryNamespaceInput{
		TenantUUID:      tenantUUID,
		NamespaceUUID:   c.Param("namespace_uuid"),
		NameI18n:        req.NameI18n,
		DescriptionI18n: req.DescriptionI18n,
		Status:          req.Status,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}

func (h *handler) listDictionaryItems(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listDictionaryItemsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	page, err := h.service.ListDictionaryItems(c.Request.Context(), metasvc.ListDictionaryItemsInput{
		TenantUUID:    tenantUUID,
		NamespaceUUID: c.Param("namespace_uuid"),
		Status:        req.Status,
		Query:         req.Q,
		Locale:        req.Locale,
		Page:          req.Page,
		PageSize:      req.PageSize,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": page.Items, "pagination": gin.H{"total": page.Total, "page": page.Page, "page_size": page.PageSize}}})
}

func (h *handler) createDictionaryItem(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req createDictionaryItemRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.CreateDictionaryItem(c.Request.Context(), metasvc.CreateDictionaryItemInput{
		TenantUUID:      tenantUUID,
		NamespaceUUID:   c.Param("namespace_uuid"),
		Code:            req.Code,
		LabelI18n:       req.LabelI18n,
		DescriptionI18n: req.DescriptionI18n,
		SortOrder:       req.SortOrder,
		Metadata:        req.Metadata,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": item})
}

func (h *handler) updateDictionaryItem(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req updateDictionaryItemRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.UpdateDictionaryItem(c.Request.Context(), metasvc.UpdateDictionaryItemInput{
		TenantUUID:      tenantUUID,
		ItemUUID:        c.Param("item_uuid"),
		LabelI18n:       req.LabelI18n,
		DescriptionI18n: req.DescriptionI18n,
		SortOrder:       req.SortOrder,
		Status:          req.Status,
		Metadata:        req.Metadata,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}

func (h *handler) deleteDictionaryItem(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	refs, err := h.service.DeleteDictionaryItem(c.Request.Context(), metasvc.DeleteDictionaryItemInput{
		TenantUUID: tenantUUID,
		ItemUUID:   c.Param("item_uuid"),
	})
	if err != nil {
		if errors.Is(err, metasvc.ErrReferenceConflict) {
			dto.ResponseErrorWithDetails(c, http.StatusConflict, "metadata.reference_conflict", err, gin.H{"references": refs})
			return
		}
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"deleted": true, "item_uuid": strings.TrimSpace(c.Param("item_uuid"))}})
}

func (h *handler) listTaxonomies(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listTaxonomiesRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	page, err := h.service.ListTaxonomies(c.Request.Context(), metasvc.ListTaxonomiesInput{
		TenantUUID: tenantUUID,
		Module:     req.Module,
		Status:     req.Status,
		Query:      req.Q,
		Locale:     req.Locale,
		Page:       req.Page,
		PageSize:   req.PageSize,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": page.Items, "pagination": gin.H{"total": page.Total, "page": page.Page, "page_size": page.PageSize}}})
}

func (h *handler) createTaxonomy(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req createTaxonomyRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.CreateTaxonomy(c.Request.Context(), metasvc.CreateTaxonomyInput{
		TenantUUID:      tenantUUID,
		Namespace:       req.Namespace,
		Module:          req.Module,
		NameI18n:        req.NameI18n,
		DescriptionI18n: req.DescriptionI18n,
		MaxDepth:        req.MaxDepth,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": item})
}

func (h *handler) updateTaxonomy(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req updateTaxonomyRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.UpdateTaxonomy(c.Request.Context(), metasvc.UpdateTaxonomyInput{
		TenantUUID:      tenantUUID,
		TaxonomyUUID:    c.Param("taxonomy_uuid"),
		NameI18n:        req.NameI18n,
		DescriptionI18n: req.DescriptionI18n,
		MaxDepth:        req.MaxDepth,
		Status:          req.Status,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}

func (h *handler) listTaxonomyNodes(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listTaxonomyNodesRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	page, err := h.service.ListTaxonomyNodesPage(c.Request.Context(), metasvc.ListTaxonomyNodesInput{
		TenantUUID:   tenantUUID,
		TaxonomyUUID: c.Param("taxonomy_uuid"),
		Status:       req.Status,
		Query:        req.Q,
		Locale:       req.Locale,
		Page:         req.Page,
		PageSize:     req.PageSize,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": page.Items, "pagination": gin.H{"total": page.Total, "page": page.Page, "page_size": page.PageSize}}})
}

func (h *handler) createTaxonomyNode(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req createTaxonomyNodeRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.CreateTaxonomyNode(c.Request.Context(), metasvc.CreateTaxonomyNodeInput{
		TenantUUID:      tenantUUID,
		TaxonomyUUID:    c.Param("taxonomy_uuid"),
		ParentUUID:      req.ParentUUID,
		Code:            req.Code,
		LabelI18n:       req.LabelI18n,
		DescriptionI18n: req.DescriptionI18n,
		SortOrder:       req.SortOrder,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": item})
}

func (h *handler) updateTaxonomyNode(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req updateTaxonomyNodeRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.UpdateTaxonomyNode(c.Request.Context(), metasvc.UpdateTaxonomyNodeInput{
		TenantUUID:      tenantUUID,
		NodeUUID:        c.Param("node_uuid"),
		LabelI18n:       req.LabelI18n,
		DescriptionI18n: req.DescriptionI18n,
		SortOrder:       req.SortOrder,
		Status:          req.Status,
		Version:         req.Version,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}

func (h *handler) moveTaxonomyNode(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req moveTaxonomyNodeRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.MoveTaxonomyNode(c.Request.Context(), metasvc.MoveTaxonomyNodeInput{
		TenantUUID:       tenantUUID,
		NodeUUID:         c.Param("node_uuid"),
		TargetParentUUID: req.TargetParentUUID,
		SortOrder:        req.SortOrder,
		Version:          req.Version,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}

func (h *handler) deleteTaxonomyNode(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	refs, err := h.service.DeleteTaxonomyNode(c.Request.Context(), metasvc.DeleteTaxonomyNodeInput{
		TenantUUID: tenantUUID,
		NodeUUID:   c.Param("node_uuid"),
	})
	if err != nil {
		if errors.Is(err, metasvc.ErrReferenceConflict) {
			dto.ResponseErrorWithDetails(c, http.StatusConflict, "metadata.reference_conflict", err, gin.H{"references": refs})
			return
		}
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"deleted": true, "node_uuid": strings.TrimSpace(c.Param("node_uuid"))}})
}

func (h *handler) listTags(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listTagsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	page, err := h.service.ListTags(c.Request.Context(), metasvc.ListTagsInput{
		TenantUUID:   tenantUUID,
		Namespace:    req.Namespace,
		ResourceType: req.ResourceType,
		Status:       req.Status,
		Query:        req.Q,
		Locale:       req.Locale,
		Page:         req.Page,
		PageSize:     req.PageSize,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": page.Items, "pagination": gin.H{"total": page.Total, "page": page.Page, "page_size": page.PageSize}}})
}

func (h *handler) createTag(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req createTagRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.CreateTag(c.Request.Context(), metasvc.CreateTagInput{
		TenantUUID:      tenantUUID,
		Namespace:       req.Namespace,
		ResourceType:    req.ResourceType,
		Code:            req.Code,
		LabelI18n:       req.LabelI18n,
		DescriptionI18n: req.DescriptionI18n,
		Color:           req.Color,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": item})
}

func (h *handler) updateTag(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req updateTagRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.UpdateTag(c.Request.Context(), metasvc.UpdateTagInput{
		TenantUUID:      tenantUUID,
		TagUUID:         c.Param("tag_uuid"),
		LabelI18n:       req.LabelI18n,
		DescriptionI18n: req.DescriptionI18n,
		Color:           req.Color,
		Status:          req.Status,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}

func (h *handler) deleteTag(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if err := h.service.DeleteTag(c.Request.Context(), metasvc.DeleteTagInput{TenantUUID: tenantUUID, TagUUID: c.Param("tag_uuid")}); err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"deleted": true, "tag_uuid": strings.TrimSpace(c.Param("tag_uuid"))}})
}

func (h *handler) mergeTags(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req mergeTagsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	moved, err := h.service.MergeTags(c.Request.Context(), metasvc.MergeTagsInput{
		TenantUUID:    tenantUUID,
		SourceTagUUID: req.SourceTagUUID,
		TargetTagUUID: req.TargetTagUUID,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"moved_bindings": moved}})
}

func (h *handler) listTagBindings(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listTagBindingsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	items, err := h.service.ListTagBindings(c.Request.Context(), metasvc.ListTagBindingsInput{
		TenantUUID:   tenantUUID,
		ResourceType: req.ResourceType,
		ResourceUUID: req.ResourceUUID,
		Locale:       req.Locale,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items}})
}

func (h *handler) replaceTagBindings(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req replaceTagBindingsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	items, err := h.service.ReplaceTagBindings(c.Request.Context(), metasvc.ReplaceTagBindingsInput{
		TenantUUID:   tenantUUID,
		ResourceType: req.ResourceType,
		ResourceUUID: req.ResourceUUID,
		TagUUIDs:     req.TagUUIDs,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items}})
}

func (h *handler) listResourceTypes(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listResourceTypesRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	page, err := h.service.ListResourceTypes(c.Request.Context(), metasvc.ListResourceTypesInput{
		TenantUUID: tenantUUID,
		Module:     req.Module,
		Status:     req.Status,
		Query:      req.Q,
		Locale:     req.Locale,
		Page:       req.Page,
		PageSize:   req.PageSize,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": page.Items, "pagination": gin.H{"total": page.Total, "page": page.Page, "page_size": page.PageSize}}})
}

func (h *handler) registerResourceType(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req registerResourceTypeRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.RegisterResourceType(c.Request.Context(), metasvc.RegisterResourceTypeInput{
		TenantUUID:      tenantUUID,
		ResourceType:    req.ResourceType,
		Module:          req.Module,
		NameI18n:        req.NameI18n,
		DescriptionI18n: req.DescriptionI18n,
		ValidatorKey:    req.ValidatorKey,
		BindingEnabled:  req.BindingEnabled,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": item})
}

func (h *handler) updateResourceType(c *gin.Context) {
	if h.service == nil {
		h.notImplemented(c)
		return
	}
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req updateResourceTypeRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.service.UpdateResourceType(c.Request.Context(), metasvc.UpdateResourceTypeInput{
		TenantUUID:       tenantUUID,
		ResourceTypeUUID: c.Param("resource_type_uuid"),
		NameI18n:         req.NameI18n,
		DescriptionI18n:  req.DescriptionI18n,
		ValidatorKey:     req.ValidatorKey,
		BindingEnabled:   req.BindingEnabled,
		Status:           req.Status,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}

func requireTenant(c *gin.Context) (string, bool) {
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenantUUID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "metadata.tenant_uuid_required", nil)
		return "", false
	}
	return tenantUUID, true
}
