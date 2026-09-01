package plugin_runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	ksmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var nonKeyChars = regexp.MustCompile(`[^a-z0-9._-]+`)

const (
	agentKeyMaxLen        = 64
	agentKeyHashHexLength = 16
)

type tenantRuntimeHandler struct {
	deps     *shared.Deps
	agentSvc *agentSvc.AgentService
}

func newTenantRuntimeHandler(deps *shared.Deps) *tenantRuntimeHandler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return &tenantRuntimeHandler{
		deps:     deps,
		agentSvc: agentSvc.NewAgentService(deps.DB),
	}
}

type knowledgeSpaceItem struct {
	UUID       string `json:"uuid"`
	SpaceName  string `json:"space_name"`
	Status     string `json:"status"`
	Department string `json:"department_code"`
	ProfileKey string `json:"rag_profile_key"`
	UpdatedAt  string `json:"updated_at"`
	CreatedAt  string `json:"created_at"`
}

func (h *tenantRuntimeHandler) ListKnowledgeSpaces(c *gin.Context) {
	tenantUUID, err := requireTenantUUID(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "20"), 20)
	if pageSize > 200 {
		pageSize = 200
	}
	status := strings.TrimSpace(c.Query("status"))
	keyword := strings.TrimSpace(c.Query("keyword"))

	db := h.deps.DB.WithContext(c.Request.Context()).Model(&ksmodel.KnowledgeSpace{}).
		Where("tenant_uuid = ?", tenantUUID)
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		db = db.Where("LOWER(space_name) LIKE ? OR LOWER(department_code) LIKE ?", like, like)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		dto.ResponseError(c, 500, "count knowledge spaces failed", err)
		return
	}

	var rows []ksmodel.KnowledgeSpace
	if err := db.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		dto.ResponseError(c, 500, "list knowledge spaces failed", err)
		return
	}
	items := make([]knowledgeSpaceItem, 0, len(rows))
	for i := range rows {
		items = append(items, knowledgeSpaceItem{
			UUID:       rows[i].UUID.String(),
			SpaceName:  rows[i].SpaceName,
			Status:     rows[i].Status,
			Department: rows[i].DepartmentCode,
			ProfileKey: rows[i].RAGProfileKey,
			UpdatedAt:  rows[i].UpdatedAt.Format(time.RFC3339),
			CreatedAt:  rows[i].CreatedAt.Format(time.RFC3339),
		})
	}
	dto.ResponseSuccess(c, gin.H{
		"items": items,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

type instantiateAgentRequest struct {
	Env              string                 `json:"env"`
	Key              string                 `json:"key"`
	Name             string                 `json:"name" binding:"required"`
	Description      string                 `json:"description"`
	TypeID           string                 `json:"type_id"`
	Scene            string                 `json:"scene"`
	PromptSeed       string                 `json:"prompt_seed"`
	Persona          string                 `json:"persona"`
	SkillIDs         []string               `json:"skill_ids"`
	KnowledgeBaseIDs []string               `json:"knowledge_base_ids"`
	Parameters       map[string]interface{} `json:"parameters"`
	Meta             map[string]interface{} `json:"meta"`
	Status           string                 `json:"status"`
	Visibility       string                 `json:"visibility"`
	Scope            string                 `json:"scope"`
}

type agentRuntimeItem struct {
	UUID             string                 `json:"uuid"`
	Key              string                 `json:"key"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Persona          string                 `json:"persona,omitempty"`
	Env              string                 `json:"env"`
	Status           string                 `json:"status"`
	Visibility       string                 `json:"visibility"`
	Scope            string                 `json:"scope"`
	Source           string                 `json:"source"`
	TypeID           string                 `json:"type_id,omitempty"`
	Scene            string                 `json:"scene,omitempty"`
	PromptSeed       string                 `json:"prompt_seed,omitempty"`
	Parameters       map[string]interface{} `json:"parameters,omitempty"`
	SkillIDs         []string               `json:"skill_ids,omitempty"`
	KnowledgeBaseIDs []string               `json:"knowledge_base_ids,omitempty"`
	Meta             map[string]interface{} `json:"meta,omitempty"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
}

func (h *tenantRuntimeHandler) InstantiateAgent(c *gin.Context) {
	if h.agentSvc == nil {
		dto.ResponseError(c, 503, "agent service unavailable", nil)
		return
	}
	tenantUUID, err := requireTenantUUID(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	var req instantiateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = strings.TrimSpace(reqctx.GetEnv(c.Request.Context()))
	}
	if env == "" {
		env = "dev"
	}
	key := normalizeAgentKey(strings.TrimSpace(req.Key), req.TypeID, req.Name)
	tenantRef := &tenantUUID

	meta := toJSONMap(req.Meta)
	if meta == nil {
		meta = datatypes.JSONMap{}
	}
	if strings.TrimSpace(req.TypeID) != "" {
		meta["type_id"] = strings.TrimSpace(req.TypeID)
	}
	if strings.TrimSpace(req.Scene) != "" {
		meta["scene"] = strings.TrimSpace(req.Scene)
	}
	if strings.TrimSpace(req.PromptSeed) != "" {
		meta["prompt_seed"] = strings.TrimSpace(req.PromptSeed)
	}
	if len(req.Parameters) > 0 {
		meta["parameters"] = req.Parameters
	}

	in := &dbmodel.Agent{
		Key:             key,
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		TypeID:          firstNonEmpty(strings.TrimSpace(req.TypeID), extractMetaStringFromMap(req.Meta, "type_id"), extractMetaStringFromMap(req.Meta, "typeId")),
		Scene:           firstNonEmpty(strings.TrimSpace(req.Scene), extractMetaStringFromMap(req.Meta, "scene")),
		PromptSeed:      strings.TrimSpace(req.PromptSeed),
		Persona:         strings.TrimSpace(req.Persona),
		Source:          "plugin-runtime",
		Scope:           firstNonEmpty(strings.TrimSpace(req.Scope), dbmodel.AgentScopeTenant),
		Visibility:      firstNonEmpty(strings.TrimSpace(req.Visibility), dbmodel.AgentVisibilityTenant),
		Status:          firstNonEmpty(strings.TrimSpace(req.Status), dbmodel.AgentStatusDraft),
		KBStrategy:      dbmodel.KBStrategyUnion,
		ManagedByPlugin: false,
		Meta:            meta,
	}

	created, createErr := h.agentSvc.Create(c.Request.Context(), env, tenantRef, in)
	if createErr != nil {
		dto.ResponseError(c, mapCreateAgentErrorCode(createErr), "instantiate agent failed", createErr)
		return
	}
	skillIDs := firstNonEmptyStringSlice(req.SkillIDs, extractStringSliceFromMap(req.Parameters, "skill_ids", "skillIds"), extractStringSliceFromMap(req.Meta, "skill_ids", "skillIds"), extractStringSliceFromParametersInMeta(req.Meta, "skill_ids", "skillIds"))
	knowledgeBaseIDs := firstNonEmptyStringSlice(req.KnowledgeBaseIDs, extractStringSliceFromMap(req.Parameters, "knowledge_base_ids", "knowledgeBaseIds"), extractStringSliceFromMap(req.Meta, "knowledge_base_ids", "knowledgeBaseIds"), extractStringSliceFromParametersInMeta(req.Meta, "knowledge_base_ids", "knowledgeBaseIds"))
	if err := h.agentSvc.ReplaceSkillBindings(c.Request.Context(), env, tenantRef, created.ID, skillIDs); err != nil {
		dto.ResponseError(c, 400, "sync skill bindings failed", err)
		return
	}
	if err := h.agentSvc.ReplaceKnowledgeBindings(c.Request.Context(), env, tenantRef, created.ID, knowledgeBaseIDs); err != nil {
		dto.ResponseError(c, 400, "sync knowledge bindings failed", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, 201, toAgentRuntimeItem(*created))
}

func (h *tenantRuntimeHandler) ListAgents(c *gin.Context) {
	if h.agentSvc == nil {
		dto.ResponseError(c, 503, "agent service unavailable", nil)
		return
	}
	tenantUUID, err := requireTenantUUID(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	env := strings.TrimSpace(c.Query("env"))
	if env == "" {
		env = strings.TrimSpace(reqctx.GetEnv(c.Request.Context()))
	}
	if env == "" {
		env = "dev"
	}

	statuses := make([]string, 0, 4)
	if statusRaw := strings.TrimSpace(c.Query("status")); statusRaw != "" {
		for _, item := range strings.Split(statusRaw, ",") {
			v := strings.TrimSpace(item)
			if v != "" {
				statuses = append(statuses, v)
			}
		}
		sort.Strings(statuses)
	}

	tenantRef := &tenantUUID
	rows, listErr := h.agentSvc.List(c.Request.Context(), env, tenantRef, "", statuses...)
	if listErr != nil {
		dto.ResponseError(c, 500, "list agents failed", listErr)
		return
	}

	items := make([]agentRuntimeItem, 0, len(rows))
	for i := range rows {
		items = append(items, toAgentRuntimeItem(rows[i]))
	}
	dto.ResponseSuccess(c, gin.H{
		"items": items,
		"count": len(items),
	})
}

func toAgentRuntimeItem(agent dbmodel.Agent) agentRuntimeItem {
	item := agentRuntimeItem{
		UUID:        agent.UUID.String(),
		Key:         agent.Key,
		Name:        agent.Name,
		Description: agent.Description,
		Persona:     strings.TrimSpace(agent.Persona),
		Env:         agent.Env,
		Status:      agent.Status,
		Visibility:  agent.Visibility,
		Scope:       agent.Scope,
		Source:      agent.Source,
		Meta:        toMap(agent.Meta),
		CreatedAt:   agent.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   agent.UpdatedAt.Format(time.RFC3339),
	}
	item.TypeID = strings.TrimSpace(agent.TypeID)
	item.Scene = strings.TrimSpace(agent.Scene)
	item.PromptSeed = strings.TrimSpace(agent.PromptSeed)
	if item.Meta != nil {
		if item.TypeID == "" {
			item.TypeID = firstNonEmpty(strings.TrimSpace(fmt.Sprint(item.Meta["type_id"])), strings.TrimSpace(fmt.Sprint(item.Meta["typeId"])))
		}
		if item.Scene == "" {
			item.Scene = strings.TrimSpace(fmt.Sprint(item.Meta["scene"]))
		}
		if item.PromptSeed == "" {
			item.PromptSeed = firstNonEmpty(strings.TrimSpace(fmt.Sprint(item.Meta["prompt_seed"])), strings.TrimSpace(fmt.Sprint(item.Meta["promptSeed"])))
		}
		if p, ok := item.Meta["parameters"].(map[string]interface{}); ok {
			item.Parameters = p
		}
	}
	item.SkillIDs = firstNonEmptyStringSlice(extractStringSliceFromMap(item.Parameters, "skill_ids", "skillIds"), extractStringSliceFromMap(item.Meta, "skill_ids", "skillIds"), extractStringSliceFromParametersInMeta(item.Meta, "skill_ids", "skillIds"))
	item.KnowledgeBaseIDs = firstNonEmptyStringSlice(extractStringSliceFromMap(item.Parameters, "knowledge_base_ids", "knowledgeBaseIds"), extractStringSliceFromMap(item.Meta, "knowledge_base_ids", "knowledgeBaseIds"), extractStringSliceFromParametersInMeta(item.Meta, "knowledge_base_ids", "knowledgeBaseIds"))
	return item
}

func requireTenantUUID(c *gin.Context) (string, error) {
	raw, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		return "", err
	}
	return reqctx.CanonicalTenantUUID(raw)
}

func parsePositiveInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func buildAgentKey(typeID, name string) string {
	base := strings.ToLower(strings.TrimSpace(typeID))
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(name))
	}
	base = nonKeyChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-_.")
	if base == "" {
		base = "agent"
	}
	if len(base) > 48 {
		base = base[:48]
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func normalizeAgentKey(rawKey, typeID, name string) string {
	base := strings.ToLower(strings.TrimSpace(rawKey))
	if base == "" {
		base = buildAgentKey(typeID, name)
	}
	base = nonKeyChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-_.")
	if base == "" {
		base = buildAgentKey(typeID, name)
		base = nonKeyChars.ReplaceAllString(base, "-")
		base = strings.Trim(base, "-_.")
	}
	if len(base) <= agentKeyMaxLen {
		return base
	}
	sum := sha256.Sum256([]byte(base))
	suffix := hex.EncodeToString(sum[:])[:agentKeyHashHexLength]
	prefixLen := agentKeyMaxLen - 1 - len(suffix)
	if prefixLen < 1 {
		prefixLen = 1
	}
	prefix := strings.Trim(base[:prefixLen], "-_.")
	if prefix == "" {
		prefix = "agent"
	}
	return prefix + "-" + suffix
}

func toJSONMap(input map[string]interface{}) datatypes.JSONMap {
	if len(input) == 0 {
		return nil
	}
	out := datatypes.JSONMap{}
	for k, v := range input {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toMap(input datatypes.JSONMap) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func extractMetaStringFromMap(meta map[string]interface{}, key string) string {
	if len(meta) == 0 || strings.TrimSpace(key) == "" {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(meta[key]))
}

func extractStringSliceFromMap(input map[string]interface{}, keys ...string) []string {
	if len(input) == 0 || len(keys) == 0 {
		return nil
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if out := parseStringSlice(input[key]); len(out) > 0 {
			return out
		}
	}
	return nil
}

func extractStringSliceFromParametersInMeta(meta map[string]interface{}, keys ...string) []string {
	if len(meta) == 0 || len(keys) == 0 {
		return nil
	}
	raw := meta["parameters"]
	params, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	return extractStringSliceFromMap(params, keys...)
}

func parseStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		return normalizeStringSlice(v)
	case []interface{}:
		values := make([]string, 0, len(v))
		for _, item := range v {
			values = append(values, strings.TrimSpace(fmt.Sprint(item)))
		}
		return normalizeStringSlice(values)
	default:
		return nil
	}
}

func firstNonEmptyStringSlice(candidates ...[]string) []string {
	for _, candidate := range candidates {
		if out := normalizeStringSlice(candidate); len(out) > 0 {
			return out
		}
	}
	return nil
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mapCreateAgentErrorCode(err error) int {
	if err == nil {
		return 200
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return 409
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") {
		return 409
	}
	return 400
}
