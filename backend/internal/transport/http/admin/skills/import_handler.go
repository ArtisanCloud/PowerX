package skills

import (
	"encoding/json"
	"net/http"
	"strings"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type importRequest struct {
	SkillID    string `json:"skill_id"`
	Version    string `json:"version"`
	Source     string `json:"source"`
	ImportType string `json:"import_type"`
	BundleURI  string `json:"bundle_uri"`
	Checksum   string `json:"checksum"`
	Signature  string `json:"signature"`
	SourceURL  string `json:"source_url"`
	SourceRef  string `json:"source_ref"`
	SourcePath string `json:"source_path"`

	SkillMarkdown string `json:"skill_markdown"`
}

func newImportHandler(importSvc *skillservice.ImportService) *importHandler {
	if importSvc == nil {
		return nil
	}
	return &importHandler{importSvc: importSvc}
}

func (h *importHandler) Import(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	var manifestJSON datatypes.JSON
	if strings.TrimSpace(req.SkillMarkdown) != "" {
		manifest, err := skillservice.ParseSkillMarkdownToManifest(req.SkillMarkdown, req.Version)
		if err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "invalid skill_markdown", err)
			return
		}
		raw, err := json.Marshal(manifest)
		if err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "invalid skill_markdown manifest", err)
			return
		}
		manifestJSON = datatypes.JSON(raw)
	}
	record, err := h.importSvc.ImportDraft(c.Request.Context(), skillservice.ImportRequest{
		SkillID:    strings.TrimSpace(req.SkillID),
		Version:    strings.TrimSpace(req.Version),
		Source:     strings.TrimSpace(req.Source),
		BundleURI:  strings.TrimSpace(req.BundleURI),
		Checksum:   strings.TrimSpace(req.Checksum),
		Signature:  strings.TrimSpace(req.Signature),
		SourceURL:  strings.TrimSpace(req.SourceURL),
		SourceRef:  strings.TrimSpace(req.SourceRef),
		SourcePath: strings.TrimSpace(req.SourcePath),
		Manifest:   manifestJSON,
		Operator:   actorFromContext(c),
		ImportType: strings.TrimSpace(req.ImportType),
	})
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, mapSkillRecord(record))
}
