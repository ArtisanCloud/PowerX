package knowledge_space

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PDFOCRTesseractProcessor implements Plan B:
// PDF → page images → tesseract TSV → paragraph units (may跨页) with bbox provenance.
//
// NOTE:
// - This processor relies on external binaries (`pdftoppm` or `mutool`, and `tesseract`).
// - It is only selected when processorProfile == "builtin/ocr_plan_b".
type PDFOCRTesseractProcessor struct{}

func (PDFOCRTesseractProcessor) Name() string { return "builtin/pdf_ocr_tesseract" }

func (PDFOCRTesseractProcessor) Process(ctx context.Context, in DocumentProcessInput) (DocumentProcessResult, error) {
	start := time.Now()
	if !in.NeedOCR {
		return DocumentProcessResult{
			Units: nil,
			OCR:   OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()},
		}, nil
	}
	if !in.OCRAvailable {
		return DocumentProcessResult{
			Units: nil,
			OCR:   OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()},
		}, ErrOCRUnavailable
	}

	src := strings.TrimSpace(in.SourceURI)
	if src == "" {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, errors.New("source uri is empty")
	}

	workDir, err := os.MkdirTemp("", "powerx-ocr-planb-*")
	if err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}
	// Best-effort cleanup; artifacts will be copied into ArtifactStore later.
	defer func() { _ = os.RemoveAll(workDir) }()

	pdfPath := filepath.Join(workDir, "source.pdf")
	if err := fetchToFile(ctx, src, pdfPath); err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}
	if maxBytes := envInt64("POWERX_OCR_MAX_BYTES", 50*1024*1024); maxBytes > 0 {
		if st, err := os.Stat(pdfPath); err == nil && st.Size() > maxBytes {
			return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, fmt.Errorf("pdf too large: %d > %d bytes", st.Size(), maxBytes)
		}
	}

	overallTimeoutSec := envInt("POWERX_OCR_TIMEOUT_SECONDS", 300)
	if overallTimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(overallTimeoutSec)*time.Second)
		defer cancel()
	}

	pagesDir := filepath.Join(workDir, "pages")
	rawDir := filepath.Join(workDir, "raw")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}

	dpi := envInt("POWERX_OCR_DPI", 200)
	if dpi < 72 {
		dpi = 72
	}
	if dpi > 600 {
		dpi = 600
	}
	if err := renderPDFToPNGs(ctx, pdfPath, pagesDir, dpi); err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}

	pageImages, err := listRenderedPages(pagesDir)
	if err != nil {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, err
	}
	if len(pageImages) == 0 {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, errors.New("no pages rendered")
	}
	if maxPages := envInt("POWERX_OCR_MAX_PAGES", 80); maxPages > 0 && len(pageImages) > maxPages {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: defaultConfidenceBuckets()}}, fmt.Errorf("too many pages: %d > %d", len(pageImages), maxPages)
	}

	lang := strings.TrimSpace(os.Getenv("POWERX_OCR_LANG"))
	if lang == "" {
		// Common default: Chinese + English.
		lang = "chi_sim+eng"
	}

	type pageResult struct {
		pageNum    int
		imgPath    string
		rawPath    string
		width      int
		height     int
		paragraphs []ocrParagraph
		err        error
	}

	buckets := defaultConfidenceBuckets()
	results := make([]pageResult, len(pageImages))
	for i := range pageImages {
		results[i] = pageResult{
			pageNum: i + 1,
			imgPath: pageImages[i],
			rawPath: filepath.Join(rawDir, fmt.Sprintf("%03d.tsv", i+1)),
		}
	}

	concurrency := envInt("POWERX_OCR_CONCURRENCY", 2)
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > 8 {
		concurrency = 8
	}
	if concurrency > len(results) {
		concurrency = len(results)
	}
	retries := envInt("POWERX_OCR_RETRIES", 1)
	if retries < 0 {
		retries = 0
	}
	retryDelayMs := envInt("POWERX_OCR_RETRY_DELAY_MS", 80)
	if retryDelayMs < 0 {
		retryDelayMs = 0
	}
	pageTimeoutSec := envInt("POWERX_OCR_PAGE_TIMEOUT_SECONDS", 60)
	if pageTimeoutSec <= 0 {
		pageTimeoutSec = 60
	}

	jobs := make(chan int)
	done := make(chan struct{})
	defer close(done)

	worker := func() {
		for idx := range jobs {
			r := &results[idx]
			w, h, err := imageSize(r.imgPath)
			if err != nil {
				r.err = err
				continue
			}
			r.width, r.height = w, h

			var tsv string
			var lastErr error
			for attempt := 0; attempt <= retries; attempt++ {
				pageCtx, cancel := context.WithTimeout(ctx, time.Duration(pageTimeoutSec)*time.Second)
				out, err := runTesseractTSV(pageCtx, r.imgPath, lang)
				cancel()
				if err == nil && strings.TrimSpace(out) != "" {
					tsv = out
					lastErr = nil
					break
				}
				lastErr = err
				if attempt < retries && retryDelayMs > 0 {
					select {
					case <-time.After(time.Duration(retryDelayMs) * time.Millisecond):
					case <-ctx.Done():
						lastErr = ctx.Err()
						break
					}
				}
			}
			if lastErr != nil {
				r.err = lastErr
				continue
			}
			if err := os.WriteFile(r.rawPath, []byte(tsv), 0o644); err != nil {
				r.err = err
				continue
			}
			lines := parseTesseractTSV(tsv, r.pageNum, w, h)
			r.paragraphs = groupLinesToParagraphs(lines)
		}
	}

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() { defer wg.Done(); worker() }()
	}
	go func() {
		for i := range results {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
		close(jobs)
	}()
	wg.Wait()

	pageParagraphs := make([][]ocrParagraph, 0, len(results))
	artPages := make([]OCRArtifactPage, 0, len(results))
	failedPages := 0
	for _, r := range results {
		if r.err != nil {
			failedPages++
			continue
		}
		pageParagraphs = append(pageParagraphs, r.paragraphs)
		for _, p := range r.paragraphs {
			bucketConfidence(buckets, p.Confidence)
		}
		artPages = append(artPages, OCRArtifactPage{
			PageNumber: r.pageNum,
			ImagePath:  r.imgPath,
			RawPath:    r.rawPath,
			Width:      r.width,
			Height:     r.height,
		})
	}

	merged := mergeParagraphsAcrossPages(pageParagraphs)
	units := make([]DocumentUnit, 0, len(merged))
	for _, p := range merged {
		content := strings.TrimSpace(p.Text)
		if content == "" {
			continue
		}
		units = append(units, DocumentUnit{
			Content:    content,
			Provenance: p.Provenance(src),
			Confidence: p.Confidence,
		})
	}

	coverage := 0.0
	bboxCoverage := 0.0
	if len(pageImages) > 0 {
		coverage = float64(len(pageImages)-failedPages) * 100 / float64(len(pageImages))
	}
	if len(units) > 0 {
		bboxCoverage = coverage
	}
	if len(units) == 0 && failedPages == len(pageImages) {
		return DocumentProcessResult{OCR: OCRStats{CoveragePct: 0, ConfidenceBuckets: buckets}}, errors.New("ocr failed for all pages")
	}

	return DocumentProcessResult{
		Units: units,
		OCR: OCRStats{
			CoveragePct:       coverage,
			ConfidenceBuckets: buckets,
			LatencyMs:         time.Since(start).Milliseconds(),
			PageCount:         len(pageImages),
			FailedPages:       failedPages,
			BboxCoveragePct:   bboxCoverage,
		},
		Artifacts: &OCRArtifacts{
			RawFormat: "tsv",
			Pages:     artPages,
		},
	}, nil
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func envInt64(key string, def int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return def
	}
	return v
}

