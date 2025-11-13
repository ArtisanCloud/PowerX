package knowledge

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// ArtifactBundle 追踪 chunk/vector/graph 等产物。
type ArtifactBundle struct {
	coremodel.PowerModel

	IngestionJobID      uint64     `gorm:"column:ingestion_job_id;not null;uniqueIndex" json:"ingestion_job_id"`
	ChunkManifestURI    string     `gorm:"column:chunk_manifest_uri;type:text;not null" json:"chunk_manifest_uri"`
	VectorManifestURI   string     `gorm:"column:vector_manifest_uri;type:text" json:"vector_manifest_uri"`
	GraphManifestURI    string     `gorm:"column:graph_manifest_uri;type:text" json:"graph_manifest_uri"`
	MaskingReportURI    string     `gorm:"column:masking_report_uri;type:text" json:"masking_report_uri"`
	SummaryChunkCount   int        `gorm:"column:summary_chunk_count;type:int;not null;default:0" json:"summary_chunk_count"`
	ParagraphChunkCount int        `gorm:"column:paragraph_chunk_count;type:int;not null;default:0" json:"paragraph_chunk_count"`
	Checksum            string     `gorm:"column:checksum;type:char(64);not null" json:"checksum"`
	StorageClass        string     `gorm:"column:storage_class;type:varchar(32);not null;default:'standard'" json:"storage_class"`
	Status              string     `gorm:"column:status;type:varchar(32);not null;default:'active';index" json:"status"`
	RetainedUntil       *time.Time `gorm:"column:retained_until" json:"retained_until,omitempty"`
}

func (ArtifactBundle) TableName() string {
	return tableName(coremodel.TableKnowledgeArtifactBundles)
}

const (
	ArtifactBundleStatusActive   = "active"
	ArtifactBundleStatusArchived = "archived"
	ArtifactBundleStatusPurged   = "purged"
)
