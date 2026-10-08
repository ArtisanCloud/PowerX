package knowledge

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// SemanticEmbeddingProfile 保存已探测的不可变模型配置与实际模型修订。
type SemanticEmbeddingProfile struct {
	coremodel.PowerUUIDModel
	TenantUUID     string `gorm:"type:varchar(128);not null;uniqueIndex:uk_semantic_embedding_version,priority:1"`
	ProfileKey     string `gorm:"type:varchar(256);not null;uniqueIndex:uk_semantic_embedding_version,priority:2"`
	Version        int    `gorm:"not null;uniqueIndex:uk_semantic_embedding_version,priority:3"`
	Status         string `gorm:"type:varchar(32);not null"`
	Provider       string `gorm:"type:varchar(128);not null"`
	Model          string `gorm:"type:varchar(256);not null"`
	ModelRevision  string `gorm:"type:varchar(256);not null"`
	Dimensions     int    `gorm:"not null"`
	ConfigChecksum string `gorm:"type:char(64);not null"`
}

func (SemanticEmbeddingProfile) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeSemanticEmbeddingProfiles
}

// SemanticSpaceBinding 的配置与可见性代次相互独立。
type SemanticSpaceBinding struct {
	coremodel.PowerUUIDModel
	TenantUUID              string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_semantic_space,priority:1"`
	SpaceUUID               string         `gorm:"type:uuid;not null;uniqueIndex:uk_semantic_space,priority:2"`
	EmbeddingProfileUUID    string         `gorm:"type:uuid;not null"`
	ConfigurationGeneration string         `gorm:"type:uuid;not null"`
	CorpusGeneration        string         `gorm:"type:uuid;not null"`
	VectorIndexKey          string         `gorm:"type:varchar(128);not null"`
	Modes                   datatypes.JSON `gorm:"type:jsonb;not null"`
}

func (SemanticSpaceBinding) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeSemanticSpaceBindings
}
