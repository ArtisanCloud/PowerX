package skills

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

var (
	allowedCatalogRiskLevels = map[string]struct{}{
		"L1": {},
		"L2": {},
		"L3": {},
	}
	allowedCatalogCategories = map[string]struct{}{
		"platform":  {},
		"dev":       {},
		"doc":       {},
		"knowledge": {},
		"comm":      {},
		"pm":        {},
		"media":     {},
		"device":    {},
		"channel":   {},
		"security":  {},
	}
)

type catalogAuditSnapshot struct {
	CatalogSkillID string `json:"catalog_skill_id,omitempty"`
	SkillID        string `json:"skill_id,omitempty"`
	Version        string `json:"version,omitempty"`
	RiskLevel      string `json:"risk_level,omitempty"`
	Category       string `json:"category,omitempty"`
	Active         bool   `json:"active"`
}

func newCatalogHandler(db *gorm.DB, auditSvc *skillservice.AuditTraceService) *catalogHandler {
	if db == nil {
		return nil
	}
	return &catalogHandler{db: db, auditSvc: auditSvc}
}

func (h *catalogHandler) ListCatalog(c *gin.Context) {
	if h == nil || h.db == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "skills catalog unavailable", nil)
		return
	}
	var rows []skillmodel.OfficialSkillCatalogEntry
	if err := h.db.WithContext(c.Request.Context()).
		Order("updated_at DESC").
		Find(&rows).Error; err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "query catalog failed", err)
		return
	}

	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		items = append(items, gin.H{
			"catalog_skill_id":      rows[i].CatalogSkillID,
			"skill_id":              rows[i].SkillID,
			"recommended_version":   rows[i].RecommendedVersion,
			"risk_level":            rows[i].RiskLevel,
			"category":              rows[i].Category,
			"summary":               rows[i].Summary,
			"active":                rows[i].Active,
			"maintainer":            rows[i].Maintainer,
			"official_release_note": rows[i].OfficialReleaseNote,
		})
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

type upsertCatalogRequest struct {
	CatalogSkillID      string `json:"catalog_skill_id"`
	SkillID             string `json:"skill_id"`
	RecommendedVersion  string `json:"recommended_version"`
	RiskLevel           string `json:"risk_level"`
	Category            string `json:"category"`
	Summary             string `json:"summary"`
	Active              *bool  `json:"active"`
	Maintainer          string `json:"maintainer"`
	OfficialReleaseNote string `json:"official_release_note"`
	BundleURI           string `json:"bundle_uri"`
	Checksum            string `json:"checksum"`
}

type setCatalogActiveRequest struct {
	Active bool `json:"active"`
}

