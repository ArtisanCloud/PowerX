package skills

import (
	"net/http"
	"strings"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type importRequest struct {
	SkillID   string `json:"skill_id"`
	Version   string `json:"version"`
	Source    string `json:"source"`
	BundleURI string `json:"bundle_uri"`
	Checksum  string `json:"checksum"`
	Signature string `json:"signature"`
	SourceURL string `json:"source_url"`
	SourceRef string `json:"source_ref"`
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
	record, err := h.importSvc.ImportDraft(c.Request.Context(), skillservice.ImportRequest{
		SkillID:    strings.TrimSpace(req.SkillID),
		Version:    strings.TrimSpace(req.Version),
		Source:     strings.TrimSpace(req.Source),
		BundleURI:  strings.TrimSpace(req.BundleURI),
		Checksum:   strings.TrimSpace(req.Checksum),
		Signature:  strings.TrimSpace(req.Signature),
		SourceURL:  strings.TrimSpace(req.SourceURL),
		SourceRef:  strings.TrimSpace(req.SourceRef),
		Operator:   actorFromContext(c),
		ImportType: skillservice.ImportTypeUpload,
	})
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, mapSkillRecord(record))
}
