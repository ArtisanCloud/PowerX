package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	intentfactory "github.com/ArtisanCloud/PowerX/internal/server/agent/factory/intent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	openaidriver "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/openai"
	imagefactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/image"
	llmfactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	ttsfactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/tts"
	videofactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/video"
	vlmfactory "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/vlm"
	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

var (
	ErrInvalidModelKey     = errors.New("invalid model_key")
	ErrModelNotConfigured  = errors.New("model not configured for tenant")
	ErrPromptRequired      = errors.New("inputs must include text/content")
	ErrProviderUnsupported = errors.New("provider driver not implemented")
	ErrInvalidLLMParams    = errors.New("invalid llm params")
)

type Service struct {
	db       *gorm.DB
	settings *agentsettings.AgentSettingService
	profiles *repoai.AIModelProfileRepository
	catalog  *catalog.Registry
}

type LLMInvokeResult struct {
	Text         string         `json:"text"`
	FinishReason string         `json:"finish_reason,omitempty"`
	Usage        map[string]any `json:"usage,omitempty"`
}

func NewService(db *gorm.DB) *Service {
	if db == nil {
		return nil
	}
	return &Service{
		db:       db,
		settings: agentsettings.NewAgentSettingService(db),
		profiles: repoai.NewAIModelProfileRepository(db),
	}
}

func (s *Service) ResolveTenantEnv(ctx context.Context, tenantUUID string) (string, bool, error) {
	if s == nil || s.settings == nil {
		return "", false, nil
	}
	return s.settings.GetTenantCurrentAIEnv(ctx, tenantUUID)
}

func (s *Service) LLMInvoke(
	ctx context.Context,
	env string,
	tenantUUID string,
	modelKey string,
	inputs []ContentItem,
	params map[string]interface{},
) (*LLMInvokeResult, error) {
	if s == nil || s.settings == nil || s.profiles == nil {
		return nil, errors.New("ai service unavailable")
	}
	provider, model := splitModelKey(modelKey)
	if provider == "" || model == "" {
		return nil, ErrInvalidModelKey
	}
	prof, err := s.profiles.FindByEquivalentScopeModalityProviderModel(ctx, env, &tenantUUID, "llm", provider, model)
	defaults, _ := resolveDefaults("llm", provider, model)
	if prof == nil || err != nil {
		if !s.allowUnprofiled(ctx, env, tenantUUID, "llm", provider) {
			return nil, ErrModelNotConfigured
		}
	} else {
		defaults = prof.Defaults
	}
	systemPrompt, prompt := BuildLLMPrompts(inputs)
	if strings.TrimSpace(prompt) == "" {
		return nil, ErrPromptRequired
	}
	temperature := floatFromAny(defaults["temperature"])
	requestTemperature, temperatureSet, err := parseRequestTemperature(params)
	if err != nil {
		return nil, err
	}
	seed, seedSet, err := parseOllamaSeed(provider, params)
	if err != nil {
		return nil, err
	}
	// 不再在未传参时注入默认 max_tokens，交给模型提供方自身默认值处理。
	maxTokens := 0
	if params != nil {
		if temperatureSet {
			temperature = requestTemperature
		}
		if v, ok := params["max_tokens"]; ok {
			if mt := intFromAny(v); mt > 0 {
				maxTokens = mt
			}
		}
		if v, ok := params["maxTokens"]; ok {
			if mt := intFromAny(v); mt > 0 {
				maxTokens = mt
			}
		}
	}
	mc, err := s.settings.BuildModelConfig(ctx, env, &tenantUUID, "llm", provider, model, "", "", "", "", "", "")
	if err != nil {
		return nil, err
	}
	applyTimeoutFromContext(ctx, mc)
	mc.SystemPrompt = "You are a helpful assistant."
	if strings.TrimSpace(systemPrompt) != "" {
		mc.SystemPrompt = strings.TrimSpace(systemPrompt)
	}
	if temperatureSet || temperature > 0 {
		mc.Temperature = temperature
		mc.TemperatureSet = temperatureSet
	}
	if maxTokens > 0 {
		mc.MaxTokens = maxTokens
	}
	applyLLMRuntimeExtras(provider, mc, defaults, params)
	if seedSet {
		if mc.Extra == nil {
			mc.Extra = map[string]any{}
		}
		mc.Extra["seed"] = seed
	}
	applyResponseSchema(mc, params)
	if err := applyResponseFormat(mc, params); err != nil {
		return nil, err
	}
	if len(mc.ResponseSchema) > 0 && !llmfactory.SupportsResponseSchema(provider) {
		return nil, errors.New("ai.response_schema_provider_unsupported")
	}
	applyReasoningConfig(provider, mc, params)

	invokeResult, err := llmfactory.Invoke(ctx, mc, prompt)
	if err != nil {
		return nil, err
	}
	if invokeResult == nil {
		return &LLMInvokeResult{}, nil
	}
	text := invokeResult.Text
	if shouldStripThinkingOutput(params) {
		text = stripThinkingTags(text)
	}
	return &LLMInvokeResult{
		Text:         text,
		FinishReason: invokeResult.FinishReason,
		Usage:        invokeResult.Usage,
	}, nil
}