var ErrUnsupportedSourceURIScheme = errors.New("unsupported source uri scheme")

func fetchToFile(ctx context.Context, sourceURI string, dstPath string) error {
	sourceURI = strings.TrimSpace(sourceURI)
	if sourceURI == "" {
		return errors.New("empty source uri")
	}
	u, err := url.Parse(sourceURI)
	if err != nil {
		// treat as local path
		return copyFile(sourceURI, dstPath)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURI, nil)
		if err != nil {
			return err
		}
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("download failed: %s", resp.Status)
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		out, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	case "file":
		return copyFile(u.Path, dstPath)
	case "":
		return copyFile(sourceURI, dstPath)
	default:
		// minio://... etc are not supported in this stage.
		return fmt.Errorf("%w: %s", ErrUnsupportedSourceURIScheme, u.Scheme)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func imageSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func renderPDFToPNGs(ctx context.Context, pdfPath string, outDir string, dpi int) error {
	if _, err := exec.LookPath("pdftoppm"); err == nil {
		prefix := filepath.Join(outDir, "page")
		cmd := exec.CommandContext(ctx, "pdftoppm", "-png", "-r", strconv.Itoa(dpi), pdfPath, prefix)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("pdftoppm failed: %w (%s)", err, string(out))
		}
		return nil
	}
	if _, err := exec.LookPath("mutool"); err == nil {
		outPattern := filepath.Join(outDir, "page-%03d.png")
		cmd := exec.CommandContext(ctx, "mutool", "draw", "-r", strconv.Itoa(dpi), "-o", outPattern, pdfPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("mutool draw failed: %w (%s)", err, string(out))
		}
		return nil
	}
	return errors.New("missing renderer: pdftoppm or mutool")
}

