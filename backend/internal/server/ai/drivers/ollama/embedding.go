// services/agent/intent/embed/ollama_vectorizer.go
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OllamaEmbedder struct {
	BaseURL  string
	Model    string
	Timeout  time.Duration
	HTTP     *http.Client
	MaxBatch int
}

type ollamaEmbReq struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"` // string or []string
}

type ollamaEmbResp struct {
	Embedding  []float64   `json:"embedding,omitempty"`  // input 为 string
	Embeddings [][]float64 `json:"embeddings,omitempty"` // input 为 []string
}

func (e *OllamaEmbedder) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	to := e.Timeout
	if to <= 0 {
		to = 10 * time.Second
	}
	return &http.Client{Timeout: to}
}

func (e *OllamaEmbedder) base() string {
	if e.BaseURL == "" {
		return "http://127.0.0.1:11434"
	}
	return e.BaseURL
}

func (e *OllamaEmbedder) batchSize() int {
	if e.MaxBatch <= 0 {
		return 128
	}
	return e.MaxBatch
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
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

func (e *OllamaEmbedder) embedOnce(ctx context.Context, batch []string) ([][]float32, error) {
	var in any
	if len(batch) == 1 {
		in = batch[0]
	} else {
		in = batch
	}
	reqBody := ollamaEmbReq{Model: e.Model, Input: in}
	bs, _ := json.Marshal(reqBody)

	// 先打 /api/embed；若 404 再试 /api/embeddings（兼容老/某些打包）
	try := func(url string) ([][]float32, int, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bs))
		if err != nil {
			return nil, 0, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.client().Do(req)
		if err != nil {
			return nil, 0, nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode/100 != 2 {
			// 非 2xx → 返回真实状态码，外层决定是否 fallback
			return nil, resp.StatusCode, body, nil
		}

		var or ollamaEmbResp
		if err := json.Unmarshal(body, &or); err != nil {
			// 解码错误 → 直接报错
			return nil, 0, body, err
		}

		switch {
		case len(or.Embeddings) > 0:
			out := make([][]float32, len(or.Embeddings))
			for i, em := range or.Embeddings {
				f := make([]float32, len(em))
				for j := range em {
					f[j] = float32(em[j])
				}
				out[i] = f
			}
			// ✅ 成功：code=0
			return out, 0, nil, nil

		case len(or.Embedding) > 0:
			f := make([]float32, len(or.Embedding))
			for i := range or.Embedding {
				f[i] = float32(or.Embedding[i])
			}
			// ✅ 成功：code=0
			return [][]float32{f}, 0, nil, nil
		}

		return nil, 0, body, fmt.Errorf("empty response")
	}

	// 1st: /api/embed
	url1 := e.base() + "/api/embed"
	if out, code, body, err := try(url1); err != nil {
		return nil, fmt.Errorf("ollama embed decode error: %v (url=%s)", err, url1)
	} else if code == 0 {
		return out, nil
	} else if code != http.StatusNotFound {
		return nil, fmt.Errorf("ollama embed HTTP %d: %s (url=%s)", code, string(body), url1)
	}

	// 2nd (fallback): /api/embeddings
	url2 := e.base() + "/api/embeddings"
	if out, code, body, err := try(url2); err != nil {
		return nil, fmt.Errorf("ollama embeddings decode error: %v (url=%s)", err, url2)
	} else if code == 0 {
		return out, nil
	} else {
		return nil, fmt.Errorf("ollama embeddings HTTP %d: %s (url=%s)", code, string(body), url2)
	}
}
