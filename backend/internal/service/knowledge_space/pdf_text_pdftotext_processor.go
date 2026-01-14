package knowledge_space

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PDFTextPdftotextProcessor extracts embedded text from PDFs using `pdftotext`.
// It is intended for "normal" PDFs (non-scanned). For scanned PDFs, use OCR Plan B.
//
// Requirements:
// - `pdftotext` installed (poppler-utils).
//
// Output:
// - DocumentUnit per page (best-effort), with provenance `{ "page": <n> }`.
type PDFTextPdftotextProcessor struct{}

func (PDFTextPdftotextProcessor) Name() string { return "builtin/pdf_text" }

func (PDFTextPdftotextProcessor) Process(ctx context.Context, in DocumentProcessInput) (DocumentProcessResult, error) {
	start := time.Now()
	src := strings.TrimSpace(in.SourceURI)
	if src == "" {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, errors.New("source uri is empty")
	}

	if _, err := exec.LookPath("pdftotext"); err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, fmt.Errorf("pdftotext not found: %w", err)
	}

	workDir, err := os.MkdirTemp("", "powerx-pdftext-*")
	if err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	pdfPath := filepath.Join(workDir, "source.pdf")
	if err := fetchToFile(ctx, src, pdfPath); err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}
	if maxBytes := envInt64("POWERX_PDF_TEXT_MAX_BYTES", 50*1024*1024); maxBytes > 0 {
		if st, err := os.Stat(pdfPath); err == nil && st.Size() > maxBytes {
			return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, fmt.Errorf("pdf too large: %d > %d bytes", st.Size(), maxBytes)
		}
	}

	timeoutSec := envInt("POWERX_PDF_TEXT_TIMEOUT_SECONDS", 30)
	if timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
	}

	// -layout: keep reading order as much as possible
	// -nopgbrk: keep pages separable by formfeed (\f) rather than extra blank lines
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-nopgbrk", pdfPath, "-")
	out, err := cmd.Output()
	if err != nil {
		// include stderr if possible
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, fmt.Errorf("pdftotext failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		// Not an error: likely scanned PDF without embedded text.
		return DocumentProcessResult{
			Units: nil,
			OCR: OCRStats{
				CoveragePct:       0,
				ConfidenceBuckets: defaultConfidenceBuckets(),
				LatencyMs:         time.Since(start).Milliseconds(),
			},
		}, nil
	}

	// pdftotext uses formfeed as page delimiter.
	pages := strings.Split(raw, "\f")
	units := make([]DocumentUnit, 0, len(pages))
	for i, p := range pages {
		txt := strings.TrimSpace(p)
		if txt == "" {
			continue
		}
		units = append(units, DocumentUnit{
			Content: txt,
			Provenance: map[string]any{
				"page": i + 1,
			},
		})
	}
	if len(units) == 0 {
		return DocumentProcessResult{
			Units: nil,
			OCR: OCRStats{
				CoveragePct:       0,
				ConfidenceBuckets: defaultConfidenceBuckets(),
				LatencyMs:         time.Since(start).Milliseconds(),
			},
		}, nil
	}

	return DocumentProcessResult{
		Units: units,
		OCR: OCRStats{
			CoveragePct:       0,
			ConfidenceBuckets: defaultConfidenceBuckets(),
			LatencyMs:         time.Since(start).Milliseconds(),
			PageCount:         len(units),
		},
	}, nil
}