func (s *Service) LLMStream(
	ctx context.Context,
	env string,
	tenantUUID string,
	modelKey string,
	inputs []ContentItem,
	params map[string]interface{},
	onDelta func(string),
) (string, error) {
	if s == nil || s.settings == nil || s.profiles == nil {
		return "", errors.New("ai service unavailable")
	}
	provider, model := splitModelKey(modelKey)
	if provider == "" || model == "" {
		return "", ErrInvalidModelKey
	}
	prof, err := s.profiles.FindByEquivalentScopeModalityProviderModel(ctx, env, &tenantUUID, "llm", provider, model)
	defaults, _ := resolveDefaults("llm", provider, model)
	if prof == nil || err != nil {
		if !s.allowUnprofiled(ctx, env, tenantUUID, "llm", provider) {
			return "", ErrModelNotConfigured
		}
	} else {
		defaults = prof.Defaults
	}
	systemPrompt, prompt := BuildLLMPrompts(inputs)
	if strings.TrimSpace(prompt) == "" {
		return "", ErrPromptRequired
	}

	temperature := floatFromAny(defaults["temperature"])
	requestTemperature, temperatureSet, err := parseRequestTemperature(params)
	if err != nil {
		return "", err
	}
	seed, seedSet, err := parseOllamaSeed(provider, params)
	if err != nil {
		return "", err
	}
	maxTokens := intFromAny(defaults["maxTokens"])
	if maxTokens == 0 {
		maxTokens = intFromAny(defaults["max_tokens"])
	}
	if params != nil {
		if temperatureSet {
			temperature = requestTemperature
		}
		if v, ok := params["max_tokens"]; ok {
			if mt := intFromAny(v); mt > 0 {
				maxTokens = mt
			}
		}
		if v, ok := params["maxTokens"]; ok {
			if mt := intFromAny(v); mt > 0 {
				maxTokens = mt
			}
		}
	}

	mc, err := s.settings.BuildModelConfig(ctx, env, &tenantUUID, "llm", provider, model, "", "", "", "", "", "")
	if err != nil {
		return "", err
	}
	applyTimeoutFromContext(ctx, mc)
	mc.SystemPrompt = "You are a helpful assistant."
	if strings.TrimSpace(systemPrompt) != "" {
		mc.SystemPrompt = strings.TrimSpace(systemPrompt)
	}
	if temperatureSet || temperature > 0 {
		mc.Temperature = temperature
		mc.TemperatureSet = temperatureSet
	}
	if maxTokens > 0 {
		mc.MaxTokens = maxTokens
	}
	applyLLMRuntimeExtras(provider, mc, defaults, params)
	if seedSet {
		if mc.Extra == nil {
			mc.Extra = map[string]any{}
		}
		mc.Extra["seed"] = seed
	}
	applyReasoningConfig(provider, mc, params)

	applyResponseSchema(mc, params)
	if err := applyResponseFormat(mc, params); err != nil {
		return "", err
	}
	if len(mc.ResponseSchema) > 0 && !llmfactory.SupportsResponseSchema(provider) {
		return "", errors.New("ai.response_schema_provider_unsupported")
	}
	final, err := llmfactory.StreamOrFallback(ctx, mc, prompt, onDelta)
	if err != nil {
		return "", err
	}
	if shouldStripThinkingOutput(params) {
		return stripThinkingTags(final), nil
	}
	return final, nil
}

func applyTimeoutFromContext(ctx context.Context, mc *aiconfig.ModelConfig) {
	if mc == nil || mc.Timeout > 0 || ctx == nil {
		return
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return
	}
	remaining := time.Until(deadline)
	if remaining > 0 {
		mc.Timeout = remaining
	}
}

func applyLLMRuntimeExtras(provider string, mc *aiconfig.ModelConfig, defaults map[string]any, params map[string]interface{}) {
	if mc == nil {
		return
	}
	for _, values := range []map[string]any{defaults, params} {
		if value, ok := values["temperature"]; ok {
			mc.Temperature = floatFromAny(value)
			mc.TemperatureSet = true
		}
		for _, key := range []string{"topP", "top_p"} {
			if value, ok := values[key]; ok {
				mc.TopP = float32(floatFromAny(value))
				mc.TopPSet = true
			}
		}
		if effort, ok := values["reasoning_effort"].(string); ok {
			if mc.Extra == nil {
				mc.Extra = map[string]any{}
			}
			mc.Extra["reasoning_effort"] = effort
		}
	}
	if !strings.EqualFold(strings.TrimSpace(provider), "ollama") {
		return
	}
	if mc.Extra == nil {
		mc.Extra = map[string]any{}
	}
	if v := firstPositiveInt(defaults, "num_ctx", "context_window", "context_length", "context_len"); v > 0 {
		mc.Extra["num_ctx"] = v
	}
	if v := firstPositiveInt(params, "num_ctx", "context_window", "context_length", "context_len"); v > 0 {
		mc.Extra["num_ctx"] = v
	}
	if len(mc.Extra) == 0 {
		mc.Extra = nil
	}
}

