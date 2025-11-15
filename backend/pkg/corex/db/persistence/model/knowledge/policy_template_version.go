package knowledge

import (
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// PolicyTemplateVersion 描述默认策略组合。
type PolicyTemplateVersion struct {
	coremodel.PowerModel

	TemplateName    string         `gorm:"column:template_name;type:varchar(128);not null;index:idx_knowledge_template_name_version,unique" json:"template_name"`
	Version         string         `gorm:"column:version;type:varchar(32);not null;index:idx_knowledge_template_name_version,unique" json:"version"`
	RAGProfile      datatypes.JSON `gorm:"column:rag_profile;type:jsonb;default:'{}'" json:"rag_profile"`
	GraphProfile    datatypes.JSON `gorm:"column:graph_profile;type:jsonb;default:'{}'" json:"graph_profile"`
	MaskingProfile  datatypes.JSON `gorm:"column:masking_profile;type:jsonb;default:'{}'" json:"masking_profile"`
	AlertingProfile datatypes.JSON `gorm:"column:alerting_profile;type:jsonb;default:'{}'" json:"alerting_profile"`
	ApprovedBy      string         `gorm:"column:approved_by;type:varchar(128)" json:"approved_by,omitempty"`
	ApprovedAt      *time.Time     `gorm:"column:approved_at" json:"approved_at,omitempty"`
	RollbackToken   string         `gorm:"column:rollback_token;type:varchar(128)" json:"rollback_token,omitempty"`
	ImmutableHash   string         `gorm:"column:immutable_hash;type:char(64);not null;uniqueIndex" json:"immutable_hash"`
}

func (PolicyTemplateVersion) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgePolicyTemplates
}
