package metadata

import (
	"errors"
	"net/http"

	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func respondError(c *gin.Context, err error) {
	if host, _ := c.Get("metadata_host_contract"); host == true {
		switch {
		case errors.Is(err, metasvc.ErrAlreadyExists), errors.Is(err, metasvc.ErrCircularMove), errors.Is(err, metasvc.ErrOptimisticConflict), errors.Is(err, metasvc.ErrHasChildNodes), errors.Is(err, metasvc.ErrTagBound), errors.Is(err, metasvc.ErrTagResourceMismatch), errors.Is(err, metasvc.ErrTagDisabled), errors.Is(err, metasvc.ErrResourceBindingDisabled), errors.Is(err, metasvc.ErrResourceValidatorMissing):
			dto.RespondErrorFrom(c, metasvc.HostConflict(err))
		case errors.Is(err, metasvc.ErrResourceTypeMissing), errors.Is(err, gorm.ErrRecordNotFound):
			dto.RespondErrorFrom(c, metasvc.HostNotFound(err))
		case errors.Is(err, metasvc.ErrInvalidMachineIdentifier), errors.Is(err, metasvc.ErrNamespaceModuleMismatch), errors.Is(err, metasvc.ErrMissingRequiredLocale), errors.Is(err, metasvc.ErrInvalidStatus), errors.Is(err, metasvc.ErrInvalidDepth), errors.Is(err, metasvc.ErrInvalidParentReference), errors.Is(err, metasvc.ErrUUIDRequired):
			dto.RespondErrorFrom(c, metasvc.HostInvalidArgument(err))
		default:
			dto.RespondErrorFrom(c, metasvc.HostUpstreamDependency(err))
		}
		return
	}
	switch {
	case errors.Is(err, metasvc.ErrInvalidMachineIdentifier):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeInvalidMachineIdentifier, err)
	case errors.Is(err, metasvc.ErrNamespaceModuleMismatch):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeNamespaceModuleMismatch, err)
	case errors.Is(err, metasvc.ErrAlreadyExists):
		dto.ResponseError(c, http.StatusConflict, metasvc.CodeAlreadyExists, err)
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
	case errors.Is(err, metasvc.ErrInvalidParentReference):
		dto.ResponseError(c, http.StatusBadRequest, metasvc.CodeInvalidParentReference, err)
	default:
		dto.ResponseError(c, http.StatusNotImplemented, metasvc.CodeNotImplemented, err)
	}
}