func (h *catalogHandler) UpsertCatalog(c *gin.Context) {
	if h == nil || h.db == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "skills catalog unavailable", nil)
		return
	}

	var req upsertCatalogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	req.CatalogSkillID = strings.TrimSpace(strings.ToLower(req.CatalogSkillID))
	req.SkillID = strings.TrimSpace(strings.ToLower(req.SkillID))
	req.RecommendedVersion = strings.TrimSpace(req.RecommendedVersion)
	req.RiskLevel = strings.TrimSpace(strings.ToUpper(req.RiskLevel))
	req.Category = strings.TrimSpace(strings.ToLower(req.Category))
	req.Summary = strings.TrimSpace(req.Summary)
	req.Maintainer = strings.TrimSpace(req.Maintainer)
	req.OfficialReleaseNote = strings.TrimSpace(req.OfficialReleaseNote)
	req.BundleURI = strings.TrimSpace(req.BundleURI)
	req.Checksum = strings.TrimSpace(req.Checksum)
	if req.CatalogSkillID == "" || req.SkillID == "" || req.RecommendedVersion == "" || req.RiskLevel == "" || req.Category == "" {
		dto.ResponseError(c, http.StatusBadRequest, "catalog_skill_id/skill_id/recommended_version/risk_level/category are required", nil)
		return
	}
	if _, ok := allowedCatalogRiskLevels[req.RiskLevel]; !ok {
		dto.ResponseError(c, http.StatusBadRequest, "risk_level must be one of: L1/L2/L3", nil)
		return
	}
	if _, ok := allowedCatalogCategories[req.Category]; !ok {
		dto.ResponseError(c, http.StatusBadRequest, "category must be one of: platform/dev/doc/knowledge/comm/pm/media/device/channel/security", nil)
		return
	}

	if req.BundleURI == "" {
		req.BundleURI = "builtin://skills/" + req.SkillID + "/" + req.RecommendedVersion
	}
	if req.Checksum == "" {
		req.Checksum = "sha256:official-" + strings.ReplaceAll(req.SkillID, ".", "-") + "-" + req.RecommendedVersion
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	now := time.Now().UTC()
	operator := actorFromContext(c)
	var before *skillmodel.OfficialSkillCatalogEntry

	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var existing skillmodel.OfficialSkillCatalogEntry
		if findErr := tx.Where("catalog_skill_id = ?", req.CatalogSkillID).Take(&existing).Error; findErr == nil {
			before = &existing
		} else if findErr != nil && findErr != gorm.ErrRecordNotFound {
			return findErr
		}

		catalog := &skillmodel.OfficialSkillCatalogEntry{
			CatalogSkillID:      req.CatalogSkillID,
			SkillID:             req.SkillID,
			RecommendedVersion:  req.RecommendedVersion,
			RiskLevel:           req.RiskLevel,
			Category:            req.Category,
			Summary:             req.Summary,
			Active:              active,
			Maintainer:          req.Maintainer,
			OfficialReleaseNote: req.OfficialReleaseNote,
		}
		catalog.Normalize()
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "catalog_skill_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"skill_id":              catalog.SkillID,
				"recommended_version":   catalog.RecommendedVersion,
				"risk_level":            catalog.RiskLevel,
				"category":              catalog.Category,
				"summary":               catalog.Summary,
				"active":                catalog.Active,
				"maintainer":            catalog.Maintainer,
				"official_release_note": catalog.OfficialReleaseNote,
				"updated_at":            now,
			}),
		}).Create(catalog).Error; err != nil {
			return err
		}

		if err := tx.Model(&skillmodel.SkillRegistryRecord{}).
			Where("skill_id = ?", catalog.SkillID).
			Updates(map[string]interface{}{
				"is_latest_published": false,
				"latest_switched_at":  now,
				"updated_by":          operator,
			}).Error; err != nil {
			return err
		}

		registry := &skillmodel.SkillRegistryRecord{
			SkillID:            catalog.SkillID,
			Version:            catalog.RecommendedVersion,
			Source:             skillmodel.SkillSourceBuiltin,
			Status:             skillmodel.SkillStatusPublished,
			IsLatestPublished:  true,
			BundleURI:          req.BundleURI,
			Checksum:           req.Checksum,
			ManifestJSON:       datatypes.JSON([]byte(`{"name":"builtin skill","entrypoints":["default"]}`)),
			ImportType:         "official_catalog",
			UpdatedBy:          operator,
			PublishedAt:        &now,
			LatestSwitchedAt:   &now,
			ApprovalNote:       "manual upsert from catalog",
			IntegrityPolicyRef: "builtin-default",
		}
		registry.Normalize()
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "skill_id"}, {Name: "version"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"source":               registry.Source,
				"status":               registry.Status,
				"is_latest_published":  registry.IsLatestPublished,
				"bundle_uri":           registry.BundleURI,
				"checksum":             registry.Checksum,
				"manifest_json":        registry.ManifestJSON,
				"import_type":          registry.ImportType,
				"updated_by":           registry.UpdatedBy,
				"published_at":         registry.PublishedAt,
				"latest_switched_at":   registry.LatestSwitchedAt,
				"approval_note":        registry.ApprovalNote,
				"integrity_policy_ref": registry.IntegrityPolicyRef,
				"updated_at":           now,
			}),
		}).Create(registry).Error
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "upsert catalog failed", err)
		return
	}
	if h.auditSvc != nil {
		action := "catalog_create"
		if before != nil {
			action = "catalog_update"
		}
		beforePayload := &catalogAuditSnapshot{}
		if before != nil {
			beforePayload = &catalogAuditSnapshot{
				CatalogSkillID: before.CatalogSkillID,
				SkillID:        before.SkillID,
				Version:        before.RecommendedVersion,
				RiskLevel:      before.RiskLevel,
				Category:       before.Category,
				Active:         before.Active,
			}
		}
		afterPayload := &catalogAuditSnapshot{
			CatalogSkillID: req.CatalogSkillID,
			SkillID:        req.SkillID,
			Version:        req.RecommendedVersion,
			RiskLevel:      req.RiskLevel,
			Category:       req.Category,
			Active:         active,
		}
		reasonRaw, _ := json.Marshal(gin.H{"catalog_change": gin.H{"before": beforePayload, "after": afterPayload}})
		_ = h.auditSvc.RecordLifecycleAudit(c.Request.Context(), skillservice.LifecycleAuditInput{
			Action:   action,
			SkillID:  req.SkillID,
			Version:  req.RecommendedVersion,
			Operator: operator,
			Source:   skillmodel.SkillSourceBuiltin,
			Reason:   string(reasonRaw),
			Result:   "success",
		})
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"catalog_skill_id":    req.CatalogSkillID,
		"skill_id":            req.SkillID,
		"recommended_version": req.RecommendedVersion,
		"risk_level":          req.RiskLevel,
		"category":            req.Category,
		"summary":             req.Summary,
		"active":              active,
	})
}

func (h *catalogHandler) SetCatalogActive(c *gin.Context) {
	if h == nil || h.db == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "skills catalog unavailable", nil)
		return
	}
	catalogSkillID := strings.TrimSpace(strings.ToLower(c.Param("catalogSkillId")))
	if catalogSkillID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "catalogSkillId is required", nil)
		return
	}

	var req setCatalogActiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	var before skillmodel.OfficialSkillCatalogEntry
	if err := h.db.WithContext(c.Request.Context()).Where("catalog_skill_id = ?", catalogSkillID).Take(&before).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			dto.ResponseError(c, http.StatusNotFound, "catalog item not found", nil)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "query catalog failed", err)
		return
	}
	res := h.db.WithContext(c.Request.Context()).
		Model(&skillmodel.OfficialSkillCatalogEntry{}).
		Where("catalog_skill_id = ?", catalogSkillID).
		Updates(map[string]interface{}{
			"active":     req.Active,
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "update catalog active failed", res.Error)
		return
	}
	if h.auditSvc != nil {
		operator := actorFromContext(c)
		reasonRaw, _ := json.Marshal(gin.H{
			"catalog_active_change": gin.H{
				"catalog_skill_id": catalogSkillID,
				"before_active":    before.Active,
				"after_active":     req.Active,
			},
		})
		_ = h.auditSvc.RecordLifecycleAudit(c.Request.Context(), skillservice.LifecycleAuditInput{
			Action:   "catalog_set_active",
			SkillID:  before.SkillID,
			Version:  before.RecommendedVersion,
			Operator: operator,
			Source:   skillmodel.SkillSourceBuiltin,
			Reason:   string(reasonRaw),
			Result:   "success",
		})
	}
	dto.ResponseSuccess(c, gin.H{"catalog_skill_id": catalogSkillID, "active": req.Active})
}
