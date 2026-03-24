package skills

import (
	"encoding/json"
	"net/http"
	"strings"

	"gorm.io/datatypes"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type listQuery struct {
	SkillID  string `form:"skill_id"`
	Status   string `form:"status"`
	Source   string `form:"source"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

type registerSkillRequest struct {
	SkillID   string `json:"skill_id"`
	Version   string `json:"version"`
	Source    string `json:"source"`
	BundleRef struct {
		URI       string `json:"uri"`
		Checksum  string `json:"checksum"`
		Signature string `json:"signature"`
	} `json:"bundle_ref"`
	Manifest map[string]interface{} `json:"manifest"`
}

func newRegistryHandler(repo *skillrepo.SkillRegistryRepository, importSvc *skillservice.ImportService) *registryHandler {
	if repo == nil || importSvc == nil {
		return nil
	}
	return &registryHandler{
		registryRepo: repo,
		importSvc:    importSvc,
	}
}

func (h *registryHandler) List(c *gin.Context) {
	var q listQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid query params", err)
		return
	}
	filter := skillrepo.SkillRegistryFilter{
		SkillID:  q.SkillID,
		Status:   splitCSV(q.Status),
		Source:   splitCSV(q.Source),
		Page:     q.Page,
		PageSize: q.PageSize,
	}
	items, total, err := h.registryRepo.List(c.Request.Context(), filter)
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dataItems := make([]gin.H, 0, len(items))
	for i := range items {
		row := items[i]
		dataItems = append(dataItems, mapSkillRecord(&row))
	}
	dto.ResponseSuccess(c, gin.H{
		"page":      maxInt(filter.Page, 1),
		"page_size": normalizedPageSize(filter.PageSize),
		"total":     total,
		"items":     dataItems,
	})
}

func (h *registryHandler) Register(c *gin.Context) {
	var req registerSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	req.SkillID = strings.TrimSpace(strings.ToLower(req.SkillID))
	req.Version = strings.TrimSpace(req.Version)
	req.Source = strings.TrimSpace(strings.ToLower(req.Source))
	req.BundleRef.URI = strings.TrimSpace(req.BundleRef.URI)
	req.BundleRef.Checksum = strings.TrimSpace(req.BundleRef.Checksum)
	req.BundleRef.Signature = strings.TrimSpace(req.BundleRef.Signature)

	if req.SkillID == "" || req.Version == "" || req.Source == "" {
		dto.ResponseError(c, http.StatusBadRequest, "skill_id/version/source are required", nil)
		return
	}
	if req.BundleRef.URI == "" || req.BundleRef.Checksum == "" {
		dto.ResponseError(c, http.StatusBadRequest, "bundle_ref.uri and bundle_ref.checksum are required", nil)
		return
	}

	manifestJSON, err := json.Marshal(req.Manifest)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "manifest should be valid object", err)
		return
	}

	record := &skillmodel.SkillRegistryRecord{
		SkillID:      req.SkillID,
		Version:      req.Version,
		Source:       req.Source,
		Status:       skillmodel.SkillStatusDraft,
		BundleURI:    req.BundleRef.URI,
		Checksum:     req.BundleRef.Checksum,
		Signature:    req.BundleRef.Signature,
		ManifestJSON: datatypes.JSON(manifestJSON),
		ImportType:   skillservice.ImportTypeUpload,
		UpdatedBy:    actorFromContext(c),
	}
	record.Normalize()

	saved, err := h.registryRepo.Upsert(c.Request.Context(), record, []clause.Column{{Name: "skill_id"}, {Name: "version"}})
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, mapSkillRecord(saved))
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		item := strings.TrimSpace(strings.ToLower(p))
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizedPageSize(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	if pageSize > 200 {
		return 200
	}
	return pageSize
}

func maxInt(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}
