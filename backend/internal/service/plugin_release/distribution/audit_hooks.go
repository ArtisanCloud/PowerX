package distribution

import (
	"context"
	"encoding/json"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
)

// AuditHooks captures important distribution lifecycle transitions.
type AuditHooks interface {
	OnOfflinePackageStored(ctx context.Context, pkg *models.OfflineDistributionPackage)
	OnListingSubmitted(ctx context.Context, listing *models.MarketplaceListing)
	OnListingEscalated(ctx context.Context, listing *models.MarketplaceListing)
	OnListingApproved(ctx context.Context, listing *models.MarketplaceListing)
	OnOfflineImportStarted(ctx context.Context, job ImportJob)
}

// NoopAuditHooks prevents nil pointer checks in callers.
type NoopAuditHooks struct{}

func (NoopAuditHooks) OnOfflinePackageStored(context.Context, *models.OfflineDistributionPackage) {}
func (NoopAuditHooks) OnListingSubmitted(context.Context, *models.MarketplaceListing)             {}
func (NoopAuditHooks) OnListingEscalated(context.Context, *models.MarketplaceListing)             {}
func (NoopAuditHooks) OnListingApproved(context.Context, *models.MarketplaceListing)              {}
func (NoopAuditHooks) OnOfflineImportStarted(context.Context, ImportJob)                          {}

type auditHook struct {
	auditor audit.Auditor
}

// NewAuditHooks wires a shared auditor into the distribution hooks.
func NewAuditHooks(auditor audit.Auditor) AuditHooks {
	if auditor == nil {
		return NoopAuditHooks{}
	}
	return &auditHook{auditor: auditor}
}

func (h *auditHook) OnOfflinePackageStored(ctx context.Context, pkg *models.OfflineDistributionPackage) {
	h.log(ctx, "plugin_release.distribution.package_stored", pkg)
}

func (h *auditHook) OnListingSubmitted(ctx context.Context, listing *models.MarketplaceListing) {
	h.log(ctx, "plugin_release.distribution.listing_submitted", listing)
}

func (h *auditHook) OnListingEscalated(ctx context.Context, listing *models.MarketplaceListing) {
	h.log(ctx, "plugin_release.distribution.listing_escalated", listing)
}

func (h *auditHook) OnListingApproved(ctx context.Context, listing *models.MarketplaceListing) {
	h.log(ctx, "plugin_release.distribution.listing_approved", listing)
}

func (h *auditHook) OnOfflineImportStarted(ctx context.Context, job ImportJob) {
	h.log(ctx, "plugin_release.distribution.offline_import", job)
}

func (h *auditHook) log(ctx context.Context, action string, payload any) {
	if h.auditor == nil {
		return
	}
	h.auditor.LogAPI(ctx, action, 200, 0)
	if payload == nil {
		return
	}
	_, _ = json.Marshal(payload)
}
