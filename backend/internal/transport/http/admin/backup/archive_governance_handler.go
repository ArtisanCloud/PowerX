package backup

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) SaveArchiveS3Credential(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in archiveops.SaveS3CredentialInput
	if !bindCredentialJSON(c, &in) {
		return
	}
	out, err := h.archives.SaveS3Credential(c.Request.Context(), in)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ImportArchiveLegacy(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !bindRestoreJSON(c, &in) {
		return
	}
	if !in.Confirm {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CONFIRM_REQUIRED", "请明确确认只读纳管历史归档", nil))
		return
	}
	out, err := h.archives.ImportLegacy(c.Request.Context(), c.Param("target_id"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ArchiveCatalog(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		page = 0
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil {
		size = 0
	}
	in := archiveops.CatalogFilter{Page: page, PageSize: size, Query: c.Query("q"), SourceID: c.Query("source_id")}
	for key, dest := range map[string]*time.Time{"from": &in.From, "to": &in.To} {
		if raw := c.Query(key); raw != "" {
			value, e := time.Parse(time.RFC3339, raw)
			if e != nil {
				dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_FILTER_INVALID", "请使用带时区的时间范围", nil))
				return
			}
			*dest = value
		}
	}
	out, err := h.archives.Catalog(c.Request.Context(), in)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) PreviewArchiveLog(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	out, err := h.archives.PreviewLog(c.Request.Context(), c.Param("job_id"), c.Query("entry"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	dto.ResponseSuccess(c, out)
}

func (h *handler) PreviewArchiveRemoteGC(c *gin.Context) {
	if !h.archiveAvailable(c) || !emptyArchiveBody(c) {
		return
	}
	out, err := h.archives.PreviewRemoteGC(c.Request.Context(), c.Param("policy_id"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) StartArchiveRemoteGC(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in struct {
		Confirmation string `json:"confirmation"`
	}
	if !bindRestoreJSON(c, &in) {
		return
	}
	if in.Confirmation != c.Param("preview_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CONFIRM_REQUIRED", "请核对预览范围后确认远端版本删除", nil))
		return
	}
	out, err := h.archives.StartRemoteGC(c.Request.Context(), in.Confirmation)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, 202, out)
}

func (h *handler) ArchiveCredentialOverview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.CredentialOverview(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ArchiveCredentialRequest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.CredentialRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) AdoptArchiveCredential(c *gin.Context) { h.controlArchiveCredential(c, "adoptions") }
func (h *handler) RevokeArchiveCredential(c *gin.Context) {
	h.controlArchiveCredential(c, "revocations")
}
func (h *handler) controlArchiveCredential(c *gin.Context, operation string) {
	if !h.archiveAvailable(c) {
		return
	}
	var in archiveops.CredentialControlInput
	if !bindCredentialJSON(c, &in) {
		return
	}
	var out archiveops.CredentialMutation
	var e error
	switch operation {
	case "adoptions":
		out, e = h.archives.AdoptS3Credential(c.Request.Context(), c.Param("credential_ref"), in)
	case "revocations":
		out, e = h.archives.RevokeS3Credential(c.Request.Context(), c.Param("credential_ref"), in)
	default:
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CREDENTIAL_OPERATION_INVALID", "不支持的凭据操作", nil))
		return
	}
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) RotateArchiveCredentialKey(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in archiveops.CredentialControlInput
	if !bindCredentialJSON(c, &in) {
		return
	}
	out, e := h.archives.RotateCredentialKey(c.Request.Context(), in)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}

// bindCredentialJSON 对敏感写入使用专用的有界严格 DTO，不影响恢复接口的 8 KiB 限制。
func bindCredentialJSON(c *gin.Context, target any) bool {
	c.Header("Cache-Control", "no-store")
	invalid := func() bool {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CREDENTIAL_REQUEST_INVALID", "请提供明确的凭据版本、请求编号和固定字段", nil))
		return false
	}
	if len(c.Request.URL.Query()) != 0 {
		return invalid()
	}
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10))
	raw = bytes.TrimSpace(raw)
	if e != nil || len(raw) == 0 || raw[0] != '{' {
		return invalid()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return invalid()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return invalid()
	}
	r, ok := fields["expected_revision"]
	if !ok || bytes.Equal(bytes.TrimSpace(r), []byte("null")) {
		return invalid()
	}
	return true
}
