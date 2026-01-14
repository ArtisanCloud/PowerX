package knowledge_space

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

const (
	ProcessorDecisionOK       = "ok"
	ProcessorDecisionDegraded = "degraded"
	ProcessorDecisionBlocked  = "blocked"
)

type ProcessorResolution struct {
	Decision     string
	Reason       string
	ErrorCode    string
	OCRAvailable bool
	OCRUsed      bool
}

type OCRStats struct {
	CoveragePct       float64
	ConfidenceBuckets map[string]int
	LatencyMs         int64
	PageCount         int
	FailedPages       int
	BboxCoveragePct   float64
}

type DocumentUnit struct {
	Content    string
	Provenance map[string]any
	Confidence float64
}

type DocumentProcessInput struct {
	SpaceID      string
	JobID        string
	Format       string
	SourceURI    string
	NeedOCR      bool
	OCRAvailable bool
}

type DocumentProcessResult struct {
	Units     []DocumentUnit
	OCR       OCRStats
	Artifacts *OCRArtifacts
}

type DocumentProcessor interface {
	Name() string
	Process(ctx context.Context, in DocumentProcessInput) (DocumentProcessResult, error)
}

type ProcessorRegistry struct {
	processors       map[string]DocumentProcessor
	ocrAvailable     bool
	pdfTextAvailable bool
	profiles         map[string]bool
}

func NewProcessorRegistry() *ProcessorRegistry {
	reg := &ProcessorRegistry{
		processors: make(map[string]DocumentProcessor),
		profiles:   make(map[string]bool),
	}
	reg.RegisterProcessor(TextProcessor{})
	reg.RegisterProcessor(TableProcessor{})
	reg.RegisterProcessor(PDFProcessor{})
	reg.RegisterProcessor(PDFOCRTesseractProcessor{})
	reg.RegisterProcessor(PDFTextPdftotextProcessor{})
	reg.profiles["builtin/default"] = true

	// Auto-detect optional external binaries.
	// - OCR Plan B requires a PDF renderer + tesseract.
	// - PDF text extraction requires `pdftotext`.
	// Env overrides:
	// - POWERX_OCR_AVAILABLE=1/0
	// - POWERX_PDF_TEXT_AVAILABLE=1/0
	if v := strings.TrimSpace(os.Getenv("POWERX_OCR_AVAILABLE")); v != "" {
		reg.SetOCRAvailable(v == "1" || strings.EqualFold(v, "true"))
	} else {
		_, tesseractErr := exec.LookPath("tesseract")
		_, pdftoppmErr := exec.LookPath("pdftoppm")
		_, mutoolErr := exec.LookPath("mutool")
		hasRenderer := pdftoppmErr == nil || mutoolErr == nil
		reg.SetOCRAvailable(tesseractErr == nil && hasRenderer)
	}
	if v := strings.TrimSpace(os.Getenv("POWERX_PDF_TEXT_AVAILABLE")); v != "" {
		reg.SetPDFTextAvailable(v == "1" || strings.EqualFold(v, "true"))
	} else {
		_, pdftotextErr := exec.LookPath("pdftotext")
		reg.SetPDFTextAvailable(pdftotextErr == nil)
	}

	return reg
}

func (r *ProcessorRegistry) RegisterProcessor(p DocumentProcessor) {
	if r == nil || p == nil {
		return
	}
	name := strings.TrimSpace(p.Name())
	if name == "" {
		return
	}
	r.processors[name] = p
}

func (r *ProcessorRegistry) SetOCRAvailable(available bool) {
	if r == nil {
		return
	}
	r.ocrAvailable = available
	if available {
		r.profiles["builtin/ocr"] = true
		r.profiles["builtin/ocr_plan_b"] = true
		return
	}
	delete(r.profiles, "builtin/ocr")
	delete(r.profiles, "builtin/ocr_plan_b")
}

func (r *ProcessorRegistry) SetPDFTextAvailable(available bool) {
	if r == nil {
		return
	}
	r.pdfTextAvailable = available
	if available {
		r.profiles["builtin/pdf_text"] = true
		return
	}
	delete(r.profiles, "builtin/pdf_text")
}

func (r *ProcessorRegistry) SupportsProfile(profile string) bool {
	if r == nil {
		return false
	}
	p := strings.TrimSpace(profile)
	if p == "" {
		return true
	}
	return r.profiles[p]
}

func (r *ProcessorRegistry) Resolve(format string, needOCR bool, ocrRequired bool, processorProfile string) (DocumentProcessor, ProcessorResolution) {
	res := ProcessorResolution{
		Decision:     ProcessorDecisionOK,
		OCRAvailable: r != nil && r.ocrAvailable,
		OCRUsed:      needOCR && r != nil && r.ocrAvailable,
	}

	if r == nil {
		res.Decision = ProcessorDecisionDegraded
		res.ErrorCode = "degraded"
		res.Reason = "processor_registry_unavailable"
		return TextProcessor{}, res
	}

	if strings.TrimSpace(processorProfile) != "" && !r.SupportsProfile(processorProfile) {
		res.Decision = ProcessorDecisionDegraded
		res.ErrorCode = "degraded"
		res.Reason = "processor_profile_unavailable"
	}

	trimmed := strings.ToLower(strings.TrimSpace(format))
	var selected DocumentProcessor
	switch trimmed {
	case "csv", "xlsx", "table":
		selected = TableProcessor{}
	case "pdf":
		profile := strings.TrimSpace(processorProfile)
		switch {
		case needOCR && res.OCRAvailable && profile == "builtin/ocr_plan_b":
			selected = PDFOCRTesseractProcessor{}
		case profile == "builtin/pdf_text":
			if r != nil && r.pdfTextAvailable {
				selected = PDFTextPdftotextProcessor{}
			} else {
				res.Decision = ProcessorDecisionDegraded
				res.ErrorCode = "degraded"
				res.Reason = "pdf_text_processor_unavailable"
				selected = PDFProcessor{}
			}
		default:
			// Prefer text extraction for normal PDFs; fallback to synthetic builtin/pdf when unavailable.
			if r != nil && r.pdfTextAvailable {
				selected = PDFTextPdftotextProcessor{}
			} else {
				selected = PDFProcessor{}
			}
		}
	default:
		selected = TextProcessor{}
	}

	if needOCR && !res.OCRAvailable {
		if ocrRequired {
			res.Decision = ProcessorDecisionBlocked
			res.ErrorCode = "ocr_required"
			res.Reason = "ocr_processor_unavailable"
		} else {
			res.Decision = ProcessorDecisionDegraded
			if res.ErrorCode == "" {
				res.ErrorCode = "degraded"
			}
			if res.Reason == "" {
				res.Reason = "ocr_unavailable"
			}
		}
	}

	return selected, res
}
