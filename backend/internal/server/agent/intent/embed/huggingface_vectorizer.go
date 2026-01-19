package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HuggingFaceEmbedder calls Hugging Face Router Inference API and pools to a single vector.
// It prefers OpenAI-compatible /v1/embeddings when configured, then falls back to the
// pipeline/feature-extraction endpoints.
//
// API (typical):
//   POST https://router.huggingface.co/hf-inference/models/{model}/pipeline/feature-extraction
//   POST https://router.huggingface.co/pipeline/feature-extraction/{model}
//   Authorization: Bearer <token>
//   Body: {"inputs": "text"} or {"inputs": ["t1","t2"]}
//
// Response shape varies by model:
// - [D]                      => already pooled vector
// - [T][D]                   => token embeddings (we avg-pool over T)
// - [B][D] or [B][T][D]      => batch
type HuggingFaceEmbedder struct {
	BaseURL  string
	APIKey   string
	Model    string
	Timeout  time.Duration
	HTTP     *http.Client
	MaxBatch int
}

type hfEmbReq struct {
	Inputs any `json:"inputs"` // string or []string
}

type hfReqTry struct {
	url  string
	body any
}

func (e *HuggingFaceEmbedder) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	to := e.Timeout
	if to <= 0 {
		to = 15 * time.Second
	}
	return &http.Client{Timeout: to}
}

func (e *HuggingFaceEmbedder) base() string {
	base := strings.TrimSpace(e.BaseURL)
	if base == "" {
		return "https://router.huggingface.co"
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	return base
}

func (e *HuggingFaceEmbedder) baseV1() string {
	base := strings.TrimSpace(e.BaseURL)
	if base == "" {
		return "https://router.huggingface.co/v1"
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func (e *HuggingFaceEmbedder) endpoint() string {
	// NOTE: model contains slashes (org/name). It must be preserved in the path.
	return e.base() + "/pipeline/feature-extraction/" + strings.TrimSpace(e.Model)
}

func (e *HuggingFaceEmbedder) endpoints(inputs any) []hfReqTry {
	model := strings.TrimSpace(e.Model)
	base := e.base()
	modelEsc := url.QueryEscape(model)
	return []hfReqTry{
		{url: base + "/hf-inference/models/" + model + "/pipeline/feature-extraction", body: hfEmbReq{Inputs: inputs}},
		{url: base + "/pipeline/feature-extraction/" + model, body: hfEmbReq{Inputs: inputs}},
		{url: base + "/hf-inference/pipeline/feature-extraction/" + model, body: hfEmbReq{Inputs: inputs}},
		{url: base + "/pipeline/feature-extraction?model=" + modelEsc, body: hfEmbReq{Inputs: inputs}},
		{url: base + "/hf-inference/pipeline/feature-extraction?model=" + modelEsc, body: hfEmbReq{Inputs: inputs}},
		{
			url: base + "/hf-inference/models/" + model,
			body: map[string]any{
				"inputs": inputs,
				"task":   "feature-extraction",
			},
		},
		{
			url: base + "/models/" + model,
			body: map[string]any{
				"inputs": inputs,
				"task":   "feature-extraction",
			},
		},
	}
}

func (e *HuggingFaceEmbedder) batchSize() int {
	if e.MaxBatch <= 0 {
		return 64
	}
	return e.MaxBatch
}

func (e *HuggingFaceEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if strings.TrimSpace(e.Model) == "" {
		return nil, fmt.Errorf("huggingface embedding: model is empty")
	}
	bs := e.batchSize()
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += bs {
		j := i + bs
		if j > len(texts) {
			j = len(texts)
		}
		vecs, err := e.embedOnce(ctx, texts[i:j])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (e *HuggingFaceEmbedder) embedOnce(ctx context.Context, batch []string) ([][]float32, error) {
	var in any
	if len(batch) == 1 {
		in = batch[0]
	} else {
		in = batch
	}
	var lastErr error
	attempted := make([]string, 0, 8)

	// 1) Prefer OpenAI-compatible embeddings when configured.
	if baseV1 := e.baseV1(); strings.TrimSpace(baseV1) != "" {
		oai := OpenAIEmbedder{
			BaseURL:  baseV1,
			APIKey:   e.APIKey,
			Model:    e.Model,
			Timeout:  e.Timeout,
			HTTP:     e.HTTP,
			MaxBatch: len(batch),
		}
		attempted = append(attempted, oai.endpoint())
		if vecs, err := oai.Embed(ctx, batch); err == nil {
			return vecs, nil
		} else if !shouldFallbackToPipeline(err) {
			return nil, err
		} else {
			lastErr = err
		}
	}

	tries := e.endpoints(in)
	for i, t := range tries {
		attempted = append(attempted, t.url)
		bs, _ := json.Marshal(t.body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(bs))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(e.APIKey) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(e.APIKey))
		}

		resp, err := e.client().Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			msg := strings.TrimSpace(string(body))
			lastErr = fmt.Errorf("huggingface embeddings HTTP %d (%s): %s", resp.StatusCode, t.url, msg)
			if resp.StatusCode == http.StatusNotFound ||
				(resp.StatusCode == http.StatusBadRequest && strings.Contains(msg, "SentenceSimilarityPipeline")) {
				if i < len(tries)-1 {
					continue
				}
			}
			return nil, lastErr
		}

		var raw any
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, err
		}

		// Normalize to batch output.
		switch v := raw.(type) {
		case []any:
			// could be [D] (single vector), [T][D] (single token embeddings), [B][D], or [B][T][D]
			if isNumberSlice(v) {
				return [][]float32{toF32(v)}, nil
			}
			if isNumberMatrix(v) {
				// token embeddings => pool to one
				return [][]float32{poolAvg(toF32Matrix(v))}, nil
			}
			if isBatchVectors(v) {
				out := make([][]float32, 0, len(v))
				for _, item := range v {
					switch vv := item.(type) {
					case []any:
						if isNumberSlice(vv) {
							out = append(out, toF32(vv))
							continue
						}
						if isNumberMatrix(vv) {
							out = append(out, poolAvg(toF32Matrix(vv)))
							continue
						}
					}
					return nil, fmt.Errorf("huggingface embeddings: unexpected batch item shape")
				}
				return out, nil
			}
		}

		return nil, fmt.Errorf("huggingface embeddings: unexpected response shape")
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%v; attempted=%s", lastErr, strings.Join(attempted, ","))
	}
	return nil, fmt.Errorf("huggingface embeddings: request failed")
}

func shouldFallbackToPipeline(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "openai embeddings http 404") ||
		strings.Contains(msg, "openai embeddings http 410")
}

