package skills

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// OfficialSkillCatalogEntry defines built-in official skill catalog rows.
type OfficialSkillCatalogEntry struct {
	coremodel.PowerUUIDModel

	CatalogSkillID      string `gorm:"column:catalog_skill_id;type:varchar(128);not null;uniqueIndex:uk_skill_catalog_skill" json:"catalog_skill_id"`
	SkillID             string `gorm:"column:skill_id;type:varchar(128);not null;index:idx_skill_catalog_active" json:"skill_id"`
	RecommendedVersion  string `gorm:"column:recommended_version;type:varchar(64);not null" json:"recommended_version"`
	RiskLevel           string `gorm:"column:risk_level;type:varchar(16);not null" json:"risk_level"`
	Category            string `gorm:"column:category;type:varchar(64);not null" json:"category"`
	Summary             string `gorm:"column:summary;type:text" json:"summary,omitempty"`
	Active              bool   `gorm:"column:active;not null;default:true;index:idx_skill_catalog_active" json:"active"`
	Maintainer          string `gorm:"column:maintainer;type:varchar(128)" json:"maintainer,omitempty"`
	OfficialReleaseNote string `gorm:"column:official_release_note;type:text" json:"official_release_note,omitempty"`
}

func (OfficialSkillCatalogEntry) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsOfficialCatalog
}

func (e *OfficialSkillCatalogEntry) Normalize() {
	e.CatalogSkillID = strings.TrimSpace(strings.ToLower(e.CatalogSkillID))
	e.SkillID = strings.TrimSpace(strings.ToLower(e.SkillID))
	e.RecommendedVersion = strings.TrimSpace(e.RecommendedVersion)
	e.RiskLevel = strings.TrimSpace(strings.ToUpper(e.RiskLevel))
	e.Category = strings.TrimSpace(strings.ToLower(e.Category))
}
