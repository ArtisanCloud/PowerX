package knowledge_space

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	knowledgeRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type StrategyHandler struct {
	svc    *ksvc.Service
	spaces *knowledgeRepo.KnowledgeSpaceRepository
}

type strategyValidateRequest struct {
	SceneKey  string `json:"sceneKey"`
	BundleKey string `json:"bundleKey"`
}

func NewStrategyHandler(deps *shared.Deps) *StrategyHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Service == nil {
		return nil
	}
	return &StrategyHandler{
		svc:    deps.KnowledgeSpace.Service,
		spaces: knowledgeRepo.NewKnowledgeSpaceRepository(deps.DB),
	}
}

func (h *StrategyHandler) Validate(c *gin.Context) {
	var req strategyValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	res, err := h.svc.ValidateStrategy(c.Request.Context(), ksvc.ValidateStrategyInput{
		SceneKey:  strings.TrimSpace(req.SceneKey),
		BundleKey: strings.TrimSpace(req.BundleKey),
	})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, res)
}

func (h *StrategyHandler) ValidateForSpace(c *gin.Context) {
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	if h.spaces == nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", nil)
		return
	}
	space, err := h.spaces.FindByUUID(c.Request.Context(), spaceID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
		return
	}
	if space == nil {
		dto.ResponseError(c, http.StatusNotFound, "知识空间不存在", ksvc.ErrSpaceNotFound)
		return
	}
	sceneKey, bundleKey := ksvc.InferSceneAndBundleForSpace(space)
	res, err := h.svc.ValidateStrategy(c.Request.Context(), ksvc.ValidateStrategyInput{SceneKey: sceneKey, BundleKey: bundleKey})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, res)
}
