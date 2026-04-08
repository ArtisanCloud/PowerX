package skills

import (
	"net/http"
	"strings"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type publishRequest struct {
	Version      string `json:"version"`
	ApprovalNote string `json:"approval_note"`
}

func newPublishHandler(repo *skillrepo.SkillRegistryRepository, lifecycleSvc *skillservice.LifecycleService) *publishHandler {
	if repo == nil || lifecycleSvc == nil {
		return nil
	}
	return &publishHandler{
		registryRepo: repo,
		lifecycleSvc: lifecycleSvc,
	}
}

func (h *publishHandler) Publish(c *gin.Context) {
	skillID := strings.TrimSpace(c.Param("skillId"))
	var req publishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	actor := actorFromContext(c)
	if err := h.lifecycleSvc.Publish(c.Request.Context(), skillID, req.Version, actor, req.ApprovalNote); err != nil {
		respondSkillError(c, err)
		return
	}
	record, err := h.registryRepo.GetBySkillVersion(c.Request.Context(), skillID, req.Version)
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccess(c, mapSkillRecord(record))
}
