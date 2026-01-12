package knowledge_space

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

type quotaPayload struct {
	CPUCores             int `json:"cpuCores" binding:"gte=1"`
	StorageGB            int `json:"storageGb" binding:"gte=50"`
	IngestionConcurrency int `json:"ingestionConcurrency" binding:"gte=1"`
}

type createSpaceRequest struct {
	SpaceName               string       `json:"spaceName" binding:"required"`
	DepartmentCode          string       `json:"departmentCode" binding:"required"`
	Quotas                  quotaPayload `json:"quotas" binding:"required"`
	PolicyTemplateVersionID string       `json:"policyTemplateVersionId" binding:"required"`
	IngestionProfileKey     string       `json:"ingestionProfileKey"`
	IndexProfileKey         string       `json:"indexProfileKey"`
	RAGProfileKey           string       `json:"ragProfileKey"`
	FeatureFlags            []string     `json:"featureFlags"`
	RequestedBy             string       `json:"requestedBy"`
}

type updateSpaceRequest struct {
	Quotas                  *quotaPayload `json:"quotas"`
	PolicyTemplateVersionID string        `json:"policyTemplateVersionId"`
	IngestionProfileKey     string        `json:"ingestionProfileKey"`
	IndexProfileKey         string        `json:"indexProfileKey"`
	RAGProfileKey           string        `json:"ragProfileKey"`
	FeatureFlags            []string      `json:"featureFlags"`
	Status                  string        `json:"status"`
	UpdatedBy               string        `json:"updatedBy"`
}

type retireSpaceRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requestedBy"`
}

type knowledgeSpaceResponse struct {
	SpaceID             string       `json:"spaceId"`
	TenantUUID          string       `json:"tenant_uuid"`
	SpaceName           string       `json:"spaceName"`
	DepartmentCode      string       `json:"departmentCode"`
	Status              string       `json:"status"`
	PolicyTemplateID    string       `json:"policyTemplateVersionId"`
	IngestionProfileKey string       `json:"ingestionProfileKey"`
	IndexProfileKey     string       `json:"indexProfileKey"`
	RAGProfileKey       string       `json:"ragProfileKey"`
	FeatureFlags        []string     `json:"featureFlags"`
	AuditToken          string       `json:"auditToken"`
	RetentionExpiresAt  *time.Time   `json:"retentionExpiresAt,omitempty"`
	Quotas              quotaPayload `json:"quotas"`
	IAMStatus           string       `json:"iamStatus"`
}

func toResponse(space *models.KnowledgeSpace) knowledgeSpaceResponse {
	if space == nil {
		return knowledgeSpaceResponse{}
	}
	flags := ksvc.FeatureFlagsFromJSON(space.FeatureFlags)
	return knowledgeSpaceResponse{
		SpaceID:             space.UUID.String(),
		TenantUUID:          space.TenantUUID,
		SpaceName:           space.SpaceName,
		DepartmentCode:      space.DepartmentCode,
		Status:              space.Status,
		PolicyTemplateID:    ksvc.PolicyIDString(space.PolicyTemplateVersionID),
		IngestionProfileKey: strings.TrimSpace(space.IngestionProfileKey),
		IndexProfileKey:     strings.TrimSpace(space.IndexProfileKey),
		RAGProfileKey:       strings.TrimSpace(space.RAGProfileKey),
		FeatureFlags:        flags,
		AuditToken:          space.AuditToken,
		RetentionExpiresAt:  space.RetentionExpiresAt,
		Quotas: quotaPayload{
			CPUCores:             space.QuotaCPU,
			StorageGB:            space.QuotaStorageGB,
			IngestionConcurrency: ksvc.ExtractConcurrencyFlag(flags),
		},
		IAMStatus: iamStatus(space.Status),
	}
}

func iamStatus(status string) string {
	if status == models.KnowledgeSpaceStatusPending {
		return "pending_iam"
	}
	return "ready"
}

func parseUUID(id string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(id))
}

type ingestionJobRequest struct {
	// Format is the preferred field; sourceType is kept for compatibility.
	Format           string `json:"format" binding:"omitempty,oneof=pdf docx xlsx csv markdown html sql image table api"`
	SourceType       string `json:"sourceType" binding:"omitempty,oneof=pdf docx xlsx csv markdown html sql image table api"`
	SourceURI        string `json:"sourceUri" binding:"required"`
	IngestionProfile string `json:"ingestionProfile"`
	ProcessorProfile string `json:"processorProfile"`
	OCRRequired      bool   `json:"ocrRequired"`
	MaskingProfile   string `json:"maskingProfile"`
	Priority         string `json:"priority" binding:"omitempty,oneof=normal high"`
	RequestedBy      string `json:"requestedBy"`
	// L1/L2/L3 selection snapshot (for audit / mapping / future profiles).
	RagSceneKey  string `json:"ragSceneKey"`
	RagBundleKey string `json:"ragBundleKey"`
	RagPrimary   string `json:"ragPrimary"`
	// Chunking controls (optional). When set, they are applied on top of processor output.
	SegmentMode  string `json:"segmentMode" binding:"omitempty,oneof=unit heading clause semantic table_row code_block conversation"`
	ChunkSize    int `json:"chunkSize" binding:"omitempty,min=0,max=20000"`
	ChunkOverlap int `json:"chunkOverlap" binding:"omitempty,min=0,max=5000"`
	// Separators are preferred boundaries applied before windowing; supports punctuation and newline tokens.
	Separators []string `json:"separators" binding:"omitempty,dive,max=16"`
	// Anchors: included in chunk metadata (best-effort).
	AnchorHeadingPath  bool `json:"anchorHeadingPath"`
	AnchorClauseID     bool `json:"anchorClauseId"`
	AnchorRowNumber    bool `json:"anchorRowNumber"`
	AnchorSpeaker      bool `json:"anchorSpeaker"`
	AnchorSentenceIndex bool `json:"anchorSentenceIndex"`
}

