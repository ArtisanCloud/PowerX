package knowledge

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// HostDocumentChunk 保留每次任务的真实分块产物，不能被文档下一版本覆盖。
type HostDocumentChunk struct {
	coremodel.PowerUUIDModel
	TenantUUID   string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_host_chunk_job,priority:1" json:"-"`
	SpaceUUID    string         `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	JobUUID      string         `gorm:"column:job_uuid;type:uuid;not null;index:idx_host_chunk_job,priority:2;uniqueIndex:uk_host_job_chunk,priority:1" json:"job_uuid"`
	DocumentUUID string         `gorm:"column:document_uuid;type:uuid;not null;index" json:"document_uuid"`
	Ordinal      int            `gorm:"column:ordinal;not null;uniqueIndex:uk_host_job_chunk,priority:2" json:"ordinal"`
	Kind         string         `gorm:"column:kind;type:varchar(32);not null" json:"kind"`
	Content      string         `gorm:"column:content;type:text;not null" json:"content"`
	Checksum     string         `gorm:"column:checksum;type:char(64);not null" json:"checksum"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:jsonb;not null" json:"metadata"`
}

func (HostDocumentChunk) TableName() string {
	return coremodel.PowerXSchema + ".knowledge_host_document_chunks"
}
