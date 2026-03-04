package knowledge

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"time"
)

// KnowledgeChunk is the online truth source for chunk text + metadata (editable).
//
// NOTE: This table is provisioned by `migration.EnsureKnowledgeChunkStoreTables` when enabled
// (see `backend/cmd/database/migrate.go` conditional on index_backends).
type KnowledgeChunk struct {
	SpaceUUID uuid.UUID      `gorm:"column:space_uuid;type:uuid;not null;primaryKey" json:"space_uuid"`
	ChunkUUID uuid.UUID      `gorm:"column:chunk_uuid;type:uuid;not null;primaryKey" json:"chunk_uuid"`
	JobUUID   *uuid.UUID     `gorm:"column:job_uuid;type:uuid;index" json:"job_uuid,omitempty"`
	Kind      string         `gorm:"column:kind;type:text;not null;default:'chunk'" json:"kind"`
	Content   string         `gorm:"column:content;type:text;not null" json:"content"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb;not null;default:'{}'::jsonb" json:"metadata"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (KnowledgeChunk) TableName() string {
	return coremodel.PowerXSchema + "." + "knowledge_chunks"
}
