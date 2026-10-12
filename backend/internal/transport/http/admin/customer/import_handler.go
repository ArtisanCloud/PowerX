package customer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type customerImportPreviewRequest struct {
	FileName string `json:"file_name"`
	CSV      string `json:"csv"`
}
type customerImportCommitRequest struct {
	Checksum string `json:"checksum"`
	Confirm  bool   `json:"confirm"`
}

func (h *Handler) importAvailable(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if h.imports == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "CUSTOMER_IMPORT_UNAVAILABLE", "客户导入服务未加载", nil))
		return false
	}
	if c.Request.URL.RawQuery != "" {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "CUSTOMER_IMPORT_REQUEST_INVALID", "导入请求不接受查询参数或租户覆盖", nil))
		return false
	}
	return true
}

func bindCustomerImport(c *gin.Context, request any) bool {
	return bindCustomerAdminJSON(c, request, 2*customersvc.CustomerImportMaxBytes, "CUSTOMER_IMPORT_REQUEST_INVALID")
}

func bindCustomerAdminJSON(c *gin.Context, request any, limit int64, code string) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			if code == "CUSTOMER_PROFILE_REQUEST_INVALID" {
				dto.RespondErrorFrom(c, dto.NewErrorWithCode(413, "CUSTOMER_PROFILE_TOO_LARGE", "客户资料请求超过大小限制", nil))
				return false
			}
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(413, "CUSTOMER_IMPORT_TOO_LARGE", "导入请求超过大小限制", nil))
			return false
		}
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, code, "客户请求 JSON 无效或包含不支持字段", nil))
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, code, "客户请求只能包含一个 JSON 对象", nil))
		return false
	}
	return true
}

func (h *Handler) PreviewAccountImport(c *gin.Context) {
	if !h.importAvailable(c) {
		return
	}
	var input customerImportPreviewRequest
	if !bindCustomerImport(c, &input) {
		return
	}
	result, err := h.imports.Preview(c.Request.Context(), input.FileName, input.CSV)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, 201, gin.H{"payload": result})
}

func (h *Handler) GetAccountImport(c *gin.Context) {
	if !h.importAvailable(c) {
		return
	}
	result, err := h.imports.Get(c.Request.Context(), c.Param("import_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": result})
}

func (h *Handler) CommitAccountImport(c *gin.Context) {
	if !h.importAvailable(c) {
		return
	}
	var input customerImportCommitRequest
	if !bindCustomerImport(c, &input) {
		return
	}
	result, err := h.imports.Commit(c.Request.Context(), c.Param("import_uuid"), input.Checksum, input.Confirm)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": result})
}