func listRenderedPages(dir string) ([]string, error) {
	// pdftoppm: page-1.png / page-2.png ...
	// mutool: page-001.png ...
	candidates, err := filepath.Glob(filepath.Join(dir, "page-*.png"))
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		candidates, err = filepath.Glob(filepath.Join(dir, "page*.png"))
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return pageIndex(candidates[i]) < pageIndex(candidates[j]) })
	return candidates, nil
}

func pageIndex(path string) int {
	base := filepath.Base(path)
	re := regexp.MustCompile(`(\d+)`)
	m := re.FindStringSubmatch(base)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func runTesseractTSV(ctx context.Context, imagePath string, lang string) (string, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return "", errors.New("missing binary: tesseract")
	}
	args := []string{imagePath, "stdout"}
	if strings.TrimSpace(lang) != "" {
		args = append(args, "-l", lang)
	}
	args = append(args, "tsv")
	cmd := exec.CommandContext(ctx, "tesseract", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tesseract failed: %w (%s)", err, string(out))
	}
	return string(out), nil
}

type ocrLine struct {
	Page int
	Text string
	X1   float64
	Y1   float64
	X2   float64
	Y2   float64
	Conf float64
}

func parseTesseractTSV(tsv string, pageNum int, width int, height int) []ocrLine {
	widthF := float64(width)
	heightF := float64(height)
	if widthF <= 0 || heightF <= 0 {
		return nil
	}
	lines := make([]ocrLine, 0, 256)
	sc := bufio.NewScanner(strings.NewReader(tsv))
	first := true
	for sc.Scan() {
		row := sc.Text()
		if first {
			first = false
			// header: level\tpage_num\tblock_num\t...
			continue
		}
		cols := strings.Split(row, "\t")
		// tesseract tsv has 12 columns
		if len(cols) < 12 {
			continue
		}
		level, _ := strconv.Atoi(cols[0])
		if level != 4 { // line-level
			continue
		}
		left, _ := strconv.Atoi(cols[6])
		top, _ := strconv.Atoi(cols[7])
		w, _ := strconv.Atoi(cols[8])
		h, _ := strconv.Atoi(cols[9])
		conf, _ := strconv.ParseFloat(cols[10], 64)
		text := strings.TrimSpace(cols[11])
		if text == "" {
			continue
		}

		x1 := float64(left) / widthF
		y1 := float64(top) / heightF
		x2 := float64(left+w) / widthF
		y2 := float64(top+h) / heightF
		x1, y1 = clamp01Float(x1), clamp01Float(y1)
		x2, y2 = clamp01Float(x2), clamp01Float(y2)
		if x2 <= x1 || y2 <= y1 {
			continue
		}
		if conf < 0 {
			conf = 0
		}
		if conf > 100 {
			conf = 100
		}
		lines = append(lines, ocrLine{
			Page: pageNum,
			Text: text,
			X1:   x1,
			Y1:   y1,
			X2:   x2,
			Y2:   y2,
			Conf: conf / 100.0,
		})
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Y1 == lines[j].Y1 {
			return lines[i].X1 < lines[j].X1
		}
		return lines[i].Y1 < lines[j].Y1
	})
	return lines
}

