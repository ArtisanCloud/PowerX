package knowledge_space

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iam "github.com/ArtisanCloud/PowerX/internal/service/iam"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func registerHostDocumentAdmin(protected *gin.RouterGroup, deps *shared.Deps) {
	if deps.KnowledgeSpace == nil || deps.KnowledgeSpace.HostDocuments == nil {
		return
	}
	group := protected.Group("/admin/knowledge-spaces")
	service := deps.KnowledgeSpace.HostDocuments
	rbac := iam.NewRBACService(deps.DB)
	authorize := func(c *gin.Context) (string, bool) {
		tenant, err := reqctx.RequireTenantUUIDFromGin(c)
		if err != nil {
			dto.RespondErrorFrom(c, ksvc.KnowledgeUnauthorizedError(err))
			return "", false
		}
		allowed, err := rbac.Enforce(c.Request.Context(), iam.ActorContext{TenantUUID: tenant, IsRoot: reqctx.IsRoot(c.Request.Context())}, tenant, reqctx.GetMemberID(c.Request.Context()), "corex", "knowledge.document", "manage")
		if err != nil || !allowed {
			dto.RespondErrorFrom(c, ksvc.KnowledgeForbiddenError(errors.New("document admin permission missing")))
			return "", false
		}
		return tenant, true
	}
	readAuthorize := func(c *gin.Context) (string, bool) {
		tenant, err := reqctx.RequireTenantUUIDFromGin(c)
		if err != nil {
			dto.RespondErrorFrom(c, ksvc.KnowledgeUnauthorizedError(err))
			return "", false
		}
		ok, err := rbac.Enforce(c.Request.Context(), iam.ActorContext{TenantUUID: tenant, IsRoot: reqctx.IsRoot(c.Request.Context())}, tenant, reqctx.GetMemberID(c.Request.Context()), "corex", "knowledge.retrieval", "read")
		if err != nil || !ok {
			dto.RespondErrorFrom(c, ksvc.KnowledgeForbiddenError(errors.New("retrieval admin permission missing")))
			return "", false
		}
		return tenant, true
	}
	group.POST("/retrieval/query", func(c *gin.Context) {
		tenant, ok := readAuthorize(c)
		if !ok {
			return
		}
		var input ksvc.SemanticQuery
		if !decodeSemanticAdmin(c, &input) {
			return
		}
		if service.Semantic() == nil {
			dto.RespondErrorFrom(c, ksvc.KnowledgeSemanticUnsupportedError())
			return
		}
		result, err := service.Semantic().Query(c.Request.Context(), tenant, input)
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	})
	group.GET("/:spaceId/semantic-index", func(c *gin.Context) {
		tenant, ok := readAuthorize(c)
		if !ok {
			return
		}
		if service.Semantic() == nil {
			dto.RespondErrorFrom(c, ksvc.KnowledgeSemanticUnsupportedError())
			return
		}
		result, err := service.Semantic().Capabilities(c.Request.Context(), tenant, c.Param("spaceId"))
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	})
	group.POST("/:spaceId/semantic-index", func(c *gin.Context) {
		tenant, ok := authorize(c)
		if !ok {
			return
		}
		var input ksvc.SemanticConfigureInput
		if !decodeSemanticAdmin(c, &input) {
			return
		}
		if service.Semantic() == nil {
			dto.RespondErrorFrom(c, ksvc.KnowledgeSemanticUnsupportedError())
			return
		}
		result, err := service.Semantic().Configure(c.Request.Context(), tenant, c.Param("spaceId"), input)
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	})
	group.PATCH("/:spaceId/documents/:documentId/visibility", func(c *gin.Context) {
		tenant, ok := authorize(c)
		if !ok {
			return
		}
		var input struct {
			Queryable     *bool  `json:"queryable"`
			ExpectedEpoch string `json:"expected_epoch,omitempty"`
		}
		if !decodeSemanticAdmin(c, &input) {
			return
		}
		if input.Queryable == nil {
			dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(errors.New("queryable required")))
			return
		}
		result, err := service.SetDocumentVisibility(c.Request.Context(), tenant, c.Param("spaceId"), c.Param("documentId"), ksvc.SemanticVisibilityInput{Queryable: *input.Queryable, ExpectedEpoch: input.ExpectedEpoch})
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	})
	group.POST("/:spaceId/documents", func(c *gin.Context) {
		tenant, ok := authorize(c)
		if !ok {
			return
		}
		var in ksvc.HostDocumentInput
		decoder := json.NewDecoder(io.LimitReader(c.Request.Body, (8<<20)+65536))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(err))
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(errors.New("expected one JSON object")))
			return
		}
		job, err := service.UpsertDocument(c.Request.Context(), tenant, c.Param("spaceId"), in)
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, job)
	})
	group.POST("/:spaceId/documents/:documentId/indexes:operation", func(c *gin.Context) {
		if c.Param("operation") != ":rebuild" {
			c.Status(404)
			return
		}
		tenant, ok := authorize(c)
		if !ok {
			return
		}
		var in ksvc.HostRebuildInput
		decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil && err != io.EOF {
			dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(err))
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(errors.New("expected one JSON object")))
			return
		}
		job, err := service.RebuildDocument(c.Request.Context(), tenant, c.Param("spaceId"), c.Param("documentId"), in)
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, job)
	})
	group.GET("/index-jobs/:jobId", func(c *gin.Context) {
		tenant, ok := authorize(c)
		if !ok {
			return
		}
		job, err := service.GetIndexJob(c.Request.Context(), tenant, c.Param("jobId"))
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, job)
	})
	group.GET("/index-jobs/:jobId/chunks", func(c *gin.Context) {
		tenant, ok := authorize(c)
		if !ok {
			return
		}
		items, err := service.GetJobChunks(c.Request.Context(), tenant, c.Param("jobId"))
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, gin.H{"items": items})
	})
}

func decodeSemanticAdmin(c *gin.Context, out any) bool {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(err))
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		dto.RespondErrorFrom(c, ksvc.KnowledgeInvalidArgumentError(errors.New("one JSON object required")))
		return false
	}
	return true
}
