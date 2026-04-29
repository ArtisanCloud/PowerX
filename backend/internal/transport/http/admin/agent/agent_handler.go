package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var agentKeyNonWordChars = regexp.MustCompile(`[^a-z0-9._-]+`)

const (
	agentKeyMaxLen        = 64
	agentKeyHashHexLength = 16
)

// ====== DTO ======

type AgentHandler struct {
	srv *agentSvc.AgentService
}

func NewAgentHandler(dep *shared.Deps) *AgentHandler {
	return &AgentHandler{
		srv: agentSvc.NewAgentService(dep.DB),
	}
}

type AgentStatusRequest struct {
	// 兼容字段：历史实现要求 agent_id（运行期 manager key），但前端不会传。
	AgentID   string `form:"agent_id" json:"agent_id,omitempty"`
	AgentUUID string `form:"agent_uuid" json:"agent_uuid,omitempty"`
}

type AgentStatusResponse struct {
	*agentschema.AgentInfo
}

// 仅意图识别的响应体（不要和 dto.ChatData 混用）
type AgentIntentResponse struct {
	Input     string                `json:"input"`
	Timestamp int64                 `json:"timestamp"`
	Intent    *schemas.IntentResult `json:"intent"`
	// 可选：给前端调试/展示
	Debug map[string]any `json:"debug,omitempty"`
}

// ====== Handlers ======

// AgentStatusHandler: 查询 Agent 状态
func (h *AgentHandler) Status(c *gin.Context) {
	var req AgentStatusRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	// 该接口用于前端“启动阶段探活”，必须允许无 agent_id/agent_uuid 的调用。
	dtoRequest.ResponseSuccess(c, gin.H{
		"status":  "ok",
		"message": "success",
	})
}

func parseAgentUUIDParam(c *gin.Context) (uuid.UUID, error) {
	raw := strings.TrimSpace(utils.FirstNonEmpty(c.Param("uuid"), c.Param("id")))
	return uuid.Parse(raw)
}

// /api/agents/intent  支持单意图(默认) 或 多任务(?multi=1)
func (h *AgentHandler) Intent(c *gin.Context) {
	var req dtoRequest.ChatRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dtoRequest.ResponseError(c, 400, "message 不能为空", nil)
		return
	}

	//multi := c.Query("multi") == "1" // ✨ 新增：多任务开关
	multi := true // ✨ 新增：多任务开关
	mgr := agent.GetAgentManager()

	if multi {
		tasks, err := mgr.DetectTasks(c, msg)
		if err != nil {
			dtoRequest.ResponseError(c, 500, "意图识别失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"input": msg,
			"mode":  "intent_multi",
			"tasks": tasks, // 直接返回多任务
			"debug": gin.H{"task_count": len(tasks)},
			"ts":    time.Now().UTC().Unix(),
		})
		return
	}

	// 单意图：原样保留
	intent, err := mgr.DetectIntent(c, msg)
	if err != nil {
		dtoRequest.ResponseError(c, 500, "意图识别失败", err)
		return
	}
	if intent == nil {
		intent = &schemas.IntentResult{Matched: false, Strategy: "none", Reason: "no strategy result"}
	}
	dtoRequest.ResponseSuccess(c, &AgentIntentResponse{
		Input:     msg,
		Timestamp: time.Now().UTC().Unix(),
		Intent:    intent,
		Debug:     map[string]any{"strategy": intent.Strategy, "score": intent.Score, "agent_id": intent.AgentID, "flow_id": intent.FlowID},
	})
	return
}

// /api/agent/intent/plan 仅识别并生成计划（dry-run），不执行
func (h *AgentHandler) PlanPreview(c *gin.Context) {
	// 1) 解析请求
	var req dtoRequest.ChatRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dtoRequest.ResponseError(c, 400, "message 不能为空", nil)
		return
	}

	// 2) 多意图识别（已内置去重与排序）
	mgr := agent.GetAgentManager()
	tasks, err := mgr.DetectTasks(c, msg)
	if err != nil {
		dtoRequest.ResponseError(c, 500, "意图识别失败", err)
		return
	}
	if len(tasks) == 0 {
		dtoRequest.ResponseSuccess(c, gin.H{
			"mode":   "plan_preview",
			"input":  msg,
			"debug":  gin.H{"task_count": 0},
			"plan":   gin.H{"tasks": []any{}},
			"tasks":  []any{},
			"reason": "no task detected",
		})
		return
	}

	// 3) 基于 YAML 的 requires 自动补全依赖 + 分层 + 参数连线
	plan := mgr.BuildPlan(tasks)

	// 4) 返回
	dtoRequest.ResponseSuccess(c, gin.H{
		"mode":  "plan_preview",
		"input": msg,
		"debug": gin.H{"task_count": len(tasks)},
		"plan":  plan,
		"tasks": tasks,
	})
	return
}

