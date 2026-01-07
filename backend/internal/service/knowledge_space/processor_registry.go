package knowledge_space

import (
	"context"
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
}

type DocumentUnit struct {
	Content    string
	Provenance map[string]any
	Confidence float64
}

type DocumentProcessInput struct {
	Format       string
	SourceURI    string
	NeedOCR      bool
	OCRAvailable bool
}

type DocumentProcessor interface {
	Name() string
	Process(ctx context.Context, in DocumentProcessInput) ([]DocumentUnit, OCRStats)
}

type ProcessorRegistry struct {
	processors   map[string]DocumentProcessor
	ocrAvailable bool
	profiles     map[string]bool
}

func NewProcessorRegistry() *ProcessorRegistry {
	reg := &ProcessorRegistry{
		processors: make(map[string]DocumentProcessor),
		profiles:   make(map[string]bool),
	}
	reg.RegisterProcessor(TextProcessor{})
	reg.RegisterProcessor(TableProcessor{})
	reg.RegisterProcessor(PDFProcessor{})
	reg.profiles["builtin/default"] = true
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
	}
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
		selected = PDFProcessor{}
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
