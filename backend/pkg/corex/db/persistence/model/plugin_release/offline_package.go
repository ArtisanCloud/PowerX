package plugin_release

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// OfflineDistributionPackage stores signed artifact metadata for offline import.
type OfflineDistributionPackage struct {
	coremodel.PowerModel

	ReleaseCandidateID   uint64         `gorm:"column:release_candidate_id;not null;index" json:"release_candidate_id"`
	PackageURI           string         `gorm:"column:package_uri;type:text;not null" json:"package_uri"`
	Checksum             string         `gorm:"column:checksum;type:char(128);not null" json:"checksum"`
	SignatureFingerprint string         `gorm:"column:signature_fingerprint;type:char(64);not null" json:"signature_fingerprint"`
	Dependencies         datatypes.JSON `gorm:"column:dependencies;type:jsonb;default:'[]'" json:"dependencies,omitempty"`
	LicenseReport        datatypes.JSON `gorm:"column:license_report;type:jsonb;default:'{}'" json:"license_report,omitempty"`
	HealthCheckLog       string         `gorm:"column:health_check_log;type:text" json:"health_check_log,omitempty"`
	Status               string         `gorm:"column:status;type:varchar(32);not null;default:'draft';index" json:"status"`
	SLADeadline          *time.Time     `gorm:"column:sla_deadline" json:"sla_deadline,omitempty"`
	CreatedBy            string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy            string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`

	Listings []MarketplaceListing `gorm:"foreignKey:OfflinePackageID;references:ID" json:"listings,omitempty"`
}

func (OfflineDistributionPackage) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginReleaseOfflinePackages
	}
	return schema + "." + coremodel.TablePluginReleaseOfflinePackages
}

const (
	OfflinePackageStatusDraft      = "draft"
	OfflinePackageStatusSubmitted  = "submitted"
	OfflinePackageStatusApproved   = "approved"
	OfflinePackageStatusRejected   = "rejected"
	OfflinePackageStatusSuperseded = "superseded"
)
