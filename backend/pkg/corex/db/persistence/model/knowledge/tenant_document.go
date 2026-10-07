package knowledge

import (
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// TenantDocument is the tenant-scoped, auditable source document accepted by
// the service-actor Knowledge Host Contract. URI is its caller-stable identity
// within a space; UUID is the only public document identifier.
type TenantDocument struct {
	coremodel.PowerUUIDModel

	TenantUUID  string         `gorm:"column:tenant_uuid;type:varchar(128);not null;uniqueIndex:uk_knowledge_tenant_document_uri" json:"tenant_uuid"`
	SpaceUUID   string         `gorm:"column:space_uuid;type:uuid;not null;uniqueIndex:uk_knowledge_tenant_document_uri;index" json:"space_uuid"`
	Title       string         `gorm:"column:title;type:varchar(256);not null" json:"title"`
	URI         string         `gorm:"column:uri;type:text;not null;uniqueIndex:uk_knowledge_tenant_document_uri" json:"uri"`
	Content     string         `gorm:"column:content;type:text;not null" json:"-"`
	ContentType string         `gorm:"column:content_type;type:varchar(128);not null" json:"content_type"`
	Checksum    string         `gorm:"column:checksum;type:char(64);not null;index" json:"checksum"`
	Version     string         `gorm:"column:version;type:varchar(128);not null" json:"version"`
	Tags        datatypes.JSON `gorm:"column:tags;type:jsonb;not null;default:'[]'" json:"tags"`
	IndexStatus string         `gorm:"column:index_status;type:varchar(32);not null;default:'queued';index" json:"index_status"`
	IndexedAt   *time.Time     `gorm:"column:indexed_at" json:"indexed_at,omitempty"`
}

func (TenantDocument) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeTenantDocuments
}
