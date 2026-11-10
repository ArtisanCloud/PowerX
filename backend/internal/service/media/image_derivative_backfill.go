package media

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
	"github.com/google/uuid"
)

// ImageDerivativeBackfillOptions is an explicit operator action. It never runs
// from startup, a migration, or a read request. Apply defaults to false.
type ImageDerivativeBackfillOptions struct {
	TenantUUID                    string
	AllTenants                    bool // Trusted offline maintenance only; never inferred by repositories.
	AssetUUIDs                    []string
	AfterID, ThroughID            uint64
	BatchSize, Concurrency, Limit int
	Apply, RetryFailed            bool
	// Expected restricts an approved plan to the observed source and missing specs.
	// It is only used by explicit maintenance, never by upload or GET paths.
	Expected map[string]ImageDerivativeBackfillExpectation
}
type ImageDerivativeBackfillExpectation struct {
	TenantUUID     string   `json:"tenant_uuid"`
	SourceChecksum string   `json:"source_checksum"`
	Missing        []string `json:"missing"`
}
type ImageDerivativeBackfillItem struct {
	ThroughID      uint64   `json:"through_id"`
	ID             uint64   `json:"id"`
	AssetUUID      string   `json:"asset_uuid"`
	TenantUUID     string   `json:"tenant_uuid"`
	Outcome        string   `json:"outcome"`
	Missing        []string `json:"missing,omitempty"`
	JobUUID        string   `json:"job_uuid,omitempty"`
	ErrorCode      string   `json:"error_code,omitempty"`
	SourceChecksum string   `json:"source_checksum,omitempty"`
}
type ImageDerivativeBackfillReport struct {
	DryRun        bool   `json:"dry_run"`
	Recipe        string `json:"recipe"`
	Scanned       int    `json:"scanned"`
	Missing       int    `json:"missing"`
	Submitted     int    `json:"submitted"`
	Active        int    `json:"active"`
	Skipped       int    `json:"skipped"`
	Failed        int    `json:"failed"`
	SourceMissing int    `json:"source_missing"`
	AfterID       uint64 `json:"after_id"`
	ThroughID     uint64 `json:"through_id"`
	TenantCount   int    `json:"tenant_count"`
}

