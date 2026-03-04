package eventfabric

import (
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type AdminDirectoryHandlerOptions struct {
	Service *directory.DirectoryService
}

type AdminDirectoryHandler struct {
	service *directory.DirectoryService
}

func NewAdminDirectoryHandler(opts AdminDirectoryHandlerOptions) *AdminDirectoryHandler {
	return &AdminDirectoryHandler{service: opts.Service}
}

type createTopicRequest struct {
	Namespace       string                 `json:"namespace"`
	Name            string                 `json:"name"`
	PayloadFormat   string                 `json:"payload_format"`
	MaxRetry        int32                  `json:"max_retry"`
	AckTimeoutSec   int32                  `json:"ack_timeout_sec"`
	VersioningMode  string                 `json:"versioning_mode"`
	RetentionPolicy string                 `json:"retention_policy"`
	Metadata        map[string]interface{} `json:"metadata"`
	CreatedBy       string                 `json:"created_by"`
}

type lifecycleRequest struct {
	TargetState  string `json:"target_state"`
	ChangeReason string `json:"change_reason"`
}

func (h *AdminDirectoryHandler) CreateTopic(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
		return
	}

	var req createTopicRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request", err))
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	topic, err := h.service.CreateTopic(c.Request.Context(), directory.CreateTopicInput{
		TenantUUID:      tenantUUID,
		Namespace:       req.Namespace,
		Name:            req.Name,
		PayloadFormat:   req.PayloadFormat,
		MaxRetry:        req.MaxRetry,
		AckTimeoutSec:   req.AckTimeoutSec,
		VersioningMode:  req.VersioningMode,
		RetentionPolicy: req.RetentionPolicy,
		Metadata:        req.Metadata,
		CreatedBy:       req.CreatedBy,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("create topic failed", err))
		return
	}

	dto.ResponseSuccess(c, topic)
}

func (h *AdminDirectoryHandler) ListTopics(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize <= 0 {
		pageSize = 20
	}

	lifecycleQuery := c.Query("lifecycle")
	var lifecycles []model.TopicLifecycle
	if lifecycleQuery != "" {
		for _, token := range strings.Split(lifecycleQuery, ",") {
			lifecycles = append(lifecycles, model.TopicLifecycle(strings.TrimSpace(token)))
		}
	}

	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	filter := repository.TopicFilter{
		Namespace: c.Query("namespace"),
		Lifecycle: lifecycles,
	}
	filter.TenantID = tenantUUID
	filter.IncludeShared = true

	list, total, err := h.service.ListTopics(c.Request.Context(), repository.QueryContext{
		Filter: filter,
		Page: repository.PageOptions{
			Limit:  pageSize,
			Offset: (page - 1) * pageSize,
		},
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list topics failed", err))
		return
	}

	dto.ResponseSuccess(c, map[string]interface{}{
		"items":     list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *AdminDirectoryHandler) UpdateLifecycle(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
		return
	}

	var req lifecycleRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request", err))
		return
	}

	topicID := c.Param("topic_id")
	res, err := h.service.UpdateLifecycle(c.Request.Context(), directory.UpdateLifecycleInput{
		TopicID:      topicID,
		TargetState:  model.TopicLifecycle(strings.TrimSpace(req.TargetState)),
		ChangeReason: strings.TrimSpace(req.ChangeReason),
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("update lifecycle failed", err))
		return
	}

	dto.ResponseSuccess(c, res)
}
