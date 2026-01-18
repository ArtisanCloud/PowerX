package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OpenAIEmbedder struct {
	BaseURL  string // 例如: https://api.openai.com/v1
	APIKey   string
	Model    string        // 例如: text-embedding-3-small / text-embedding-3-large
	Timeout  time.Duration // 可选，默认 10s
	HTTP     *http.Client  // 可选：自定义 client
	MaxBatch int           // 可选：拆批大小，默认 128
}

type openaiEmbReq struct {
	Input any    `json:"input"` // string or []string
	Model string `json:"model"`
}

type openaiEmbResp struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	// 省略 usage 等字段
}

func (e *OpenAIEmbedder) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	to := e.Timeout
	if to <= 0 {
		to = 10 * time.Second
	}
	return &http.Client{Timeout: to}
}

func (e *OpenAIEmbedder) endpoint() string {
	base := e.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return base + "/embeddings"
}

func (e *OpenAIEmbedder) batchSize() int {
	if e.MaxBatch <= 0 {
		return 128
	}
	return e.MaxBatch
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	// 分批
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

func (e *OpenAIEmbedder) embedOnce(ctx context.Context, batch []string) ([][]float32, error) {
	reqBody := openaiEmbReq{
		Input: batch,
		Model: e.Model,
	}
	bs, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint(), bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	resp, err := e.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		msg := string(bytes.TrimSpace(body))
		if msg != "" {
			return nil, fmt.Errorf("openai embeddings HTTP %d (%s): %s", resp.StatusCode, e.endpoint(), msg)
		}
		return nil, fmt.Errorf("openai embeddings HTTP %d (%s)", resp.StatusCode, e.endpoint())
	}

	var or openaiEmbResp
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, err
	}
	// OpenAI 不保证返回顺序绝对与输入一致，但一般 index 对应；按 index 排序更严谨（此处直接按顺序读取）
	vecs := make([][]float32, len(or.Data))
	for _, d := range or.Data {
		// 转成 float32
		f := make([]float32, len(d.Embedding))
		for i := range d.Embedding {
			f[i] = float32(d.Embedding[i])
		}
		if d.Index >= 0 && d.Index < len(or.Data) {
			vecs[d.Index] = f
		} else {
			// fallback: 追加（极少发生）
			vecs = append(vecs, f)
		}
	}
	// 去掉可能的空位
	out := make([][]float32, 0, len(vecs))
	for _, v := range vecs {
		if v != nil {
			out = append(out, v)
		}
	}
	return out, nil
}
