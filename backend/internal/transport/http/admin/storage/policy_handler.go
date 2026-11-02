package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
)

func (h *handler) stateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, lifecycle.ErrBlocked), errors.Is(err, budget.ErrInsufficient):
		fail(c, 409, "STORAGE_RUNTIME_BLOCKED", "来源、空间、归档或旧执行器条件未满足，请核对运行状态")
	case errors.Is(err, lifecycle.ErrConflict), errors.Is(err, budget.ErrBusy):
		fail(c, 409, "STORAGE_RUNTIME_CONFLICT", "主机状态忙或版本变化，请读回后重试")
	case errors.Is(err, lifecycle.ErrInvalid), errors.Is(err, budget.ErrInvalid):
		fail(c, 400, "STORAGE_RUNTIME_INVALID", "运行策略字段或来源无效")
	case errors.Is(err, storagegov.ErrCleanupBlocked):
		fail(c, 409, "STORAGE_CLEANUP_BLOCKED", "当前来源或保护条件不允许执行清理，请查看预览说明")
	case errors.Is(err, storagegov.ErrCleanupConflict):
		fail(c, 409, "STORAGE_CLEANUP_PREVIEW_CHANGED", "预览已过期、策略已变化或本批已执行，请重新读取记录")
	case errors.Is(err, storagegov.ErrCleanupNotFound):
		fail(c, 404, "STORAGE_CLEANUP_NOT_FOUND", "清理记录不存在")
	case errors.Is(err, storagegov.ErrPolicyInvalid):
		fail(c, 400, "STORAGE_POLICY_INVALID", "策略草稿字段、来源或主机身份无效")
	case errors.Is(err, storagegov.ErrPolicyConflict):
		fail(c, 409, "STORAGE_POLICY_REVISION_CONFLICT", "策略已被修改，请重新读取并核对草稿")
	case errors.Is(err, storagegov.ErrRootRequired):
		fail(c, 403, "STORAGE_ROOT_REQUIRED", "策略修改需要平台 Root 用户会话")
	default:
		fail(c, 503, "STORAGE_STATE_UNAVAILABLE", "观测存储未就绪，请检查迁移、部署身份和数据库连接")
	}
}

func (h *handler) policy(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, err := h.service.Policy(c.Request.Context())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) savePolicy(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		fail(c, 415, "STORAGE_JSON_REQUIRED", "策略草稿必须使用 application/json")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32768))
	raw = bytes.TrimSpace(raw)
	if err != nil || len(raw) == 0 || raw[0] != '{' {
		h.stateError(c, storagegov.ErrPolicyInvalid)
		return
	}
	if !policyFieldsPresent(raw) {
		h.stateError(c, storagegov.ErrPolicyInvalid)
		return
	}
	var in storagegov.PolicyInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		h.stateError(c, storagegov.ErrPolicyInvalid)
		return
	}
	out, err := h.service.SavePolicy(c.Request.Context(), reqctx.GetClaims(c.Request.Context()).UserID, in)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

// 零 revision 和 false 保护选项是合法的显式值；不能把缺字段／null 当成这些值。
func policyFieldsPresent(raw []byte) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	has := func(obj map[string]json.RawMessage, keys ...string) bool {
		for _, key := range keys {
			value := bytes.TrimSpace(obj[key])
			if len(value) == 0 || bytes.Equal(value, []byte("null")) {
				return false
			}
		}
		return true
	}
	if !has(object, "schema_version", "expected_revision", "state", "budget", "resources") {
		return false
	}
	var resources []map[string]json.RawMessage
	if json.Unmarshal(object["resources"], &resources) != nil {
		return false
	}
	for _, resource := range resources {
		if raw, present := resource["retention_mode"]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
		if !has(resource, "resource_id", "target_days", "max_bytes", "retention_count", "min_verified_points", "protect_latest", "require_verified_archive") {
			return false
		}
	}
	return true
}

func (h *handler) history(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, err := h.service.History(c.Request.Context())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