func parseRequestTemperature(params map[string]interface{}) (float64, bool, error) {
	value, present := params["temperature"]
	if !present {
		return 0, false, nil
	}
	var temperature float64
	switch v := value.(type) {
	case float64:
		temperature = v
	case float32:
		temperature = float64(v)
	case int:
		temperature = float64(v)
	case int64:
		temperature = float64(v)
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false, fmt.Errorf("%w: temperature must be a finite non-negative number", ErrInvalidLLMParams)
		}
		temperature = parsed
	default:
		return 0, false, fmt.Errorf("%w: temperature must be a finite non-negative number", ErrInvalidLLMParams)
	}
	if math.IsNaN(temperature) || math.IsInf(temperature, 0) || temperature < 0 {
		return 0, false, fmt.Errorf("%w: temperature must be a finite non-negative number", ErrInvalidLLMParams)
	}
	return temperature, true, nil
}

func parseOllamaSeed(provider string, params map[string]interface{}) (int64, bool, error) {
	value, present := params["seed"]
	if !present {
		return 0, false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(provider), "ollama") {
		return 0, false, fmt.Errorf("%w: seed is supported only for Ollama", ErrInvalidLLMParams)
	}
	const maxExactJSONInteger = int64(1<<53 - 1)
	var seed int64
	switch v := value.(type) {
	case int:
		seed = int64(v)
	case int64:
		seed = v
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false, fmt.Errorf("%w: seed must be an integer", ErrInvalidLLMParams)
		}
		seed = parsed
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || math.Abs(v) > float64(maxExactJSONInteger) {
			return 0, false, fmt.Errorf("%w: seed must be an integer", ErrInvalidLLMParams)
		}
		seed = int64(v)
	default:
		return 0, false, fmt.Errorf("%w: seed must be an integer", ErrInvalidLLMParams)
	}
	if seed < -maxExactJSONInteger || seed > maxExactJSONInteger {
		return 0, false, fmt.Errorf("%w: seed exceeds the exact JSON integer range", ErrInvalidLLMParams)
	}
	return seed, true, nil
}

// applyResponseSchema accepts a typed caller's JSON Schema. It is deliberately
// separate from provider options: response format is a top-level provider
// contract, never an Ollama generation option.
func applyResponseSchema(mc *aiconfig.ModelConfig, params map[string]interface{}) {
	if mc == nil || len(params) == 0 {
		return
	}
	raw, ok := params["response_schema"]
	if !ok || raw == nil {
		return
	}
	schema, ok := raw.(map[string]any)
	if !ok || len(schema) == 0 {
		return
	}
	mc.ResponseSchema = cloneResponseSchema(schema)
}

// JSON Object is an explicit provider output mode, not a downgrade from a
// requested JSON Schema. The caller must still validate its business schema.
func applyResponseFormat(mc *aiconfig.ModelConfig, params map[string]interface{}) error {
	raw, present := params["response_format"]
	if !present {
		return nil
	}
	format, ok := raw.(map[string]any)
	_, hasSchema := params["response_schema"]
	if !ok || len(format) != 1 || format["type"] != "json_object" || mc == nil || len(mc.ResponseSchema) > 0 || hasSchema {
		return fmt.Errorf("%w: AI_RESPONSE_FORMAT_INVALID", ErrInvalidLLMParams)
	}
	if !llmfactory.SupportsResponseSchema(mc.Provider) {
		return fmt.Errorf("%w: AI_RESPONSE_FORMAT_UNSUPPORTED", ErrInvalidLLMParams)
	}
	mc.ResponseFormat = "json_object"
	return nil
}

func cloneResponseSchema(schema map[string]any) map[string]any {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

func firstPositiveInt(values map[string]interface{}, keys ...string) int {
	if len(values) == 0 {
		return 0
	}
	for _, key := range keys {
		if v := intFromAny(values[key]); v > 0 {
			return v
		}
	}
	return 0
}

type reasoningConfig struct {
	Enabled       bool
	EnabledSet    bool
	Effort        string
	Expose        string
	ProviderThink *bool
	Budget        int
}

func parseReasoningConfig(params map[string]interface{}) reasoningConfig {
	cfg := reasoningConfig{
		Enabled: false,
		Effort:  "medium",
		Expose:  "none",
	}
	if len(params) == 0 {
		return cfg
	}
	if v, ok := params["thinking"]; ok {
		if b, ok := parseBool(v); ok {
			cfg.Enabled = b
			cfg.EnabledSet = true
		}
	}
	if v, ok := params["enable_thinking"]; ok {
		if b, ok := parseBool(v); ok {
			cfg.ProviderThink = &b
		}
	}
	if v, ok := params["thinking_budget"]; ok {
		if budget := intFromAny(v); budget > 0 {
			cfg.Budget = budget
		}
	}
	if cfg.Budget == 0 {
		if v, ok := params["thinkingBudget"]; ok {
			if budget := intFromAny(v); budget > 0 {
				cfg.Budget = budget
			}
		}
	}
	if v, ok := params["reasoning_effort"]; ok {
		if effort := normalizeReasoningEffort(stringFromAny(v)); effort != "" {
			cfg.Effort = effort
		}
	}
	if v, ok := params["reasoning_expose"]; ok {
		if expose := normalizeReasoningExpose(stringFromAny(v)); expose != "" {
			cfg.Expose = expose
		}
	}
	raw, ok := params["reasoning"]
	if !ok || raw == nil {
		return cfg
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return cfg
	}
	if v, ok := obj["enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Enabled = b
		}
	}
	if effort := normalizeReasoningEffort(stringFromAny(obj["effort"])); effort != "" {
		cfg.Effort = effort
	}
	if expose := normalizeReasoningExpose(stringFromAny(obj["expose"])); expose != "" {
		cfg.Expose = expose
	}
	return cfg
}