// BackfillImageDerivatives only submits shared TaskBus jobs. Submitted is not
// generated/succeeded: final results remain observable through status/events.
// report is called in ID order, so the last emitted ID is a safe resume cursor.
func (s *MediaService) BackfillImageDerivatives(ctx context.Context, o ImageDerivativeBackfillOptions, report func(ImageDerivativeBackfillItem) error) (ImageDerivativeBackfillReport, error) {
	result := ImageDerivativeBackfillReport{DryRun: !o.Apply, Recipe: ImageDerivativeRecipe, AfterID: o.AfterID, ThroughID: o.ThroughID}
	if s == nil || s.DB == nil || s.manager == nil || (o.Apply && s.derivativeQueue == nil) {
		return result, MediaUpstreamDependencyError(errors.New("media backfill dependencies unavailable"))
	}
	if (o.AllTenants && o.TenantUUID != "") || (!o.AllTenants && !canonicalBackfillTenant(o.TenantUUID)) || o.BatchSize < 1 || o.BatchSize > 200 || o.Concurrency < 1 || o.Concurrency > 8 || o.Limit < 0 || (o.ThroughID > 0 && o.AfterID > o.ThroughID) {
		return result, errors.New("explicit all-tenant maintenance or canonical tenant UUID, valid batch/concurrency/ID range required")
	}
	for _, raw := range o.AssetUUIDs {
		if id, e := uuid.Parse(raw); e != nil || id == uuid.Nil || id.String() != raw {
			return result, errors.New("canonical asset UUID required")
		}
	}
	if o.Expected != nil {
		if len(o.AssetUUIDs) == 0 || len(o.AssetUUIDs) != len(o.Expected) {
			return result, errors.New("MEDIA_BACKFILL_PLAN_SCOPE_INVALID")
		}
		for _, asset := range o.AssetUUIDs {
			if expected, ok := o.Expected[asset]; !ok || !expected.Valid() || (!o.AllTenants && expected.TenantUUID != o.TenantUUID) {
				return result, errors.New("MEDIA_BACKFILL_PLAN_SCOPE_INVALID")
			}
		}
	}
	seenTenants := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		limit := o.BatchSize
		if o.Limit > 0 {
			limit = min(limit, o.Limit-result.Scanned)
			if limit == 0 {
				return result, nil
			}
		}
		repo := r.NewAssetRepository(s.DB)
		var assets []m.MediaAsset
		var through uint64
		var err error
		if o.AllTenants {
			assets, through, err = repo.AllTenantDerivativeCandidates(ctx, o.AssetUUIDs, result.AfterID, result.ThroughID, limit)
		} else {
			assets, through, err = repo.DerivativeCandidates(ctx, o.TenantUUID, o.AssetUUIDs, result.AfterID, result.ThroughID, limit)
		}
		if err != nil {
			return result, err
		}
		result.ThroughID = through
		if len(assets) == 0 {
			return result, nil
		}
		items := make([]ImageDerivativeBackfillItem, len(assets))
		jobs := make(chan int)
		var wg sync.WaitGroup
		for range min(o.Concurrency, len(assets)) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					items[i] = s.backfillImage(ctx, assets[i], o)
				}
			}()
		}
		for i := range assets {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		for _, item := range items {
			item.ThroughID = result.ThroughID
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if report != nil {
				if err := report(item); err != nil {
					return result, err
				}
			}
			result.Scanned++
			if canonicalBackfillTenant(item.TenantUUID) {
				seenTenants[item.TenantUUID] = true
			}
			result.TenantCount = len(seenTenants)
			result.AfterID = item.ID
			if len(item.Missing) > 0 {
				result.Missing++
			}
			switch item.Outcome {
			case "would_enqueue":
			case "submitted":
				result.Submitted++
			case "active":
				result.Active++
			case "source_missing":
				result.SourceMissing++
			case "failed":
				result.Failed++
			default:
				result.Skipped++
			}
		}
	}
}
func (s *MediaService) backfillImage(ctx context.Context, a m.MediaAsset, o ImageDerivativeBackfillOptions) ImageDerivativeBackfillItem {
	out := ImageDerivativeBackfillItem{ID: a.ID, AssetUUID: a.UUID.String(), TenantUUID: a.TenantUUID, Outcome: "skipped"}
	fail := func(code string) ImageDerivativeBackfillItem { out.Outcome, out.ErrorCode = "failed", code; return out }
	if !canonicalBackfillTenant(a.TenantUUID) {
		return fail("MEDIA_BACKFILL_TENANT_INVALID")
	}
	if a.UploadState != m.UploadStateReady || !supportedImageMIME(a.MimeType) || stringFromMeta(extractMeta(a.Meta), "upload_method") == string(UploadMethodExternalLink) {
		return out
	}
	variants, err := s.repo.ListVariants(ctx, a.TenantUUID, a.UUID.String(), false)
	if err != nil {
		return fail("MEDIA_BACKFILL_READ_FAILED")
	}
	if a.ExpectedChecksum == "" {
		checksum, err := s.storedImageChecksum(ctx, &a)
		if err != nil {
			for _, p := range imageProfiles {
				out.Missing = append(out.Missing, p.Name)
			}
			if errors.Is(err, driver.ErrNotFound) {
				out.Outcome = "source_missing"
				return out
			}
			return fail(sourceBackfillError(err))
		}
		a.ExpectedChecksum = checksum // planning copy only; ensure persists under the parent lock.
	}
	out.SourceChecksum = a.ExpectedChecksum
	var expected *ImageDerivativeBackfillExpectation
	if o.Expected != nil {
		value := o.Expected[a.UUID.String()]
		expected = &value
		if value.TenantUUID != a.TenantUUID || value.SourceChecksum != a.ExpectedChecksum {
			return fail("MEDIA_BACKFILL_PLAN_CHANGED")
		}
	}
	for _, p := range imageProfiles {
		valid := false
		for i := range variants {
			if variants[i].Variant == p.Name {
				valid, err = s.usableImageVariant(ctx, &a, &variants[i])
				if err != nil {
					return fail("MEDIA_BACKFILL_STORAGE_UNAVAILABLE")
				}
			}
		}
		if !valid {
			out.Missing = append(out.Missing, p.Name)
		}
	}
	if len(out.Missing) == 0 {
		return out
	}
	if expected != nil && !expected.allows(out.Missing) {
		return fail("MEDIA_BACKFILL_PLAN_CHANGED")
	}
	stat, err := s.manager.Stat(ctx, a.Driver, driver.StatObjectInput{Bucket: a.Bucket, ObjectKey: a.StorageKey})
	if errors.Is(err, driver.ErrNotFound) || (err == nil && (stat == nil || stat.Size == 0)) {
		out.Outcome = "source_missing"
		return out
	}
	if err != nil {
		return fail("MEDIA_BACKFILL_STORAGE_UNAVAILABLE")
	}
	if !o.Apply {
		out.Outcome = "would_enqueue"
		return out
	}
	before := derivativeRef(&a)
	state, err := s.ensureImageDerivatives(ctx, a.TenantUUID, a.UUID.String(), o.RetryFailed, expected)
	if err != nil {
		if errors.Is(err, errBackfillPlanChanged) {
			return fail("MEDIA_BACKFILL_PLAN_CHANGED")
		}
		return fail("MEDIA_BACKFILL_ENSURE_FAILED")
	}
	out.JobUUID = state.JobUUID
	switch state.Status {
	case "ready":
		out.Outcome = "skipped"
	case "queued", "processing":
		out.Outcome = "submitted"
		if state.JobUUID == before.TaskID && before.Recipe == ImageDerivativeRecipe {
			out.Outcome = "active"
		}
	default:
		return fail(fmt.Sprint(state.ErrorCode))
	}
	return out
}
