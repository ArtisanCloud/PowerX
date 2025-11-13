package knowledge_space

import (
	"strings"
	"time"

	"github.com/google/uuid"

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