func normalizeReasoningEffort(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func normalizeReasoningExpose(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none", "summary", "full":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func applyReasoningConfig(provider string, mc *aiconfig.ModelConfig, params map[string]interface{}) {
	cfg := parseReasoningConfig(params)
	driver := resolveProviderDriver(provider, "llm")
	p := strings.ToLower(strings.TrimSpace(driver))
	pp := strings.ToLower(strings.TrimSpace(provider))
	extra := mc.Extra
	if extra == nil {
		extra = map[string]any{}
	}
	if p == "ollama" || pp == "ollama" {
		if cfg.ProviderThink != nil {
			extra["think"] = *cfg.ProviderThink
			extra["thinking"] = *cfg.ProviderThink
		}
		if cfg.EnabledSet {
			extra["think"] = cfg.Enabled
			extra["thinking"] = cfg.Enabled
		}
		mc.Extra = extra
		return
	}

	isOpenAICompatible := p == "openai" ||
		pp == "openai" || pp == "openrouter" || pp == "vllm" || pp == "deepseek" || pp == "moonshot" ||
		pp == "huggingface" || pp == "hf" || pp == "qwen" || pp == "qwen-cn" || pp == "qwen-intl" ||
		pp == "dashscope" || pp == "dashscope-cn" ||
		pp == "tongyi" || pp == "tongyi-cn" ||
		pp == "alibaba" || pp == "alibaba-cn"
	if !isOpenAICompatible {
		return
	}
	isQwenLike := pp == "qwen" || pp == "qwen-cn" || pp == "qwen-intl" ||
		pp == "dashscope" || pp == "dashscope-cn" ||
		pp == "tongyi" || pp == "tongyi-cn" ||
		pp == "alibaba" || pp == "alibaba-cn"
	if !isQwenLike && strings.Contains(strings.ToLower(strings.TrimSpace(mc.Endpoint)), "dashscope") {
		isQwenLike = true
	}
	if cfg.ProviderThink != nil {
		extra["enable_thinking"] = *cfg.ProviderThink
	}
	if cfg.EnabledSet && isQwenLike {
		extra["enable_thinking"] = cfg.Enabled
	}
	if cfg.Budget > 0 && isQwenLike {
		extra["thinking_budget"] = cfg.Budget
	}
	if cfg.EnabledSet && !cfg.Enabled {
		extra["enable_thinking"] = false
		mc.Extra = extra
		return
	}
	if !cfg.Enabled {
		mc.Extra = extra
		return
	}
	extra["reasoning"] = map[string]any{
		"effort": cfg.Effort,
	}
	extra["reasoning_expose"] = cfg.Expose
	mc.Extra = extra
}

func parseBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "true", "1", "yes", "y", "on":
			return true, true
		case "false", "0", "no", "n", "off":
			return false, true
		}
	}
	return false, false
}

var thinkTagRE = regexp.MustCompile(`(?is)<think>.*?</think>`)

func shouldStripThinkingOutput(params map[string]interface{}) bool {
	if len(params) == 0 {
		return false
	}
	cfg := parseReasoningConfig(params)
	if cfg.ProviderThink != nil {
		return !*cfg.ProviderThink
	}
	return cfg.EnabledSet && !cfg.Enabled
}

func stripThinkingTags(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	withoutThink := thinkTagRE.ReplaceAllString(s, "")
	return strings.TrimSpace(withoutThink)
}

