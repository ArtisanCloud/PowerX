package plugin_release

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// MarketplaceListing captures review workflow and publication state for each channel.
type MarketplaceListing struct {
	coremodel.PowerModel

	OfflinePackageID   uint64         `gorm:"column:offline_package_id;not null;index" json:"offline_package_id"`
	Channel            string         `gorm:"column:channel;type:varchar(32);not null" json:"channel"`
	Pricing            datatypes.JSON `gorm:"column:pricing;type:jsonb;default:'{}'" json:"pricing,omitempty"`
	SupportPolicy      datatypes.JSON `gorm:"column:support_policy;type:jsonb;default:'{}'" json:"support_policy,omitempty"`
	SubmissionForm     datatypes.JSON `gorm:"column:submission_form;type:jsonb;default:'{}'" json:"submission_form,omitempty"`
	ReviewStatus       string         `gorm:"column:review_status;type:varchar(32);not null;default:'pending';index" json:"review_status"`
	ReviewCount        int            `gorm:"column:review_count;not null;default:0" json:"review_count"`
	EscalatedAt        *time.Time     `gorm:"column:escalated_at" json:"escalated_at,omitempty"`
	NotificationTicket *uuid.UUID     `gorm:"column:notification_ticket;type:uuid" json:"notification_ticket,omitempty"`
	PublishedAt        *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	CreatedBy          string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy          string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (MarketplaceListing) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginReleaseMarketplaceListings
	}
	return schema + "." + coremodel.TablePluginReleaseMarketplaceListings
}

const (
	MarketplaceListingStatusPending   = "pending"
	MarketplaceListingStatusNeedFix   = "need_fix"
	MarketplaceListingStatusApproved  = "approved"
	MarketplaceListingStatusRejected  = "rejected"
	MarketplaceListingStatusPublished = "published"
)
