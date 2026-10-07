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