func (s *Service) EmbeddingInvoke(
	ctx context.Context,
	env string,
	tenantUUID string,
	modelKey string,
	inputs []string,
	params map[string]interface{},
) ([][]float32, error) {
	if s == nil || s.settings == nil || s.profiles == nil {
		return nil, errors.New("ai service unavailable")
	}
	provider, model := splitModelKey(modelKey)
	if provider == "" || model == "" {
		return nil, ErrInvalidModelKey
	}
	prof, err := s.profiles.FindByEquivalentScopeModalityProviderModel(ctx, env, &tenantUUID, "embedding", provider, model)
	defaults, ok := resolveDefaults("embedding", provider, model)
	if prof == nil || err != nil {
		if !s.allowUnprofiled(ctx, env, tenantUUID, "embedding", provider) {
			return nil, ErrModelNotConfigured
		}
		if !ok {
			return nil, ErrInvalidModelKey
		}
	} else {
		defaults = prof.Defaults
	}
	cfg, err := s.settings.BuildEmbeddingConfig(ctx, env, &tenantUUID, provider, model, "", "")
	if err != nil {
		return nil, err
	}
	if batch := intFromAny(defaults["batch"]); batch > 0 {
		cfg.MaxBatch = batch
	}
	vec, err := intentfactory.NewVectorizerFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if vec == nil {
		return nil, errors.New("embedding vectorizer unavailable")
	}
	return vec.Embed(ctx, inputs)
}

// 媒体入口共用逐型号能力、参数和实时授权校验，供应商选择来自正式 operation。
func (s *Service) ImageInvoke(ctx context.Context, env, tenant, modelKey string, inputs []ContentItem, params map[string]any) (map[string]any, error) {
	model, mc, p, err := s.prepareMedia(ctx, env, tenant, modelKey, catalog.ImageGeneration, inputs, params)
	if err != nil {
		return nil, err
	}
	cli, err := imagefactory.NewClient(model.Model.Operations[catalog.ImageGeneration].Driver)
	if err != nil {
		return nil, MediaError(422, "AI_DRIVER_UNSUPPORTED")
	}
	runtime := map[string]any{"config": mc}
	for k, v := range p {
		runtime[k] = v
	}
	resp, err := cli.Generate(ctx, contract.ImageRequest{Prompt: BuildPrompt(inputs), Size: stringFromAny(p["size"]), Quality: stringFromAny(p["quality"]), Format: stringFromAny(p["format"]), RefImages: buildImageRefParts(inputs), Runtime: runtime})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Images)+len(resp.ImageURLs) == 0 {
		return nil, MediaError(502, "AI_OUTPUT_INVALID")
	}
	return map[string]any{"type": "image", "provider": model.Provider, "model": model.Model.ID, "images": resp.Images, "image_urls": resp.ImageURLs, "usage": resp.Usage, "trace_id": resp.TraceID}, nil
}
func (s *Service) VLMInvoke(ctx context.Context, env, tenant, modelKey string, inputs []ContentItem, params map[string]any) (map[string]any, error) {
	model, mc, p, err := s.prepareMedia(ctx, env, tenant, modelKey, catalog.VisionUnderstanding, inputs, params)
	if err != nil {
		return nil, err
	}
	return s.invokeVision(ctx, model, mc, inputs, p)
}

// invokeVision 共用正式驱动与解析；配置来源由正式调用或管理试跑各自决定。
func (s *Service) invokeVision(ctx context.Context, model *catalog.MediaModel, mc *aiconfig.ModelConfig, inputs []ContentItem, p map[string]any) (map[string]any, error) {
	var err error
	inputs, err = normalizeVisionInputs(inputs)
	if err != nil {
		return nil, err
	}
	if model.Provider == "openai" {
		if err := openaidriver.ValidateVLMParameters(model.Model.ID, p); err != nil {
			return nil, err
		}
	}
	cli, err := vlmfactory.NewClient(model.Model.Operations[catalog.VisionUnderstanding].Driver)
	if err != nil {
		return nil, MediaError(422, "AI_DRIVER_UNSUPPORTED")
	}
	_, tempSet := p["temperature"]
	_, topPSet := p["top_p"]
	applyTimeoutFromContext(ctx, mc)
	resp, err := cli.Invoke(ctx, contract.VLMRequest{Messages: buildVLMMessage(inputs), Temperature: floatFromAny(p["temperature"]), TopP: floatFromAny(p["top_p"]), MaxTokens: intFromAny(p["max_tokens"]), JSONMode: p["json_mode"] == true, Runtime: map[string]any{"config": mc, "temperature_set": tempSet, "top_p_set": topPSet, "reasoning_effort": p["reasoning_effort"]}})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, MediaError(504, "AI_PROVIDER_TIMEOUT")
		}
		var business *dto.AppError
		if errors.As(err, &business) {
			return nil, business
		}
		return nil, MediaError(502, "AI_PROVIDER_FAILED")
	}
	if resp == nil || strings.TrimSpace(resp.Text) == "" {
		return nil, MediaError(502, "AI_OUTPUT_INVALID")
	}
	return map[string]any{"type": "analysis", "provider": model.Provider, "model": model.Model.ID, "text": resp.Text, "usage": resp.Usage}, nil
}
func (s *Service) VideoInvoke(ctx context.Context, env, tenant, modelKey string, inputs []ContentItem, params map[string]any) (map[string]any, error) {
	model, mc, p, err := s.prepareMedia(ctx, env, tenant, modelKey, catalog.VideoGeneration, inputs, params)
	if err != nil {
		return nil, err
	}
	cli, err := videofactory.NewClient(model.Model.Operations[catalog.VideoGeneration].Driver)
	if err != nil {
		return nil, MediaError(422, "AI_DRIVER_UNSUPPORTED")
	}
	resp, err := cli.Generate(ctx, contract.VideoRequest{Prompt: BuildPrompt(inputs), Resolution: stringFromAny(p["resolution"]), FPS: intFromAny(p["fps"]), MaxDurationS: intFromAny(p["max_duration_sec"]), RefImages: buildImageRefParts(inputs), RefVideos: buildVideoRefParts(inputs), Runtime: map[string]any{"config": mc}})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, MediaError(502, "AI_OUTPUT_INVALID")
	}
	return map[string]any{"type": "video", "provider": model.Provider, "model": model.Model.ID, "video_urls": resp.VideoURLs, "task_id": resp.TaskID, "poll_url": resp.PollURL, "trace_id": resp.TraceID}, nil
}
func (s *Service) TTSInvoke(ctx context.Context, env, tenant, modelKey string, inputs []ContentItem, params map[string]any) (map[string]any, error) {
	model, mc, p, err := s.prepareMedia(ctx, env, tenant, modelKey, catalog.SpeechSynthesis, inputs, params)
	if err != nil {
		return nil, err
	}
	cli, err := ttsfactory.NewClient(model.Model.Operations[catalog.SpeechSynthesis].Driver)
	if err != nil {
		return nil, MediaError(422, "AI_DRIVER_UNSUPPORTED")
	}
	runtime := map[string]any{"config": mc}
	for k, v := range p {
		runtime[k] = v
	}
	resp, err := cli.Synthesize(ctx, contract.TTSRequest{Text: BuildPrompt(inputs), Voice: stringFromAny(p["voice"]), Speed: floatFromAny(p["speed"]), Format: stringFromAny(p["format"]), Runtime: runtime})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Audio) == 0 && resp.AudioURL == "" {
		return nil, MediaError(502, "AI_OUTPUT_INVALID")
	}
	return map[string]any{"type": "audio", "provider": model.Provider, "model": model.Model.ID, "audio_base64": base64.StdEncoding.EncodeToString(resp.Audio), "audio_url": resp.AudioURL, "usage": resp.Usage, "trace_id": resp.TraceID}, nil
}

