package storage

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func requireDatasetRootSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !auth.IsPlatformRootSession(c.Request.Context()) {
		fail(c, 403, "STORAGE_ROOT_REQUIRED", "存储治理仅允许平台 Root 用户会话")
		c.Abort()
		return
	}
	c.Next()
}

func (h *handler) listDatasets(c *gin.Context) {
	if h.service == nil {
		fail(c, 503, "STORAGE_OBSERVER_UNAVAILABLE", "存储观测未就绪")
		return
	}
	filter := storagegov.DatasetFilter{}
	query, ok := parseDatasetQuery(c)
	if !ok {
		fail(c, 400, "DATASET_FILTER_INVALID", "数据集筛选条件无效")
		return
	}
	filter.OwnerKind, filter.PluginID = query.Get("owner_kind"), query.Get("plugin_id")
	filter.AvailableAction, filter.CapacityLevel, filter.Cursor = query.Get("available_action"), query.Get("capacity_level"), query.Get("cursor")
	if v := query.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			fail(c, 400, "DATASET_FILTER_INVALID", "数据集筛选条件无效")
			return
		}
		filter.Limit = n
	}
	page, e := h.service.ListDatasets(c.Request.Context(), filter)
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, page)
}

func (h *handler) getDataset(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, 400, "STORAGE_QUERY_FORBIDDEN", "存储观测不接受路径或范围覆盖参数")
		return
	}
	if h.service == nil {
		fail(c, 503, "STORAGE_OBSERVER_UNAVAILABLE", "存储观测未就绪")
		return
	}
	item, err := h.service.DatasetDetails(c.Request.Context(), c.Param("dataset_id"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, item)
}

func parseDatasetQuery(c *gin.Context) (url.Values, bool) {
	if len(c.Request.URL.RawQuery) > 4096 {
		return nil, false
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return nil, false
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" {
			return nil, false
		}
		switch key {
		case "owner_kind", "plugin_id", "available_action", "capacity_level", "cursor", "limit":
		default:
			return nil, false
		}
	}
	return query, true
}

func (h *handler) createDatasetPreview(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, 400, "STORAGE_QUERY_FORBIDDEN", "预览不接受范围覆盖参数")
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		fail(c, 415, "STORAGE_JSON_REQUIRED", "预览请求必须使用 application/json")
		return
	}
	var input storagegov.DatasetPreviewInput
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&input) != nil || dec.Decode(&struct{}{}) != io.EOF {
		fail(c, 400, "DATASET_PREVIEW_INVALID", "预览参数无效")
		return
	}
	ctx := c.Request.Context()
	out, created, err := h.service.StartDatasetPreview(ctx, reqctx.GetClaims(ctx).UserID, c.Param("dataset_id"), input)
	if err != nil {
		h.stateError(c, err)
		return
	}
	if created {
		dto.ResponseSuccessWithStatus(c, 202, out)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) getDatasetPreview(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, 400, "STORAGE_QUERY_FORBIDDEN", "预览不接受范围覆盖参数")
		return
	}
	ctx := c.Request.Context()
	out, err := h.service.GetDatasetPreview(ctx, reqctx.GetClaims(ctx).UserID, c.Param("dataset_id"), c.Param("request_uuid"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
