package writer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

type LokiWriter struct {
	url        string
	labels     map[string]string
	client     *http.Client
	retryCount int
}

func NewLokiWriter(conf *config.LokiConfig) *LokiWriter {
	rawURL := ""
	labels := defaultLokiLabels()
	if conf != nil {
		rawURL = strings.TrimSpace(conf.URL)
		labels = mergeLokiLabels(labels, conf.Labels)
	}

	return &LokiWriter{
		url:        normalizeLokiPushURL(rawURL),
		labels:     labels,
		client:     &http.Client{Timeout: 10 * time.Second},
		retryCount: 3,
	}
}

// Write 方法实现
func (w *LokiWriter) Write(p []byte) (n int, err error) {
	if strings.TrimSpace(w.url) == "" {
		return 0, fmt.Errorf("failed to send log to Loki: push url is empty")
	}
	// 获取当前时间的 Unix 纳秒时间戳
	timestamp := time.Now().UnixNano()

	entry := map[string]interface{}{
		"streams": []map[string]interface{}{
			{
				"stream": w.labels,
				"values": [][]string{
					{
						fmt.Sprintf("%d", timestamp), // 使用 Unix 纳秒时间戳
						string(p),                    // 日志内容
					},
				},
			},
		},
	}

	// 将日志转换为 JSON
	body, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	// fmt2.Dump(string(body))

	// 发送 HTTP 请求到 Loki，带有重试机制
	for attempt := 1; attempt <= w.retryCount; attempt++ {
		resp, err := w.client.Post(w.url, "application/json", bytes.NewBuffer(body))
		if err != nil {
			// 在重试次数内失败时，继续尝试
			if attempt == w.retryCount {
				return 0, fmt.Errorf("failed to send log to Loki: %v", err)
			}
			time.Sleep(time.Second * time.Duration(attempt)) // 按照尝试次数递增等待时间
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			if attempt == w.retryCount {
				errorBody, _ := io.ReadAll(resp.Body) // 读取响应体的内容
				return 0, fmt.Errorf("failed to send log to Loki: status %d - %s, response body: %s", resp.StatusCode, resp.Status, string(errorBody))
			}
			time.Sleep(time.Second * time.Duration(attempt)) // 按照尝试次数递增等待时间
			continue
		}

		return len(p), nil
	}

	return 0, fmt.Errorf("failed to send log to Loki after %d attempts", w.retryCount)
}

func (w *LokiWriter) Sync() error {
	// 如果需要处理清理工作，可以在这里做（如连接池关闭等），目前不需要做任何操作
	return nil
}

func normalizeLokiPushURL(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return ""
	}
	base = strings.TrimRight(base, "/")
	if strings.Contains(base, "/loki/api/v1/push") {
		return base
	}
	return base + "/loki/api/v1/push"
}

func defaultLokiLabels() map[string]string {
	return map[string]string{
		"system":   "powerx",
		"service":  "powerx-backend",
		"env":      "dev",
		"instance": "local",
		"module":   "runtime",
	}
}

func mergeLokiLabels(base map[string]string, overrides map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	if len(overrides) == 0 {
		return out
	}
	for _, key := range sortedKeys(overrides) {
		cleanKey := sanitizeLabelKey(key)
		cleanVal := sanitizeLabelValue(overrides[key])
		if cleanKey == "" || cleanVal == "" {
			continue
		}
		out[cleanKey] = cleanVal
	}
	return out
}

var lokiLabelKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func sanitizeLabelKey(v string) string {
	k := strings.TrimSpace(v)
	if k == "" || !lokiLabelKeyPattern.MatchString(k) {
		return ""
	}
	return k
}

func sanitizeLabelValue(v string) string {
	return strings.TrimSpace(v)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
