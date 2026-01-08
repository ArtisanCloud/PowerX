package corpus_check

import (
	"encoding/json"
	"strings"

	"gorm.io/datatypes"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

// BuildMetrics 根据 ingestion_jobs 的 metrics_snapshot 生成最小体检指标与推荐卡片。
func BuildMetrics(sampleJobs []models.IngestionJob) (metrics datatypes.JSON, recommendations datatypes.JSON) {
	type ocrStats struct {
		Required int `json:"required"`
		Needed   int `json:"needed"`
		Used     int `json:"used"`
	}
	type dist map[string]int

	formatDist := make(dist)
	sourceDist := make(dist)
	ocr := ocrStats{}
	total := 0
	dupKey := make(map[string]int)
	dups := 0

	for _, job := range sampleJobs {
		total++
		format := strings.ToLower(strings.TrimSpace(job.SourceType))
		if format == "" {
			format = "unknown"
		}
		formatDist[format]++
		sourceDist[formatCategory(format)]++
		dupKey[job.SourceID]++
		if dupKey[job.SourceID] > 1 {
			dups++
		}

		var snap map[string]any
		_ = json.Unmarshal(job.MetricsSnapshot, &snap)
		if b, ok := snap["ocr_required"].(bool); ok && b {
			ocr.Required++
		}
		if b, ok := snap["ocr_needed"].(bool); ok && b {
			ocr.Needed++
		}
		if b, ok := snap["ocr_used"].(bool); ok && b {
			ocr.Used++
		}
	}

	metricsMap := map[string]any{
		"sample_total": total,
		"format_dist":  formatDist,
		"type_dist":    sourceDist,
		"ocr":          ocr,
		"duplicate": map[string]any{
			"count": dups,
		},
	}

	recs := make([]map[string]any, 0, 3)
	if total > 0 && float64(ocr.Needed)/float64(total) >= 0.3 {
		recs = append(recs, map[string]any{
			"key":   "enable_ocr",
			"title": "扫描件占比偏高：建议启用 OCR",
			"reason": map[string]any{
				"ocr_needed_ratio": float64(ocr.Needed) / float64(total),
			},
			"plugin": "com.powerx.plugin.data_forge",
			"risk":   "若不启用 OCR，检索召回与引用覆盖会显著下降。",
			"cost":   "OCR 会提升入库耗时与成本；可先对高价值文档启用。",
		})
	}
	if total > 0 && float64(sourceDist["table_like"])/float64(total) >= 0.3 {
		recs = append(recs, map[string]any{
			"key":   "table_heavy",
			"title": "表格/结构化内容占比偏高：建议调整分块与索引策略",
			"reason": map[string]any{
				"table_like_ratio": float64(sourceDist["table_like"]) / float64(total),
			},
			"risk": "不做表格感知分块可能导致问答命中不稳定。",
			"cost": "改用更细粒度 chunk 会增大向量写入量。",
		})
	}
	if len(recs) == 0 {
		recs = append(recs, map[string]any{
			"key":   "default",
			"title": "语料分布正常：可沿用默认 RAG 策略",
			"risk":  "建议在 Playground 中做 A/B 检索对比，确认召回与引用覆盖。",
		})
	}

	metricsBytes, _ := json.Marshal(metricsMap)
	recsBytes, _ := json.Marshal(recs)
	return datatypes.JSON(metricsBytes), datatypes.JSON(recsBytes)
}

func formatCategory(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "xlsx", "csv", "table":
		return "table_like"
	case "sql":
		return "code_like"
	case "image":
		return "image"
	case "pdf", "docx", "markdown", "html":
		return "doc"
	default:
		return "other"
	}
}