type createAgentReq struct {
	Env              string            `json:"env" validate:"required"`
	Key              string            `json:"key" validate:"required"`
	Name             string            `json:"name" validate:"required"`
	Description      string            `json:"description"`
	TypeID           string            `json:"typeId,omitempty"`
	Scene            string            `json:"scene,omitempty"`
	PromptSeed       string            `json:"promptSeed,omitempty"`
	Persona          string            `json:"persona,omitempty"`
	SkillIDs         []string          `json:"skillIds,omitempty"`
	KnowledgeBaseIDs []string          `json:"knowledgeBaseIds,omitempty"`
	Visibility       string            `json:"visibility"` // private|tenant|public
	Status           string            `json:"status"`     // draft|active...
	Scope            string            `json:"scope"`      // system|tenant（语义标签）
	DefaultPersonaID *uint64           `json:"defaultPersonaId"`
	BlueprintRefs    datatypes.JSON    `json:"blueprintRefs"`
	IntentCardsRef   datatypes.JSON    `json:"intentCardsRef"`
	ToolAllowlist    datatypes.JSON    `json:"toolAllowlist"`
	KBStrategy       string            `json:"kbStrategy"`
	Meta             datatypes.JSONMap `json:"meta"`
	Source           string            `json:"source,omitempty"`          // core | plugin:<plugin_id>
	OwnerPluginID    string            `json:"ownerPluginId,omitempty"`   // 插件归属
	ManagedByPlugin  *bool             `json:"managedByPlugin,omitempty"` // true 时归属插件托管
}

type updateAgentReq struct {
	Name                  *string           `json:"name,omitempty"`
	Description           *string           `json:"description,omitempty"`
	TypeID                *string           `json:"typeId,omitempty"`
	Scene                 *string           `json:"scene,omitempty"`
	PromptSeed            *string           `json:"promptSeed,omitempty"`
	Persona               *string           `json:"persona,omitempty"`
	SkillIDs              *[]string         `json:"skillIds,omitempty"`
	KnowledgeBaseIDs      *[]string         `json:"knowledgeBaseIds,omitempty"`
	Visibility            *string           `json:"visibility,omitempty"`
	Status                *string           `json:"status,omitempty"`
	Scope                 *string           `json:"scope,omitempty"`
	DefaultPersonaID      *uint64           `json:"defaultPersonaId,omitempty"`
	BlueprintRefs         datatypes.JSON    `json:"blueprintRefs,omitempty"`
	IntentCardsRef        datatypes.JSON    `json:"intentCardsRef,omitempty"`
	ToolAllowlist         datatypes.JSON    `json:"toolAllowlist,omitempty"`
	KBStrategy            *string           `json:"kbStrategy,omitempty"`
	Meta                  datatypes.JSONMap `json:"meta,omitempty"`
	ExpectedOwnerPluginID *string           `json:"expectedOwnerPluginId,omitempty"`
}

