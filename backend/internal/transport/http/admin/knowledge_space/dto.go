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
	TenantID                string       `json:"tenantId" binding:"required,uuid4"`
	SpaceName               string       `json:"spaceName" binding:"required"`
	DepartmentCode          string       `json:"departmentCode" binding:"required"`
	Quotas                  quotaPayload `json:"quotas" binding:"required"`
	PolicyTemplateVersionID string       `json:"policyTemplateVersionId" binding:"required"`
	FeatureFlags            []string     `json:"featureFlags"`
	RequestedBy             string       `json:"requestedBy"`
}

type updateSpaceRequest struct {
	Quotas                  *quotaPayload `json:"quotas"`
	PolicyTemplateVersionID string        `json:"policyTemplateVersionId"`
	FeatureFlags            []string      `json:"featureFlags"`
	Status                  string        `json:"status"`
	UpdatedBy               string        `json:"updatedBy"`
}

type retireSpaceRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requestedBy"`
}

type knowledgeSpaceResponse struct {
	SpaceID            string       `json:"spaceId"`
	TenantID           string       `json:"tenantId"`
	SpaceName          string       `json:"spaceName"`
	DepartmentCode     string       `json:"departmentCode"`
	Status             string       `json:"status"`
	PolicyTemplateID   string       `json:"policyTemplateVersionId"`
	FeatureFlags       []string     `json:"featureFlags"`
	AuditToken         string       `json:"auditToken"`
	RetentionExpiresAt *time.Time   `json:"retentionExpiresAt,omitempty"`
	Quotas             quotaPayload `json:"quotas"`
	IAMStatus          string       `json:"iamStatus"`
}

func toResponse(space *models.KnowledgeSpace) knowledgeSpaceResponse {
	if space == nil {
		return knowledgeSpaceResponse{}
	}
	flags := ksvc.FeatureFlagsFromJSON(space.FeatureFlags)
	return knowledgeSpaceResponse{
		SpaceID:            space.UUID.String(),
		TenantID:           space.TenantID.String(),
		SpaceName:          space.SpaceName,
		DepartmentCode:     space.DepartmentCode,
		Status:             space.Status,
		PolicyTemplateID:   ksvc.PolicyIDString(space.PolicyTemplateVersionID),
		FeatureFlags:       flags,
		AuditToken:         space.AuditToken,
		RetentionExpiresAt: space.RetentionExpiresAt,
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
	SourceType     string `json:"sourceType" binding:"required,oneof=pdf markdown table api"`
	SourceURI      string `json:"sourceUri" binding:"required"`
	MaskingProfile string `json:"maskingProfile"`
	Priority       string `json:"priority" binding:"omitempty,oneof=normal high"`
	RequestedBy    string `json:"requestedBy"`
}

type ingestionJobView struct {
	JobID               string  `json:"jobId"`
	Status              string  `json:"status"`
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
	ToolTraceRef  string     `json:"toolTraceRef,omitempty"`
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
		ToolTraceRef:  caseModel.ToolTraceRef,
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
	PublishedAt     *time.Time `json:"publishedAt,omitempty"`
}

func toFusionStrategyResponse(strategy *models.FusionStrategyVersion) fusionStrategyResponse {
	if strategy == nil {
		return fusionStrategyResponse{}
	}
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
