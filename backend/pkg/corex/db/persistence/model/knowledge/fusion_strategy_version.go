package knowledge

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// FusionStrategyVersion 记录多源融合策略的版本，支持权重/回滚。
type FusionStrategyVersion struct {
	coremodel.PowerModel

	SpaceUUID             uuid.UUID      `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	Label                 string         `gorm:"column:label;type:varchar(128);not null" json:"label"`
	BM25Weight            float64        `gorm:"column:bm25_weight;type:numeric(4,3);not null" json:"bm25_weight"`
	VectorWeight          float64        `gorm:"column:vector_weight;type:numeric(4,3);not null" json:"vector_weight"`
	GraphConstraint       string         `gorm:"column:graph_constraint;type:text" json:"graph_constraint,omitempty"`
	RerankerModel         string         `gorm:"column:reranker_model;type:varchar(128)" json:"reranker_model,omitempty"`
	ConflictPolicy        string         `gorm:"column:conflict_policy;type:varchar(32);not null;default:'block'" json:"conflict_policy"`
	DeploymentState       string         `gorm:"column:deployment_state;type:varchar(32);not null;default:'draft';index" json:"deployment_state"`
	BenchmarkMetrics      datatypes.JSON `gorm:"column:benchmark_metrics;type:jsonb;default:'{}'" json:"benchmark_metrics"`
	PublishedBy           string         `gorm:"column:published_by;type:varchar(128)" json:"published_by,omitempty"`
	PublishedAt           *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	RollbackFromVersionID *uint64        `gorm:"column:rollback_from_version_id" json:"rollback_from_version_id,omitempty"`
}

func (FusionStrategyVersion) TableName() string {
	return tableName(coremodel.TableKnowledgeFusionStrategies)
}

const (
	FusionDeploymentDraft    = "draft"
	FusionDeploymentActive   = "active"
	FusionDeploymentRollback = "rollback"
)