func (h *AgentHandler) CreateAgent(c *gin.Context) {
	var req createAgentReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	callerPluginID := callerPluginIDFromAudience(c)
	reqOwnerPluginID := strings.TrimSpace(req.OwnerPluginID)
	if callerPluginID != "" {
		if reqOwnerPluginID != "" && !strings.EqualFold(reqOwnerPluginID, callerPluginID) {
			dtoRequest.ResponseError(c, 403, "agent.owner_forbidden", nil)
			return
		}
		reqOwnerPluginID = callerPluginID
	}
	var ownerPluginID *string
	if reqOwnerPluginID != "" {
		ownerPluginID = &reqOwnerPluginID
	}
	managedByPlugin := ownerPluginID != nil
	if req.ManagedByPlugin != nil {
		managedByPlugin = *req.ManagedByPlugin
	}
	if managedByPlugin && ownerPluginID == nil {
		dtoRequest.ResponseError(c, 400, "managedByPlugin=true 时必须提供 ownerPluginId", nil)
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		if ownerPluginID != nil {
			source = "plugin:" + *ownerPluginID
		} else {
			source = "core"
		}
	}
	var ownerTenantUUID *string
	if managedByPlugin {
		ownerTenantUUID = tenantRef
	}

	normalizedKey := normalizeAgentKey(strings.TrimSpace(req.Key))
	if normalizedKey == "" {
		dtoRequest.ResponseError(c, 400, "key 不能为空", nil)
		return
	}

	in := &dbmodel.Agent{
		Env:              req.Env,
		TenantUUID:       tenantRef,
		Key:              normalizedKey,
		Name:             strings.TrimSpace(req.Name),
		Description:      req.Description,
		TypeID:           utils.FirstNonEmpty(req.TypeID, extractMetaString(req.Meta, "type_id"), extractMetaString(req.Meta, "typeId")),
		Scene:            utils.FirstNonEmpty(req.Scene, extractMetaString(req.Meta, "scene")),
		PromptSeed:       utils.FirstNonEmpty(req.PromptSeed, extractMetaString(req.Meta, "prompt_seed"), extractMetaString(req.Meta, "promptSeed")),
		Persona:          utils.FirstNonEmpty(req.Persona, extractMetaPersona(req.Meta)),
		Source:           source,
		OwnerPluginID:    ownerPluginID,
		OwnerTenantUUID:  ownerTenantUUID,
		ManagedByPlugin:  managedByPlugin,
		Scope:            utils.FirstNonEmpty(req.Scope, "tenant"),
		Visibility:       utils.FirstNonEmpty(req.Visibility, "tenant"),
		Status:           utils.FirstNonEmpty(req.Status, "draft"),
		DefaultPersonaID: req.DefaultPersonaID,
		BlueprintRefs:    req.BlueprintRefs,
		IntentCardsRef:   req.IntentCardsRef,
		ToolAllowlist:    req.ToolAllowlist,
		KBStrategy:       utils.FirstNonEmpty(req.KBStrategy, "union"),
		Meta:             req.Meta,
	}
	out, err := h.srv.Create(c.Request.Context(), req.Env, tenantRef, in)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	skillIDs := firstNonEmptyStringSlice(req.SkillIDs, extractMetaStringSlice(req.Meta, "skill_ids", "skillIds"), extractMetaParameterStringSlice(req.Meta, "skill_ids", "skillIds"))
	knowledgeIDs := firstNonEmptyStringSlice(req.KnowledgeBaseIDs, extractMetaStringSlice(req.Meta, "knowledge_base_ids", "knowledgeBaseIds"), extractMetaParameterStringSlice(req.Meta, "knowledge_base_ids", "knowledgeBaseIds"))
	if err := h.srv.ReplaceSkillBindings(c.Request.Context(), req.Env, tenantRef, out.ID, skillIDs); err != nil {
		dtoRequest.ResponseError(c, 400, "sync skill bindings failed", err)
		return
	}
	if err := h.srv.ReplaceKnowledgeBindings(c.Request.Context(), req.Env, tenantRef, out.ID, knowledgeIDs); err != nil {
		dtoRequest.ResponseError(c, 400, "sync knowledge bindings failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func normalizeAgentKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return ""
	}
	key = agentKeyNonWordChars.ReplaceAllString(key, "-")
	key = strings.Trim(key, "-_.")
	if key == "" {
		return ""
	}
	if len(key) <= agentKeyMaxLen {
		return key
	}
	sum := sha256.Sum256([]byte(key))
	suffix := hex.EncodeToString(sum[:])[:agentKeyHashHexLength]
	prefixLen := agentKeyMaxLen - 1 - len(suffix)
	if prefixLen < 1 {
		prefixLen = 1
	}
	prefix := strings.Trim(key[:prefixLen], "-_.")
	if prefix == "" {
		prefix = "agent"
	}
	return prefix + "-" + suffix
}

func (h *AgentHandler) ListAgents(c *gin.Context) {
	envVal := strings.TrimSpace(c.Query("env"))
	if envVal == "" {
		if e := reqctx.GetEnv(c.Request.Context()); e != "" {
			envVal = e
		} else {
			envVal = "dev" // 你的种子数据是 dev
		}
	}

	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	callerPluginID := callerPluginIDFromAudience(c)

	var statuses []string
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		for _, it := range strings.Split(s, ",") {
			if v := strings.TrimSpace(it); v != "" {
				statuses = append(statuses, v)
			}
		}
	}
	ownerPluginID := strings.TrimSpace(c.Query("owner_plugin_id"))
	if callerPluginID != "" {
		if ownerPluginID != "" && !strings.EqualFold(ownerPluginID, callerPluginID) {
			dtoRequest.ResponseError(c, 403, "agent.owner_forbidden", nil)
			return
		}
		ownerPluginID = callerPluginID
	}

	list, err := h.srv.List(c.Request.Context(), envVal, tenantRef, ownerPluginID, statuses...)
	if err != nil {
		dtoRequest.ResponseError(c, 500, "查询失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": list})
}

