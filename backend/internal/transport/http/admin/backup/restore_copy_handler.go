package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// RequireRestoreRoot 与普通备份策略权限分离；整库副本不能由插件或租户管理员调用。
func RequireRestoreRoot() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := reqctx.GetClaims(c.Request.Context())
		service := false
		if claims != nil {
			service = claims.Issuer == "powerx-sts"
			for _, plane := range claims.Platforms {
				if plane == "api_key" {
					service = true
				}
			}
		}
		if !reqctx.IsRoot(c.Request.Context()) || service {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(403, "RESTORE_ROOT_REQUIRED", "完整数据库恢复仅允许平台 Root 用户会话", nil))
			c.Abort()
			return
		}
		c.Next()
	}
}

type restoreCopyRequest struct {
	SourceJobID      string `json:"source_job_id,omitempty"`
	ImportID         string `json:"import_id,omitempty"`
	AllowOlderServer bool   `json:"allow_older_server,omitempty"`
}

func bindRestoreJSON(c *gin.Context, target any) bool {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_QUERY_FORBIDDEN", "恢复接口不接受 query 覆盖", nil))
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	raw = bytes.TrimSpace(raw)
	if err != nil || len(raw) == 0 || raw[0] != '{' {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_REQUEST_INVALID", "恢复请求必须是 JSON 对象", nil))
		return false
	}
	reader := json.NewDecoder(bytes.NewReader(raw))
	reader.DisallowUnknownFields()
	if err := reader.Decode(target); err != nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_REQUEST_INVALID", "恢复请求格式错误", nil))
		return false
	}
	if reader.Decode(&struct{}{}) != io.EOF {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_REQUEST_INVALID", "恢复请求只能包含一个 JSON 对象", nil))
		return false
	}
	return true
}

func (h *handler) copyAvailable(c *gin.Context) bool {
	if h.copySvc == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "RESTORE_SERVICE_UNAVAILABLE", "恢复服务未加载，请重启新版本 Core", nil))
		return false
	}
	return true
}

func copySourceRequest(c *gin.Context) (backupops.RestoreSource, restoreCopyRequest, bool) {
	var req restoreCopyRequest
	if !bindRestoreJSON(c, &req) {
		return backupops.RestoreSource{}, req, false
	}
	var id uint64
	var err error
	if req.SourceJobID != "" {
		id, err = strconv.ParseUint(req.SourceJobID, 10, 64)
		if err != nil || id == 0 {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_SOURCE_INVALID", "备份作业标识无效", nil))
			return backupops.RestoreSource{}, req, false
		}
	}
	return backupops.RestoreSource{SourceJobID: id, ImportID: req.ImportID}, req, true
}

func (h *handler) ImportRestoreCopy(c *gin.Context) {
	if !h.copyAvailable(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, backupops.RestoreUploadLimit+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(413, "RESTORE_IMPORT_SIZE", "归档超过 512 MiB 上传上限", nil))
		} else {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_IMPORT_INVALID", "归档上传格式错误", nil))
		}
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	if len(c.Request.MultipartForm.Value) > 0 || len(c.Request.MultipartForm.File) != 1 || len(c.Request.MultipartForm.File["file"]) != 1 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_IMPORT_INVALID", "只允许上传一个 file 字段", nil))
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_IMPORT_INVALID", "缺少 dump 文件", nil))
		return
	}
	defer file.Close()
	result, err := h.copySvc.Import(c.Request.Context(), header.Filename, file)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"import": result})
}

func (h *handler) PlanRestoreCopy(c *gin.Context) {
	if !h.copyAvailable(c) {
		return
	}
	source, _, ok := copySourceRequest(c)
	if !ok {
		return
	}
	plan, err := h.copySvc.Plan(c.Request.Context(), source)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"plan": plan})
}

func (h *handler) CreateRestoreCopy(c *gin.Context) {
	if !h.copyAvailable(c) {
		return
	}
	source, req, ok := copySourceRequest(c)
	if !ok {
		return
	}
	row, err := h.copySvc.Create(c.Request.Context(), backupops.CreateRestoreCopy{RestoreSource: source, IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")), AllowOlderServer: req.AllowOlderServer, Operator: resolveOperator(c), TraceID: reqctx.GetTraceID(c.Request.Context())})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"copy": row})
}

func (h *handler) ListRestoreCopies(c *gin.Context) {
	if !h.copyAvailable(c) {
		return
	}
	rows, err := h.copySvc.List(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": rows})
}

func (h *handler) GetRestoreCopy(c *gin.Context) {
	if !h.copyAvailable(c) {
		return
	}
	row, err := h.copySvc.Get(c.Request.Context(), c.Param("copy_id"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"copy": row})
}

func (h *handler) RestoreCopyConnection(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if !h.copyAvailable(c) {
		return
	}
	var req struct{}
	if !bindRestoreJSON(c, &req) {
		return
	}
	connection, err := h.copySvc.Connection(c.Request.Context(), c.Param("copy_id"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"connection": connection})
}

func (h *handler) DeleteRestoreCopy(c *gin.Context) {
	if !h.copyAvailable(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 || c.Request.ContentLength > 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RESTORE_REQUEST_INVALID", "删除确认仅接受 X-Restore-Confirmation 请求头", nil))
		return
	}
	if err := h.copySvc.Delete(c.Request.Context(), c.Param("copy_id"), c.GetHeader("X-Restore-Confirmation")); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"deleted": true})
}