func clamp01Float(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

type ocrParagraph struct {
	Text       string
	Pages      []ocrParagraphPage
	Confidence float64
	Y1         float64
	Y2         float64
}

type ocrParagraphPage struct {
	PageNumber int
	Region     ocrRegion
}

type ocrRegion struct {
	X1         float64
	Y1         float64
	X2         float64
	Y2         float64
	Confidence float64
}

func (p ocrParagraph) Provenance(sourceURI string) map[string]any {
	pages := make([]any, 0, len(p.Pages))
	for _, pg := range p.Pages {
		pages = append(pages, map[string]any{
			"page_number": pg.PageNumber,
			"regions": []any{
				map[string]any{
					"x1":         pg.Region.X1,
					"y1":         pg.Region.Y1,
					"x2":         pg.Region.X2,
					"y2":         pg.Region.Y2,
					"confidence": pg.Region.Confidence,
				},
			},
		})
	}
	return map[string]any{
		"source_uri": strings.TrimSpace(sourceURI),
		"pages":      pages,
	}
}

var clauseStartRe = regexp.MustCompile(`^(\\d+(?:\\.\\d+)*|第[一二三四五六七八九十百千]+条|\\([0-9]+\\)|\\（[0-9]+\\）)`)

func groupLinesToParagraphs(lines []ocrLine) []ocrParagraph {
	if len(lines) == 0 {
		return nil
	}
	out := make([]ocrParagraph, 0, 64)
	var cur []ocrLine
	flush := func() {
		if len(cur) == 0 {
			return
		}
		p := paragraphFromLines(cur)
		if strings.TrimSpace(p.Text) != "" {
			out = append(out, p)
		}
		cur = nil
	}
	prevY2 := 0.0
	for i, ln := range lines {
		if i == 0 {
			cur = append(cur, ln)
			prevY2 = ln.Y2
			continue
		}
		gap := ln.Y1 - prevY2
		newPara := gap > 0.03
		if !newPara && clauseStartRe.MatchString(strings.TrimSpace(ln.Text)) && len(cur) > 0 {
			// clause boundary is a good paragraph boundary for scanned legal docs.
			newPara = true
		}
		if newPara {
			flush()
		}
		cur = append(cur, ln)
		if ln.Y2 > prevY2 {
			prevY2 = ln.Y2
		}
	}
	flush()
	return out
}

func paragraphFromLines(lines []ocrLine) ocrParagraph {
	if len(lines) == 0 {
		return ocrParagraph{}
	}
	page := lines[0].Page
	minX1, minY1 := 1.0, 1.0
	maxX2, maxY2 := 0.0, 0.0
	sumConf := 0.0
	texts := make([]string, 0, len(lines))
	for _, ln := range lines {
		texts = append(texts, strings.TrimSpace(ln.Text))
		if ln.X1 < minX1 {
			minX1 = ln.X1
		}
		if ln.Y1 < minY1 {
			minY1 = ln.Y1
		}
		if ln.X2 > maxX2 {
			maxX2 = ln.X2
		}
		if ln.Y2 > maxY2 {
			maxY2 = ln.Y2
		}
		sumConf += ln.Conf
	}
	conf := sumConf / float64(len(lines))
	txt := strings.Join(texts, "\n")
	return ocrParagraph{
		Text: txt,
		Pages: []ocrParagraphPage{{
			PageNumber: page,
			Region: ocrRegion{
				X1:         clamp01Float(minX1),
				Y1:         clamp01Float(minY1),
				X2:         clamp01Float(maxX2),
				Y2:         clamp01Float(maxY2),
				Confidence: conf,
			},
		}},
		Confidence: conf,
		Y1:         minY1,
		Y2:         maxY2,
	}
}

func mergeParagraphsAcrossPages(pages [][]ocrParagraph) []ocrParagraph {
	if len(pages) == 0 {
		return nil
	}
	out := make([]ocrParagraph, 0, 256)
	var prev *ocrParagraph
	for pageIdx := 0; pageIdx < len(pages); pageIdx++ {
		for paraIdx := 0; paraIdx < len(pages[pageIdx]); paraIdx++ {
			cur := pages[pageIdx][paraIdx]
			if prev != nil && shouldMergeAcrossPages(*prev, cur) {
				prev.Text = strings.TrimSpace(prev.Text) + "\n" + strings.TrimSpace(cur.Text)
				prev.Pages = append(prev.Pages, cur.Pages...)
				prev.Confidence = (prev.Confidence + cur.Confidence) / 2
				prev.Y2 = cur.Y2
				continue
			}
			out = append(out, cur)
			prev = &out[len(out)-1]
		}
	}
	return out
}

func shouldMergeAcrossPages(prev ocrParagraph, next ocrParagraph) bool {
	if len(prev.Pages) == 0 || len(next.Pages) == 0 {
		return false
	}
	prevPage := prev.Pages[len(prev.Pages)-1].PageNumber
	nextPage := next.Pages[0].PageNumber
	if nextPage != prevPage+1 {
		return false
	}
	// Only consider boundary regions.
	if prev.Y2 < 0.85 || next.Y1 > 0.15 {
		return false
	}
	prevText := strings.TrimSpace(prev.Text)
	nextText := strings.TrimSpace(next.Text)
	if prevText == "" || nextText == "" {
		return false
	}
	if endsWithStrongPunct(prevText) {
		return false
	}
	if clauseStartRe.MatchString(nextText) {
		return false
	}
	return true
}

func endsWithStrongPunct(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	r := []rune(s)
	last := r[len(r)-1]
	switch last {
	case '。', '！', '？', '!', '?', '.', ';', '；', '：', ':':
		return true
	default:
		return false
	}
}
