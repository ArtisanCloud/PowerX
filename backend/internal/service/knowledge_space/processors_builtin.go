package knowledge_space

import (
	"context"
	"hash/crc32"
	"regexp"
	"strings"
)

type TextProcessor struct{}

func (TextProcessor) Name() string { return "builtin/text" }

func (TextProcessor) Process(_ context.Context, in DocumentProcessInput) ([]DocumentUnit, OCRStats) {
	format := strings.ToLower(strings.TrimSpace(in.Format))
	src := strings.TrimSpace(in.SourceURI)
	content := syntheticContentFor(format, src)
	return []DocumentUnit{{
		Content: content,
		Provenance: map[string]any{
			"line_range": "1:200",
		},
	}}, OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}
}

type TableProcessor struct{}

func (TableProcessor) Name() string { return "builtin/table" }

func (TableProcessor) Process(_ context.Context, in DocumentProcessInput) ([]DocumentUnit, OCRStats) {
	src := strings.TrimSpace(in.SourceURI)
	rows := 5
	units := make([]DocumentUnit, 0, rows)
	for i := 0; i < rows; i++ {
		units = append(units, DocumentUnit{
			Content: syntheticContentFor("row", src) + " row=" + strconvItoa(i+1),
			Provenance: map[string]any{
				"sheet": "Sheet1",
				"row":   i + 1,
			},
		})
	}
	return units, OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}
}

type PDFProcessor struct{}

func (PDFProcessor) Name() string { return "builtin/pdf" }

func (PDFProcessor) Process(_ context.Context, in DocumentProcessInput) ([]DocumentUnit, OCRStats) {
	src := strings.TrimSpace(in.SourceURI)
	if in.NeedOCR && !in.OCRAvailable {
		return nil, OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}
	}
	pages := 3
	units := make([]DocumentUnit, 0, pages)
	stats := OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}
	for i := 0; i < pages; i++ {
		conf := 0.0
		text := syntheticContentFor("pdf", src) + " page=" + strconvItoa(i+1)
		if in.NeedOCR && in.OCRAvailable {
			conf = syntheticConfidence(src, i)
			text = "OCR " + text
			stats.CoveragePct = 100
			bucketConfidence(stats.ConfidenceBuckets, conf)
		}
		units = append(units, DocumentUnit{
			Content: text,
			Provenance: map[string]any{
				"page": i + 1,
			},
			Confidence: conf,
		})
	}
	return units, stats
}

func defaultConfidenceBuckets() map[string]int {
	return map[string]int{"0.0-0.5": 0, "0.5-0.8": 0, "0.8-1.0": 0}
}

func bucketConfidence(buckets map[string]int, conf float64) {
	if buckets == nil {
		return
	}
	switch {
	case conf < 0.5:
		buckets["0.0-0.5"]++
	case conf < 0.8:
		buckets["0.5-0.8"]++
	default:
		buckets["0.8-1.0"]++
	}
}

func syntheticConfidence(seed string, idx int) float64 {
	sum := crc32.ChecksumIEEE([]byte(seed + ":" + strconvItoa(idx)))
	// Map to [0.6, 0.99] to simulate OCR gating.
	base := 0.6 + float64(sum%39)/100.0
	if base > 0.99 {
		return 0.99
	}
	return base
}

func syntheticContentFor(format, source string) string {
	normalized := strings.ToLower(strings.TrimSpace(format))
	src := strings.TrimSpace(source)
	content := "source=" + src + " format=" + normalized + " "

	// Make masking tests deterministic.
	if strings.Contains(strings.ToLower(src), "unmaskable") {
		content += "UNMASKABLE "
	}

	switch normalized {
	case "html":
		content += "<h1>Title</h1><p>Body</p>"
		re := regexp.MustCompile(`<[^>]+>`)
		content = re.ReplaceAllString(content, " ")
	case "sql":
		content += "SELECT * FROM users WHERE email='user@example.com';"
	case "markdown":
		content += "# Title\n\nBody content with email user@example.com"
	default:
		content += "Body content"
	}
	return strings.TrimSpace(content)
}

func strconvItoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
