package knowledge_space

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type BuiltinHandler struct {
	initializer *ksvc.BuiltinKnowledgeInitializer
}

func NewBuiltinHandler(deps *shared.Deps) *BuiltinHandler {
	if deps == nil || deps.DB == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Service == nil {
		return nil
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return nil
	}
	return &BuiltinHandler{
		initializer: ksvc.NewBuiltinKnowledgeInitializer(ksvc.BuiltinKnowledgeInitializerOptions{
			DB:           deps.DB,
			SpaceService: deps.KnowledgeSpace.Service,
			VectorIndex: ksvc.NewVectorIndexService(ksvc.VectorIndexServiceOptions{
				DB:       deps.DB,
				PGVector: buildPGVectorConfig(cfg),
			}),
		}),
	}
}

type seedBuiltinKnowledgeRequest struct {
	RequestedBy string `json:"requestedBy"`
}

func (h *BuiltinHandler) Seed(c *gin.Context) {
	tenantUUID, ok := tenantUUIDFromContext(c)
	if !ok {
		return
	}
	var req seedBuiltinKnowledgeRequest
	if c.Request != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			dto.ResponseValidationError(c, err)
			return
		}
	}
	if h == nil || h.initializer == nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", ksvc.ErrBuiltinKnowledgeSeedUnavailable)
		return
	}
	result, err := h.initializer.EnsureTenantBuiltinKnowledge(c.Request.Context(), ksvc.BuiltinKnowledgeSeedInput{
		TenantUUID:  tenantUUID.String(),
		RequestedBy: req.RequestedBy,
	})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	spaces := make([]knowledgeSpaceResponse, 0, len(result.Spaces))
	for _, space := range result.Spaces {
		spaces = append(spaces, toResponse(space))
	}
	dto.ResponseSuccess(c, gin.H{
		"spaces":  spaces,
		"created": result.Created,
		"updated": result.Updated,
		"skipped": result.Skipped,
	})
}
