package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type pluginRegistrySyncRequest struct {
	SkillID        string          `json:"skill_id"`
	PluginSkillID  string          `json:"plugin_skill_id"`
	Provider       string          `json:"provider"`
	Version        string          `json:"version"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	IntentExamples json.RawMessage `json:"intent_examples"`
	InputSchema    json.RawMessage `json:"input_schema"`
	OutputSchema   json.RawMessage `json:"output_schema"`
	PromptSpec     json.RawMessage `json:"prompt_spec"`
	Executor       json.RawMessage `json:"executor"`
	Capability     string          `json:"capability"`
	Checksum       string          `json:"checksum"`
}

func newPluginRegistryHandler(importSvc *skillservice.ImportService) *pluginRegistryHandler {
	if importSvc == nil {
		return nil
	}
	return &pluginRegistryHandler{importSvc: importSvc}
}

type pluginRegistryHandler struct {
	importSvc *skillservice.ImportService
}

func (h *pluginRegistryHandler) Sync(c *gin.Context) {
	var req pluginRegistrySyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	skillID := strings.ToLower(strings.TrimSpace(firstNonEmpty(req.SkillID, req.PluginSkillID)))
	version := strings.TrimSpace(firstNonEmpty(req.Version, "1.0.0"))
	provider := strings.TrimSpace(req.Provider)
	capability := strings.TrimSpace(req.Capability)
	if skillID == "" || version == "" || provider == "" || capability == "" {
		dto.ResponseError(c, http.StatusBadRequest, "skill_id, version, provider and capability are required", nil)
		return
	}
	manifest := map[string]any{
		"skill_id":        skillID,
		"plugin_skill_id": strings.TrimSpace(req.PluginSkillID),
		"provider":        provider,
		"version":         version,
		"title":           strings.TrimSpace(req.Title),
		"description":     strings.TrimSpace(req.Description),
		"intent_examples": decodeRaw(req.IntentExamples, []any{}),
		"input_schema":    decodeRaw(req.InputSchema, map[string]any{}),
		"output_schema":   decodeRaw(req.OutputSchema, map[string]any{}),
		"prompt_spec":     decodeRaw(req.PromptSpec, map[string]any{}),
		"executor":        decodeRaw(req.Executor, map[string]any{}),
		"capability":      capability,
		"source":          "plugin",
		"sync_source":     "plugin_registry",
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid manifest", err)
		return
	}
	checksum := strings.TrimSpace(req.Checksum)
	if checksum == "" {
		sum := sha256.Sum256(raw)
		checksum = hex.EncodeToString(sum[:])
	}
	record, err := h.importSvc.ImportDraft(c.Request.Context(), skillservice.ImportRequest{
		SkillID:    skillID,
		Version:    version,
		Source:     "plugin",
		BundleURI:  "plugin-registry://" + provider + "/" + skillID + "/" + version,
		Checksum:   checksum,
		SourceRef:  strings.TrimSpace(req.PluginSkillID),
		Manifest:   datatypes.JSON(raw),
		Operator:   actorFromContext(c),
		ImportType: skillservice.ImportTypeUpload,
	})
	if err != nil {
		respondSkillError(c, err)
		return
	}
	if err := h.importSvc.PublishLatest(c.Request.Context(), skillID, version, actorFromContext(c), "plugin registry sync"); err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"powerx_skill_id": record.SkillID,
		"skill_id":        record.SkillID,
		"version":         record.Version,
		"status":          "published",
		"source":          record.Source,
		"checksum":        record.Checksum,
	})
}

func decodeRaw(raw json.RawMessage, fallback any) any {
	if len(raw) == 0 {
		return fallback
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return fallback
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
