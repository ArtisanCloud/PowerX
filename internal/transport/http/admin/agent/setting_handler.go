package agent

import (
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"

	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

// 依赖注入
type AgentSettingHandler struct {
	svc *agentSvc.AgentSettingService
}

func NewAgentSettingHandler(deps *shared.Deps) *AgentSettingHandler {
	return &AgentSettingHandler{svc: agentSvc.NewAgentSettingService(deps.DB)}
}

type modality = string

type baseConn struct {
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

type saveSettingsReq struct {
	Env       string    `json:"env" validate:"required"`
	Modality  modality  `json:"modality" validate:"required"`
	LLM       *modLLM   `json:"llm,omitempty"`
	Image     *modImage `json:"image,omitempty"`
	Embedding *modEmbed `json:"embedding,omitempty"`
	Video     *modVideo `json:"video,omitempty"`
}

type testReq struct {
	Env       string    `json:"env" validate:"required"`
	Modality  modality  `json:"modality" validate:"required"`
	LLM       *modLLM   `json:"llm,omitempty"`
	Image     *modImage `json:"image,omitempty"`
	Embedding *modEmbed `json:"embedding,omitempty"`
	Video     *modVideo `json:"video,omitempty"`
}

type testCallReq struct {
	Env       string   `json:"env"       validate:"required"`
	Modality  modality `json:"modality"  validate:"required"`
	Prompt    string   `json:"prompt"`
	LLM       modLLM   `json:"llm"`
	Image     modImage `json:"image"`
	Embedding modEmbed `json:"embedding"`
	Video     modVideo `json:"video"`
}

// ---------- Providers / Models ----------

func (h *AgentSettingHandler) listProviders(c *gin.Context) {
	mod := c.Query("modality")
	dtoRequest.ResponseSuccess(c, gin.H{"providers": h.svc.Providers(mod)})
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
	tenantID := auth.GetTenantID(c.Request.Context())

	// 仅按当前模态做最小校验
	switch strings.ToLower(req.Modality) {
	case "llm":
		if req.LLM == nil || strings.TrimSpace(req.LLM.Provider) == "" || strings.TrimSpace(req.LLM.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm.provider/model 不能为空", nil)
			return
		}
	}

	// 组装两张表的实体并落库
	credName, credProvider, credData, prof := buildEntitiesFromPayload(&req, &tenantID)
	if credName == "" || prof == nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少当前模态设置", nil)
		return
	}
	cred := &dbmodel.AIProviderCredential{
		Env:        req.Env,
		TenantID:   &tenantID,
		Name:       credName,
		Provider:   credProvider,
		AuthScheme: "bearer",
		Data:       credData,
	}
	if err := h.svc.SaveCredentialAndProfile(c.Request.Context(), req.Env, &tenantID, cred, prof); err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "保存失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

// ---------- Tests ----------

func (h *AgentSettingHandler) testConnection(c *gin.Context) {
	var req testReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	switch strings.ToLower(req.Modality) {
	case "llm":
		if req.LLM == nil || strings.TrimSpace(req.LLM.Provider) == "" || strings.TrimSpace(req.LLM.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm.provider/model 不能为空", nil)
			return
		}
		if err := h.svc.PingLLM(c.Request.Context(),
			req.Env, &tid,
			req.LLM.Provider, req.LLM.Model, req.LLM.BaseURL, req.LLM.APIKey,
		); err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "连接测试失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	default:
		dtoRequest.ResponseError(c, http.StatusNotImplemented, "暂未实现该模态测试: "+req.Modality, nil)
	}
}

func (h *AgentSettingHandler) testQuickCall(c *gin.Context) {
	var req testCallReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	switch strings.ToLower(req.Modality) {
	case "llm":
		out, err := h.svc.QuickCallLLM(
			c.Request.Context(),
			req.Env, &tid,
			req.LLM.Provider, req.LLM.Model, req.LLM.BaseURL, req.LLM.APIKey,
			req.LLM.Temperature, req.LLM.MaxTokens,
			req.Prompt,
		)
		if err != nil {
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": out})
	default:
		dtoRequest.ResponseError(c, http.StatusNotImplemented, "暂未实现该模态试跑: "+req.Modality, nil)
	}
}

// ---------- helpers（保留你已有的） ----------

func buildEntitiesFromPayload(req *saveSettingsReq, tenantID *uint64) (credName, credProvider string, cred datatypes.JSONMap, prof *dbmodel.AIModelProfile) {
	cred = datatypes.JSONMap{}

	switch contract.Modality(strings.ToLower(req.Modality)) {
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
			Env:      req.Env,
			TenantID: tenantID,
			Modality: "llm",
			Provider: req.LLM.Provider,
			Model:    req.LLM.Model,
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
			Env:      req.Env,
			TenantID: tenantID,
			Modality: "image",
			Provider: req.Image.Provider,
			Model:    req.Image.Model,
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
			Env:      req.Env,
			TenantID: tenantID,
			Modality: "embedding",
			Provider: req.Embedding.Provider,
			Model:    req.Embedding.Model,
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
			Env:      req.Env,
			TenantID: tenantID,
			Modality: "video",
			Provider: req.Video.Provider,
			Model:    req.Video.Model,
			Defaults: datatypes.JSONMap{
				"resolution":     req.Video.Resolution,
				"fps":            req.Video.FPS,
				"maxDurationSec": req.Video.MaxDurationSec,
				"promptHint":     req.Video.PromptHint,
			},
			Tags: []string{"video"},
		}
	}
	return
}

// GET /api/agents/settings/profiles?env=default&modalities=llm,image
func (h *AgentSettingHandler) listProfiles(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	var mods []string
	if s := strings.TrimSpace(c.Query("modalities")); s != "" {
		for _, m := range strings.Split(s, ",") {
			m = strings.TrimSpace(strings.ToLower(m))
			if m != "" {
				mods = append(mods, m)
			}
		}
	}

	out, err := h.svc.ListProfiles(c.Request.Context(), env, &tid, mods...)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	// 直接返回 GORM 实体数组（Env/TenantID 因为 json:"-" 不会出现在响应里）
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":      env,
		"profiles": out,
	})
}

// （可选）GET /api/agents/settings/credentials?env=default
func (h *AgentSettingHandler) listCredentials(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	out, err := h.svc.ListCredentials(c.Request.Context(), env, &tid)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":         env,
		"credentials": out,
	})
}
