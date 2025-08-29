// api/http/admin/agent/setting_handler.go
package agent

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"net/http"
	"strings"
	"time"

	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"

	// 直接复用你已经写好的 LLM 工厂和一次性聊天能力
	// services/agent/drivers/eino/llm/{llm.go, openai.go, ollama.go, baidu.go}
	agentllm "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/llm"
)

/*
	路由在 api/http/admin/agent/api.go 里已经注册为：
	- GET  /api/agents/providers
	- GET  /api/agents/models
	- POST /api/agents/settings/save
	- POST /api/agents/test/connection
	- POST /api/agents/test/call
*/

// ---------- DTO ----------

// 与你前端字段一一对应（只做最小必需字段，后续可扩展）
type modality = string

// baseConn 保持 required，这样只有当 *modLLM 非 nil 时才会去校验
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

// GET /api/agents/providers
func listProviders(c *gin.Context) {
	mod := c.Query("modality")
	items := catalog.GetGlobalAIRegister().Providers(mod)
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name) // 也可以把 id+name 一起返给前端
	}
	dtoRequest.ResponseSuccess(c, gin.H{"providers": names})
}

// GET /api/agents/models?modality=llm&provider=OpenAI
func listModels(c *gin.Context) {
	mod := c.Query("modality")
	prov := c.Query("provider")
	models, err := catalog.GetGlobalAIRegister().Models(mod, prov)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.ID)
	}
	dtoRequest.ResponseSuccess(c, gin.H{"models": out})
}

// ---------- Settings ----------

// POST /api/agents/settings/save
// 现在先回显（不入库），等你定义好 model/repo 后在此 upsert(credential/model_profile/route_policy)
func saveSettings(c *gin.Context) {
	var req saveSettingsReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	// 仅按当前模态做必填校验
	switch strings.ToLower(req.Modality) {
	case "llm":
		if strings.TrimSpace(req.LLM.Provider) == "" || strings.TrimSpace(req.LLM.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm.provider/model 不能为空", nil)
			return
		}
		// 其他模态等接入后再补
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "echo": req})
}

// ---------- Tests ----------

// POST /api/agents/test/connection
func testConnection(c *gin.Context) {
	var req testReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	switch strings.ToLower(req.Modality) {
	case "llm":
		if strings.TrimSpace(req.LLM.Provider) == "" || strings.TrimSpace(req.LLM.Model) == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "llm.provider/model 不能为空", nil)
			return
		}
		// ...
	default:
		dtoRequest.ResponseError(c, http.StatusNotImplemented, "暂未实现该模态测试: "+req.Modality, nil)
	}
}

// POST /api/agents/test/call
func testQuickCall(c *gin.Context) {
	var req testCallReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}

	switch strings.ToLower(req.Modality) {
	case "llm":
		out, err := quickCallLLM(c.Request.Context(), req.LLM, req.Prompt)
		if err != nil {
			dtoRequest.ResponseSuccess(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "result": out})
	default:
		dtoRequest.ResponseError(c, http.StatusNotImplemented, "暂未实现该模态试跑: "+req.Modality, nil)
	}
}

// ---------- helpers ----------

func pingLLM(ctx context.Context, s modLLM) error {
	if s.Provider == "" || s.Model == "" {
		return errors.New("provider/model 不能为空")
	}
	mc := agentllm.ModelConfig{
		Provider:     s.Provider,
		Endpoint:     s.BaseURL,
		APIKey:       s.APIKey,
		Model:        s.Model,
		SystemPrompt: "You are a health check probe.",
		Temperature:  0.0,
		MaxTokens:    8,
		AccessToken:  s.APIKey, // 兼容部分厂商用 access_token
	}
	cli, err := agentllm.NewClient(s.Provider)
	if err != nil {
		return err
	}
	_, err = cli.ChatOnce(ctx, mc, "ping")
	return err
}

func quickCallLLM(ctx context.Context, s modLLM, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Say hello in one short sentence."
	}
	mc := agentllm.ModelConfig{
		Provider:     s.Provider,
		Endpoint:     s.BaseURL,
		APIKey:       s.APIKey,
		Model:        s.Model,
		SystemPrompt: "You are a helpful assistant.",
		Temperature:  s.Temperature,
		MaxTokens:    maxInt(s.MaxTokens, 64),
		AccessToken:  s.APIKey,
	}
	cli, err := agentllm.NewClient(s.Provider)
	if err != nil {
		return "", err
	}
	// 设置一个合理超时（防止卡住）
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.ChatOnce(ctx, mc, prompt)
}

// 小工具
func maxInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
