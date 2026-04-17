package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
)

type loadTask struct {
	EventID string
}

type result struct {
	latency time.Duration
	err     error
}

type report struct {
	Endpoint       string             `json:"endpoint"`
	Tenant         string             `json:"tenant"`
	Topic          string             `json:"topic"`
	Events         int                `json:"events"`
	Concurrency    int                `json:"concurrency"`
	Success        int                `json:"success"`
	Failed         int                `json:"failed"`
	DurationMS     int64              `json:"duration_ms"`
	Throughput     float64            `json:"throughput_per_sec"`
	AverageLatency float64            `json:"average_latency_ms"`
	Percentiles    map[string]float64 `json:"percentiles_ms"`
}

func main() {
	var (
		endpoint    = flag.String("endpoint", "http://localhost:8077/admin/event-fabric/events:publish", "事件发布 API 地址")
		tenant      = flag.String("tenant", "", "租户 ID (tenant_id)")
		topic       = flag.String("topic", "", "事件主题全名 (<tenant>.<namespace>.<name>)")
		totalEvents = flag.Int("events", 1000, "发送事件总数")
		concurrency = flag.Int("concurrency", 20, "并发数")
		payloadStr  = flag.String("payload", `{"demo":"loadtest"}`, "事件 payload（JSON 字符串）")
		principal   = flag.String("principal", "svc-loadtest", "principal_id（发布主体）")
		version     = flag.String("version", "v1", "事件版本")
		signSecret  = flag.String("signature-secret", "", "签名密钥（为空则不附加签名）")
		signKeyID   = flag.String("signature-key", "loadtest", "签名 key id，用于构造 Signature Header")
		reportPath  = flag.String("report", "", "输出报告 JSON 文件路径，留空则仅打印")
		timeout     = flag.Duration("timeout", 10*time.Second, "单个请求超时时间")
	)
	flag.Parse()

	if strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*topic) == "" {
		logger.ErrorF(context.Background(), "tenant 与 topic 为必填参数")
		os.Exit(1)
	}
	if *totalEvents <= 0 {
		logger.ErrorF(context.Background(), "events 必须大于 0")
		os.Exit(1)
	}
	if *concurrency <= 0 {
		logger.ErrorF(context.Background(), "concurrency 必须大于 0")
		os.Exit(1)
	}

	client := &http.Client{Timeout: *timeout}
	payloadBase64 := base64.StdEncoding.EncodeToString([]byte(*payloadStr))

	jobs := make(chan loadTask, *totalEvents)
	results := make(chan result, *totalEvents)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var seq atomic.Uint64
	for i := 0; i < *totalEvents; i++ {
		jobs <- loadTask{}
	}
	close(jobs)

	var wg sync.WaitGroup
	overallStart := time.Now()
	wg.Add(*concurrency)
	for w := 0; w < *concurrency; w++ {
		go func() {
			defer wg.Done()
			for range jobs {
				id := seq.Add(1)
				eventID := fmt.Sprintf("evt-loadtest-%d-%s", id, uuid.NewString()[:8])
				traceID := fmt.Sprintf("trace-%s", uuid.NewString()[:8])
				body := map[string]interface{}{
					"tenant_id":      *tenant,
					"topic":          *topic,
					"event_id":       eventID,
					"trace_id":       traceID,
					"version":        *version,
					"payload":        payloadBase64,
					"payload_format": "json",
					"attributes": map[string]string{
						"principal_id": *principal,
					},
				}

				buf, err := json.Marshal(body)
				if err != nil {
					results <- result{err: fmt.Errorf("marshal request: %w", err)}
					continue
				}

				req, err := http.NewRequestWithContext(ctx, http.MethodPost, *endpoint, bytes.NewReader(buf))
				if err != nil {
					results <- result{err: fmt.Errorf("create request: %w", err)}
					continue
				}
				req.Header.Set("Content-Type", "application/json")

				if secret := strings.TrimSpace(*signSecret); secret != "" {
					timestamp := time.Now().UTC().Format(time.RFC3339Nano)
					signature := signRequest(secret, *signKeyID, timestamp, http.MethodPost, req.URL.RequestURI(), buf)
					req.Header.Set("X-PowerX-Timestamp", timestamp)
					req.Header.Set("X-PowerX-Signature", signature)
				}

				start := time.Now()
				resp, err := client.Do(req)
				latency := time.Since(start)
				if err != nil {
					results <- result{err: fmt.Errorf("request failed: %w", err)}
					continue
				}
				_ = resp.Body.Close()

				if resp.StatusCode >= 300 {
					results <- result{err: fmt.Errorf("unexpected status %d", resp.StatusCode)}
					continue
				}

				results <- result{latency: latency}
			}
		}()
	}

	wg.Wait()
	close(results)

	var (
		latencies []time.Duration
		success   int
		failed    int
	)

	for res := range results {
		if res.err != nil {
			failed++
			logger.ErrorF(context.Background(), "[error] %v", res.err)
			continue
		}
		success++
		latencies = append(latencies, res.latency)
	}

	totalDuration := time.Since(overallStart)
	if totalDuration <= 0 {
		totalDuration = time.Millisecond
	}

	rep := buildReport(*endpoint, *tenant, *topic, *totalEvents, *concurrency, success, failed, totalDuration, latencies)

	if *reportPath != "" {
		if err := writeReport(*reportPath, rep); err != nil {
			logger.ErrorF(context.Background(), "写入报告失败: %v", err)
			os.Exit(1)
		}
	}

	printReport(rep)

	if failed > 0 {
		os.Exit(2)
	}
}