func (h *AgentHandler) GetAgent(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	callerPluginID := callerPluginIDFromAudience(c)
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	out, err := h.srv.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	if callerPluginID != "" && !agentOwnedByPlugin(out, callerPluginID) {
		dtoRequest.ResponseError(c, 403, "agent.owner_forbidden", nil)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func (h *AgentHandler) UpdateAgent(c *gin.Context) {
	var req updateAgentReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	callerPluginID := callerPluginIDFromAudience(c)
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	exist, err := h.srv.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	if req.SkillIDs == nil {
		if metaSkillIDs := firstNonEmptyStringSlice(extractMetaStringSlice(req.Meta, "skill_ids", "skillIds"), extractMetaParameterStringSlice(req.Meta, "skill_ids", "skillIds")); len(metaSkillIDs) > 0 {
			req.SkillIDs = &metaSkillIDs
		}
	}
	if req.KnowledgeBaseIDs == nil {
		if metaKBIDs := firstNonEmptyStringSlice(extractMetaStringSlice(req.Meta, "knowledge_base_ids", "knowledgeBaseIds"), extractMetaParameterStringSlice(req.Meta, "knowledge_base_ids", "knowledgeBaseIds")); len(metaKBIDs) > 0 {
			req.KnowledgeBaseIDs = &metaKBIDs
		}
	}

	patch := agentSvc.AgentPatch{
		Name:                  req.Name,
		Description:           req.Description,
		TypeID:                req.TypeID,
		Scene:                 req.Scene,
		PromptSeed:            req.PromptSeed,
		Persona:               req.Persona,
		SkillIDs:              req.SkillIDs,
		KnowledgeBaseUUIDs:    req.KnowledgeBaseIDs,
		Visibility:            req.Visibility,
		Status:                req.Status,
		Scope:                 req.Scope,
		DefaultPersonaID:      req.DefaultPersonaID,
		BlueprintRefs:         req.BlueprintRefs,
		IntentCardsRef:        req.IntentCardsRef,
		ToolAllowlist:         req.ToolAllowlist,
		KBStrategy:            req.KBStrategy,
		Meta:                  req.Meta,
		ExpectedOwnerPluginID: req.ExpectedOwnerPluginID,
		CallerPluginID:        callerPluginID,
	}
	out, err := h.srv.Update(c.Request.Context(), env, tenantRef, exist.ID, patch)
	if err != nil {
		if errors.Is(err, agentSvc.ErrAgentOwnerForbidden) {
			dtoRequest.ResponseError(c, 403, "agent.owner_forbidden", nil)
			return
		}
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func extractMetaString(meta datatypes.JSONMap, key string) string {
	if len(meta) == 0 || strings.TrimSpace(key) == "" {
		return ""
	}
	v, ok := meta[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func extractMetaPersona(meta datatypes.JSONMap) string {
	if p := extractMetaString(meta, "persona"); p != "" {
		return p
	}
	raw, ok := meta["parameters"]
	if !ok {
		return ""
	}
	switch m := raw.(type) {
	case map[string]interface{}:
		return strings.TrimSpace(fmt.Sprint(m["persona"]))
	default:
		return ""
	}
}

func extractMetaStringSlice(meta datatypes.JSONMap, keys ...string) []string {
	if len(meta) == 0 || len(keys) == 0 {
		return nil
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if raw, ok := meta[key]; ok {
			if out := parseStringSlice(raw); len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

func extractMetaParameterStringSlice(meta datatypes.JSONMap, keys ...string) []string {
	if len(meta) == 0 || len(keys) == 0 {
		return nil
	}
	raw, ok := meta["parameters"]
	if !ok {
		return nil
	}
	params, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if out := parseStringSlice(params[key]); len(out) > 0 {
			return out
		}
	}
	return nil
}

func parseStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		return normalizeStringSlice(v)
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, strings.TrimSpace(fmt.Sprint(item)))
		}
		return normalizeStringSlice(items)
	default:
		return nil
	}
}

func firstNonEmptyStringSlice(candidates ...[]string) []string {
	for _, c := range candidates {
		if out := normalizeStringSlice(c); len(out) > 0 {
			return out
		}
	}
	return nil
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *AgentHandler) EnableAgent(c *gin.Context)  { h.setAgentStatus(c, dbmodel.AgentStatusActive) }
func (h *AgentHandler) DisableAgent(c *gin.Context) { h.setAgentStatus(c, dbmodel.AgentStatusDisabled) }
func (h *AgentHandler) setAgentStatus(c *gin.Context, status string) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	callerPluginID := callerPluginIDFromAudience(c)
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	exist, err := h.srv.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	if callerPluginID != "" && !agentOwnedByPlugin(exist, callerPluginID) {
		dtoRequest.ResponseError(c, 403, "agent.owner_forbidden", nil)
		return
	}
	if err := h.srv.SetStatus(c.Request.Context(), env, tenantRef, exist.ID, status); err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *AgentHandler) DeleteAgent(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	callerPluginID := callerPluginIDFromAudience(c)
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	exist, err := h.srv.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	var expectedOwnerPluginID *string
	if v := strings.TrimSpace(c.Query("owner_plugin_id")); v != "" {
		expectedOwnerPluginID = &v
	}
	if err := h.srv.Delete(c.Request.Context(), env, tenantRef, exist.ID, expectedOwnerPluginID, callerPluginID); err != nil {
		if errors.Is(err, agentSvc.ErrAgentOwnerForbidden) {
			dtoRequest.ResponseError(c, 403, "agent.owner_forbidden", nil)
			return
		}
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

func callerPluginIDFromAudience(c *gin.Context) string {
	aud := strings.TrimSpace(reqctx.GetAudience(c.Request.Context()))
	lower := strings.ToLower(aud)
	if strings.HasPrefix(lower, "plugin:") {
		return strings.TrimSpace(aud[len("plugin:"):])
	}
	return ""
}

func agentOwnedByPlugin(agent *dbmodel.Agent, pluginID string) bool {
	if agent == nil || strings.TrimSpace(pluginID) == "" {
		return false
	}
	if agent.OwnerPluginID != nil && strings.EqualFold(strings.TrimSpace(*agent.OwnerPluginID), strings.TrimSpace(pluginID)) {
		return true
	}
	src := strings.TrimSpace(agent.Source)
	lower := strings.ToLower(src)
	if strings.HasPrefix(lower, "plugin:") && strings.EqualFold(strings.TrimSpace(src[len("plugin:"):]), strings.TrimSpace(pluginID)) {
		return true
	}
	return false
}

// ====== 管理接口：Agent 级 AI Setting ======

type upsertAgentAISettingReq struct {
	Env           string            `json:"env" validate:"required"`
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	Params        datatypes.JSONMap `json:"params"`
	OverrideFlags datatypes.JSONMap `json:"overrideFlags"`
	QuotaPolicy   datatypes.JSONMap `json:"quotaPolicy"`
}

func (h *AgentHandler) GetAgentAISetting(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	exist, err := h.srv.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	setting, err := h.srv.GetAgentAISetting(c.Request.Context(), env, tenantRef, exist.ID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	dtoRequest.ResponseSuccess(c, setting)
}

func (h *AgentHandler) UpsertAgentAISetting(c *gin.Context) {
	var req upsertAgentAISettingReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	exist, err := h.srv.GetByUUID(c.Request.Context(), req.Env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}

	in := &dbmodel.AgentSetting{
		Env:           req.Env,
		AgentID:       exist.ID,
		Provider:      strings.TrimSpace(req.Provider),
		Model:         strings.TrimSpace(req.Model),
		Params:        req.Params,
		OverrideFlags: req.OverrideFlags,
		QuotaPolicy:   req.QuotaPolicy,
		HealthStatus:  "unknown",
		HealthInfo:    datatypes.JSONMap{},
	}
	out, err := h.srv.UpsertAgentAISetting(c.Request.Context(), req.Env, tenantRef, in)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func (h *AgentHandler) DeleteAgentAISetting(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	exist, err := h.srv.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	if err := h.srv.DeleteAgentAISetting(c.Request.Context(), env, tenantRef, exist.ID); err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *AgentHandler) AgentHealthCheck(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "uuid 非法", nil)
		return
	}
	exist, err := h.srv.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	info, err := h.srv.HealthCheck(c.Request.Context(), env, tenantRef, exist.ID)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "检查失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "probe": info})
}