type ingestionJobView struct {
	JobID               string  `json:"jobId"`
	Status              string  `json:"status"`
	RetryCount          int     `json:"retryCount"`
	ErrorCode           string  `json:"errorCode,omitempty"`
	Reason              string  `json:"reason,omitempty"`
	ChunkTotal          int     `json:"chunkTotal"`
	ChunkCoveragePct    float64 `json:"chunkCoveragePct"`
	EmbeddingSuccessPct float64 `json:"embeddingSuccessPct"`
	MaskingCoveragePct  float64 `json:"maskingCoveragePct"`
}

func toIngestionJobView(job *models.IngestionJob) ingestionJobView {
	if job == nil {
		return ingestionJobView{}
	}
	return ingestionJobView{
		JobID:               job.UUID.String(),
		Status:              job.Status,
		RetryCount:          job.RetryCount,
		ErrorCode:           job.ErrorCode,
		Reason:              job.BlockedReason,
		ChunkTotal:          job.ChunkTotal,
		ChunkCoveragePct:    job.ChunkCoveredPct,
		EmbeddingSuccessPct: job.EmbeddingSuccessPct,
		MaskingCoveragePct:  job.MaskingCoveragePct,
	}
}

type feedbackRequest struct {
	Severity     string   `json:"severity" binding:"required,oneof=low medium high critical"`
	IssueType    string   `json:"issueType" binding:"required,oneof=accuracy freshness compliance"`
	LinkedChunks []string `json:"linkedChunks" binding:"required,dive,uuid4"`
	Notes        string   `json:"notes"`
	ToolTraceRef string   `json:"toolTraceRef"`
	ReportedBy   string   `json:"reportedBy"`
}

type feedbackCaseActionRequest struct {
	RequestedBy     string `json:"requestedBy"`
	ResolutionNotes string `json:"resolutionNotes"`
	Reason          string `json:"reason"`
}