type ContentItem struct {
	Role    string `json:"role"`
	Type    string `json:"type"`
	Content string `json:"content"`
	URL     string `json:"url"`
}

type LLMModelItem struct {
	ModelKey          string         `json:"model_key"`
	Modality          string         `json:"modality"`
	Capabilities      []string       `json:"capabilities"`
	Provider          string         `json:"provider"`
	Model             string         `json:"model"`
	Label             string         `json:"label,omitempty"`
	Source            string         `json:"source"`
	Configured        bool           `json:"configured"`
	ProfileConfigured bool           `json:"profile_configured"`
	Defaults          map[string]any `json:"defaults,omitempty"`
	Tags              []string       `json:"tags,omitempty"`
}

func (s *Service) ListLLMModels(ctx context.Context, env, tenantUUID, providerFilter string) ([]LLMModelItem, error) {
	if s == nil || s.settings == nil || s.profiles == nil {
		return nil, errors.New("ai service unavailable")
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil, errors.New("tenant uuid required")
	}
	tenantPtr := &tenantUUID
	reg := s.catalog
	if reg == nil {
		reg = catalog.GetGlobalAIRegister()
	}
	providerFilter = normalizeProviderKey(reg, providerFilter)

	creds, credErr := s.settings.ListCredentials(ctx, env, tenantPtr)
	credentialProviders := map[string]struct{}{}
	if credErr == nil {
		for i := range creds {
			key := normalizeProviderKey(reg, creds[i].Provider)
			if key == "" {
				continue
			}
			if providerFilter != "" && key != providerFilter {
				continue
			}
			credentialProviders[key] = struct{}{}
		}
	}

	tenantProfiles, err := s.profiles.ListByEquivalentScope(ctx, env, tenantPtr, "llm")
	if err != nil {
		return nil, err
	}
	globalProfiles, err := s.profiles.ListByEquivalentScope(ctx, env, nil, "llm")
	if err != nil {
		return nil, err
	}

	type mergedProfile struct {
		profile dbModelProfileView
		source  string
	}
	merged := make(map[string]mergedProfile)
	for i := range globalProfiles {
		p := toProfileView(globalProfiles[i])
		if providerFilter != "" && normalizeProviderKey(reg, p.Provider) != providerFilter {
			continue
		}
		merged[normalizeProviderKey(reg, p.Provider)+"/"+strings.ToLower(p.Model)] = mergedProfile{profile: p, source: "global_profile"}
	}
	for i := range tenantProfiles {
		p := toProfileView(tenantProfiles[i])
		if providerFilter != "" && normalizeProviderKey(reg, p.Provider) != providerFilter {
			continue
		}
		merged[normalizeProviderKey(reg, p.Provider)+"/"+strings.ToLower(p.Model)] = mergedProfile{profile: p, source: "tenant_profile"}
	}

	items := make([]LLMModelItem, 0, len(merged))
	for _, v := range merged {
		capabilities, text := textModelProfileCapabilities(reg, v.profile)
		if !text {
			continue
		}
		providerKey := normalizeProviderKey(reg, v.profile.Provider)
		_, credentialConfigured := credentialProviders[providerKey]
		items = append(items, LLMModelItem{
			ModelKey:          v.profile.Provider + "/" + v.profile.Model,
			Modality:          string(contract.ModLLM),
			Capabilities:      capabilities,
			Provider:          v.profile.Provider,
			Model:             v.profile.Model,
			Label:             v.profile.Label,
			Source:            v.source,
			Configured:        credentialConfigured,
			ProfileConfigured: true,
			Defaults:          v.profile.Defaults,
			Tags:              v.profile.Tags,
		})
	}

	if credErr == nil {
		seen := make(map[string]struct{}, len(items))
		for i := range items {
			seen[normalizeProviderKey(reg, items[i].Provider)+"/"+strings.ToLower(items[i].Model)] = struct{}{}
		}
		for i := range creds {
			providerKey := normalizeProviderKey(reg, creds[i].Provider)
			if providerKey == "" {
				continue
			}
			if providerFilter != "" && providerKey != providerFilter {
				continue
			}
			providerDisplay := strings.TrimSpace(creds[i].Provider)
			if providerDisplay == "" {
				providerDisplay = providerKey
			}
			models := textCatalogModels(reg, providerKey)
			for _, m := range models {
				model := strings.TrimSpace(m.ID)
				if model == "" {
					continue
				}
				key := providerKey + "/" + strings.ToLower(model)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				items = append(items, LLMModelItem{
					ModelKey:          providerDisplay + "/" + model,
					Modality:          string(contract.ModLLM),
					Capabilities:      append([]string(nil), m.Capabilities...),
					Provider:          providerDisplay,
					Model:             model,
					Label:             strings.TrimSpace(m.Label),
					Source:            "provider_catalog",
					Configured:        true,
					ProfileConfigured: false,
					Defaults:          copyDefaults(m.Defaults),
				})
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Configured != items[j].Configured {
			return items[i].Configured
		}
		if items[i].ProfileConfigured != items[j].ProfileConfigured {
			return items[i].ProfileConfigured
		}
		li := strings.ToLower(items[i].Provider + "/" + items[i].Model)
		lj := strings.ToLower(items[j].Provider + "/" + items[j].Model)
		return li < lj
	})
	return items, nil
}

func normalizeProviderKey(reg *catalog.Registry, provider string) string {
	key := strings.ToLower(strings.TrimSpace(provider))
	if key == "" {
		return ""
	}
	if reg == nil {
		return key
	}
	if canon := strings.ToLower(strings.TrimSpace(reg.CanonicalProvider(key))); canon != "" {
		return canon
	}
	return key
}

type dbModelProfileView struct {
	Modality string
	Provider string
	Model    string
	Label    string
	Defaults map[string]any
	Tags     []string
}

func toProfileView(in dbmodel.AIModelProfile) dbModelProfileView {
	return dbModelProfileView{
		Modality: in.Modality,
		Provider: strings.TrimSpace(in.Provider),
		Model:    strings.TrimSpace(in.Model),
		Label:    strings.TrimSpace(in.Label),
		Defaults: copyDefaults(map[string]any(in.Defaults)),
		Tags:     append([]string(nil), in.Tags...),
	}
}

func copyDefaults(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func BuildPrompt(items []ContentItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") && strings.TrimSpace(item.Content) != "" {
			parts = append(parts, strings.TrimSpace(item.Content))
		}
	}
	return strings.Join(parts, "\n")
}

