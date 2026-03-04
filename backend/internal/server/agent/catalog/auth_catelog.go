package catalog

import "strings"

type authReq struct {
	NeedKey        bool
	NeedBaseURL    bool
	DefaultBaseURL string
}

// 从全局 registry 读取 provider 的 AuthSpec 与 defaults；
// - needKey：fields 含 "api_key" 或 scheme 属于 bearer/oauth2/aksk 之一
// - needBaseURL：fields 含 "base_url"
// - defaultBaseURL：优先取 auth.defaults["base_url"]；若为 ollama 且没给 defaults，则回退为 http://localhost:11434
func AuthReqFromCatalog(provider string) authReq {
	reg := GetGlobalAIRegister()
	m, ok := reg.Manifest(provider)
	if !ok || m == nil {
		// 极端情况：catalog 未初始化或没这个 provider，给一个尽量中性的保守默认
		return authReq{NeedKey: true, NeedBaseURL: false, DefaultBaseURL: ""}
	}

	// 判断是否需要 key
	scheme := strings.ToLower(strings.TrimSpace(m.Auth.Scheme))
	fields := make(map[string]struct{}, len(m.Auth.Fields))
	for _, f := range m.Auth.Fields {
		fields[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
	}
	_, hasAPIKey := fields["api_key"]

	req := authReq{
		NeedKey:        hasAPIKey || scheme == "bearer" || scheme == "oauth2" || scheme == "aksk",
		NeedBaseURL:    false,
		DefaultBaseURL: "",
	}

	// base_url 要求与默认值
	if _, ok := fields["base_url"]; ok {
		req.NeedBaseURL = true
	}
	if m.Auth.Defaults != nil {
		if v := strings.TrimSpace(m.Auth.Defaults["base_url"]); v != "" {
			req.DefaultBaseURL = v
		}
	}

	// 对于 Ollama：很多人不会在 YAML 里写 defaults，我们给一个合理本地默认
	llmDriver := strings.ToLower(strings.TrimSpace(m.Drivers["llm"]))
	if req.DefaultBaseURL == "" &&
		(req.NeedBaseURL && (llmDriver == "ollama" || strings.ToLower(m.ID) == "ollama" || strings.Contains(strings.ToLower(m.Name), "ollama"))) {
		req.DefaultBaseURL = "http://localhost:11434"
	}

	return req
}

// DefaultBaseURLForModel returns a model-specific base URL when a provider requires it.
// Currently used to map Hugging Face embeddings to the OpenAI-compatible endpoint.
func DefaultBaseURLForModel(provider, model string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "huggingface" || p == "hf" {
		_ = model
		return "https://router.huggingface.co/v1"
	}
	return ""
}
