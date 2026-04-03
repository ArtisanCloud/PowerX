package skills

import (
	"net/http"
	"strings"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func newMarketplaceHandler(importSvc *skillservice.ImportService) *marketplaceHandler {
	if importSvc == nil {
		return nil
	}
	return &marketplaceHandler{importSvc: importSvc}
}

func (h *marketplaceHandler) Preview(c *gin.Context) {
	req := skillservice.ImportRequest{
		SkillID:    strings.TrimSpace(c.Query("skill_id")),
		SourceURL:  strings.TrimSpace(c.Query("source_url")),
		SourceRef:  strings.TrimSpace(c.Query("source_ref")),
		SourcePath: strings.TrimSpace(c.Query("source_path")),
	}
	preview, err := h.importSvc.PreviewMarketplace(c.Request.Context(), req)
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusOK, preview)
}
