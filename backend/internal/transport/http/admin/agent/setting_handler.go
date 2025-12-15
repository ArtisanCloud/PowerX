package agent

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/ArtisanCloud/PowerX/pkg/utils"

	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

// 依赖注入
type AgentSettingHandler struct {
	svc   *agentSvc.AgentSettingService
	audit auditsvc.Service
}

const (
	auditSourceAgentSettingHandler = "agent.setting_handler"
	auditResourceTypeModalityTest  = "agent.modality_test"
	auditOpTestConnection          = "agent.settings.test_connection"
	auditOpTestQuickCall           = "agent.settings.test_quick_call"
	auditMessageLimit              = 240
)

func NewAgentSettingHandler(deps *shared.Deps) *AgentSettingHandler {
	return &AgentSettingHandler{
		svc:   agentSvc.NewAgentSettingService(deps.DB),
		audit: deps.AuditSvc,
	}
}

type baseConn struct {
	Name            string `form:"name"`
	Provider        string `json:"provider" validate:"required"`
	Model           string `json:"model"    validate:"required"`
	APIKey          string `json:"apiKey"`
	BaseURL         string `json:"baseURL"`
	Region          string `json:"region"`
	Organization    string `json:"organization"`
	AzureDeployment string `json:"azureDeployment"`
}
type modLLM struct {
	baseConn
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"maxTokens"`
	TopP        float64 `json:"topP"`
	Stream      bool    `json:"stream"`
}

type modImage struct {
	baseConn
	Size       string `json:"size"`
	Quality    string `json:"quality"`
	Format     string `json:"format"`
	PromptHint string `json:"promptHint"`
}

type modEmbed struct {
	baseConn
	Dimensions int    `json:"dimensions"`
	Truncate   string `json:"truncate"` // none|start|end
	Batch      int    `json:"batch"`
}

type modVideo struct {
	baseConn
	Resolution     string `json:"resolution"`
	FPS            int    `json:"fps"`
	MaxDurationSec int    `json:"maxDurationSec"`
	PromptHint     string `json:"promptHint"`
}

type modAudioTTS struct {
	baseConn
	Voice   string  `json:"voice"`
	Speed   float64 `json:"speed"`
	Format  string  `json:"format"`
	Quality string  `json:"quality"`
}

type modAudioASR struct {
	baseConn
	Language       string  `json:"language"`
	ResponseFormat string  `json:"responseFormat"`
	Temperature    float64 `json:"temperature"`
	Prompt         string  `json:"prompt"`
}

type modRerank struct {
	baseConn
	TopK            int  `json:"topK"`
	ReturnDocuments bool `json:"returnDocuments"`
	MaxChunksPerDoc int  `json:"maxChunksPerDoc"`
}

type saveSettingsReq struct {
	Env       string            `json:"env" validate:"required"`
	Modality  contract.Modality `json:"modality" validate:"required"`
	LLM       *modLLM           `json:"llm,omitempty"`
	Image     *modImage         `json:"image,omitempty"`
	Embedding *modEmbed         `json:"embedding,omitempty"`
	Video     *modVideo         `json:"video,omitempty"`
	AudioTTS  *modAudioTTS      `json:"audio_tts,omitempty"`
	AudioASR  *modAudioASR      `json:"audio_asr,omitempty"`
	Rerank    *modRerank        `json:"rerank,omitempty"`
}

type testReq struct {
	Env       string            `json:"env" validate:"required"`
	Modality  contract.Modality `json:"modality" validate:"required"`
	LLM       *modLLM           `json:"llm,omitempty"`
	Image     *modImage         `json:"image,omitempty"`
	Embedding *modEmbed         `json:"embedding,omitempty"`
	Video     *modVideo         `json:"video,omitempty"`
	AudioTTS  *modAudioTTS      `json:"audio_tts,omitempty"`
	AudioASR  *modAudioASR      `json:"audio_asr,omitempty"`
	Rerank    *modRerank        `json:"rerank,omitempty"`
}

type testCallReq struct {
	Env       string            `json:"env"       validate:"required"`
	Modality  contract.Modality `json:"modality"  validate:"required"`
	Prompt    string            `json:"prompt"`
	LLM       *modLLM           `json:"llm,omitempty"`
	Image     *modImage         `json:"image,omitempty"`
	Embedding *modEmbed         `json:"embedding,omitempty"`
	Video     *modVideo         `json:"video,omitempty"`
	AudioTTS  *modAudioTTS      `json:"audio_tts,omitempty"`
	AudioASR  *modAudioASR      `json:"audio_asr,omitempty"`
	Rerank    *modRerank        `json:"rerank,omitempty"`
}

// ---------- Providers / Models ----------

func (h *AgentSettingHandler) listProviders(c *gin.Context) {
	mod := c.Query("modality")
	list := h.svc.Providers(mod)
	dtoRequest.ResponseSuccess(c, gin.H{"providers": list})
	return
}

func (h *AgentSettingHandler) listModels(c *gin.Context) {
	mod := c.Query("modality")
	prov := c.Query("provider")
	models, err := h.svc.Models(mod, prov)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"models": models})
}

// ---------- Settings ----------

func (h *AgentSettingHandler) saveSettings(c *gin.Context) {
	var req saveSettingsReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	tenantUUID := tenantCtx.UUID()

	// 仅按当前模态做最小校验 + 先严格连通性校验（不读库不解封）
	switch req.Modality {
	case contract.ModLLM:
		if req.LLM == nil || strings.TrimSpace(req.LLM.Provider) == "" || strings.TrimSpace(req.LLM.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm.provider/model 不能为空", nil)
			return
		}
		// 🔒 用本次表单直连校验，失败不落库
		if err := h.svc.PingLLM(
			c.Request.Context(),
			req.Env, tenantRef,
			req.LLM.Provider,
			req.LLM.Model,
			req.LLM.BaseURL,
			req.LLM.APIKey,
		); err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "连通性校验失败", err)
			return
		}
	case contract.ModImage:
		if req.Image == nil || strings.TrimSpace(req.Image.Provider) == "" || strings.TrimSpace(req.Image.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "image.provider/model 不能为空", nil)
			return
		}
	case contract.ModEmbed:
		if req.Embedding == nil || strings.TrimSpace(req.Embedding.Provider) == "" || strings.TrimSpace(req.Embedding.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "embedding.provider/model 不能为空", nil)
			return
		}
	case contract.ModVideo:
		if req.Video == nil || strings.TrimSpace(req.Video.Provider) == "" || strings.TrimSpace(req.Video.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "video.provider/model 不能为空", nil)
			return
		}
	case contract.ModAudioTTS:
		if req.AudioTTS == nil || strings.TrimSpace(req.AudioTTS.Provider) == "" || strings.TrimSpace(req.AudioTTS.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_tts.provider/model 不能为空", nil)
			return
		}
	case contract.ModAudioASR:
		if req.AudioASR == nil || strings.TrimSpace(req.AudioASR.Provider) == "" || strings.TrimSpace(req.AudioASR.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_asr.provider/model 不能为空", nil)
			return
		}
	case contract.ModRerank:
		if req.Rerank == nil || strings.TrimSpace(req.Rerank.Provider) == "" || strings.TrimSpace(req.Rerank.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "rerank.provider/model 不能为空", nil)
			return
		}
	}

	// 组装两张表的实体并落库（Service 内会 SealSensitive + Upsert）
	credName, credProvider, credData, prof := buildEntitiesFromPayload(&req, tenantRef)
	if credName == "" || prof == nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少当前模态设置", nil)
		return
	}
	cred := &dbmodel.AIProviderCredential{
		Env:        req.Env,
		TenantUUID: tenantRef,
		Name:       credName,
		Provider:   credProvider,
		AuthScheme: "bearer",
		Data:       credData,
	}
	if err := h.svc.SaveCredentialAndProfile(c.Request.Context(), req.Env, tenantRef, cred, prof, true); err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "保存失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "tenant_uuid": tenantUUID})
}

// ---------- Tests ----------

func (h *AgentSettingHandler) testConnection(c *gin.Context) {
	var req testReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	tenantUUID := tenantCtx.UUID()

	switch req.Modality {
	case contract.ModLLM:
		if req.LLM == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm 配置不能为空", nil)
			return
		}
		provider := req.LLM.Provider
		model := req.LLM.Model
		err := h.svc.TestConnectionPreferInput(
			c.Request.Context(),
			req.Env, tenantRef,
			string(req.Modality),
			provider, model, req.LLM.BaseURL, req.LLM.APIKey,
		)
		if err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, provider, model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, "连接测试失败", err)
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, provider, model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModImage:
		if req.Image == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "image 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Image.Provider, req.Image.Model, req.Image.BaseURL, req.Image.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Image.Provider, req.Image.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Image.Provider, req.Image.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModEmbed:
		if req.Embedding == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "embedding 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Embedding.Provider, req.Embedding.Model, req.Embedding.BaseURL, req.Embedding.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Embedding.Provider, req.Embedding.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Embedding.Provider, req.Embedding.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModVideo:
		if req.Video == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "video 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Video.Provider, req.Video.Model, req.Video.BaseURL, req.Video.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Video.Provider, req.Video.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Video.Provider, req.Video.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModAudioTTS:
		if req.AudioTTS == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_tts 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, req.AudioTTS.BaseURL, req.AudioTTS.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModAudioASR:
		if req.AudioASR == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_asr 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, req.AudioASR.BaseURL, req.AudioASR.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModRerank:
		if req.Rerank == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "rerank 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Rerank.Provider, req.Rerank.Model, req.Rerank.BaseURL, req.Rerank.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Rerank.Provider, req.Rerank.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Rerank.Provider, req.Rerank.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	default:
		dtoRequest.ResponseError(c, http.StatusBadRequest, "未知模态: "+string(req.Modality), nil)
	}
}

func (h *AgentSettingHandler) testQuickCall(c *gin.Context) {
	var req testCallReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	tenantUUID := tenantCtx.UUID()
	switch req.Modality {
	case contract.ModLLM:
		if req.LLM == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm 配置不能为空", nil)
			return
		}
		out, err := h.svc.QuickCallLLM(
			c.Request.Context(),
			req.Env, tenantRef,
			req.LLM.Provider, req.LLM.Model, req.LLM.BaseURL, req.LLM.APIKey,
			req.LLM.Temperature, req.LLM.MaxTokens,
			req.Prompt,
		)
		if err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.LLM.Provider, req.LLM.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.LLM.Provider, req.LLM.Model, true, snippet(out, auditMessageLimit))
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": out})
	case contract.ModImage:
		if req.Image == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "image 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Image.Provider, req.Image.Model, req.Image.BaseURL, req.Image.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Image.Provider, req.Image.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		msg := describeImageQuickCall(req.Image, req.Prompt)
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Image.Provider, req.Image.Model, true, msg)
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": msg})
	case contract.ModEmbed:
		if req.Embedding == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "embedding 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Embedding.Provider, req.Embedding.Model, req.Embedding.BaseURL, req.Embedding.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Embedding.Provider, req.Embedding.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		msg := describeEmbeddingQuickCall(req.Embedding)
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Embedding.Provider, req.Embedding.Model, true, msg)
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": msg})
	case contract.ModVideo:
		if req.Video == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "video 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Video.Provider, req.Video.Model, req.Video.BaseURL, req.Video.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Video.Provider, req.Video.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		msg := describeVideoQuickCall(req.Video, req.Prompt)
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Video.Provider, req.Video.Model, true, msg)
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": msg})
	case contract.ModAudioTTS:
		if req.AudioTTS == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_tts 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, req.AudioTTS.BaseURL, req.AudioTTS.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		msg := describeAudioTTSQuickCall(req.AudioTTS, req.Prompt)
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, true, msg)
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": msg})
	case contract.ModAudioASR:
		if req.AudioASR == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_asr 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, req.AudioASR.BaseURL, req.AudioASR.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		msg := describeAudioASRQuickCall(req.AudioASR)
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, true, msg)
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": msg})
	case contract.ModRerank:
		if req.Rerank == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "rerank 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Rerank.Provider, req.Rerank.Model, req.Rerank.BaseURL, req.Rerank.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Rerank.Provider, req.Rerank.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		msg := describeRerankQuickCall(req.Rerank)
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Rerank.Provider, req.Rerank.Model, true, msg)
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": msg})
	default:
		dtoRequest.ResponseError(c, http.StatusBadRequest, "未知模态: "+string(req.Modality), nil)
	}
}

// ---------- helpers（保留你已有的） ----------

func buildEntitiesFromPayload(req *saveSettingsReq, tenantUUID *string) (credName, credProvider string, cred datatypes.JSONMap, prof *dbmodel.AIModelProfile) {
	cred = datatypes.JSONMap{}

	switch req.Modality {
	case contract.ModLLM:
		if req.LLM == nil {
			return
		}
		// 基本必填：provider/model
		p := strings.TrimSpace(req.LLM.Provider)
		m := strings.TrimSpace(req.LLM.Model)
		if p == "" || m == "" {
			return
		}

		credProvider = req.LLM.Provider
		credName = utils.Slug(req.Env + "-" + req.LLM.Provider) // e.g. "default-ollama"
		cred = datatypes.JSONMap{
			"api_key":          req.LLM.APIKey, // 允许为空（本地 ollama 不需要）
			"base_url":         req.LLM.BaseURL,
			"region":           req.LLM.Region,
			"organization":     req.LLM.Organization,
			"azure_deployment": req.LLM.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "llm",
			Provider:   req.LLM.Provider,
			Model:      req.LLM.Model,
			Defaults: datatypes.JSONMap{
				"temperature": req.LLM.Temperature,
				"maxTokens":   req.LLM.MaxTokens,
				"topP":        req.LLM.TopP,
				"stream":      req.LLM.Stream,
			},
			Tags: []string{"llm"},
		}

	case contract.ModImage:
		if req.Image == nil {
			return
		}
		p := strings.TrimSpace(req.Image.Provider)
		m := strings.TrimSpace(req.Image.Model)
		if p == "" || m == "" {
			return
		}
		credProvider = req.Image.Provider
		credName = utils.Slug(req.Env + "-" + req.Image.Provider)
		cred = datatypes.JSONMap{
			"api_key":          req.Image.APIKey,
			"base_url":         req.Image.BaseURL,
			"region":           req.Image.Region,
			"organization":     req.Image.Organization,
			"azure_deployment": req.Image.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "image",
			Provider:   req.Image.Provider,
			Model:      req.Image.Model,
			Defaults: datatypes.JSONMap{
				"size":       req.Image.Size,
				"quality":    req.Image.Quality,
				"format":     req.Image.Format,
				"promptHint": req.Image.PromptHint,
			},
			Tags: []string{"image"},
		}

	case contract.ModEmbed:
		if req.Embedding == nil {
			return
		}
		p := strings.TrimSpace(req.Embedding.Provider)
		m := strings.TrimSpace(req.Embedding.Model)
		if p == "" || m == "" {
			return
		}
		credProvider = req.Embedding.Provider
		credName = utils.Slug(req.Env + "-" + req.Embedding.Provider)
		cred = datatypes.JSONMap{
			"api_key":          req.Embedding.APIKey,
			"base_url":         req.Embedding.BaseURL,
			"region":           req.Embedding.Region,
			"organization":     req.Embedding.Organization,
			"azure_deployment": req.Embedding.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "embedding",
			Provider:   req.Embedding.Provider,
			Model:      req.Embedding.Model,
			Defaults: datatypes.JSONMap{
				"dimensions": req.Embedding.Dimensions,
				"truncate":   req.Embedding.Truncate,
				"batch":      req.Embedding.Batch,
			},
			Tags: []string{"embedding"},
		}

	case contract.ModVideo:
		if req.Video == nil {
			return
		}
		p := strings.TrimSpace(req.Video.Provider)
		m := strings.TrimSpace(req.Video.Model)
		if p == "" || m == "" {
			return
		}
		credProvider = req.Video.Provider
		credName = utils.Slug(req.Env + "-" + req.Video.Provider)
		cred = datatypes.JSONMap{
			"api_key":          req.Video.APIKey,
			"base_url":         req.Video.BaseURL,
			"region":           req.Video.Region,
			"organization":     req.Video.Organization,
			"azure_deployment": req.Video.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "video",
			Provider:   req.Video.Provider,
			Model:      req.Video.Model,
			Defaults: datatypes.JSONMap{
				"resolution":     req.Video.Resolution,
				"fps":            req.Video.FPS,
				"maxDurationSec": req.Video.MaxDurationSec,
				"promptHint":     req.Video.PromptHint,
			},
			Tags: []string{"video"},
		}

	case contract.ModAudioTTS:
		if req.AudioTTS == nil {
			return
		}
		p := strings.TrimSpace(req.AudioTTS.Provider)
		m := strings.TrimSpace(req.AudioTTS.Model)
		if p == "" || m == "" {
			return
		}
		credProvider = req.AudioTTS.Provider
		credName = utils.Slug(req.Env + "-" + req.AudioTTS.Provider)
		cred = datatypes.JSONMap{
			"api_key":          req.AudioTTS.APIKey,
			"base_url":         req.AudioTTS.BaseURL,
			"region":           req.AudioTTS.Region,
			"organization":     req.AudioTTS.Organization,
			"azure_deployment": req.AudioTTS.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "audio_tts",
			Provider:   req.AudioTTS.Provider,
			Model:      req.AudioTTS.Model,
			Defaults: datatypes.JSONMap{
				"voice":   req.AudioTTS.Voice,
				"speed":   req.AudioTTS.Speed,
				"format":  req.AudioTTS.Format,
				"quality": req.AudioTTS.Quality,
			},
			Tags: []string{"audio", "tts"},
		}

	case contract.ModAudioASR:
		if req.AudioASR == nil {
			return
		}
		p := strings.TrimSpace(req.AudioASR.Provider)
		m := strings.TrimSpace(req.AudioASR.Model)
		if p == "" || m == "" {
			return
		}
		credProvider = req.AudioASR.Provider
		credName = utils.Slug(req.Env + "-" + req.AudioASR.Provider)
		cred = datatypes.JSONMap{
			"api_key":          req.AudioASR.APIKey,
			"base_url":         req.AudioASR.BaseURL,
			"region":           req.AudioASR.Region,
			"organization":     req.AudioASR.Organization,
			"azure_deployment": req.AudioASR.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "audio_asr",
			Provider:   req.AudioASR.Provider,
			Model:      req.AudioASR.Model,
			Defaults: datatypes.JSONMap{
				"language":       req.AudioASR.Language,
				"responseFormat": req.AudioASR.ResponseFormat,
				"temperature":    req.AudioASR.Temperature,
				"prompt":         req.AudioASR.Prompt,
			},
			Tags: []string{"audio", "asr"},
		}

	case contract.ModRerank:
		if req.Rerank == nil {
			return
		}
		p := strings.TrimSpace(req.Rerank.Provider)
		m := strings.TrimSpace(req.Rerank.Model)
		if p == "" || m == "" {
			return
		}
		credProvider = req.Rerank.Provider
		credName = utils.Slug(req.Env + "-" + req.Rerank.Provider)
		cred = datatypes.JSONMap{
			"api_key":          req.Rerank.APIKey,
			"base_url":         req.Rerank.BaseURL,
			"region":           req.Rerank.Region,
			"organization":     req.Rerank.Organization,
			"azure_deployment": req.Rerank.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "rerank",
			Provider:   req.Rerank.Provider,
			Model:      req.Rerank.Model,
			Defaults: datatypes.JSONMap{
				"topK":            req.Rerank.TopK,
				"returnDocuments": req.Rerank.ReturnDocuments,
				"maxChunksPerDoc": req.Rerank.MaxChunksPerDoc,
			},
			Tags: []string{"rerank"},
		}
	}
	return
}

func describeImageQuickCall(m *modImage, prompt string) string {
	return fmt.Sprintf("已校验 Image provider=%s model=%s size=%s quality=%s format=%s prompt=%s",
		m.Provider, m.Model, defaultIfEmpty(m.Size, "1024x1024"), defaultIfEmpty(m.Quality, "standard"), defaultIfEmpty(m.Format, "png"), snippet(prompt, 60))
}

func describeEmbeddingQuickCall(m *modEmbed) string {
	return fmt.Sprintf("已校验 Embedding provider=%s model=%s dims=%d truncate=%s batch=%d",
		m.Provider, m.Model, m.Dimensions, defaultIfEmpty(m.Truncate, "none"), m.Batch)
}

func describeVideoQuickCall(m *modVideo, prompt string) string {
	return fmt.Sprintf("已校验 Video provider=%s model=%s res=%s fps=%d maxDuration=%ds prompt=%s",
		m.Provider, m.Model, defaultIfEmpty(m.Resolution, "720p"), m.FPS, m.MaxDurationSec, snippet(prompt, 60))
}

func describeAudioTTSQuickCall(m *modAudioTTS, prompt string) string {
	return fmt.Sprintf("已校验 AudioTTS provider=%s model=%s voice=%s speed=%.2f format=%s prompt=%s",
		m.Provider, m.Model, defaultIfEmpty(m.Voice, "default"), m.Speed, defaultIfEmpty(m.Format, "mp3"), snippet(prompt, 60))
}

func describeAudioASRQuickCall(m *modAudioASR) string {
	return fmt.Sprintf("已校验 AudioASR provider=%s model=%s language=%s format=%s temperature=%.2f",
		m.Provider, m.Model, defaultIfEmpty(m.Language, "auto"), defaultIfEmpty(m.ResponseFormat, "json"), m.Temperature)
}

func describeRerankQuickCall(m *modRerank) string {
	return fmt.Sprintf("已校验 Rerank provider=%s model=%s topK=%d returnDocs=%t maxChunksPerDoc=%d",
		m.Provider, m.Model, m.TopK, m.ReturnDocuments, m.MaxChunksPerDoc)
}

func snippet(s string, limit int) string {
	txt := strings.TrimSpace(s)
	if txt == "" {
		return "(empty)"
	}
	runes := []rune(txt)
	if len(runes) <= limit {
		return txt
	}
	return string(runes[:limit]) + "..."
}

func defaultIfEmpty(val string, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

func (h *AgentSettingHandler) getActiveProfile(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	mod := strings.TrimSpace(strings.ToLower(c.DefaultQuery("modality", "llm")))
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	prof, err := h.svc.GetActiveProfile(c.Request.Context(), env, tenantRef, mod)
	if err != nil || prof == nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "未找到激活画像", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":      env,
		"modality": mod,
		"profile":  prof, // ✅ 只有一条
	})
}

type setActiveReq struct {
	Env      string `json:"env" validate:"required"`
	Modality string `json:"modality" validate:"required"`
	Provider string `json:"provider" validate:"required"`
	Model    string `json:"model" validate:"required"`
}

func (h *AgentSettingHandler) setActiveProfile(c *gin.Context) {
	var req setActiveReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	if err := h.svc.SetActiveProfile(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Provider, req.Model); err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "设置失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

// GET /api/agents/settings/profiles?env=default&modalities=llm,image
func (h *AgentSettingHandler) listProfiles(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	var mods []string
	if s := strings.TrimSpace(c.Query("modalities")); s != "" {
		for _, m := range strings.Split(s, ",") {
			m = strings.TrimSpace(strings.ToLower(m))
			if m != "" {
				mods = append(mods, m)
			}
		}
	}

	out, err := h.svc.ListProfiles(c.Request.Context(), env, tenantRef, mods...)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	// 直接返回 GORM 实体数组（Env/TenantUUID 因为 json:"-" 不会出现在响应里）
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":      env,
		"profiles": out,
	})
}

// （可选）GET /api/agents/settings/credentials?env=default
func (h *AgentSettingHandler) listCredentials(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	out, err := h.svc.ListCredentials(c.Request.Context(), env, tenantRef)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	for i := range out {
		redactCredentialSecrets(&out[i])
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":         env,
		"credentials": out,
	})
}

func redactCredentialSecrets(cred *dbmodel.AIProviderCredential) {
	if cred == nil || cred.Data == nil {
		return
	}
	sensitive := []string{"api_key", "secret", "client_secret", "access_token"}
	copied := datatypes.JSONMap{}
	for k, v := range cred.Data {
		copied[k] = v
	}
	for _, key := range sensitive {
		delete(copied, key)
	}
	cred.Data = copied
}

func (h *AgentSettingHandler) emitAuditEvent(
	c *gin.Context,
	tenantUUID string,
	env string,
	operation string,
	modality contract.Modality,
	provider string,
	model string,
	success bool,
	message string,
) {
	if h.audit == nil || c == nil {
		return
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	resourceID := fmt.Sprintf(
		"%s:%s:%s",
		strings.ToLower(string(modality)),
		defaultIfEmpty(provider, "unknown"),
		defaultIfEmpty(model, "unknown"),
	)
	resourceName := strings.Trim(strings.Trim(fmt.Sprintf("%s/%s", provider, model), "/"), " ")
	if resourceName == "" || resourceName == "/" {
		resourceName = ""
	}

	outcome := "SUCCESS"
	severity := "INFO"
	if !success {
		outcome = "FAIL"
		severity = "WARN"
	}

	meta := map[string]any{
		"env":      env,
		"modality": string(modality),
		"provider": provider,
		"model":    model,
		"success":  success,
		"message":  snippet(message, auditMessageLimit),
	}

	var clientIP *string
	if ip := strings.TrimSpace(c.ClientIP()); ip != "" {
		clientIP = &ip
	}

	_ = h.audit.Emit(c.Request.Context(), &dbmaudit.AuditEvent{
		TenantUUID:    tenantUUID,
		Source:        auditSourceAgentSettingHandler,
		Operation:     operation,
		ResourceType:  auditResourceTypeModalityTest,
		ResourceID:    resourceID,
		ResourceName:  resourceName,
		Outcome:       outcome,
		Severity:      severity,
		ClientIP:      clientIP,
		ClientUA:      c.Request.UserAgent(),
		Meta:          datatypes.JSON(utils.MustJSONBytes(meta)),
		OccurredAt:    time.Now(),
		CorrelationID: auditsvc.CorrelationIDFromContext(c.Request.Context()),
	})
}
