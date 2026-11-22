package storage

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"

	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func datasetExecutionRequest(c *gin.Context, body any) bool {
	if c.Request.URL.RawQuery != "" {
		fail(c, 400, "STORAGE_QUERY_FORBIDDEN", "数据集执行不接受范围覆盖参数")
		return false
	}
	if body == nil {
		return true
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		fail(c, 415, "STORAGE_JSON_REQUIRED", "数据集执行请求必须使用 application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(body) != nil || dec.Decode(&struct{}{}) != io.EOF {
		fail(c, 400, "DATASET_EXECUTION_INVALID", "数据集执行参数无效")
		return false
	}
	return true
}

func (h *handler) getDatasetPolicy(c *gin.Context) {
	if !datasetExecutionRequest(c, nil) {
		return
	}
	out, err := h.service.DatasetPolicy(c.Request.Context(), c.Param("dataset_id"), c.Param("tenant_uuid"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) changeDatasetPolicy(c *gin.Context) {
	var in storagegov.DatasetPolicyChange
	if !datasetExecutionRequest(c, &in) {
		return
	}
	ctx := c.Request.Context()
	out, err := h.service.ChangeDatasetPolicy(ctx, reqctx.GetUserID(ctx), c.Param("dataset_id"), c.Param("tenant_uuid"), in)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) createDatasetExecution(c *gin.Context) {
	var in storagegov.DatasetRunInput
	if !datasetExecutionRequest(c, &in) {
		return
	}
	ctx := c.Request.Context()
	out, created, err := h.service.StartDatasetExecution(ctx, reqctx.GetUserID(ctx), c.Param("dataset_id"), in)
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

func (h *handler) getDatasetExecution(c *gin.Context) {
	if !datasetExecutionRequest(c, nil) {
		return
	}
	ctx := c.Request.Context()
	out, err := h.service.DatasetExecution(ctx, reqctx.GetUserID(ctx), c.Param("dataset_id"), c.Param("request_uuid"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) cancelDatasetExecution(c *gin.Context) {
	var input struct{}
	if !datasetExecutionRequest(c, &input) {
		return
	}
	ctx := c.Request.Context()
	out, err := h.service.CancelDatasetExecution(ctx, reqctx.GetUserID(ctx), c.Param("dataset_id"), c.Param("request_uuid"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) downloadDatasetArchive(c *gin.Context) {
	if !datasetExecutionRequest(c, nil) {
		return
	}
	ctx := c.Request.Context()
	request := c.Param("request_uuid")
	raw, err := h.service.DatasetArchive(ctx, reqctx.GetUserID(ctx), c.Param("dataset_id"), request)
	if err != nil {
		h.stateError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="dataset-`+request+`.jsonl"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, "application/x-ndjson", raw)
}
