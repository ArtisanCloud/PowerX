package notifications

import (
	"encoding/json"
	"io"
	"net/http"

	svc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) lookupCreationReceipt(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	var input svc.CreationReceiptLookupInput
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if len(c.Request.URL.Query()) != 0 || decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		dto.ResponseError(c, 400, "NOTIFICATION_CREATION_LOOKUP_INVALID", nil)
		return
	}
	result, err := h.svc.LookupCreationReceipt(c.Request.Context(), input)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