// BuildLLMPrompts supports OpenAI-style role split:
// - role=system -> system prompt
// - role=user/assistant/empty -> user prompt
// It keeps compatibility with legacy payloads where role is omitted.
func BuildLLMPrompts(items []ContentItem) (string, string) {
	systemParts := make([]string, 0, len(items))
	userParts := make([]string, 0, len(items))
	for _, item := range items {
		itemType := strings.ToLower(strings.TrimSpace(item.Type))
		if itemType != "" && itemType != contract.ContentTypeText {
			continue
		}
		text := strings.TrimSpace(item.Content)
		if text == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.Role)) {
		case "system":
			systemParts = append(systemParts, text)
		default:
			userParts = append(userParts, text)
		}
	}
	return strings.Join(systemParts, "\n"), strings.Join(userParts, "\n")
}

func buildImageRefParts(items []ContentItem) []contract.ContentPart {
	out := make([]contract.ContentPart, 0, len(items))
	for _, item := range items {
		t := strings.TrimSpace(strings.ToLower(item.Type))
		switch t {
		case contract.ContentTypeImageURL:
			if strings.TrimSpace(item.URL) == "" {
				continue
			}
			out = append(out, contract.ContentPart{Type: contract.ContentTypeImageURL, URL: strings.TrimSpace(item.URL)})
		case contract.ContentTypeImageBase64:
			raw := strings.TrimSpace(item.Content)
			if raw == "" {
				raw = strings.TrimSpace(item.URL)
			}
			if raw == "" {
				continue
			}
			out = append(out, contract.ContentPart{Type: contract.ContentTypeImageBase64, Text: raw})
		}
	}
	return out
}

