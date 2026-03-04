package knowledge

import (
	"time"

	"github.com/google/uuid"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// KnowledgeVectorIndex 记录 space 当前/历史的 dense 向量索引落点（按维度分表）。
// - 表名/维度决定了 pgvector 的 `vector(D)` 列类型
// - 一个 space 可保留多条记录用于回滚，但同一时刻只允许一个 active
type KnowledgeVectorIndex struct {
	coremodel.PowerModel

	SpaceUUID uuid.UUID `gorm:"column:space_uuid;type:uuid;not null;index:idx_kvi_space_status,priority:1;index:idx_kvi_space_key,unique,priority:1" json:"space_uuid"`

	IndexKey  string `gorm:"column:index_key;type:varchar(128);not null;index:idx_kvi_space_key,unique,priority:2;index:idx_kvi_index_key,priority:1" json:"index_key"`
	VectorTable string `gorm:"column:table_name;type:varchar(128);not null" json:"table_name"`
	Dimensions int   `gorm:"column:dimensions;type:int;not null" json:"dimensions"`

	EmbeddingProvider   string `gorm:"column:embedding_provider;type:varchar(64);not null" json:"embedding_provider"`
	EmbeddingModel      string `gorm:"column:embedding_model;type:varchar(128);not null" json:"embedding_model"`
	EmbeddingProfileRef string `gorm:"column:embedding_profile_ref;type:varchar(128)" json:"embedding_profile_ref,omitempty"`

	Status      string     `gorm:"column:status;type:varchar(32);not null;default:'creating';index:idx_kvi_space_status,priority:2" json:"status"`
	LastUsedAt  *time.Time `gorm:"column:last_used_at;index" json:"last_used_at,omitempty"`
	LastError   string     `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
}

func (KnowledgeVectorIndex) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeVectorIndexes
}

const (
	KnowledgeVectorIndexStatusCreating = "creating"
	KnowledgeVectorIndexStatusActive   = "active"
	KnowledgeVectorIndexStatusRetired  = "retired"
	KnowledgeVectorIndexStatusFailed   = "failed"
)
