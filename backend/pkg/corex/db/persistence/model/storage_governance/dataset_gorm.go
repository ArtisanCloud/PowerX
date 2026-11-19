package storagegovmodel

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// DatasetRegistration stores the current admission and immutable request
// snapshots. DatasetUUID identifies the dataset across revisions.
type DatasetRegistration struct {
	coremodel.PowerModel
	TenantID           uint64         `gorm:"not null;default:0"`
	DeploymentID       string         `gorm:"size:128;not null;uniqueIndex:uk_ops_dataset_head,where:is_current;uniqueIndex:uk_ops_dataset_uuid,where:is_current;uniqueIndex:uk_ops_dataset_request,where:request_uuid IS NOT NULL"`
	OwnerID            string         `gorm:"size:128;not null;uniqueIndex:uk_ops_dataset_head,where:is_current"`
	BindingUUID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uk_ops_dataset_head,where:is_current"`
	DatasetKey         string         `gorm:"size:64;not null;uniqueIndex:uk_ops_dataset_head,where:is_current"`
	DatasetUUID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uk_ops_dataset_uuid,where:is_current"`
	RequestUUID        *uuid.UUID     `gorm:"type:uuid;uniqueIndex:uk_ops_dataset_request,where:request_uuid IS NOT NULL"`
	RequestDigest      string         `gorm:"size:64;not null"`
	Actor              string         `gorm:"size:256;not null"`
	ServiceTenantUUID  uuid.UUID      `gorm:"type:uuid;not null"`
	Installation       string         `gorm:"size:128;not null"`
	DefinitionDigest   string         `gorm:"size:64;not null"`
	SourceFingerprint  string         `gorm:"size:64;not null"`
	DefinitionVersion  int64          `gorm:"not null"`
	AcceptanceRevision int64          `gorm:"not null"`
	State              string         `gorm:"size:32;not null"`
	IsCurrent          bool           `gorm:"not null"`
	Declaration        datatypes.JSON `gorm:"type:jsonb;not null"`
	Status             int16          `gorm:"not null;default:1"`
}

// TableName returns the centralized Core governance table.
func (*DatasetRegistration) TableName() string {
	return coremodel.PowerXSchema + "." + TableDatasetRegistration
}
