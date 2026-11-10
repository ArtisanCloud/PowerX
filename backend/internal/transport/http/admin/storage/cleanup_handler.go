package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func cleanupBody(c *gin.Context, in any) bool {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		fail(c, 415, "STORAGE_JSON_REQUIRED", "请求需要 application/json")
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	raw = bytes.TrimSpace(raw)
	if err != nil || len(raw) == 0 || raw[0] != '{' {
		fail(c, 400, "STORAGE_CLEANUP_INVALID", "清理请求无效")
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(in) != nil || !errors.Is(dec.Decode(&struct{}{}), io.EOF) {
		fail(c, 400, "STORAGE_CLEANUP_INVALID", "清理请求字段或格式无效")
		return false
	}
	return true
}
func (h *handler) previewCleanup(c *gin.Context) {
	if h.service == nil {
		fail(c, 503, "STORAGE_STATE_UNAVAILABLE", "存储治理未就绪")
		return
	}
	var in struct {
		ResourceID string `json:"resource_id"`
		TargetDays int    `json:"target_days"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	out, err := h.service.PreviewCleanup(c.Request.Context(), in.ResourceID, in.TargetDays)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) executeCleanup(c *gin.Context) {
	if h.service == nil {
		fail(c, 503, "STORAGE_STATE_UNAVAILABLE", "存储治理未就绪")
		return
	}
	var in struct {
		PreviewID string `json:"preview_id"`
		Confirm   bool   `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm {
		fail(c, 400, "STORAGE_CLEANUP_CONFIRM_REQUIRED", "请先预览并明确确认本批清理")
		return
	}
	out, err := h.service.ExecuteCleanup(c.Request.Context(), in.PreviewID)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) cleanupRuns(c *gin.Context) {
	if h.service == nil {
		fail(c, 503, "STORAGE_STATE_UNAVAILABLE", "存储治理未就绪")
		return
	}
	out, err := h.service.CleanupRuns(c.Request.Context())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, map[string]any{"items": out})
}
func (h *handler) cleanupRun(c *gin.Context) {
	if h.service == nil {
		fail(c, 503, "STORAGE_STATE_UNAVAILABLE", "存储治理未就绪")
		return
	}
	out, err := h.service.CleanupRun(c.Request.Context(), c.Param("run_id"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
