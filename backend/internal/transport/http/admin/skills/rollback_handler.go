package skills

import (
	"net/http"
	"strings"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type rollbackRequest struct {
	TargetVersion string `json:"target_version"`
	Reason        string `json:"reason"`
}

func newRollbackHandler(repo *skillrepo.SkillRegistryRepository, lifecycleSvc *skillservice.LifecycleService) *rollbackHandler {
	if repo == nil || lifecycleSvc == nil {
		return nil
	}
	return &rollbackHandler{
		registryRepo: repo,
		lifecycleSvc: lifecycleSvc,
	}
}

func (h *rollbackHandler) Rollback(c *gin.Context) {
	skillID := strings.TrimSpace(c.Param("skillId"))
	var req rollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	actor := actorFromContext(c)
	if err := h.lifecycleSvc.Rollback(c.Request.Context(), skillID, req.TargetVersion, actor, req.Reason); err != nil {
		respondSkillError(c, err)
		return
	}
	record, err := h.registryRepo.GetBySkillVersion(c.Request.Context(), skillID, req.TargetVersion)
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccess(c, mapSkillRecord(record))
}
