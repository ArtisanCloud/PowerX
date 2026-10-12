package knowledge_space

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *hostContractHandler) semanticQuery(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	tenant, err := h.access.AuthorizeRetrievalRead(c.Request.Context(), apiKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var input ksvc.SemanticQuery
	if err := decodeSemanticBody(c, &input); err != nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(err))
		return
	}
	if h.service.Semantic() == nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeUpstreamDependencyError(errors.New("semantic runtime unavailable")))
		return
	}
	result, err := h.service.Semantic().Query(c.Request.Context(), tenant, input)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *hostContractHandler) configureSemantic(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	tenant, err := h.authorizeDocument(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	space, err := requiredUUID(c.Param("space_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(err))
		return
	}
	var input ksvc.SemanticConfigureInput
	if err := decodeSemanticBody(c, &input); err != nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(err))
		return
	}
	if h.service.Semantic() == nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeUpstreamDependencyError(errors.New("semantic runtime unavailable")))
		return
	}
	result, err := h.service.Semantic().Configure(c.Request.Context(), tenant, space, input)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusOK, result)
}

func (h *hostContractHandler) semanticCapabilities(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	tenant, err := h.access.AuthorizeRetrievalRead(c.Request.Context(), apiKeyHash(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	space, err := requiredUUID(c.Param("space_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(err))
		return
	}
	if h.service.Semantic() == nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeUpstreamDependencyError(errors.New("semantic runtime unavailable")))
		return
	}
	result, err := h.service.Semantic().Capabilities(c.Request.Context(), tenant, space)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func decodeSemanticBody(c *gin.Context, out any) error {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var tail any
	if err := decoder.Decode(&tail); err != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}

func (h *hostContractHandler) setSemanticVisibility(c *gin.Context) {
	if !rejectTenantOverride(c) {
		return
	}
	tenant, err := h.authorizeDocument(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var input struct {
		Queryable     *bool  `json:"queryable"`
		ExpectedEpoch string `json:"expected_epoch,omitempty"`
	}
	if err := decodeSemanticBody(c, &input); err != nil || input.Queryable == nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(errors.New("queryable must be supplied")))
		return
	}
	result, err := h.service.SetDocumentVisibility(c.Request.Context(), tenant, c.Param("space_uuid"), c.Param("document_uuid"), ksvc.SemanticVisibilityInput{Queryable: *input.Queryable, ExpectedEpoch: input.ExpectedEpoch})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