type feedbackResponse struct {
	CaseID        string     `json:"caseId"`
	Status        string     `json:"status"`
	Severity      string     `json:"severity"`
	IssueType     string     `json:"issueType"`
	LinkedChunks  []string   `json:"linkedChunks"`
	ReportedBy    string     `json:"reportedBy"`
	SlaDueAt      *time.Time `json:"slaDueAt,omitempty"`
	QualityScore  float64    `json:"qualityScore"`
	ReprocessJob  string     `json:"reprocessJobId,omitempty"`
	TraceID       string     `json:"traceId,omitempty"`
	ToolTraceRef  string     `json:"toolTraceRef,omitempty"`
	EscalatedAt   *time.Time `json:"escalatedAt,omitempty"`
	ClosedAt      *time.Time `json:"closedAt,omitempty"`
	Resolution    string     `json:"resolutionNotes,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastUpdatedAt time.Time  `json:"updatedAt"`
}

func toFeedbackResponse(caseModel *models.FeedbackCase) feedbackResponse {
	if caseModel == nil {
		return feedbackResponse{}
	}
	chunks := decodeChunks(caseModel.LinkedChunks)
	resp := feedbackResponse{
		CaseID:        caseModel.UUID.String(),
		Status:        caseModel.Status,
		Severity:      caseModel.Severity,
		IssueType:     caseModel.IssueType,
		LinkedChunks:  chunks,
		ReportedBy:    caseModel.ReportedBy,
		SlaDueAt:      caseModel.SLADueAt,
		QualityScore:  caseModel.QualityScore,
		TraceID:       caseModel.ToolTraceRef,
		ToolTraceRef:  caseModel.ToolTraceRef,
		EscalatedAt:   caseModel.EscalatedAt,
		ClosedAt:      caseModel.ClosedAt,
		Resolution:    caseModel.ResolutionNotes,
		CreatedAt:     caseModel.CreatedAt,
		LastUpdatedAt: caseModel.UpdatedAt,
	}
	if caseModel.ReprocessJobID != nil {
		resp.ReprocessJob = strconv.FormatUint(*caseModel.ReprocessJobID, 10)
	}
	return resp
}

func decodeChunks(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var chunks []string
	if err := json.Unmarshal(raw, &chunks); err != nil {
		return nil
	}
	return chunks
}

type fusionStrategyRequest struct {
	Label           string  `json:"label" binding:"required"`
	BM25Weight      float64 `json:"bm25Weight" binding:"gte=0"`
	VectorWeight    float64 `json:"vectorWeight" binding:"gte=0"`
	GraphConstraint string  `json:"graphConstraint" binding:"required"`
	RerankerModel   string  `json:"rerankerModel"`
	ConflictPolicy  string  `json:"conflictPolicy" binding:"omitempty,oneof=block queue allow_with_flag"`
	RequestedBy     string  `json:"requestedBy"`
}

type fusionStrategyResponse struct {
	StrategyID      string     `json:"strategyId"`
	SpaceID         string     `json:"spaceId"`
	Label           string     `json:"label"`
	BM25Weight      float64    `json:"bm25Weight"`
	VectorWeight    float64    `json:"vectorWeight"`
	GraphConstraint string     `json:"graphConstraint"`
	RerankerModel   string     `json:"rerankerModel"`
	ConflictPolicy  string     `json:"conflictPolicy"`
	DeploymentState string     `json:"deploymentState"`
	Degraded        bool       `json:"degraded"`
	DegradeReasons  []string   `json:"degradeReasons,omitempty"`
	PublishedAt     *time.Time `json:"publishedAt,omitempty"`
}

type eventApplyRequest struct {
	EventID    string         `json:"eventId" binding:"required"`
	EventType  string         `json:"eventType" binding:"required"`
	Payload    map[string]any `json:"payload"`
	RetryCount int            `json:"retryCount"`
	ReceivedAt *time.Time     `json:"receivedAt"`
}

type eventResponse struct {
	Status    string    `json:"status"`
	EventID   string    `json:"eventId"`
	Processed time.Time `json:"processedAt"`
}

type startDeltaJobRequest struct {
	SpaceID      string  `json:"spaceId" binding:"required,uuid4"`
	Source       string  `json:"source" binding:"required"`
	PackageURI   string  `json:"packageUri"`
	DiffAccuracy float64 `json:"diffAccuracy"`
	RequestedBy  string  `json:"requestedBy"`
	Notes        string  `json:"notes"`
}

type publishDeltaJobRequest struct {
	JobID          string  `json:"jobId" binding:"required,uuid4"`
	Decision       string  `json:"decision" binding:"required"`
	ApprovedBy     string  `json:"approvedBy"`
	DiffAccuracy   float64 `json:"diffAccuracy"`
	PartialRelease bool    `json:"partialRelease"`
}

type rollbackDeltaRequest struct {
	JobID       string `json:"jobId" binding:"required,uuid4"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requestedBy"`
}

type deltaJobView struct {
	JobID          string     `json:"jobId"`
	SpaceID        string     `json:"spaceId"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	ApprovalState  string     `json:"approvalState"`
	DiffAccuracy   float64    `json:"diffAccuracy"`
	PartialRelease bool       `json:"partialRelease"`
	RollbackCount  int        `json:"rollbackCount"`
	CreatedAt      time.Time  `json:"createdAt"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
	Report         any        `json:"report"`
}

func toDeltaJobView(job *models.DeltaJob) deltaJobView {
	if job == nil {
		return deltaJobView{}
	}
	var report any
	if len(job.Report) > 0 {
		_ = json.Unmarshal(job.Report, &report)
	}
	return deltaJobView{
		JobID:          job.UUID.String(),
		SpaceID:        job.SpaceUUID.String(),
		Source:         job.Source,
		Status:         job.Status,
		ApprovalState:  job.ApprovalState,
		DiffAccuracy:   job.DiffAccuracy,
		PartialRelease: job.PartialRelease,
		RollbackCount:  job.RollbackCount,
		CreatedAt:      job.CreatedAt,
		PublishedAt:    job.PublishedAt,
		Report:         report,
	}
}

func toFusionStrategyResponse(strategy *models.FusionStrategyVersion) fusionStrategyResponse {
	if strategy == nil {
		return fusionStrategyResponse{}
	}
	var snap struct {
		DegradeReasons []string `json:"degrade_reasons"`
	}
	if len(strategy.BenchmarkMetrics) > 0 {
		_ = json.Unmarshal(strategy.BenchmarkMetrics, &snap)
	}
	degraded := len(snap.DegradeReasons) > 0
	return fusionStrategyResponse{
		StrategyID:      strategyIDString(strategy.ID),
		SpaceID:         strategy.SpaceUUID.String(),
		Label:           strategy.Label,
		BM25Weight:      strategy.BM25Weight,
		VectorWeight:    strategy.VectorWeight,
		GraphConstraint: strategy.GraphConstraint,
		RerankerModel:   strategy.RerankerModel,
		ConflictPolicy:  strategy.ConflictPolicy,
		DeploymentState: strategy.DeploymentState,
		Degraded:        degraded,
		DegradeReasons:  snap.DegradeReasons,
		PublishedAt:     strategy.PublishedAt,
	}
}

func toFusionStrategyList(strategies []*models.FusionStrategyVersion) []fusionStrategyResponse {
	out := make([]fusionStrategyResponse, 0, len(strategies))
	for _, item := range strategies {
		out = append(out, toFusionStrategyResponse(item))
	}
	return out
}

func strategyIDString(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}
