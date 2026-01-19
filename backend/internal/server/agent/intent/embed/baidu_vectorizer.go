package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BaiduQianfanEmbedder implements a best-effort qianfan v2 embeddings caller.
//
// Endpoint (best-effort):
//   POST {base_url}/embeddings
// Body (OpenAI-like):
//   {"model":"embedding-v1","input":["t1","t2"]}
//
// NOTE: 百度千帆 embedding 的字段/鉴权可能因版本而异；如你们有网关做 OpenAI-compatible，
// 建议改用 provider=openai_compatible 复用 OpenAIEmbedder。
type BaiduQianfanEmbedder struct {
	BaseURL  string
	APIKey   string
	Model    string
	Timeout  time.Duration
	HTTP     *http.Client
	MaxBatch int
}

type baiduEmbReq struct {
	Input any    `json:"input"`
	Model string `json:"model,omitempty"`
}

type baiduEmbResp struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (e *BaiduQianfanEmbedder) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	to := e.Timeout
	if to <= 0 {
		to = 15 * time.Second
	}
	return &http.Client{Timeout: to}
}

func (e *BaiduQianfanEmbedder) base() string {
	if strings.TrimSpace(e.BaseURL) == "" {
		return "https://qianfan.baidubce.com/v2"
	}
	return strings.TrimRight(strings.TrimSpace(e.BaseURL), "/")
}

func (e *BaiduQianfanEmbedder) endpoint() string {
	return e.base() + "/embeddings"
}

func (e *BaiduQianfanEmbedder) batchSize() int {
	if e.MaxBatch <= 0 {
		return 64
	}
	return e.MaxBatch
}

func (e *BaiduQianfanEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if strings.TrimSpace(e.APIKey) == "" {
		return nil, fmt.Errorf("baidu embeddings: api_key is empty")
	}
	if strings.TrimSpace(e.Model) == "" {
		return nil, fmt.Errorf("baidu embeddings: model is empty")
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

func (e *BaiduQianfanEmbedder) embedOnce(ctx context.Context, batch []string) ([][]float32, error) {
	reqBody := baiduEmbReq{
		Input: batch,
		Model: strings.TrimSpace(e.Model),
	}
	bs, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint(), bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(e.APIKey))

	resp, err := e.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("baidu embeddings HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var or baiduEmbResp
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(or.Data))
	for _, d := range or.Data {
		f := make([]float32, len(d.Embedding))
		for i := range d.Embedding {
			f[i] = float32(d.Embedding[i])
		}
		if d.Index >= 0 && d.Index < len(or.Data) {
			vecs[d.Index] = f
		} else {
			vecs = append(vecs, f)
		}
	}
	out := make([][]float32, 0, len(vecs))
	for _, v := range vecs {
		if v != nil {
			out = append(out, v)
		}
	}
	return out, nil
}

