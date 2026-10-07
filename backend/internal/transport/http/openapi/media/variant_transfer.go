package media

import (
	"encoding/json"
	"errors"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *hostContractHandler) variantPresignUpload(c *gin.Context) { h.variantTransfer(c, "upload") }
func (h *hostContractHandler) variantPresignDownload(c *gin.Context) {
	h.variantTransfer(c, "download")
}
func (h *hostContractHandler) variantComplete(c *gin.Context) { h.variantTransfer(c, "complete") }
func (h *hostContractHandler) variantTransfer(c *gin.Context, action string) {
	if !hostRejectTenantOverride(c) {
		return
	}
	if len(c.Request.URL.Query()) > 0 {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("media.query_not_allowed")))
		return
	}
	asset, err := hostUUID(c.Param("asset_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	variant, err := hostUUID(c.Param("variant_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	tenant, err := h.access.AuthorizeTransfer(c.Request.Context(), hostAPIKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var req struct {
		Expires  *int64 `json:"expires_in_seconds"`
		Checksum string `json:"checksum"`
	}
	allowed := []string{"expires_in_seconds"}
	if action == "complete" {
		allowed = []string{"checksum"}
	}
	if err = decodeVariantBody(c, &req, allowed...); err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
		return
	}
	if action == "complete" {
		if req.Expires != nil {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("media.invalid_body")))
			return
		}
		result, err := h.svc.CompleteVariantUpload(c.Request.Context(), tenant, asset, variant, req.Checksum)
		if err != nil {
			dto.RespondErrorFrom(c, hostMediaError(err))
			return
		}
		dto.ResponseSuccess(c, gin.H{"asset_uuid": result.AssetUUID, "variant_uuid": result.UUID, "status": result.UploadState, "completed_at": result.CompletedAt})
		return
	}
	if req.Checksum != "" {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("media.invalid_body")))
		return
	}
	expires := int64(900)
	if req.Expires != nil {
		expires = *req.Expires
	}
	if expires < 60 || expires > 3600 {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("media.invalid_expiry")))
		return
	}
	ticket, err := h.svc.IssueVariantTransferTicket(c.Request.Context(), tenant, asset, variant, action, time.Duration(expires)*time.Second)
	if err != nil {
		dto.RespondErrorFrom(c, hostMediaError(err))
		return
	}
	method := http.MethodGet
	headers := map[string]string{}
	if action == "upload" {
		method = http.MethodPut
		headers["Content-Type"] = ticket.MimeType
		headers["Content-Length"] = strconv.FormatInt(ticket.SizeBytes, 10)
	}
	url := "/api/v1/media/transfers/" + asset + "/variants/" + variant + "/" + action + "?exp=" + strconv.FormatInt(ticket.ExpiresAt.Unix(), 10) + "&version=" + strconv.FormatUint(ticket.Version, 10) + "&parent_version=" + strconv.FormatUint(ticket.ParentVersion, 10) + "&ticket=" + ticket.Token
	dto.ResponseSuccess(c, gin.H{"url": url, "method": method, "expires_at": ticket.ExpiresAt, "headers": headers})
}

func serveVariantTransfer(svc *mediasvc.MediaService, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !hostRejectTenantOverride(c) {
			return
		}
		for key, values := range c.Request.URL.Query() {
			if (key != "exp" && key != "version" && key != "parent_version" && key != "ticket") || len(values) != 1 {
				dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("media.invalid_query")))
				return
			}
		}
		object, err := svc.ConsumeVariantTransfer(c.Request.Context(), c.Param("asset_uuid"), c.Param("variant_uuid"), action, c.Query("exp"), c.Query("version"), c.Query("parent_version"), c.Query("ticket"), c.Request.Body, c.Request.ContentLength, strings.TrimSpace(c.GetHeader("Content-Type")))
		if err != nil {
			dto.RespondErrorFrom(c, hostMediaError(err))
			return
		}
		if action == "upload" {
			c.Status(http.StatusNoContent)
			return
		}
		defer object.Body.Close()
		c.DataFromReader(http.StatusOK, object.Size, object.ContentType, object.Body, nil)
	}
}

// Validate literal keys before decoding: encoding/json alone accepts duplicate
// keys, case-insensitive field aliases, and null in place of an object.
func decodeVariantBody(c *gin.Context, target any, allowed ...string) error {
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("media.invalid_body")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("media.invalid_body")
		}
		valid := false
		for _, candidate := range allowed {
			if key == candidate {
				valid = true
			}
		}
		if !valid || fields[key] != nil {
			return errors.New("media.invalid_field")
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return err
		}
		if string(value) == "null" {
			return errors.New("media.invalid_field")
		}
		fields[key] = value
	}
	if _, err = d.Token(); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("media.invalid_body")
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, target); err != nil {
		return err
	}
	return binding.Validator.ValidateStruct(target)
}