func buildReport(endpoint, tenant, topic string, events, concurrency, success, failed int, duration time.Duration, latencies []time.Duration) report {
	rep := report{
		Endpoint:    endpoint,
		Tenant:      tenant,
		Topic:       topic,
		Events:      events,
		Concurrency: concurrency,
		Success:     success,
		Failed:      failed,
		DurationMS:  duration.Milliseconds(),
		Percentiles: map[string]float64{},
	}

	if duration > 0 && success > 0 {
		rep.Throughput = float64(success) / duration.Seconds()
	}
	if len(latencies) == 0 {
		return rep
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	rep.AverageLatency = (total / time.Duration(len(latencies))).Seconds() * 1000

	rep.Percentiles["p50"] = percentile(latencies, 0.50)
	rep.Percentiles["p90"] = percentile(latencies, 0.90)
	rep.Percentiles["p95"] = percentile(latencies, 0.95)
	rep.Percentiles["p99"] = percentile(latencies, 0.99)

	return rep
}

func percentile(values []time.Duration, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return float64(values[0].Milliseconds())
	}
	if p >= 1 {
		return float64(values[len(values)-1].Milliseconds())
	}
	index := int(float64(len(values)-1) * p)
	return float64(values[index].Milliseconds())
}

func writeReport(path string, rep report) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printReport(rep report) {
	logger.InfoF(context.Background(), "Load Test Summary")
	logger.InfoF(context.Background(), "Endpoint      : %s", rep.Endpoint)
	logger.InfoF(context.Background(), "Tenant/Topic  : %s / %s", rep.Tenant, rep.Topic)
	logger.InfoF(context.Background(), "Total Events  : %d (success=%d, failed=%d)", rep.Events, rep.Success, rep.Failed)
	logger.InfoF(context.Background(), "Concurrency   : %d", rep.Concurrency)
	logger.InfoF(context.Background(), "Duration      : %d ms", rep.DurationMS)
	logger.InfoF(context.Background(), "Throughput    : %.2f events/s", rep.Throughput)
	logger.InfoF(context.Background(), "Avg Latency   : %.2f ms", rep.AverageLatency)
	if len(rep.Percentiles) > 0 {
		logger.InfoF(context.Background(), "P50/P90/P95/P99: %.2f / %.2f / %.2f / %.2f ms",
			rep.Percentiles["p50"],
			rep.Percentiles["p90"],
			rep.Percentiles["p95"],
			rep.Percentiles["p99"],
		)
	}
}

func signRequest(secret, keyID, timestamp, method, path string, body []byte) string {
	payload := strings.Join([]string{timestamp, strings.ToUpper(method), path, string(body)}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s:%s", strings.TrimSpace(keyID), signature)
}
