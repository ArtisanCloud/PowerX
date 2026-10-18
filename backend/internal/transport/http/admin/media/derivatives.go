package media

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) ImageDerivatives(c *gin.Context)       { h.imageDerivatives(c, false) }
func (h *Handler) EnsureImageDerivatives(c *gin.Context) { h.imageDerivatives(c, true) }

func (h *Handler) imageDerivatives(c *gin.Context, ensure bool) {
	tenant, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaUnauthorizedError(err))
		return
	}
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil || id == uuid.Nil {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("asset uuid required")))
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("query parameters are not supported")))
		return
	}
	var req struct {
		RetryFailed bool `json:"retry_failed"`
	}
	if ensure && c.Request.Body != nil {
		d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
		d.DisallowUnknownFields()
		err = d.Decode(&req)
		if err != nil && err != io.EOF {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(err))
			return
		}
		if err == nil && d.Decode(&struct{}{}) != io.EOF {
			dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("invalid request body")))
			return
		}
	}
	var result *mediasvc.ImageDerivatives
	if ensure {
		result, err = h.svc.EnsureImageDerivatives(c.Request.Context(), tenant, id.String(), req.RetryFailed)
	} else {
		result, err = h.svc.ImageDerivativeStatus(c.Request.Context(), tenant, id.String())
	}
	if errors.Is(err, mediasvc.ErrAssetNotFound) {
		err = mediasvc.MediaAssetNotFoundError(err)
	}
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
