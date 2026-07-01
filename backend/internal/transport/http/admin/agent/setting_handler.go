package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/migration"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	pgvectorcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 依赖注入
type AgentSettingHandler struct {
	svc            *agentSvc.AgentSettingService
	aiSvc          *aisvc.Service
	skillPolicySvc *skillservice.SourcePolicyAdminService
	ctxOptSvc      *agentSvc.ContextOptimizerConfigService
	audit          auditsvc.Service
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
		svc:            agentSvc.NewAgentSettingService(deps.DB),
		aiSvc:          aisvc.NewService(deps.DB),
		skillPolicySvc: skillservice.NewSourcePolicyAdminService(deps.DB),
		ctxOptSvc:      agentSvc.NewContextOptimizerConfigService(deps.DB),
		audit:          deps.AuditSvc,
	}
}

type baseConn struct {
	Name            string `form:"name"`
	Provider        string `json:"provider" validate:"required"`
	App             string `json:"app"`
	Model           string `json:"model"    validate:"required"`
	AuthMode        string `json:"authMode"`
	APIKey          string `json:"apiKey"`
	SecretID        string `json:"secretId"`
	SecretKey       string `json:"secretKey"`
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

type modModel3D struct {
	baseConn
	OutputFormat string `json:"outputFormat"`
	PromptHint   string `json:"promptHint"`
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
	Model3D   *modModel3D       `json:"model3d,omitempty"`
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
	Model3D   *modModel3D       `json:"model3d,omitempty"`
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
	Model3D   *modModel3D       `json:"model3d,omitempty"`
	AudioTTS  *modAudioTTS      `json:"audio_tts,omitempty"`
	AudioASR  *modAudioASR      `json:"audio_asr,omitempty"`
	Rerank    *modRerank        `json:"rerank,omitempty"`
}

func normalizeBaseConn(v *baseConn) {
	if v == nil {
		return
	}
	v.Name = strings.TrimSpace(v.Name)
	v.Provider = strings.TrimSpace(v.Provider)
	v.App = strings.TrimSpace(v.App)
	v.Model = strings.TrimSpace(v.Model)
	v.AuthMode = strings.TrimSpace(v.AuthMode)
	v.APIKey = strings.TrimSpace(v.APIKey)
	v.SecretID = strings.TrimSpace(v.SecretID)
	v.SecretKey = strings.TrimSpace(v.SecretKey)
	v.BaseURL = strings.TrimSpace(v.BaseURL)
	v.Region = strings.TrimSpace(v.Region)
	v.Organization = strings.TrimSpace(v.Organization)
	v.AzureDeployment = strings.TrimSpace(v.AzureDeployment)
}

func normalizeSettingsRequest(req *saveSettingsReq) {
	if req == nil {
		return
	}
	req.Env = strings.TrimSpace(req.Env)
	normalizeBaseConnFromAny(req.LLM, req.Image, req.Embedding, req.Video, req.Model3D, req.AudioTTS, req.AudioASR, req.Rerank)
}

func normalizeTestRequest(req *testReq) {
	if req == nil {
		return
	}
	req.Env = strings.TrimSpace(req.Env)
	normalizeBaseConnFromAny(req.LLM, req.Image, req.Embedding, req.Video, req.Model3D, req.AudioTTS, req.AudioASR, req.Rerank)
}

func normalizeTestCallRequest(req *testCallReq) {
	if req == nil {
		return
	}
	req.Env = strings.TrimSpace(req.Env)
	req.Prompt = strings.TrimSpace(req.Prompt)
	normalizeBaseConnFromAny(req.LLM, req.Image, req.Embedding, req.Video, req.Model3D, req.AudioTTS, req.AudioASR, req.Rerank)
}

func normalizeBaseConnFromAny(items ...any) {
	for _, item := range items {
		switch v := item.(type) {
		case *modLLM:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		case *modImage:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		case *modEmbed:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		case *modVideo:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		case *modModel3D:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		case *modAudioTTS:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		case *modAudioASR:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		case *modRerank:
			if v != nil {
				normalizeBaseConn(&v.baseConn)
			}
		}
	}
}

// ---------- Providers / Models ----------

func (h *AgentSettingHandler) listProviders(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	mod := strings.TrimSpace(strings.ToLower(c.Query("modality")))
	if mod == "" {
		mod = "llm"
	}
	list := h.svc.Providers(mod)
	reg := catalog.GetGlobalAIRegister()

	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	tenantUUID := tenantCtx.UUID()

	// 1) 已配置：credentials 是否存在（仅按 provider 维度）
	creds, err := h.svc.ListCredentials(c.Request.Context(), env, tenantRef)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询凭据失败", err)
		return
	}
	configured := map[string]struct{}{}
	for i := range creds {
		p := strings.ToLower(strings.TrimSpace(creds[i].Provider))
		if p != "" {
			// 兼容历史 provider/别名（例如 yuanbao -> hunyuan）
			if canon := reg.CanonicalProvider(p); canon != "" {
				p = canon
			}
			configured[p] = struct{}{}
		}
	}

	// 2) 连接测试：记录在租户设置里（env+modality 维度）
	healthMap, _, err := h.svc.GetTenantProviderHealthMap(c.Request.Context(), tenantUUID, env, mod)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询连接测试结果失败", err)
		return
	}
	// 同样把 healthMap key 规范化到 canonical provider id
	healthNorm := map[string]agentSvc.ProviderHealthRecord{}
	for k, v := range healthMap {
		p := strings.ToLower(strings.TrimSpace(k))
		if p == "" {
			continue
		}
		if canon := reg.CanonicalProvider(p); canon != "" {
			p = canon
		}
		// 冲突时保留第一次写入（更贴近用户原始配置）
		if _, ok := healthNorm[p]; ok {
			continue
		}
		healthNorm[p] = v
	}

	type providerView struct {
		ID   string `json:"ID"`
		Name string `json:"Name"`
		Apps []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"apps,omitempty"`
		Configured bool                           `json:"configured"`
		Health     *agentSvc.ProviderHealthRecord `json:"health,omitempty"`
		Auth       *struct {
			Scheme   string            `json:"scheme,omitempty"`
			Fields   []string          `json:"fields,omitempty"`
			Defaults map[string]string `json:"defaults,omitempty"`
			Modes    []struct {
				ID       string            `json:"id"`
				Label    string            `json:"label,omitempty"`
				Scheme   string            `json:"scheme,omitempty"`
				Fields   []string          `json:"fields,omitempty"`
				Defaults map[string]string `json:"defaults,omitempty"`
			} `json:"modes,omitempty"`
		} `json:"auth,omitempty"`
	}
	out := make([]providerView, 0, len(list))
	for _, it := range list {
		p := strings.ToLower(strings.TrimSpace(it.ID))
		_, ok := configured[p]
		var hr *agentSvc.ProviderHealthRecord
		if v, hit := healthNorm[p]; hit {
			clone := v
			hr = &clone
		}
		var authView *struct {
			Scheme   string            `json:"scheme,omitempty"`
			Fields   []string          `json:"fields,omitempty"`
			Defaults map[string]string `json:"defaults,omitempty"`
			Modes    []struct {
				ID       string            `json:"id"`
				Label    string            `json:"label,omitempty"`
				Scheme   string            `json:"scheme,omitempty"`
				Fields   []string          `json:"fields,omitempty"`
				Defaults map[string]string `json:"defaults,omitempty"`
			} `json:"modes,omitempty"`
		}
		if m, ok2 := reg.Manifest(it.ID); ok2 && m != nil {
			authView = &struct {
				Scheme   string            `json:"scheme,omitempty"`
				Fields   []string          `json:"fields,omitempty"`
				Defaults map[string]string `json:"defaults,omitempty"`
				Modes    []struct {
					ID       string            `json:"id"`
					Label    string            `json:"label,omitempty"`
					Scheme   string            `json:"scheme,omitempty"`
					Fields   []string          `json:"fields,omitempty"`
					Defaults map[string]string `json:"defaults,omitempty"`
				} `json:"modes,omitempty"`
			}{
				Scheme:   m.Auth.Scheme,
				Fields:   m.Auth.Fields,
				Defaults: m.Auth.Defaults,
				Modes: func() []struct {
					ID       string            `json:"id"`
					Label    string            `json:"label,omitempty"`
					Scheme   string            `json:"scheme,omitempty"`
					Fields   []string          `json:"fields,omitempty"`
					Defaults map[string]string `json:"defaults,omitempty"`
				} {
					if len(m.Auth.Modes) == 0 {
						return nil
					}
					out := make([]struct {
						ID       string            `json:"id"`
						Label    string            `json:"label,omitempty"`
						Scheme   string            `json:"scheme,omitempty"`
						Fields   []string          `json:"fields,omitempty"`
						Defaults map[string]string `json:"defaults,omitempty"`
					}, 0, len(m.Auth.Modes))
					for _, md := range m.Auth.Modes {
						out = append(out, struct {
							ID       string            `json:"id"`
							Label    string            `json:"label,omitempty"`
							Scheme   string            `json:"scheme,omitempty"`
							Fields   []string          `json:"fields,omitempty"`
							Defaults map[string]string `json:"defaults,omitempty"`
						}{
							ID:       md.ID,
							Label:    md.Label,
							Scheme:   md.Scheme,
							Fields:   md.Fields,
							Defaults: md.Defaults,
						})
					}
					return out
				}(),
			}
		}
		apps := make([]struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}, 0, len(it.Apps))
		for _, a := range it.Apps {
			apps = append(apps, struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{ID: a.ID, Name: a.Name})
		}
		out = append(out, providerView{
			ID:         it.ID,
			Name:       it.Name,
			Apps:       apps,
			Configured: ok,
			Health:     hr,
			Auth:       authView,
		})
	}

	dtoRequest.ResponseSuccess(c, gin.H{
		"env":       env,
		"modality":  mod,
		"providers": out,
	})
	return
}

func (h *AgentSettingHandler) listModels(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	mod := c.Query("modality")
	prov := c.Query("provider")
	app := c.Query("app")

	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	models, err := h.svc.ModelsForTenant(c.Request.Context(), env, tenantCtx.UUIDPtr(), mod, prov, app)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"models": models})
}

// listOpenAILLMModels maps admin route to the same model catalog used by openapi /ai/llm/models.
func (h *AgentSettingHandler) listOpenAILLMModels(c *gin.Context) {
	if h == nil || h.aiSvc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	env := strings.TrimSpace(c.Query("env"))
	if env == "" {
		resolved, _, resolveErr := h.aiSvc.ResolveTenantEnv(c.Request.Context(), tenantCtx.UUID())
		if resolveErr != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, resolveErr.Error(), nil)
			return
		}
		if strings.TrimSpace(resolved) != "" {
			env = resolved
		} else {
			env = "dev"
		}
	}
	provider := strings.TrimSpace(c.Query("provider"))
	items, err := h.aiSvc.ListLLMModels(c.Request.Context(), env, tenantCtx.UUID(), provider)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadGateway, "failed to load llm model list", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":   env,
		"items": items,
	})
}

func applyAppToModel(app, model string) string {
	a := strings.TrimSpace(app)
	m := strings.TrimSpace(model)
	if a == "" || m == "" {
		return m
	}
	if strings.Contains(m, ":") {
		return m
	}
	return a + ":" + m
}

// ---------- Settings ----------

func (h *AgentSettingHandler) saveSettings(c *gin.Context) {
	var req saveSettingsReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	normalizeSettingsRequest(&req)
	// Normalize app:model if app provided
	if req.LLM != nil {
		req.LLM.Model = applyAppToModel(req.LLM.App, req.LLM.Model)
	}
	if req.Image != nil {
		req.Image.Model = applyAppToModel(req.Image.App, req.Image.Model)
	}
	if req.Embedding != nil {
		req.Embedding.Model = applyAppToModel(req.Embedding.App, req.Embedding.Model)
	}
	if req.Video != nil {
		req.Video.Model = applyAppToModel(req.Video.App, req.Video.Model)
	}
	if req.Model3D != nil {
		req.Model3D.Model = applyAppToModel(req.Model3D.App, req.Model3D.Model)
	}
	if req.AudioTTS != nil {
		req.AudioTTS.Model = applyAppToModel(req.AudioTTS.App, req.AudioTTS.Model)
	}
	if req.AudioASR != nil {
		req.AudioASR.Model = applyAppToModel(req.AudioASR.App, req.AudioASR.Model)
	}
	if req.Rerank != nil {
		req.Rerank.Model = applyAppToModel(req.Rerank.App, req.Rerank.Model)
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
			req.LLM.SecretID,
			req.LLM.SecretKey,
			req.LLM.Region,
			req.LLM.AuthMode,
		); err != nil {
			// 记录最近一次测试状态（便于智能体配置页筛选/排障）
			_ = h.svc.UpsertTenantProviderHealth(
				c.Request.Context(),
				tenantUUID,
				req.Env,
				string(req.Modality),
				req.LLM.Provider,
				"unhealthy",
				err.Error(),
			)
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
	case contract.ModModel3D:
		if req.Model3D == nil || strings.TrimSpace(req.Model3D.Provider) == "" || strings.TrimSpace(req.Model3D.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "model3d.provider/model 不能为空", nil)
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
		AuthScheme: func() string {
			scheme := "bearer"
			if m, ok := catalog.GetGlobalAIRegister().Manifest(credProvider); ok && m != nil {
				if s := strings.TrimSpace(m.Auth.Scheme); s != "" {
					scheme = s
				}
			}
			return scheme
		}(),
		Data: credData,
	}
	if req.Modality == contract.ModEmbed && req.Embedding != nil && prof != nil {
		if existing, err := h.svc.GetProfile(c.Request.Context(), req.Env, tenantRef, "embedding", prof.Provider, prof.Model); err == nil && existing != nil {
			if prof.Defaults == nil {
				prof.Defaults = datatypes.JSONMap{}
			}
			if existing.Defaults != nil {
				if _, ok := prof.Defaults["dimensions"]; !ok {
					if dim, ok := existing.Defaults["dimensions"]; ok {
						prof.Defaults["dimensions"] = dim
					}
				}
			}
			// 保留测试探针结果（cap_cache），避免保存配置把 probed_at 清空。
			if existing.CapCache != nil && (prof.CapCache == nil || len(prof.CapCache) == 0) {
				prof.CapCache = existing.CapCache
			}
		}
	}
	if err := h.svc.SaveCredentialAndProfile(c.Request.Context(), req.Env, tenantRef, cred, prof, true); err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "保存失败", err)
		return
	}
	// ✅ 产品语义：任意模态保存成功后，更新“租户当前 AI 环境”
	if strings.TrimSpace(req.Env) != "" {
		_ = h.svc.SetTenantCurrentAIEnv(c.Request.Context(), tenantUUID, req.Env)
	}
	// 保存成功代表连通性校验通过：同步写入“可用 Provider”缓存（仅 LLM 维持原语义）
	if req.Modality == contract.ModLLM {
		_ = h.svc.UpsertTenantProviderHealth(
			c.Request.Context(),
			tenantUUID,
			req.Env,
			string(req.Modality),
			req.LLM.Provider,
			"healthy",
			"ok",
		)
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
	normalizeTestRequest(&req)
	logger.InfoF(c.Request.Context(), "[agent_setting] test_connection enter path=%s", c.FullPath())
	// Normalize app:model if app provided
	if req.LLM != nil {
		req.LLM.Model = applyAppToModel(req.LLM.App, req.LLM.Model)
	}
	if req.Image != nil {
		req.Image.Model = applyAppToModel(req.Image.App, req.Image.Model)
	}
	if req.Embedding != nil {
		req.Embedding.Model = applyAppToModel(req.Embedding.App, req.Embedding.Model)
	}
	if req.Video != nil {
		req.Video.Model = applyAppToModel(req.Video.App, req.Video.Model)
	}
	if req.Model3D != nil {
		req.Model3D.Model = applyAppToModel(req.Model3D.App, req.Model3D.Model)
	}
	if req.AudioTTS != nil {
		req.AudioTTS.Model = applyAppToModel(req.AudioTTS.App, req.AudioTTS.Model)
	}
	if req.AudioASR != nil {
		req.AudioASR.Model = applyAppToModel(req.AudioASR.App, req.AudioASR.Model)
	}
	if req.Rerank != nil {
		req.Rerank.Model = applyAppToModel(req.Rerank.App, req.Rerank.Model)
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	tenantUUID := tenantCtx.UUID()

	saveVerifiedCredential := func(provider, apiKey, secretID, secretKey, baseURL, region, organization, azureDeployment, authMode string) error {
		p := strings.TrimSpace(provider)
		if p == "" {
			return nil
		}
		scheme := "bearer"
		if m, ok := catalog.GetGlobalAIRegister().Manifest(p); ok && m != nil {
			if s := strings.TrimSpace(m.Auth.Scheme); s != "" {
				scheme = s
			}
		}
		cred := &dbmodel.AIProviderCredential{
			Env:        req.Env,
			TenantUUID: tenantRef,
			Name:       utils.Slug(req.Env + "-" + p),
			Provider:   p,
			AuthScheme: scheme,
			Data: datatypes.JSONMap{
				"api_key":          apiKey,
				"secret_id":        secretID,
				"secret_key":       secretKey,
				"auth_mode":        authMode,
				"base_url":         baseURL,
				"region":           region,
				"organization":     organization,
				"azure_deployment": azureDeployment,
			},
		}
		return h.svc.SaveCredentialOnly(c.Request.Context(), req.Env, tenantRef, cred)
	}

	switch req.Modality {
	case contract.ModLLM:
		if req.LLM == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm 配置不能为空", nil)
			return
		}
		if strings.TrimSpace(req.LLM.Provider) == "" || strings.TrimSpace(req.LLM.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm.provider/model 不能为空", nil)
			return
		}
		provider := req.LLM.Provider
		model := req.LLM.Model
		err := h.svc.TestConnectionPreferInput(
			c.Request.Context(),
			req.Env, tenantRef,
			string(req.Modality),
			provider, model, req.LLM.BaseURL, req.LLM.APIKey, req.LLM.SecretID, req.LLM.SecretKey, req.LLM.Region, req.LLM.AuthMode,
		)
		if err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, provider, model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, "连接测试失败", err)
			return
		}
		// ✅ 测试通过：自动保存该 provider 的凭据（不激活默认路由）
		_ = saveVerifiedCredential(provider, req.LLM.APIKey, req.LLM.SecretID, req.LLM.SecretKey, req.LLM.BaseURL, req.LLM.Region, req.LLM.Organization, req.LLM.AzureDeployment, req.LLM.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), provider, "healthy", "ok")
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, provider, model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModImage:
		if req.Image == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "image 配置不能为空", nil)
			return
		}
		if err := h.svc.PingImage(
			c.Request.Context(),
			req.Env, tenantRef,
			req.Image.Provider, req.Image.Model,
			req.Image.BaseURL, req.Image.APIKey, req.Image.SecretID, req.Image.SecretKey,
			req.Image.Region, req.Image.Organization,
		); err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Image.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Image.Provider, req.Image.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		_ = saveVerifiedCredential(req.Image.Provider, req.Image.APIKey, "", "", req.Image.BaseURL, req.Image.Region, req.Image.Organization, req.Image.AzureDeployment, req.Image.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Image.Provider, "healthy", "ok")
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Image.Provider, req.Image.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModEmbed:
		if req.Embedding == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "embedding 配置不能为空", nil)
			return
		}
		dim, err := h.svc.ProbeEmbeddingDimensionsPreferInput(
			c.Request.Context(),
			req.Env, tenantRef,
			req.Embedding.Provider, req.Embedding.Model,
			req.Embedding.BaseURL, req.Embedding.APIKey,
		)
		if err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Embedding.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Embedding.Provider, req.Embedding.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		if err := h.ensureEmbeddingVectorTable(c.Request.Context(), dim); err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Embedding.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Embedding.Provider, req.Embedding.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "embedding 向量表创建失败", err)
			return
		}
		_ = saveVerifiedCredential(req.Embedding.Provider, req.Embedding.APIKey, "", "", req.Embedding.BaseURL, req.Embedding.Region, req.Embedding.Organization, req.Embedding.AzureDeployment, req.Embedding.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Embedding.Provider, "healthy", "ok")
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Embedding.Provider, req.Embedding.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "dimensions": dim})
	case contract.ModVideo:
		if req.Video == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "video 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Video.Provider, req.Video.Model, req.Video.BaseURL, req.Video.APIKey); err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Video.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Video.Provider, req.Video.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		_ = saveVerifiedCredential(req.Video.Provider, req.Video.APIKey, "", "", req.Video.BaseURL, req.Video.Region, req.Video.Organization, req.Video.AzureDeployment, req.Video.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Video.Provider, "healthy", "ok")
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Video.Provider, req.Video.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModModel3D:
		if req.Model3D == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "model3d 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Model3D.Provider, req.Model3D.Model, req.Model3D.BaseURL, req.Model3D.APIKey); err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Model3D.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Model3D.Provider, req.Model3D.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		_ = saveVerifiedCredential(req.Model3D.Provider, req.Model3D.APIKey, req.Model3D.SecretID, req.Model3D.SecretKey, req.Model3D.BaseURL, req.Model3D.Region, req.Model3D.Organization, req.Model3D.AzureDeployment, req.Model3D.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Model3D.Provider, "healthy", "ok")
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Model3D.Provider, req.Model3D.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModAudioTTS:
		if req.AudioTTS == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_tts 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, req.AudioTTS.BaseURL, req.AudioTTS.APIKey); err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.AudioTTS.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		_ = saveVerifiedCredential(req.AudioTTS.Provider, req.AudioTTS.APIKey, "", "", req.AudioTTS.BaseURL, req.AudioTTS.Region, req.AudioTTS.Organization, req.AudioTTS.AzureDeployment, req.AudioTTS.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.AudioTTS.Provider, "healthy", "ok")
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioTTS.Provider, req.AudioTTS.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModAudioASR:
		if req.AudioASR == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "audio_asr 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, req.AudioASR.BaseURL, req.AudioASR.APIKey); err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.AudioASR.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		_ = saveVerifiedCredential(req.AudioASR.Provider, req.AudioASR.APIKey, "", "", req.AudioASR.BaseURL, req.AudioASR.Region, req.AudioASR.Organization, req.AudioASR.AzureDeployment, req.AudioASR.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.AudioASR.Provider, "healthy", "ok")
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.AudioASR.Provider, req.AudioASR.Model, true, "ok")
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	case contract.ModRerank:
		if req.Rerank == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "rerank 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Rerank.Provider, req.Rerank.Model, req.Rerank.BaseURL, req.Rerank.APIKey); err != nil {
			_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Rerank.Provider, "unhealthy", err.Error())
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestConnection, req.Modality, req.Rerank.Provider, req.Rerank.Model, false, err.Error())
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
		_ = saveVerifiedCredential(req.Rerank.Provider, req.Rerank.APIKey, "", "", req.Rerank.BaseURL, req.Rerank.Region, req.Rerank.Organization, req.Rerank.AzureDeployment, req.Rerank.AuthMode)
		_ = h.svc.UpsertTenantProviderHealth(c.Request.Context(), tenantUUID, req.Env, string(req.Modality), req.Rerank.Provider, "healthy", "ok")
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
	normalizeTestCallRequest(&req)
	// Normalize app:model if app provided
	if req.LLM != nil {
		req.LLM.Model = applyAppToModel(req.LLM.App, req.LLM.Model)
	}
	if req.Image != nil {
		req.Image.Model = applyAppToModel(req.Image.App, req.Image.Model)
	}
	if req.Embedding != nil {
		req.Embedding.Model = applyAppToModel(req.Embedding.App, req.Embedding.Model)
	}
	if req.Video != nil {
		req.Video.Model = applyAppToModel(req.Video.App, req.Video.Model)
	}
	if req.Model3D != nil {
		req.Model3D.Model = applyAppToModel(req.Model3D.App, req.Model3D.Model)
	}
	if req.AudioTTS != nil {
		req.AudioTTS.Model = applyAppToModel(req.AudioTTS.App, req.AudioTTS.Model)
	}
	if req.AudioASR != nil {
		req.AudioASR.Model = applyAppToModel(req.AudioASR.App, req.AudioASR.Model)
	}
	if req.Rerank != nil {
		req.Rerank.Model = applyAppToModel(req.Rerank.App, req.Rerank.Model)
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
			req.LLM.Provider, req.LLM.Model, req.LLM.BaseURL, req.LLM.APIKey, req.LLM.SecretID, req.LLM.SecretKey, req.LLM.Region, req.LLM.AuthMode,
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
	case contract.ModModel3D:
		if req.Model3D == nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "model3d 配置不能为空", nil)
			return
		}
		if err := h.svc.PingGeneric(c.Request.Context(), req.Env, tenantRef, req.Modality, req.Model3D.Provider, req.Model3D.Model, req.Model3D.BaseURL, req.Model3D.APIKey); err != nil {
			h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Model3D.Provider, req.Model3D.Model, false, err.Error())
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		msg := describeModel3DQuickCall(req.Model3D, req.Prompt)
		h.emitAuditEvent(c, tenantUUID, req.Env, auditOpTestQuickCall, req.Modality, req.Model3D.Provider, req.Model3D.Model, true, msg)
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
			"secret_id":        req.LLM.SecretID,
			"secret_key":       req.LLM.SecretKey,
			"auth_mode":        req.LLM.AuthMode,
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
			"secret_id":        req.Image.SecretID,
			"secret_key":       req.Image.SecretKey,
			"auth_mode":        req.Image.AuthMode,
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
			"secret_id":        req.Embedding.SecretID,
			"secret_key":       req.Embedding.SecretKey,
			"auth_mode":        req.Embedding.AuthMode,
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
		if req.Embedding.Dimensions <= 0 {
			delete(prof.Defaults, "dimensions")
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
			"secret_id":        req.Video.SecretID,
			"secret_key":       req.Video.SecretKey,
			"auth_mode":        req.Video.AuthMode,
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
	case contract.ModModel3D:
		if req.Model3D == nil {
			return
		}
		p := strings.TrimSpace(req.Model3D.Provider)
		m := strings.TrimSpace(req.Model3D.Model)
		if p == "" || m == "" {
			return
		}
		credProvider = req.Model3D.Provider
		credName = utils.Slug(req.Env + "-" + req.Model3D.Provider)
		cred = datatypes.JSONMap{
			"api_key":          req.Model3D.APIKey,
			"secret_id":        req.Model3D.SecretID,
			"secret_key":       req.Model3D.SecretKey,
			"auth_mode":        req.Model3D.AuthMode,
			"base_url":         req.Model3D.BaseURL,
			"region":           req.Model3D.Region,
			"organization":     req.Model3D.Organization,
			"azure_deployment": req.Model3D.AzureDeployment,
		}
		prof = &dbmodel.AIModelProfile{
			Env:        req.Env,
			TenantUUID: tenantUUID,
			Modality:   "model3d",
			Provider:   req.Model3D.Provider,
			Model:      req.Model3D.Model,
			Defaults: datatypes.JSONMap{
				"outputFormat": req.Model3D.OutputFormat,
				"promptHint":   req.Model3D.PromptHint,
			},
			Tags: []string{"model3d"},
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
			"secret_id":        req.AudioTTS.SecretID,
			"secret_key":       req.AudioTTS.SecretKey,
			"auth_mode":        req.AudioTTS.AuthMode,
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
			"secret_id":        req.AudioASR.SecretID,
			"secret_key":       req.AudioASR.SecretKey,
			"auth_mode":        req.AudioASR.AuthMode,
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
			"secret_id":        req.Rerank.SecretID,
			"secret_key":       req.Rerank.SecretKey,
			"auth_mode":        req.Rerank.AuthMode,
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

func describeModel3DQuickCall(m *modModel3D, prompt string) string {
	return fmt.Sprintf("已校验 3D provider=%s model=%s outputFormat=%s prompt=%s",
		m.Provider, m.Model, defaultIfEmpty(m.OutputFormat, "glb"), snippet(prompt, 60))
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

func (h *AgentSettingHandler) ensureEmbeddingVectorTable(ctx context.Context, dim int) error {
	if dim <= 0 {
		return nil
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config unavailable")
	}
	driver := strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.Driver)
	if driver != "" && !strings.EqualFold(driver, "pgvector") {
		return nil
	}
	pgCfg := pgvectorcfg.Config{
		DSN:    strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.DSN),
		Schema: strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.Schema),
		Lists:  cfg.KnowledgeSpace.VectorStore.PgVector.Lists,
	}.WithDefaults()

	dsn := pgCfg.DSN
	if dsn == "" {
		dsn = strings.TrimSpace(cfg.Database.DSN)
	}
	if dsn == "" && strings.TrimSpace(cfg.Database.Host) != "" {
		sslmode := strings.TrimSpace(cfg.Database.SSLMode)
		if sslmode == "" {
			sslmode = "disable"
		}
		tz := strings.TrimSpace(cfg.Database.Timezone)
		if tz == "" {
			tz = "UTC"
		}
		dsn = "host=" + strings.TrimSpace(cfg.Database.Host) +
			" port=" + strconv.Itoa(cfg.Database.Port) +
			" user=" + strings.TrimSpace(cfg.Database.UserName) +
			" password=" + strings.TrimSpace(cfg.Database.Password) +
			" dbname=" + strings.TrimSpace(cfg.Database.Database) +
			" sslmode=" + sslmode +
			" TimeZone=" + tz
	}
	if dsn == "" {
		return fmt.Errorf("pgvector dsn is empty (configure knowledge_space.vector_store.pgvector.dsn or database.dsn)")
	}
	tableName := fmt.Sprintf("knowledge_vectors_v1_%d", dim)
	logger.InfoF(ctx, "[agent_setting] ensure embedding vector table schema=%s table=%s dim=%d", pgCfg.Schema, tableName, dim)
	if err := migration.EnsureKnowledgeVectorsPGVectorTable(ctx, dsn, pgCfg.Schema, tableName, dim, pgCfg.Lists); err != nil {
		return err
	}
	logger.InfoF(ctx, "[agent_setting] embedding vector table ready schema=%s table=%s dim=%d", pgCfg.Schema, tableName, dim)
	return nil
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
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询激活配置失败", err)
		return
	}
	if prof == nil {
		dtoRequest.ResponseSuccess(c, gin.H{
			"env":        env,
			"modality":   mod,
			"profile":    nil,
			"configured": false,
			"message":    "当前模态尚未配置模型，请先在 AI 设置中保存。",
		})
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":        env,
		"modality":   mod,
		"profile":    prof, // ✅ 只有一条
		"configured": true,
	})
}

type setCurrentEnvReq struct {
	Env string `json:"env" validate:"required"`
}

type setSkillSourcePolicyReq struct {
	Allowlist []string `json:"allowlist"`
}

func (h *AgentSettingHandler) getCurrentEnv(c *gin.Context) {
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantUUID := tenantCtx.UUID()
	env, configured, err := h.svc.GetTenantCurrentAIEnv(c.Request.Context(), tenantUUID)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	if !configured {
		dtoRequest.ResponseSuccess(c, gin.H{
			"configured": false,
			"env":        "",
			"fallback":   "dev",
			"message":    "租户尚未设置当前 AI 环境，将使用默认值。",
		})
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"configured": true,
		"env":        env,
	})
}

func (h *AgentSettingHandler) setCurrentEnv(c *gin.Context) {
	var req setCurrentEnvReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tenantUUID := tenantCtx.UUID()
	if err := h.svc.SetTenantCurrentAIEnv(c.Request.Context(), tenantUUID, req.Env); err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "设置失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "env": req.Env})
}

func (h *AgentSettingHandler) getSkillSourcePolicy(c *gin.Context) {
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	view, err := h.skillPolicySvc.GetTenantSourcePolicy(c.Request.Context(), tenantCtx.UUID())
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询 Skills 来源策略失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"allowlist":        view.Allowlist,
		"effective_source": view.EffectiveSource,
		"updated_at":       view.UpdatedAt,
	})
}

func (h *AgentSettingHandler) setSkillSourcePolicy(c *gin.Context) {
	var req setSkillSourcePolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	view, err := h.skillPolicySvc.SetTenantSourcePolicy(c.Request.Context(), tenantCtx.UUID(), req.Allowlist)
	if err != nil {
		if errors.Is(err, skillservice.ErrSkillSourcePolicyInvalid) {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "allowlist 至少包含一个合法来源（builtin/plugin/third_party）", err)
			return
		}
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "保存 Skills 来源策略失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"ok":               true,
		"allowlist":        view.Allowlist,
		"effective_source": view.EffectiveSource,
		"updated_at":       view.UpdatedAt,
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
