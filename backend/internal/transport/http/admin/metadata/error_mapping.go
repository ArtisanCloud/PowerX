package metadata

import (
	"errors"
	"net/http"

	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, metasvc.ErrInvalidMachineIdentifier):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeInvalidMachineIdentifier, err)
	case errors.Is(err, metasvc.ErrMissingRequiredLocale):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeMissingRequiredLocale, err)
	case errors.Is(err, metasvc.ErrInvalidStatus):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeInvalidStatus, err)
	case errors.Is(err, metasvc.ErrInvalidDepth):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeInvalidDepth, err)
	case errors.Is(err, metasvc.ErrCircularMove):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeCircularMove, err)
	case errors.Is(err, metasvc.ErrOptimisticConflict):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeOptimisticConflict, err)
	case errors.Is(err, metasvc.ErrHasChildNodes):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeHasChildNodes, err)
	case errors.Is(err, metasvc.ErrTagBound):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeTagBound, err)
	case errors.Is(err, metasvc.ErrTagResourceMismatch):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeTagResourceMismatch, err)
	case errors.Is(err, metasvc.ErrReferenceResourceMismatch):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeReferenceResourceMismatch, err)
	case errors.Is(err, metasvc.ErrTagDisabled):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeTagDisabled, err)
	case errors.Is(err, metasvc.ErrMergeSameTag):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeMergeSameTag, err)
	case errors.Is(err, metasvc.ErrResourceTypeMissing):
		dto.ResponseError(c, http.StatusNotFound, metasvc.CodeResourceTypeMissing, err)
	case errors.Is(err, metasvc.ErrResourceBindingDisabled):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeResourceBindingDisabled, err)
	case errors.Is(err, metasvc.ErrResourceValidatorMissing):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeResourceValidatorMissing, err)
	default:
		dto.ResponseError(c, http.StatusNotImplemented, metasvc.CodeNotImplemented, err)
	}
}
