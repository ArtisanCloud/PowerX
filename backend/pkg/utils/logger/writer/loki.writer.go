package writer

import (
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

type LokiWriter struct {
	url        string
	labels     map[string]string
	client     *http.Client
	retryCount int
	delivery   *lokiDelivery
}

func NewLokiWriter(conf *config.LokiConfig) *LokiWriter {
	rawURL := ""
	labels := defaultLokiLabels()
	if conf != nil {
		rawURL = strings.TrimSpace(conf.URL)
		labels = mergeLokiLabels(labels, conf.Labels)
	}

	settings := deliverySettings(conf)
	w := &LokiWriter{
		url:        normalizeLokiPushURL(rawURL),
		labels:     labels,
		client:     &http.Client{Timeout: settings.requestTimeout},
		retryCount: 3,
	}
	w.delivery = newLokiDelivery(w, settings)
	return w
}

// Write 方法实现
func (w *LokiWriter) Write(p []byte) (n int, err error) {
	return w.delivery.write(p)
}

func (w *LokiWriter) Sync() error {
	return w.delivery.sync()
}

// Close 有界等待已接收的普通日志，超时取消网络请求并计量未交付项。
func (w *LokiWriter) Close() error { return w.delivery.close() }

// Stats 只返回计数和固定诊断，不暴露日志正文、目标 URL 或凭据。
func (w *LokiWriter) Stats() LokiDeliveryStats { return w.delivery.stats() }

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