func buildVideoRefParts(items []ContentItem) []contract.ContentPart {
	out := make([]contract.ContentPart, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(strings.ToLower(item.Type)) != contract.ContentTypeVideoURL {
			continue
		}
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		out = append(out, contract.ContentPart{Type: contract.ContentTypeVideoURL, URL: strings.TrimSpace(item.URL)})
	}
	return out
}

func buildVLMMessage(items []ContentItem) []contract.Message {
	messages := make([]contract.Message, 0, len(items))
	appendPart := func(role string, part contract.ContentPart) {
		if len(messages) == 0 || messages[len(messages)-1].Role != role {
			messages = append(messages, contract.Message{
				Role:    role,
				Content: []contract.ContentPart{part},
			})
			return
		}
		messages[len(messages)-1].Content = append(messages[len(messages)-1].Content, part)
	}
	for _, item := range items {
		role := strings.ToLower(strings.TrimSpace(item.Role))
		switch role {
		case "system", "assistant", "tool", "user":
		default:
			role = "user"
		}
		switch strings.TrimSpace(strings.ToLower(item.Type)) {
		case contract.ContentTypeImageURL:
			if strings.TrimSpace(item.URL) == "" {
				continue
			}
			appendPart(role, contract.ContentPart{
				Type: contract.ContentTypeImageURL,
				URL:  strings.TrimSpace(item.URL),
			})
		default:
			txt := strings.TrimSpace(item.Content)
			if txt == "" {
				continue
			}
			appendPart(role, contract.ContentPart{
				Type: contract.ContentTypeText,
				Text: txt,
			})
		}
	}
	if len(messages) == 0 {
		messages = append(messages, contract.Message{
			Role:    "user",
			Content: []contract.ContentPart{{Type: contract.ContentTypeText, Text: "Describe the image."}},
		})
	}
	return messages
}

func splitModelKey(modelKey string) (string, string) {
	if strings.Contains(modelKey, "/") {
		parts := strings.SplitN(modelKey, "/", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if strings.Contains(modelKey, ":") {
		parts := strings.SplitN(modelKey, ":", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", ""
}

func resolveDefaults(modality, provider, model string) (map[string]any, bool) {
	app, modelID := splitAppModel(model)
	manifest := findModelManifest(modality, provider, app, modelID)
	if manifest == nil || manifest.Defaults == nil {
		return map[string]any{}, false
	}
	out := map[string]any{}
	for k, v := range manifest.Defaults {
		out[k] = v
	}
	return out, true
}

func splitAppModel(model string) (string, string) {
	raw := strings.TrimSpace(model)
	if raw == "" {
		return "", ""
	}
	if strings.Contains(raw, ":") {
		parts := strings.SplitN(raw, ":", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", raw
}

func findModelManifest(modality, provider, app, model string) *catalog.ModelManifest {
	reg := catalog.GetGlobalAIRegister()
	candidates := []string{strings.TrimSpace(strings.ToLower(modality))}
	for _, mod := range candidates {
		models, err := reg.ModelsByApp(mod, provider, app)
		if err != nil {
			continue
		}
		for _, m := range models {
			if strings.EqualFold(m.ID, model) {
				copy := m
				return &copy
			}
		}
	}
	return nil
}

func resolveProviderDriver(provider, modality string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return ""
	}
	mod := strings.ToLower(strings.TrimSpace(modality))
	if mod == "" {
		return p
	}
	reg := catalog.GetGlobalAIRegister()
	if man, ok := reg.Manifest(p); ok && man != nil {
		if dk := strings.ToLower(strings.TrimSpace(man.Drivers[mod])); dk != "" {
			return dk
		}
	}
	return p
}

func (s *Service) allowUnprofiled(ctx context.Context, env, tenantUUID, modality, provider string) bool {
	if s == nil || s.settings == nil {
		return false
	}
	healthMap, configured, err := s.settings.GetTenantProviderHealthMap(ctx, tenantUUID, env, modality)
	if err != nil {
		return false
	}
	if !configured {
		return false
	}
	status := ""
	if healthMap != nil {
		if rec, ok := healthMap[strings.ToLower(strings.TrimSpace(provider))]; ok {
			status = strings.ToLower(strings.TrimSpace(rec.Status))
		}
	}
	if status != "healthy" {
		return false
	}
	creds, err := s.settings.ListCredentials(ctx, env, &tenantUUID)
	if err != nil {
		return false
	}
	for _, c := range creds {
		if strings.EqualFold(strings.TrimSpace(c.Provider), strings.TrimSpace(provider)) {
			return true
		}
	}
	return false
}

func floatFromAny(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return 0
}

func intFromAny(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
		}
	}
	return 0
}

func stringFromAny(val interface{}) string {
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		return ""
	}
}
