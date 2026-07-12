package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type pluginRegistrySyncRequest struct {
	SkillID            string          `json:"skill_id"`
	PluginSkillID      string          `json:"plugin_skill_id"`
	Provider           string          `json:"provider"`
	Version            string          `json:"version"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	IntentExamples     json.RawMessage `json:"intent_examples"`
	ResponseGuidance   json.RawMessage `json:"response_guidance"`
	ActionRequiredArgs json.RawMessage `json:"action_required_args"`
	ActionOptionalArgs json.RawMessage `json:"action_optional_args"`
	SlotMapping        json.RawMessage `json:"slot_mapping"`
	PendingTaskPolicy  json.RawMessage `json:"pending_task_policy"`
	StateContract      json.RawMessage `json:"state_contract"`
	ResultPresentation json.RawMessage `json:"result_presentation"`
	InputSchema        json.RawMessage `json:"input_schema"`
	OutputSchema       json.RawMessage `json:"output_schema"`
	PromptSpec         json.RawMessage `json:"prompt_spec"`
	Executor           json.RawMessage `json:"executor"`
	Capability         string          `json:"capability"`
	Checksum           string          `json:"checksum"`
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
	pathSkillID := strings.ToLower(strings.TrimSpace(c.Param("skillId")))
	if pathSkillID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "skill_id path parameter is required", nil)
		return
	}
	if skillID != "" && skillID != pathSkillID {
		dto.ResponseError(c, http.StatusBadRequest, "skill_id path and body mismatch", fmt.Errorf("path=%s body=%s", pathSkillID, skillID))
		return
	}
	skillID = pathSkillID
	version := strings.TrimSpace(firstNonEmpty(req.Version, "1.0.0"))
	provider := strings.TrimSpace(req.Provider)
	capability := strings.TrimSpace(req.Capability)
	if skillID == "" || version == "" || provider == "" || capability == "" {
		dto.ResponseError(c, http.StatusBadRequest, "skill_id, version, provider and capability are required", nil)
		return
	}
	manifest := map[string]any{
		"skill_id":             skillID,
		"plugin_skill_id":      strings.TrimSpace(req.PluginSkillID),
		"provider":             provider,
		"version":              version,
		"title":                strings.TrimSpace(req.Title),
		"description":          strings.TrimSpace(req.Description),
		"intent_examples":      decodeRaw(req.IntentExamples, []any{}),
		"response_guidance":    decodeRaw(req.ResponseGuidance, map[string]any{}),
		"action_required_args": decodeRaw(req.ActionRequiredArgs, map[string]any{}),
		"action_optional_args": decodeRaw(req.ActionOptionalArgs, map[string]any{}),
		"slot_mapping":         decodeRaw(req.SlotMapping, map[string]any{}),
		"pending_task_policy":  decodeRaw(req.PendingTaskPolicy, map[string]any{}),
		"state_contract":       decodeRaw(req.StateContract, map[string]any{}),
		"result_presentation":  decodeRaw(req.ResultPresentation, map[string]any{}),
		"input_schema":         decodeRaw(req.InputSchema, map[string]any{}),
		"output_schema":        decodeRaw(req.OutputSchema, map[string]any{}),
		"prompt_spec":          decodeRaw(req.PromptSpec, map[string]any{}),
		"executor":             decodeRaw(req.Executor, map[string]any{}),
		"capability":           capability,
		"source":               "plugin",
		"sync_source":          "plugin_registry",
	}
	if err := validatePluginRegistryManifest(manifest); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid plugin skill manifest", err)
		return
	}
	if actions := actionCapabilitiesFromExecutor(manifest["executor"]); len(actions) > 0 {
		manifest["action_capabilities"] = actions
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
	record, err := h.importSvc.ImportPluginPublished(c.Request.Context(), skillservice.ImportRequest{
		SkillID:    skillID,
		Version:    version,
		Source:     "plugin",
		BundleURI:  "plugin-registry://" + provider + "/" + skillID + "/" + version,
		Checksum:   checksum,
		SourceRef:  strings.TrimSpace(req.PluginSkillID),
		Manifest:   datatypes.JSON(raw),
		Operator:   actorFromContext(c),
		ImportType: skillservice.ImportTypeUpload,
	}, "plugin registry sync")
	if err != nil {
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

func validatePluginRegistryManifest(manifest map[string]any) error {
	executor, ok := manifest["executor"].(map[string]any)
	if !ok || len(executor) == 0 {
		return fmt.Errorf("executor is required")
	}
	required := map[string]string{
		"executor.type":               strings.TrimSpace(fmt.Sprint(executor["type"])),
		"executor.capability":         strings.TrimSpace(fmt.Sprint(executor["capability"])),
		"executor.prepare_capability": strings.TrimSpace(fmt.Sprint(executor["prepare_capability"])),
	}
	for field, value := range required {
		if value == "" || value == "<nil>" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if required["executor.type"] != "capability" {
		return fmt.Errorf("executor.type must be capability")
	}
	if len(actionCapabilitiesFromExecutor(executor)) == 0 {
		return fmt.Errorf("executor.action_map is required")
	}
	return nil
}

func actionCapabilitiesFromExecutor(executor any) map[string]string {
	raw, ok := executor.(map[string]any)
	if !ok {
		if rawInterface, okInterface := executor.(map[string]interface{}); okInterface {
			raw = rawInterface
		}
	}
	if len(raw) == 0 {
		return nil
	}
	actionMap, ok := raw["action_map"].(map[string]any)
	if !ok {
		if typed, okTyped := raw["action_map"].(map[string]string); okTyped {
			out := make(map[string]string, len(typed))
			for k, v := range typed {
				if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
					out[strings.TrimSpace(k)] = strings.TrimSpace(v)
				}
			}
			return out
		}
		return nil
	}
	out := make(map[string]string, len(actionMap))
	for k, v := range actionMap {
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(firstNonEmpty(fmt.Sprint(v)))
		if key != "" && value != "" && value != "<nil>" {
			out[key] = value
		}
	}
	return out
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