func isNumberSlice(v []any) bool {
	if len(v) == 0 {
		return false
	}
	for _, x := range v {
		if _, ok := x.(float64); !ok {
			return false
		}
	}
	return true
}

func isNumberMatrix(v []any) bool {
	if len(v) == 0 {
		return false
	}
	for _, row := range v {
		r, ok := row.([]any)
		if !ok || !isNumberSlice(r) {
			return false
		}
	}
	return true
}

func isBatchVectors(v []any) bool {
	// detect [B][D] or [B][T][D]
	if len(v) == 0 {
		return false
	}
	_, ok := v[0].([]any)
	return ok
}

func toF32(v []any) []float32 {
	out := make([]float32, len(v))
	for i := range v {
		out[i] = float32(v[i].(float64))
	}
	return out
}

func toF32Matrix(v []any) [][]float32 {
	out := make([][]float32, len(v))
	for i := range v {
		row := v[i].([]any)
		out[i] = toF32(row)
	}
	return out
}

func poolAvg(tokens [][]float32) []float32 {
	if len(tokens) == 0 {
		return nil
	}
	dim := len(tokens[0])
	if dim == 0 {
		return nil
	}
	sum := make([]float32, dim)
	var n float32
	for _, t := range tokens {
		if len(t) != dim {
			continue
		}
		for i := 0; i < dim; i++ {
			sum[i] += t[i]
		}
		n++
	}
	if n <= 0 {
		return sum
	}
	for i := 0; i < dim; i++ {
		sum[i] /= n
	}
	return sum
}
