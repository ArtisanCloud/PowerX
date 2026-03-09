package skills

import (
	"net/http"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newCatalogHandler(db *gorm.DB) *catalogHandler {
	if db == nil {
		return nil
	}
	return &catalogHandler{db: db}
}

func (h *catalogHandler) ListCatalog(c *gin.Context) {
	if h == nil || h.db == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "skills catalog unavailable", nil)
		return
	}
	var rows []skillmodel.OfficialSkillCatalogEntry
	if err := h.db.WithContext(c.Request.Context()).
		Where("active = ?", true).
		Order("updated_at DESC").
		Find(&rows).Error; err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "query catalog failed", err)
		return
	}

	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		items = append(items, gin.H{
			"skill_id":            rows[i].SkillID,
			"recommended_version": rows[i].RecommendedVersion,
			"risk_level":          rows[i].RiskLevel,
			"category":            rows[i].Category,
			"summary":             rows[i].Summary,
		})
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}
